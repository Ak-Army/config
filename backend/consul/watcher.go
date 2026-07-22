package consul

import (
	"log"
	"sync"

	"github.com/Ak-Army/xlog"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"

	"github.com/Ak-Army/config/backend"
)

type watcher struct {
	c *consul

	wp       *watch.Plan
	ch       chan *backend.Content
	exit     chan bool
	logger   xlog.Logger
	stopOnce sync.Once
}

func newWatcher(c *consul) (backend.Watcher, error) {
	w := &watcher{
		c:      c,
		ch:     make(chan *backend.Content),
		exit:   make(chan bool),
		logger: xlog.FromContext(c.opts.Context),
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
		// A swallowed update means the app silently keeps stale config until
		// the next KV change, so leave a trace.
		w.logger.Warnf("consul watcher: unexpected data type %T, update dropped", data)
		return
	}
	cs, err := w.c.read(kvs)
	if err != nil {
		w.logger.Errorf("consul watcher: read failed, update dropped: %s", err)
		return
	}
	// The consumer may have stopped reading (context cancelled); a bare send
	// would block this handler and leak the watch-plan goroutine forever.
	select {
	case w.ch <- cs:
	case <-w.exit:
	}
}

func (w *watcher) Watch() <-chan *backend.Content {
	go w.wp.RunWithClientAndLogger(w.c.client, log.New(w.logger, "watch:", 0)) //lint:ignore SA1019 .

	return w.ch
}

func (w *watcher) Stop() {
	w.stopOnce.Do(func() {
		w.wp.Stop()
		close(w.exit)
	})
}
