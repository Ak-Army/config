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

var notFoundError = errors.New("not found")

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
	source    string
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
	value     reflect.Value
	origValue reflect.Value
	required  bool
	encrypted bool
	isList    bool
	source    string
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
	for _, s := range sources {
		if err := l.syncSource(s); err != nil {
			gerr = append(gerr, err.Error())
			continue
		}
		l.mu.Lock()
		l.backend = append(l.backend, s)
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
	l.backendWatcher = append(l.backendWatcher, s)
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
				parseTagSpec(tag, spec)
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
				parseTagSpec(tag, spec)
				list = append(list, spec)
				continue
			}
			if tag == "-" {
				continue
			}
			parseTagSpec(tag, spec)
			list = append(list, spec)
		case reflect.Ptr:
			if typ.Elem().Kind() == reflect.Struct {
				if tag == "-" {
					spec.handling = handleFlattenPtr
					spec.subSpecs = parseType(typ.Elem())
				} else {
					spec.handling = handlePtrStruct
					spec.subSpecs = parseType(typ.Elem())
					parseTagSpec(tag, spec)
				}
				list = append(list, spec)
				continue
			}
			if tag == "-" {
				continue
			}
			parseTagSpec(tag, spec)
			list = append(list, spec)
		default:
			if tag == "-" {
				continue
			}
			parseTagSpec(tag, spec)
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
			value := reflect.New(originalValue.Type()).Elem()
			value.Set(originalValue)
			list = append(list, bind(spec.subSpecs, value)...)
			continue
		case handleFlattenPtr:
			list = append(list, bind(spec.subSpecs, ptrElem(originalValue))...)
			continue
		}
		f := &backing[bi]
		bi++
		f.name = spec.name
		f.key = spec.key
		f.origValue = originalValue
		f.required = spec.required
		f.encrypted = spec.encrypted
		f.isList = spec.isList
		f.source = spec.source
		switch spec.handling {
		case handleStruct:
			value := reflect.New(originalValue.Type()).Elem()
			value.Set(originalValue)
			f.value = value
			f.subFields = bind(spec.subSpecs, value)
		case handlePtrStruct:
			value := reflect.New(originalValue.Type()).Elem()
			value.Set(originalValue)
			f.value = value
			f.subFields = bind(spec.subSpecs, ptrElem(originalValue))
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

// ptrElem returns the struct value a *struct field points at, allocating a
// fresh one when the field is nil.
func ptrElem(originalValue reflect.Value) reflect.Value {
	if originalValue.IsNil() {
		return reflect.New(originalValue.Type().Elem()).Elem()
	}
	return originalValue.Elem()
}

func parseTagSpec(tag string, spec *fieldSpec) {
	if idx := strings.Index(tag, ","); idx != -1 {
		spec.key = tag[:idx]
		opts := strings.Split(tag[idx+1:], ",")

		for _, opt := range opts {
			if opt == "required" {
				spec.required = true
			}
			if opt == "encrypted" {
				spec.encrypted = true
			}
			if strings.HasPrefix(opt, "backend=") {
				spec.source = opt[len("backend="):]
			}
		}
	}
}

func (l *Loader) resolve(fields []*field) error {
	var gerr []string
	for _, f := range fields {
		var backendFound bool
		// Iterate the backends in registration order so that source
		// precedence is deterministic (the first registered source that
		// provides the key wins), rather than depending on Go's random
		// map iteration order.
		for _, s := range l.backend {
			data, ok := l.maps[s]
			if !ok {
				continue
			}
			if f.source != "" && f.source != s.String() {
				continue
			}
			backendFound = true
			if err := l.getFieldData(f, data, data.Data); err != nil {
				if !errors.Is(err, notFoundError) {
					gerr = append(gerr, err.Error())
				}
				continue
			}
			break
		}
		if f.found {
			f.origValue.Set(f.value)
		}
		if f.source != "" && !backendFound {
			return fmt.Errorf("the backend: '%s' is not supported", f.source)
		}
		if f.required && !f.found {
			return fmt.Errorf("required key '%s' for field '%s' not found", f.key, f.name)
		}
		if len(f.subFields) != 0 {
			for _, subF := range f.subFields {
				if subF.required && !subF.found {
					return fmt.Errorf("required key '%s' for field '%s' not found", subF.key, subF.name)
				}
			}
		}
	}
	if len(gerr) > 0 {
		return fmt.Errorf("data loading errors: %s", strings.Join(gerr, "\n"))
	}
	return nil
}

func (l *Loader) getFieldData(f *field, c *backend.Content, data encoder.Data) error {
	v, found := data[f.key]
	if !found {
		return errors.WithMessage(notFoundError, fmt.Sprintf("data %s", f.key))
	}

	if len(f.subFields) != 0 {
		if f.isList {
			newDatas, err := c.Encoder.DecodeDataList(v)
			if err != nil {
				return err
			}
			val := reflect.MakeSlice(f.value.Type(), len(newDatas), len(newDatas))
			f.value.Set(val)
			for i, newData := range newDatas {
				for a, subF := range f.subFields {
					subF.value = reflect.New(subF.value.Type()).Elem()
					f.subFields[a].value = subF.value
					// Reset found for every element so that a required
					// subfield is validated per list element instead of
					// leaking a stale "found" from a previous element.
					subF.found = false
					if err := l.getFieldData(subF, c, newData); err != nil {
						// A missing key just leaves the subfield unset;
						// a real decode/decrypt failure must not be
						// swallowed, or e.g. a tampered encrypted value
						// would silently load as a zero value.
						if !errors.Is(err, notFoundError) {
							return err
						}
						continue
					}
					f.value.Index(i).Field(a).Set(subF.value)
				}
				for _, subF := range f.subFields {
					if subF.required && !subF.found {
						return fmt.Errorf("required key '%s' for field '%s' not found", subF.key, subF.name)
					}
				}
			}
			f.found = true
			return nil
		}
		newData, err := c.Encoder.DecodeData(v)
		if err != nil {
			return err
		}
		for a, subF := range f.subFields {
			origValue := f.value
			kind := f.value.Type().Kind()
			if kind == reflect.Ptr && f.value.IsNil() {
				f.value = reflect.New(f.value.Type().Elem())
			}
			if err := l.getFieldData(subF, c, newData); err != nil {
				f.value = origValue
				if !errors.Is(err, notFoundError) {
					return err
				}
				continue
			}
			if kind == reflect.Struct {
				f.value.Field(a).Set(subF.value)
			} else {
				f.value.Elem().Field(a).Set(subF.value)
			}

		}
		f.found = true
		return nil
	}
	if f.encrypted {
		return l.decodeEncrypted(f, c, v)
	}
	var to interface{}
	if f.value.CanAddr() {
		to = f.value.Addr().Interface()
	} else {
		to = f.value.Interface()
	}
	if err := c.Encoder.Decode(v, to); err != nil {
		return err
	}
	f.found = true
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
