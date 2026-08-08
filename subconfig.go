package config

import (
	"reflect"

	"github.com/pkg/errors"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
)

// subConfigType is matched by parseType to recognise a deferred sub-document
// field, whether it is declared as a SubConfig or a *SubConfig.
var subConfigType = reflect.TypeOf(SubConfig{})

// SubConfig is a deferred sub-document. A field typed *SubConfig (or SubConfig)
// captures the document behind its key instead of decoding it, so the
// application decides at runtime which struct the document is loaded into:
//
//	type Amd2Config struct {
//		Active    bool              `config:"active"`
//		AppParams *config.SubConfig `config:"app-params"`
//	}
//
//	params := &Amd2AppParams{Record: 1} // pre-filled values act as defaults
//	if err := cfg.Amd2Config.AppParams.Load(params); err != nil { ... }
//
// The same sub-document can be loaded into any number of unrelated structs.
// Load resolves the target exactly like a nested struct field is resolved:
// `config` tags, nested structs and lists, the `required` and `encrypted`
// options and the per-field source precedence all behave the same, and a
// `backend=` pin on the SubConfig field itself locks the target's fields to
// those sources.
//
// A SubConfig belongs to the snapshot it was loaded into and holds no decoded
// state, so it is safe for concurrent use. A watcher-triggered reload builds a
// new snapshot with a new SubConfig: take it from a fresh Store.Config() and
// call Load again to pick up changed values.
type SubConfig struct {
	loader *Loader
	docs   []subDoc
	// pin is the effective `backend=` restriction of the field the document was
	// captured from and locked marks a []struct element subtree. Both are
	// threaded into the target's parse, so its fields inherit exactly what a
	// nested struct's fields would inherit at that position.
	pin    []string
	locked bool
}

// subDoc is one source's view of the sub-document: the content it came from
// (carrying the encoder that decodes its values) and its top-level members.
type subDoc struct {
	name    string
	content *backend.Content
	data    encoder.Data
}

// Load resolves the sub-document into target, which must be a pointer to
// struct. Values target already holds are kept for every key the sources do not
// provide, so a pre-filled struct supplies the defaults.
//
// Load on a nil or empty SubConfig — the key was in none of the sources — leaves
// target untouched and returns nil, mirroring a missing nested struct. Tag the
// field `required` to turn that absence into a load error instead.
//
// Errors are returned per load: missing required keys, decode failures and
// values that cannot be decrypted are collected the same way they are for the
// enclosing snapshot.
func (s *SubConfig) Load(target interface{}) error {
	ref := reflect.ValueOf(target)
	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.IsNil() || ref.Elem().Kind() != reflect.Struct {
		return errors.New("provided target must be a pointer to struct")
	}
	// No documents means the loader never populated this SubConfig, so there is
	// no loader to resolve with either.
	if s == nil || len(s.docs) == 0 {
		return nil
	}
	s.loader.mu.Lock()
	defer s.loader.mu.Unlock()

	elem := ref.Elem()
	fields := bind(s.loader.specsFor(elem.Type(), s.pin, s.locked), elem)
	return s.loader.resolveWith(fields, s.candidates())
}

// candidates turns the captured sub-documents into the candidate list the
// resolver consumes, keeping the source precedence they were captured in.
func (s *SubConfig) candidates() []candidate {
	cands := make([]candidate, len(s.docs))
	for i, d := range s.docs {
		cands[i] = candidate{name: d.name, content: d.content, data: d.data}
	}
	return cands
}
