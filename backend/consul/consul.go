package consul

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
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
	if c.client == nil {
		return nil, fmt.Errorf("consul client not set, use consul.WithClient")
	}
	kv, _, err := c.client.KV().List(c.prefix, nil)
	if err != nil {
		return nil, err
	}

	if len(kv) == 0 {
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
	tree := make(map[string]interface{})
	for _, v := range kv {
		pathString := strings.TrimPrefix(strings.TrimPrefix(v.Key, strings.TrimPrefix(c.stripPrefix, "/")), "/")
		if pathString == "" {
			continue
		}
		val, err := c.opts.Encoder.DecodeValue(v.Value)
		if err != nil {
			return nil, fmt.Errorf("consul value at %q: %w", v.Key, err)
		}
		if err := insert(tree, strings.Split(pathString, "/"), val); err != nil {
			return nil, err
		}
	}
	// The tree holds parsed Go values, so a single json.Marshal per top-level key
	// yields the neutral json.RawMessage leaf representation the loader consumes,
	// with no format mixing.
	s.Data = make(encoder.Data, len(tree))
	for k, v := range tree {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		s.Data[k] = json.RawMessage(b)
	}
	return s, nil
}

// insert places val into tree at the given key path, creating intermediate
// tables for the path segments and deep-merging tables so path-based nesting
// and in-value tables compose (e.g. `db` = {host} together with `db/pass` = "x"
// yields {host, pass}). Putting a leaf where a table already lives, or a table
// where a leaf lives, is a conflict.
func insert(tree map[string]interface{}, path []string, val interface{}) error {
	m := tree
	for _, dir := range path[:len(path)-1] {
		existing, ok := m[dir]
		if !ok {
			next := make(map[string]interface{})
			m[dir] = next
			m = next
			continue
		}
		next, ok := existing.(map[string]interface{})
		if !ok {
			return fmt.Errorf("consul key path conflict at %q: %q is both a leaf and a prefix",
				strings.Join(path, "/"),
				dir)
		}
		m = next
	}
	leaf := path[len(path)-1]
	existing, ok := m[leaf]
	if !ok {
		m[leaf] = val
		return nil
	}
	// Both sides are tables: deep-merge so path nesting and in-value tables compose.
	if em, eok := existing.(map[string]interface{}); eok {
		if vm, vok := val.(map[string]interface{}); vok {
			mergeMaps(em, vm)
			return nil
		}
	}
	return fmt.Errorf("consul key path conflict at %q: %q is set with both a value and a table",
		strings.Join(path, "/"),
		leaf)
}

// mergeMaps deep-merges src into dst, recursing into tables present on both
// sides. A non-table collision lets src win, matching a plain map assignment.
func mergeMaps(dst, src map[string]interface{}) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dm, dok := dv.(map[string]interface{}); dok {
				if sm, sok := sv.(map[string]interface{}); sok {
					mergeMaps(dm, sm)
					continue
				}
			}
		}
		dst[k] = sv
	}
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
