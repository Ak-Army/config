package consul

import (
	"github.com/hashicorp/consul/api"

	"github.com/Ak-Army/config/backend"
)

type Option func(o *consul)

func WithStripPrefix(stripPrefix string) Option {
	return func(c *consul) {
		c.stripPrefix = stripPrefix
	}
}

func WithPrefix(prefix string) Option {
	return func(c *consul) {
		c.prefix = prefix
	}
}

func WithClient(client *api.Client) Option {
	return func(c *consul) {
		c.client = client
	}
}

func WithOption(opt backend.Option) Option {
	return func(c *consul) {
		opt(&c.opts)
	}
}
