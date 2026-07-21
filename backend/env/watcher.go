package env

import (
	"github.com/Ak-Army/config/backend"
)

func newWatcher(e *env) (backend.Watcher, error) {
	// Without a defaults file there is nothing to poll: NewPollWatcher with an
	// empty path returns a dormant watcher whose channel never delivers.
	return backend.NewPollWatcher(e.defaults, e.watchInterval, e.Read)
}
