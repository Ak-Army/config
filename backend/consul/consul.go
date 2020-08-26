package consul

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/Ak-Army/config/backend"
)

type consul struct {
	prefix      string
	stripPrefix string
	opts        backend.Options
	client      *api.Client
}

func New(opts ...Option) backend.Backend {
	c := &consul{
		opts: backend.NewOptions(),
	}
	c.opts.Name = "consul"
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *consul) Read() (*backend.Content, error) {
	kv, _, err := c.client.KV().List(c.prefix, nil)
	if err != nil {
		return nil, err
	}

	if kv == nil || len(kv) == 0 {
		return nil, fmt.Errorf("source not found: %s", c.prefix)
	}

	return c.read(kv)
}

func (c *consul) read(kv api.KVPairs) (*backend.Content, error) {
	s := &backend.Content{
		Encoder:   c.opts.Encoder,
		Source:    c.String(),
		Timestamp: time.Now(),
	}
	data := make(map[string]interface{})
	for _, v := range kv {
		pathString := strings.TrimPrefix(strings.TrimPrefix(v.Key, strings.TrimPrefix(c.stripPrefix, "/")), "/")
		if pathString == "" {
			continue
		}
		target := data
		path := strings.Split(pathString, "/")
		for _, dir := range path[:len(path)-1] {
			if _, ok := target[dir]; !ok {
				target[dir] = make(map[string]interface{})
			}
			target = target[dir].(map[string]interface{})
		}
		leafDir := path[len(path)-1]
		target[leafDir] = v.Value
	}
	d, err := c.opts.Encoder.Encode(data)
	if err != nil {
		return nil, err
	}
	s.Data, err = c.opts.Encoder.DecodeData(d)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (c *consul) String() string {
	return c.opts.Name
}

func (c *consul) Watcher() (backend.Watcher, error) {
	if !c.opts.Watcher {
		return nil, nil
	}
	return newWatcher(c)
}
