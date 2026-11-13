package consul

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ak-Army/xlog"
	"github.com/hashicorp/consul/api"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
)

type consul struct {
	prefix      string
	stripPrefix string
	opts        backend.Options
	client      *api.Client
	// Only read touches these: it runs once from Read before the watcher starts,
	// then only on the single watch-plan goroutine, so they need no lock.
	lastValid map[string][]byte // Consul key -> raw value of its last usable version
	reported  map[string]uint64 // Consul key -> ModifyIndex already logged as unusable
}

// kvRead collects the outcome of one pass over the KV pairs.
type kvRead struct {
	tree     map[string]any
	valid    map[string][]byte
	unusable map[string]uint64
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

func (c *consul) String() string {
	return c.opts.Name
}

func (c *consul) Watcher() (backend.Watcher, error) {
	if !c.opts.Watcher {
		return nil, nil
	}
	return newWatcher(c)
}

// read builds the content tree key by key. An unusable value (undecodable, or
// clashing with another key's path) drops only its own key and falls back to
// that key's last usable value, so one bad key cannot freeze all the others.
func (c *consul) read(kvs api.KVPairs) (*backend.Content, error) {
	r := &kvRead{
		tree:     make(map[string]any),
		valid:    make(map[string][]byte, len(kvs)),
		unusable: make(map[string]uint64),
	}
	for _, kv := range kvs {
		c.readKey(r, kv)
	}
	c.lastValid, c.reported = r.valid, r.unusable
	return c.content(r.tree)
}

func (c *consul) readKey(r *kvRead, kv *api.KVPair) {
	path := c.keyPath(kv.Key)
	if path == nil {
		return
	}
	err := c.place(r.tree, path, kv.Value)
	if err == nil {
		r.valid[kv.Key] = kv.Value
		return
	}
	r.unusable[kv.Key] = kv.ModifyIndex
	kept := c.placeLastValid(r, path, kv.Key)
	c.reportUnusable(kv, kept, err)
}

// keyPath returns the tree path of a Consul key below stripPrefix, or nil for
// the stripped prefix itself and for folder keys (trailing "/"), which only
// group other keys and hold no value of their own.
func (c *consul) keyPath(key string) []string {
	pathString := strings.TrimPrefix(strings.TrimPrefix(key, strings.TrimPrefix(c.stripPrefix, "/")), "/")
	if pathString == "" || strings.HasSuffix(pathString, "/") {
		return nil
	}
	return strings.Split(pathString, "/")
}

func (c *consul) place(tree map[string]any, path []string, raw []byte) error {
	val, err := c.opts.Encoder.DecodeValue(raw)
	if err != nil {
		return fmt.Errorf("consul value: %w", err)
	}
	return insert(tree, path, val)
}

// placeLastValid re-decodes the stored raw value instead of caching the decoded
// one: insert merges into tables in place, so a cached table would pick up keys
// from other paths.
func (c *consul) placeLastValid(r *kvRead, path []string, key string) bool {
	raw, ok := c.lastValid[key]
	if !ok || c.place(r.tree, path, raw) != nil {
		return false
	}
	r.valid[key] = raw
	return true
}

// reportUnusable logs a key once per version: every watch event re-reads the
// whole prefix, so without the ModifyIndex check the same broken key would be
// logged again on every unrelated change.
func (c *consul) reportUnusable(kv *api.KVPair, kept bool, err error) {
	if index, ok := c.reported[kv.Key]; ok && index == kv.ModifyIndex {
		return
	}
	xlog.FromContext(c.opts.Context).Debug("Unable to use Consul value", xlog.F{
		"key":           kv.Key,
		"modifyIndex":   kv.ModifyIndex,
		"lastValidKept": kept,
	}, err)
}

// content marshals each top-level subtree once: the tree holds parsed Go values,
// so this yields the neutral json.RawMessage leaf representation the loader
// consumes, with no format mixing.
func (c *consul) content(tree map[string]any) (*backend.Content, error) {
	data := make(encoder.Data, len(tree))
	for k, v := range tree {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		data[k] = json.RawMessage(b)
	}
	return &backend.Content{
		Data:      data,
		Encoder:   c.opts.Encoder,
		Source:    c.String(),
		Timestamp: time.Now(),
	}, nil
}

// insert places val into tree at the given key path, creating intermediate
// tables for the path segments and deep-merging tables so path-based nesting
// and in-value tables compose (e.g. `db` = {host} together with `db/pass` = "x"
// yields {host, pass}). Putting a leaf where a table already lives, or a table
// where a leaf lives, is a conflict.
func insert(tree map[string]any, path []string, val any) error {
	m := tree
	for _, dir := range path[:len(path)-1] {
		existing, ok := m[dir]
		if !ok {
			next := make(map[string]any)
			m[dir] = next
			m = next
			continue
		}
		next, ok := existing.(map[string]any)
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
	if em, eok := existing.(map[string]any); eok {
		if vm, vok := val.(map[string]any); vok {
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
func mergeMaps(dst, src map[string]any) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dm, dok := dv.(map[string]any); dok {
				if sm, sok := sv.(map[string]any); sok {
					mergeMaps(dm, sm)
					continue
				}
			}
		}
		dst[k] = sv
	}
}
