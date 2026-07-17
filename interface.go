package config

// loadable is the internal contract the Loader resolves configuration into and
// re-resolves on watcher-triggered reloads. Its methods are unexported, so only
// types declared in this package can satisfy it — Store is the sole
// implementation, which is why Load accepts only a *Store.
type loadable interface {
	newSnapshot() interface{}
	setSnapshot(interface{}, error)
}
