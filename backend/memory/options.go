package memory

import (
	"github.com/Ak-Army/config/backend"
)

type Option func(m *memory)

// WithValue serves v, typically a map[string]interface{}, encoded with the
// backend's encoder.
func WithValue(v interface{}) Option {
	return func(m *memory) {
		m.value, m.doc, m.set = v, nil, true
	}
}

// WithDocument serves doc, written in the backend's encoder format.
func WithDocument(doc []byte) Option {
	return func(m *memory) {
		m.value, m.doc, m.set = nil, doc, true
	}
}

func WithOption(opt backend.Option) Option {
	return func(m *memory) {
		opt(&m.opts)
	}
}
