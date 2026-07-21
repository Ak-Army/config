package env

import (
	"encoding/json"
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
	// defaultsLoaded flips after the first successful Read. The first load
	// must not override real environment variables (env wins over the
	// defaults file), but reloads triggered by the watcher must override,
	// otherwise a changed defaults file never takes effect.
	defaultsLoaded bool
}

func New(opts ...Option) backend.Backend {
	e := &env{
		opts:          backend.NewOptions(),
		watchInterval: 5 * time.Second,
	}
	e.opts.Name = "env"
	for _, o := range opts {
		o(e)
	}
	return e
}

func (e *env) Read() (*backend.Content, error) {
	// Merge the real environment with the defaults file into a single map
	// instead of mutating the process environment (godotenv.Load/Overload).
	// This also fixes reload staleness: a key removed from the defaults file
	// simply drops out of the map instead of lingering in os.Environ.
	merged := envToMap(os.Environ())
	if e.defaults != "" {
		defaults, err := godotenv.Read(e.defaults)
		if err != nil {
			return nil, err
		}
		for k, v := range defaults {
			// On the first load real environment variables win over the
			// defaults file; on reloads the (possibly changed) file wins,
			// otherwise a changed default would never take effect.
			if _, ok := merged[k]; ok && !e.defaultsLoaded {
				continue
			}
			merged[k] = v
		}
		e.defaultsLoaded = true
	}

	s := &backend.Content{
		Encoder:   e.opts.Encoder,
		Source:    e.String(),
		Timestamp: time.Now(),
		Data:      make(encoder.Data),
	}

	for k, v := range merged {
		if len(e.prefixes) > 0 || len(e.stripPrefixes) > 0 {
			notFound := true
			if _, ok := matchPrefix(e.prefixes, k); ok {
				notFound = false
			}
			if match, ok := matchPrefix(e.stripPrefixes, k); ok {
				k = strings.TrimPrefix(k, match)
				notFound = false
			}
			if notFound {
				continue
			}
		}
		// Store the value as json.RawMessage: every encoder's Decode accepts
		// it, unlike the raw []byte an encoder-specific Encode would produce
		// (and TOML cannot encode a bare string at all).
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		value := json.RawMessage(raw)
		key := strings.ToLower(k)
		s.Data[key] = value
		s.Data[strings.Replace(key, "_", "-", -1)] = value
	}
	return s, nil
}

// envToMap splits a `KEY=VALUE` slice (as returned by os.Environ) into a map.
func envToMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, env := range environ {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		m[pair[0]] = pair[1]
	}
	return m
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
