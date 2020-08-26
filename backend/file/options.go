package file

import (
	"time"

	"github.com/Ak-Army/config/backend"
)

type Option func(o *file)

func WithWatchInterval(t time.Duration) Option {
	return func(f *file) {
		f.watchInterval = t
	}
}

func WithPath(path string) Option {
	return func(f *file) {
		f.path = path
	}
}

func WithOption(opt backend.Option) Option {
	return func(f *file) {
		opt(&f.opts)
	}
}
