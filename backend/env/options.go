package env

import (
	"time"

	"github.com/Ak-Army/config/backend"
)

type Option func(o *env)

func WithStripPrefix(stripPrefix string) Option {
	return func(e *env) {
		e.stripPrefixes = append(e.stripPrefixes, stripPrefix)
	}
}

func WithPrefix(prefix string) Option {
	return func(e *env) {
		e.prefixes = append(e.prefixes, prefix)
	}
}

func WithDefaults(defaults string) Option {
	return func(e *env) {
		e.defaults = defaults
	}
}

func WithWatchInterval(t time.Duration) Option {
	return func(e *env) {
		e.watchInterval = t
	}
}

func WithOption(opt backend.Option) Option {
	return func(c *env) {
		opt(&c.opts)
	}
}
