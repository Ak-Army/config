package env

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
)

type env struct {
	prefixes      []string
	stripPrefixes []string
	defaults      string
	watchInterval time.Duration
	opts          backend.Options
}

func New(opts ...Option) backend.Backend {
	e := &env{
		opts: backend.NewOptions(),
	}
	e.opts.Name = "env"
	for _, o := range opts {
		o(e)
	}
	return e
}

func (e *env) Read() (*backend.Content, error) {
	if e.defaults != "" {
		err := godotenv.Load(e.defaults)
		if err != nil {
			return nil, err
		}
	}
	s := &backend.Content{
		Encoder:   e.opts.Encoder,
		Source:    e.String(),
		Timestamp: time.Now(),
		Data:      make(encoder.Data),
	}

	for _, env := range os.Environ() {
		if len(e.prefixes) > 0 || len(e.stripPrefixes) > 0 {
			notFound := true
			if _, ok := matchPrefix(e.prefixes, env); ok {
				notFound = false
			}
			if match, ok := matchPrefix(e.stripPrefixes, env); ok {
				env = strings.TrimPrefix(env, match)
				notFound = false
			}
			if notFound {
				continue
			}
		}
		pair := strings.SplitN(env, "=", 2)
		value, _ := e.opts.Encoder.Encode(pair[1])
		key := strings.ToLower(pair[0])
		s.Data[key] = value
		s.Data[strings.Replace(key, "_", "-", -1)] = value
	}
	return s, nil
}

func matchPrefix(pre []string, s string) (string, bool) {
	for _, p := range pre {
		if strings.HasPrefix(s, p) {
			return p, true
		}
	}

	return "", false
}

func (e *env) Watcher() (backend.Watcher, error) {
	if !e.opts.Watcher {
		return nil, nil
	}
	return newWatcher(e)
}

func (e *env) String() string {
	return e.opts.Name
}
