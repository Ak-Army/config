package config

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/encoder"
)

type Loader struct {
	mu             sync.Mutex
	ctx            context.Context
	backend        []backend.Backend
	backendWatcher []loadable
	maps           map[backend.Backend]*backend.Content
	// structCache holds the parsed, instance-independent description of a
	// snapshot type. It lets reloads (e.g. triggered by watchers) skip the
	// tag parsing done by parseType and only rebind the reflect values.
	structCache map[reflect.Type][]*fieldSpec
	crypto      *crypto.Crypto
}

// fieldSpec is the cached, instance-independent description of one struct
// field. It is derived from the struct type once (parseType) and reused for
// every load, where bind turns it into a value-bound field.
type fieldSpec struct {
	index     int
	name      string
	key       string
	required  bool
	encrypted bool
	isList    bool
	sources   []string
	handling  handling
	subSpecs  []*fieldSpec
}

// handling describes how a field is bound and resolved.
type handling int

const (
	handleScalar        handling = iota // leaf value
	handleStruct                        // nested struct (by value)
	handlePtrStruct                     // nested *struct
	handleListStruct                    // []struct
	handleFlattenStruct                 // tag == "-" struct: children promoted
	handleFlattenPtr                    // tag == "-" *struct: children promoted
)

type field struct {
	name      string
	key       string
	spec      *fieldSpec
	value     reflect.Value
	origValue reflect.Value
	required  bool
	encrypted bool
	isList    bool
	sources   []string
	subFields []*field
	found     bool
}

func NewLoader(ctx context.Context, sources ...backend.Backend) (*Loader, error) {
	l := &Loader{
		backend:     sources,
		ctx:         ctx,
		maps:        make(map[backend.Backend]*backend.Content),
		structCache: make(map[reflect.Type][]*fieldSpec),
	}
	for _, s := range l.backend {
		if err := l.syncSource(s); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// SetCrypto sets the crypto used to decode ENC(...) values of fields tagged
// with the `encrypted` option. Call it before Load.
func (l *Loader) SetCrypto(c *crypto.Crypto) {
	l.mu.Lock()
	l.crypto = c
	l.mu.Unlock()
}

func (l *Loader) AddSource(sources ...backend.Backend) error {
	var gerr []string
	added := false
	for _, s := range sources {
		if err := l.syncSource(s); err != nil {
			gerr = append(gerr, err.Error())
			continue
		}
		l.mu.Lock()
		l.backend = append(l.backend, s)
		l.mu.Unlock()
		added = true
	}
	// Re-resolve the already-registered stores so the new source's values take
	// effect immediately, rather than only after an unrelated watcher event.
	if added {
		l.mu.Lock()
		for _, c := range l.backendWatcher {
			l.load(c)
		}
		l.mu.Unlock()
	}
	if len(gerr) > 0 {
		return fmt.Errorf("source loading errors: %s", strings.Join(gerr, "\n"))
	}
	return nil
}

// Load resolves the registered sources into a fresh snapshot produced by the
// store and stores the populated result. The store is also registered for
// watcher-triggered reloads.
//
// Load returns only structural errors (a store whose snapshot is not a pointer
// to struct). Per-load errors (missing required keys, decode failures) are
// delivered to the store and surfaced via Store.Config, so a bad reload never
// silently replaces a good snapshot.
func Load[T any](l *Loader, s *Store[T]) error {
	to := s.newSnapshot()
	ref := reflect.ValueOf(to)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return errors.New("provided target must be a pointer to struct")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	registered := false
	for _, w := range l.backendWatcher {
		if w == loadable(s) {
			registered = true
			break
		}
	}
	if !registered {
		l.backendWatcher = append(l.backendWatcher, s)
	}
	l.loadInto(s, to)
	return nil
}

// load builds a fresh snapshot and stores it. It must be called with l.mu held.
func (l *Loader) load(c loadable) {
	l.loadInto(c, c.newSnapshot())
}

// loadInto resolves the sources into the given snapshot and stores it.
// It must be called with l.mu held.
func (l *Loader) loadInto(c loadable, to interface{}) {
	ref := reflect.ValueOf(to).Elem()
	fields := bind(l.specsFor(ref.Type()), ref)
	err := l.resolve(fields)
	c.setSnapshot(to, err)
}

// specsFor returns the cached field description for t, parsing it on the first
// use. It must be called with l.mu held.
func (l *Loader) specsFor(t reflect.Type) []*fieldSpec {
	if specs, ok := l.structCache[t]; ok {
		return specs
	}
	specs := parseType(t)
	l.structCache[t] = specs
	return specs
}

func (l *Loader) syncSource(s backend.Backend) error {
	c, err := s.Read()
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.maps[s] = c

	return l.watch(s)
}

func (l *Loader) watch(s backend.Backend) error {
	w, err := s.Watcher()
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	ch := w.Watch()
	go func() {
		for {
			select {
			case <-l.ctx.Done():
				w.Stop()
				return
			case content := <-ch:
				l.mu.Lock()
				l.maps[s] = content
				for _, config := range l.backendWatcher {
					l.load(config)
				}
				l.mu.Unlock()
			}
		}
	}()
	return nil
}

// parseType derives the instance-independent field description for a struct
// type. The result depends only on the type (field names, tags, kinds), so it
// is cached and reused across loads; bind turns it into value-bound fields.
func parseType(t reflect.Type) []*fieldSpec {
	var list []*fieldSpec
	for i := 0; i < t.NumField(); i++ {
		structField := t.Field(i)
		if structField.PkgPath != "" {
			continue
		}
		tag := structField.Tag.Get("config")
		typ := structField.Type
		spec := &fieldSpec{
			index: i,
			name:  structField.Name,
			key:   tag,
		}
		switch typ.Kind() {
		case reflect.Struct:
			if tag == "-" {
				spec.handling = handleFlattenStruct
				spec.subSpecs = parseType(typ)
			} else {
				spec.handling = handleStruct
				spec.subSpecs = parseType(typ)
				spec.parseTagSpec(tag)
			}
			list = append(list, spec)
		case reflect.Slice:
			if typ.Elem().Kind() == reflect.Struct {
				if tag == "-" {
					continue
				}
				spec.handling = handleListStruct
				spec.isList = true
				spec.subSpecs = parseType(typ.Elem())
				spec.parseTagSpec(tag)
				list = append(list, spec)
				continue
			}
			if tag == "-" {
				continue
			}
			spec.parseTagSpec(tag)
			list = append(list, spec)
		case reflect.Ptr:
			if typ.Elem().Kind() == reflect.Struct {
				if tag == "-" {
					spec.handling = handleFlattenPtr
					spec.subSpecs = parseType(typ.Elem())
				} else {
					spec.handling = handlePtrStruct
					spec.subSpecs = parseType(typ.Elem())
					spec.parseTagSpec(tag)
				}
				list = append(list, spec)
				continue
			}
			if tag == "-" {
				continue
			}
			spec.parseTagSpec(tag)
			list = append(list, spec)
		default:
			if tag == "-" {
				continue
			}
			spec.parseTagSpec(tag)
			list = append(list, spec)
		}
	}
	return list
}

// bind turns cached field specs into value-bound fields for the given struct
// value. It performs the same reflect wiring parseType could not (values differ
// per snapshot) but skips the tag parsing already done by parseType.
//
// The fields bound at one level share a single backing array (one allocation
// instead of one per field), and leaf/list fields alias their value onto the
// target field instead of allocating a scratch copy, so a scratch value is only
// materialised where the resolve step genuinely accumulates into it (nested and
// pointer structs).
func bind(specs []*fieldSpec, ref reflect.Value) []*field {
	direct := 0
	for _, spec := range specs {
		if spec.handling != handleFlattenStruct && spec.handling != handleFlattenPtr {
			direct++
		}
	}
	backing := make([]field, direct)
	list := make([]*field, 0, len(specs))
	bi := 0
	for _, spec := range specs {
		originalValue := ref.Field(spec.index)
		switch spec.handling {
		case handleFlattenStruct:
			// Bind the promoted children directly to the real field so their
			// decoded values land in the snapshot instead of a scratch copy.
			list = append(list, bind(spec.subSpecs, originalValue)...)
			continue
		case handleFlattenPtr:
			if originalValue.IsNil() {
				originalValue.Set(reflect.New(originalValue.Type().Elem()))
			}
			list = append(list, bind(spec.subSpecs, originalValue.Elem())...)
			continue
		}
		f := &backing[bi]
		bi++
		f.name = spec.name
		f.key = spec.key
		f.spec = spec
		f.origValue = originalValue
		f.required = spec.required
		f.encrypted = spec.encrypted
		f.isList = spec.isList
		f.sources = spec.sources
		switch spec.handling {
		case handleStruct:
			value := reflect.New(originalValue.Type()).Elem()
			value.Set(originalValue)
			f.value = value
			f.subFields = bind(spec.subSpecs, value)
		case handlePtrStruct:
			// Copy the pointer onto a scratch, allocating the pointee there
			// when nil, so every subfield aliases f.value.Elem() and the real
			// field is only replaced by resolve's found write-back.
			value := reflect.New(originalValue.Type()).Elem()
			value.Set(originalValue)
			if value.IsNil() {
				value.Set(reflect.New(originalValue.Type().Elem()))
			}
			f.value = value
			f.subFields = bind(spec.subSpecs, value.Elem())
		case handleListStruct:
			f.value = originalValue
			elem := reflect.New(originalValue.Type().Elem()).Elem()
			f.subFields = bind(spec.subSpecs, elem)
		default: // handleScalar: decode straight into the target field.
			f.value = originalValue
		}
		list = append(list, f)
	}
	return list
}

func (s *fieldSpec) parseTagSpec(tag string) {
	if idx := strings.Index(tag, ","); idx != -1 {
		s.key = tag[:idx]
		opts := strings.Split(tag[idx+1:], ",")

		for _, opt := range opts {
			if opt == "required" {
				s.required = true
			}
			if opt == "encrypted" {
				s.encrypted = true
			}
			if strings.HasPrefix(opt, "backend=") {
				s.sources = append(s.sources, opt[len("backend="):])
			}
		}
	}
}

// candidate is one backend's view at the current nesting level: the content
// (carrying the encoder used to decode its values) and the data map to look a
// field's key up in.
type candidate struct {
	name    string
	content *backend.Content
	data    encoder.Data
}

// resolve populates the bound field tree from the registered backends. Fields
// are resolved per leaf: each field independently picks the first backend (in
// registration order) that provides its key, honouring the `backend=` pins, so
// a nested struct can be filled from several backends at once.
func (l *Loader) resolve(fields []*field) error {
	var gerr []string
	if err := l.resolveFields(fields, l.topCandidates(), nil, &gerr); err != nil {
		return err
	}
	// Validate required fields only after every field has been loaded and
	// written back, so a missing required key never discards the values that
	// did load into a partially populated struct.
	if err := validateRequired(fields); err != nil {
		return err
	}
	if len(gerr) > 0 {
		return fmt.Errorf("data loading errors: %s", strings.Join(gerr, "\n"))
	}
	return nil
}

// topCandidates builds the top-level candidate list from the registered
// backends, in registration order so that source precedence is deterministic
// (the first registered source that provides a key wins), rather than depending
// on Go's random map iteration order.
func (l *Loader) topCandidates() []candidate {
	cands := make([]candidate, 0, len(l.backend))
	for _, s := range l.backend {
		c, ok := l.maps[s]
		if !ok {
			continue
		}
		cands = append(cands, candidate{name: s.String(), content: c, data: c.Data})
	}
	return cands
}

// backendRegistered reports whether a backend with the given name is registered.
func (l *Loader) backendRegistered(name string) bool {
	for _, s := range l.backend {
		if s.String() == name {
			return true
		}
	}
	return false
}

// effectiveSources returns the backends a field may be read from. A restriction
// imposed by an ancestor (a struct or list pinned with `backend=`) wins and
// cannot be widened or changed by a nested field, so a pinned struct forces all
// of its subfields onto the same backends. Only when no ancestor restricts does
// a field's own pins take effect.
func effectiveSources(inherited, own []string) []string {
	if len(inherited) > 0 {
		return inherited
	}
	return own
}

// filterCandidates keeps only the candidates whose backend is named in sources,
// preserving registration order. An empty sources means "any backend".
func filterCandidates(cands []candidate, sources []string) []candidate {
	if len(sources) == 0 {
		return cands
	}
	out := make([]candidate, 0, len(cands))
	for _, c := range cands {
		for _, name := range sources {
			if c.name == name {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// resolveFields resolves every field against the candidate backends visible at
// this nesting level. inherited carries the source restriction imposed by an
// enclosing pinned struct/list (nil at the top level).
func (l *Loader) resolveFields(fields []*field, cands []candidate, inherited []string, gerr *[]string) error {
	for _, f := range fields {
		eff := effectiveSources(inherited, f.sources)
		for _, name := range eff {
			if !l.backendRegistered(name) {
				return fmt.Errorf("the backend: '%s' is not supported", name)
			}
		}
		if err := l.resolveField(f, filterCandidates(cands, eff), eff, gerr); err != nil {
			return err
		}
		if f.found {
			f.origValue.Set(f.value)
		}
	}
	return nil
}

// resolveField dispatches to the leaf, struct or list resolver for f.
func (l *Loader) resolveField(f *field, cands []candidate, eff []string, gerr *[]string) error {
	switch {
	case len(f.subFields) != 0 && f.isList:
		return l.resolveListField(f, cands, gerr)
	case len(f.subFields) != 0:
		return l.resolveStructField(f, cands, eff, gerr)
	default:
		l.resolveLeaf(f, cands, gerr)
		return nil
	}
}

// resolveLeaf decodes a scalar (or encrypted) field from the first candidate
// that holds its key. A decode/decrypt failure is recorded but does not abort
// the whole load, and it leaves the field unfound so a tampered encrypted value
// is never silently loaded as a zero value.
func (l *Loader) resolveLeaf(f *field, cands []candidate, gerr *[]string) {
	for _, c := range cands {
		v, ok := c.data[f.key]
		if !ok {
			continue
		}
		if err := l.decodeLeaf(f, c.content, v); err != nil {
			*gerr = append(*gerr, err.Error())
			continue
		}
		f.found = true
		return
	}
}

// decodeLeaf decodes a single leaf value into f, decrypting it first when the
// field carries the `encrypted` option.
func (l *Loader) decodeLeaf(f *field, c *backend.Content, v interface{}) error {
	if f.encrypted {
		return l.decodeEncrypted(f, c, v)
	}
	var to interface{}
	if f.value.CanAddr() {
		to = f.value.Addr().Interface()
	} else {
		to = f.value.Interface()
	}
	return c.Encoder.Decode(v, to)
}

// resolveStructField resolves a nested struct field. Every candidate that holds
// the struct's key contributes its sub-document, so the subfields can be filled
// from different backends (each honouring precedence) — unless the struct is
// pinned, in which case eff locks the subfields to the same backends.
func (l *Loader) resolveStructField(f *field, cands []candidate, eff []string, gerr *[]string) error {
	childCands := make([]candidate, 0, len(cands))
	for _, c := range cands {
		v, ok := c.data[f.key]
		if !ok {
			continue
		}
		childData, err := c.content.Encoder.DecodeData(v)
		if err != nil {
			*gerr = append(*gerr, err.Error())
			continue
		}
		childCands = append(childCands, candidate{name: c.name, content: c.content, data: childData})
	}
	if len(childCands) == 0 {
		return nil
	}
	f.found = true
	return l.resolveFields(f.subFields, childCands, eff, gerr)
}

// resolveListField resolves a []struct field. A list is taken whole from the
// first candidate that holds its key (list elements cannot be merged across
// backends), and each element's subfields are locked to that same backend.
func (l *Loader) resolveListField(f *field, cands []candidate, gerr *[]string) error {
	for _, c := range cands {
		v, ok := c.data[f.key]
		if !ok {
			continue
		}
		newDatas, err := c.content.Encoder.DecodeDataList(v)
		if err != nil {
			*gerr = append(*gerr, err.Error())
			continue
		}
		val := reflect.MakeSlice(f.value.Type(), len(newDatas), len(newDatas))
		f.value.Set(val)
		elemSources := []string{c.name}
		for i, newData := range newDatas {
			// Re-bind onto the real slice element so each subfield's origValue
			// targets the right struct field (by spec.index), regardless of
			// unexported/skipped/flattened fields. It also gives every element
			// fresh found flags, so a required subfield is validated per element.
			elemFields := bind(f.spec.subSpecs, f.value.Index(i))
			elemCands := []candidate{{name: c.name, content: c.content, data: newData}}
			if err := l.resolveFields(elemFields, elemCands, elemSources, gerr); err != nil {
				return err
			}
			// Each element has its own fresh found flags, so a required subfield
			// is validated per element — after write-back, so a missing required
			// key keeps the values that did load.
			if err := validateRequired(elemFields); err != nil {
				return err
			}
		}
		f.found = true
		return nil
	}
	return nil
}

// validateRequired walks the bound field tree and returns the first required
// field that was not found. getFieldData sets the found flags at every depth,
// so a required field nested arbitrarily deep is validated too. List elements
// are validated per element during decode (the template subFields of a list
// only carry an "any element" found flag), so lists are not descended here.
func validateRequired(fields []*field) error {
	for _, f := range fields {
		if f.required && !f.found {
			return fmt.Errorf("required key '%s' for field '%s' not found", f.key, f.name)
		}
		if f.isList {
			continue
		}
		if err := validateRequired(f.subFields); err != nil {
			return err
		}
	}
	return nil
}

// decodeEncrypted resolves a leaf field tagged with the `encrypted` option.
// The raw value is decoded into a string with the content's own encoder and
// handed to the crypto, which decrypts ENC(...) values and passes plain
// values through unchanged.
func (l *Loader) decodeEncrypted(f *field, c *backend.Content, v interface{}) error {
	var s string
	if err := c.Encoder.Decode(v, &s); err != nil {
		return err
	}
	s, err := l.crypto.DecryptValue(s)
	if err != nil {
		return fmt.Errorf("field '%s': %s", f.name, err)
	}
	target := f.value
	switch {
	case target.Kind() == reflect.Ptr && target.Type().Elem().Kind() == reflect.String:
		p := reflect.New(target.Type().Elem())
		p.Elem().SetString(s)
		target.Set(p)
	case target.Kind() == reflect.String:
		target.SetString(s)
	default:
		return fmt.Errorf("field '%s': 'encrypted' option requires a string or *string field", f.name)
	}
	f.found = true
	return nil
}
