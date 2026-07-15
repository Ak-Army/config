# config

A small, reflection-based configuration loader for Go with pluggable
**backends** (file, environment, Consul) and **encoders** (JSON, YAML, TOML).
It fills a target struct from one or more sources based on `config:"..."`
struct tags, supports nested structs, slices, defaults, required keys,
per-field source pinning, and hot-reloading (watch).

## Install

```sh
go get github.com/Ak-Army/config
```

## Quick start

```go
package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Ak-Army/config"
	"github.com/Ak-Army/config/backend/env"
	"github.com/Ak-Army/config/backend/file"
)

type Settings struct {
	Host    string        `config:"host,required"`
	Port    int           `config:"port"`
	Timeout time.Duration `config:"timeout"`
}

// A type implementing config.Config provides a fresh snapshot to fill and
// receives the populated result. It is responsible for its own locking.
type store struct {
	sync.RWMutex
	cfg Settings
	err error
}

func (s *store) NewSnapshot() interface{} {
	// Return a pointer to a struct pre-filled with defaults.
	return &Settings{Port: 8080}
}

func (s *store) SetSnapshot(v interface{}, err error) {
	s.Lock()
	defer s.Unlock()
	s.cfg, s.err = *v.(*Settings), err
}

func main() {
	loader, err := config.NewLoader(context.Background(),
		env.New(env.WithDefaults(".env")),
		file.New(file.WithPath("config.json")),
	)
	if err != nil {
		log.Fatal(err)
	}
	s := &store{}
	if err := loader.Load(s); err != nil {
		log.Fatal(err) // only structural errors (e.g. target is not a pointer to struct)
	}
	// Per-load errors (missing required keys, decode failures) are delivered
	// to SetSnapshot's err argument, not returned by Load.
}
```

## The `config` tag

```
`config:"<key>[,required][,backend=<name>]"`
```

| Token           | Meaning                                                                 |
|-----------------|-------------------------------------------------------------------------|
| `<key>`         | Key looked up in the source data.                                       |
| `required`      | Load fails (via the snapshot error) if the key is not found.            |
| `backend=<name>`| Only read this field from the source whose name matches `<name>`.       |
| `-` (as `<key>`)| For a struct field: inline its fields into the parent. Otherwise: skip. |

Nested `struct`, `*struct`, and `[]struct` fields are resolved recursively;
the key names the sub-document. Fields with no tag are ignored.

> **Note:** the option order matters — the key must come first
> (`"key,required"`, not `"required,key"`).

## Sources (backends)

| Backend  | Constructor        | Notable options                                             |
|----------|--------------------|-------------------------------------------------------------|
| File     | `file.New(...)`    | `WithPath`, `WithWatchInterval`, `WithOption(backend...)`   |
| Env      | `env.New(...)`     | `WithDefaults` (dotenv file), `WithPrefix`, `WithStripPrefix`, `WithWatchInterval` |
| Consul   | `consul.New(...)`  | `WithClient` (required), `WithPrefix`, `WithStripPrefix`     |

Common backend options (via `WithOption`): `backend.WithName(...)` sets the
name matched by `backend=`, `backend.WithEncoder(...)` picks the encoder,
`backend.WithWatcher()` enables watching.

### Source precedence

When several sources provide the same key and the field is **not** pinned with
`backend=`, the **first registered source that has the key wins**
(deterministic, in registration order). Pin a field to a specific source with
`backend=<name>`.

## Encoders

`encoder/json` (default), `encoder/yaml`, `encoder/toml`. Set per source:

```go
file.New(file.WithPath("config.yaml"),
	file.WithOption(backend.WithEncoder(yaml.New())))
```

## Watching / hot-reload

Enable with `backend.WithWatcher()`. When a watched source changes, every
target previously passed to `Load` is re-populated and its `SetSnapshot` is
called again. Watching stops when the `context.Context` given to `NewLoader`
is cancelled.

## Error handling

There are two error channels:

- `Load` (and `NewLoader`/`AddSource`) return **structural** errors — a bad
  target type, or a source that fails its initial read.
- **Per-field** errors (missing required keys, decode failures) are passed to
  your `SetSnapshot(v, err)` implementation so that a bad reload never replaces
  a good snapshot silently.
