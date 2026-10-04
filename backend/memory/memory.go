package memory

import (
	"errors"
	"time"

	"github.com/Ak-Army/config/backend"
)

type memory struct {
	opts  backend.Options
	value interface{}
	doc   []byte
	set   bool
}

func New(opts ...Option) backend.Backend {
	m := &memory{opts: backend.NewOptions()}
	m.opts.Name = "memory"
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *memory) Read() (*backend.Content, error) {
	if !m.set {
		return nil, errors.New("document not set")
	}
	doc := m.doc
	if doc == nil {
		var err error
		if doc, err = m.opts.Encoder.Encode(m.value); err != nil {
			return nil, err
		}
	}
	data, err := m.opts.Encoder.DecodeData(doc)
	if err != nil {
		return nil, err
	}
	return &backend.Content{
		Data:      data,
		Encoder:   m.opts.Encoder,
		Source:    m.String(),
		Timestamp: time.Now(),
	}, nil
}

// Watcher returns nil: the document never changes.
func (m *memory) Watcher() (backend.Watcher, error) {
	return nil, nil
}

func (m *memory) String() string {
	return m.opts.Name
}
