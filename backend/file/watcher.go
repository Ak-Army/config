package file

import (
	"github.com/Ak-Army/config/backend"
)

func newWatcher(f *file) (backend.Watcher, error) {
	return backend.NewPollWatcher(f.path, f.watchInterval, f.Read)
}
