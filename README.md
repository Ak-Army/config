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

The easiest way to hold a configuration is the generic `config.Store`. You give
it a `Handler` that supplies the defaults (and optional post-processing) and read
the resolved value back with `Store.Config()` — the store takes care of the
snapshot plumbing and locking for you.

```go
package main

import (
	"context"
	"fmt"
	"log"
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

// Settings is its own config.Handler[Settings]: Default supplies the defaults
// and Set post-processes the resolved snapshot.
func (*Settings) Default() *Settings {
	return &Settings{Port: 8080, Timeout: 30}
}

func (*Settings) Set(s *Settings) {
	s.Timeout *= time.Second // scale the raw number into a duration
}

func main() {
	loader, err := config.NewLoader(context.Background(),
		env.New(env.WithDefaults(".env")),
		file.New(file.WithPath("config.json")),
	)
	if err != nil {
		log.Fatal(err)
	}

	store := config.NewStore[Settings](&Settings{})
	if err := config.Load(loader, store); err != nil {
		log.Fatal(err) // only structural errors (e.g. target is not a pointer to struct)
	}

	cfg, err := store.Config()
	// Per-load errors (missing required keys, decode failures) are surfaced
	// here, not returned by Load.
	fmt.Printf("%+v, err: %v\n", cfg, err)
}
```

## Holding the configuration

The loader loads into a `config.Store[T]` — a generic, concurrency-safe holder
for the configuration struct `T`. It is the only target `Load` accepts:

```go
func Load[T any](l *config.Loader, s *config.Store[T]) error
```

Build a store with `config.NewStore[T]` and pass it a `config.Handler[T]` that
supplies the type-specific parts:

```go
type Handler[T any] interface {
	Default() *T // fresh *T with defaults; called for every (re)load
	Set(*T)      // post-process the resolved snapshot (scale durations, derive fields)
}
```

On every load (including watcher-triggered reloads) the store hands the loader a
fresh `*T` from `Default()`, the loader resolves the sources into it, `Set()`
post-processes the result, and the value is stored. Read the current value back
with `store.Config()`, which is safe to call concurrently with reloads and
returns the last load's error alongside the config, so a bad reload never
silently replaces a good snapshot.

`NewStore` also accepts a `nil` handler, in which case a zero-valued `*T` is used
for every load and no post-processing runs:

```go
store := config.NewStore[Settings](nil)
```

A common pattern is to let the configuration struct be its own handler by
defining `Default` and `Set` on it (as in the quick start above), then passing a
zero value: `config.NewStore[Settings](&Settings{})`.

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

Enable with `backend.WithWatcher()`. When a watched source changes, every store
previously passed to `Load` is re-populated and its next `Config()` returns the
new snapshot. Watching stops when the `context.Context` given to `NewLoader`
is cancelled.

## Error handling

There are two error channels:

- `Load` (and `NewLoader`/`AddSource`) return **structural** errors — a bad
  target type, or a source that fails its initial read.
- **Per-field** errors (missing required keys, decode failures) are surfaced via
  `store.Config()`'s second return value, so a bad reload never replaces a good
  snapshot silently.
