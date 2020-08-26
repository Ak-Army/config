package file

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"

	"github.com/Ak-Army/config/backend"
)

type watcher struct {
	f    *file
	hash string
	exit chan bool
}

func newWatcher(f *file) (backend.Watcher, error) {
	w := &watcher{
		f:    f,
		exit: make(chan bool),
	}
	return w, w.updateHash()
}

func (w *watcher) Watch() <-chan *backend.Content {
	ch := make(chan *backend.Content)
	go func() {
		timer := time.NewTimer(w.f.watchInterval)
		for {
			select {
			case <-w.exit:
				return
			case <-timer.C:
				lastHash := w.hash
				if err := w.updateHash(); err != nil {
					break
				}
				if lastHash == w.hash {
					break
				}
				c, err := w.f.Read()
				if err != nil {
					break
				}
				select {
				case ch <- c:
				case <-w.exit:
					return
				}
			}
			timer.Reset(w.f.watchInterval)
		}
	}()

	return ch
}

func (w *watcher) Stop() {
	close(w.exit)
}

func (w *watcher) updateHash() error {
	file, err := os.Open(w.f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Annotate(err, "Open file error")
	}
	defer file.Close()
	s, err := file.Stat()
	if err != nil {
		return errors.Annotate(err, "Config file stat error")
	}
	w.hash = fmt.Sprintf("%d|%d", s.ModTime().UnixNano(), s.Size())
	return nil
}
