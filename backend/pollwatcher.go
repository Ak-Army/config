package backend

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const defaultPollInterval = 5 * time.Second

// NewPollWatcher polls path's mtime+size every interval and delivers a fresh
// Content from read when it changes. A non-positive interval falls back to a
// 5s default (a zero interval would busy-loop). An empty path yields a dormant
// watcher whose channel never delivers, so backends without a file to watch
// (e.g. env without a defaults file) can still return a valid Watcher.
func NewPollWatcher(path string, interval time.Duration, read func() (*Content, error)) (Watcher, error) {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	w := &pollWatcher{
		path:     path,
		interval: interval,
		read:     read,
		exit:     make(chan bool),
	}
	if path == "" {
		return w, nil
	}
	return w, w.updateHash()
}

type pollWatcher struct {
	path     string
	interval time.Duration
	read     func() (*Content, error)
	hash     string
	exit     chan bool
	stopOnce sync.Once
}

func (w *pollWatcher) Watch() <-chan *Content {
	ch := make(chan *Content)
	if w.path == "" {
		return ch
	}
	go func() {
		timer := time.NewTimer(w.interval)
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
				c, err := w.read()
				if err != nil {
					// Roll the hash back so the change is retried on the
					// next tick instead of being permanently swallowed
					// (e.g. a transient error or a half-written file).
					w.hash = lastHash
					break
				}
				select {
				case ch <- c:
				case <-w.exit:
					return
				}
			}
			timer.Reset(w.interval)
		}
	}()

	return ch
}

func (w *pollWatcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.exit)
	})
}

func (w *pollWatcher) updateHash() error {
	file, err := os.Open(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.WithMessage(err, "open file error")
	}
	defer file.Close()
	s, err := file.Stat()
	if err != nil {
		return errors.WithMessage(err, "config file stat error")
	}
	w.hash = fmt.Sprintf("%d|%d", s.ModTime().UnixNano(), s.Size())
	return nil
}
