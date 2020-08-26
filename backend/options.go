package backend

import (
	"context"

	"github.com/Ak-Army/config/encoder"
	"github.com/Ak-Army/config/encoder/json"
)

type Options struct {
	Name    string
	Encoder encoder.Encoder
	Context context.Context
	Watcher bool
}

type Option func(o *Options)

func NewOptions(opts ...Option) Options {
	options := Options{
		Encoder: json.New(),
		Context: context.Background(),
	}
	for _, o := range opts {
		o(&options)
	}

	return options
}

func WithEncoder(e encoder.Encoder) Option {
	return func(o *Options) {
		o.Encoder = e
	}
}

func WithName(name string) Option {
	return func(o *Options) {
		o.Name = name
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *Options) {
		o.Context = ctx
	}
}

func WithWatcher() Option {
	return func(o *Options) {
		o.Watcher = true
	}
}
