package consul

import (
	"log"

	"github.com/Ak-Army/xlog"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"

	"github.com/Ak-Army/config/backend"
)

type watcher struct {
	c *consul

	wp   *watch.Plan
	ch   chan *backend.Content
	exit chan bool
}

func newWatcher(c *consul) (backend.Watcher, error) {
	w := &watcher{
		c:    c,
		ch:   make(chan *backend.Content),
		exit: make(chan bool),
	}
	wp, err := watch.Parse(map[string]interface{}{"type": "keyprefix", "prefix": c.prefix})
	if err != nil {
		return nil, err
	}
	wp.Handler = w.handle
	w.wp = wp
	return w, nil
}

func (w *watcher) handle(idx uint64, data interface{}) {
	if data == nil {
		return
	}
	kvs, ok := data.(api.KVPairs)
	if !ok {
		return
	}
	cs, err := w.c.read(kvs)
	if err != nil {
		return
	}
	w.ch <- cs
}

func (w *watcher) Watch() <-chan *backend.Content {
	logger := xlog.FromContext(w.c.opts.Context)
	go w.wp.RunWithClientAndLogger(w.c.client, log.New(logger, "watch:", 0)) //lint:ignore SA1019 .

	return w.ch
}

func (w *watcher) Stop() {
	w.wp.Stop()
	close(w.exit)
}
