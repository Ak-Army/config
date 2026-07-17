package config

import "sync"

// Handler supplies the type-specific behaviour a Store needs for the
// configuration struct T.
type Handler[T any] interface {
	// Default returns a freshly initialised *T holding the default values. It is
	// called for every (re)load, so it must return a new value each time.
	Default() *T
	// Set is applied to the resolved snapshot before it is stored, e.g. to
	// scale durations or derive computed fields.
	Set(*T)
}

// Store is the target Load resolves configuration into. It removes the
// boilerplate of snapshot creation, post-processing and locking for a concrete
// configuration struct T, delegating the type-specific parts to a Handler. It is
// the only type the loader can load into (see Load).
//
// Typical usage:
//
//	type myHandler struct{}
//	func (myHandler) Default() *MyConfig { return &MyConfig{Timeout: 30} }
//	func (myHandler) Set(c *MyConfig) { c.Timeout *= time.Second }
//
//	store := config.NewStore[MyConfig](myHandler{})
//	config.Load(loader, store)
//	cfg, err := store.Config()
type Store[T any] struct {
	mu      sync.RWMutex
	config  T
	err     error
	handler Handler[T]
}

// NewStore builds a Store for the configuration struct T, delegating snapshot
// creation and post-processing to handler. handler may be nil, in which case a
// zero-valued *T is used for every load and no post-processing is applied.
func NewStore[T any](handler Handler[T]) *Store[T] {
	return &Store[T]{handler: handler}
}

// newSnapshot returns a fresh target for the loader to resolve into. It
// satisfies the internal loadable interface and is called by the loader, not
// directly.
func (s *Store[T]) newSnapshot() interface{} {
	if s.handler != nil {
		return s.handler.Default()
	}
	return new(T)
}

// setSnapshot stores the resolved snapshot (and any error) after letting the
// handler post-process it. It satisfies the internal loadable interface.
func (s *Store[T]) setSnapshot(snapshot interface{}, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conf := snapshot.(*T)
	if s.handler != nil {
		s.handler.Set(conf)
	}
	s.config = *conf
	s.err = err
}

// Config returns the most recently loaded configuration and the error from that
// load. It is safe to call concurrently with reloads triggered by watchers.
func (s *Store[T]) Config() (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config, s.err
}
