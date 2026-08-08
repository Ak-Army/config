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
`config:"<key>[,required][,encrypted][,backend=<name>]"`
```

| Token           | Meaning                                                                 |
|-----------------|-------------------------------------------------------------------------|
| `<key>`         | Key looked up in the source data.                                       |
| `required`      | Load fails (via the snapshot error) if the key is not found.            |
| `encrypted`     | The value may be an `ENC(...)` encrypted string; see [Encrypted values](#encrypted-values). |
| `backend=<name>`| Only read this field from the source whose name matches `<name>`. May be repeated (`backend=a,backend=b`) to allow several sources. |
| `-` (as `<key>`)| For a struct field: inline its fields into the parent. Otherwise: skip. |

Nested `struct`, `*struct`, and `[]struct` fields are resolved recursively;
the key names the sub-document. A `*config.SubConfig` field keeps its
sub-document undecoded, see [Sub-configs](#sub-configs). Fields with no tag are
ignored.

> **Note:** the option order matters — the key must come first
> (`"key,required"`, not `"required,key"`).

## Sub-configs

Sometimes the *shape* of a configuration block is not known when the
configuration struct is written: the same block means one thing for one app and
something entirely different for another. Type such a field
`*config.SubConfig` — the loader then captures the sub-document instead of
decoding it, and the application decides at runtime what to load it into:

```go
type Amd2Config struct {
	Active    bool              `config:"active"`
	AppParams *config.SubConfig `config:"app-params"`
}

type Amd2AppParams struct {
	Record   int    `config:"record"`
	Filepath string `config:"filepath"`
}

type Amd2AppParamsSecond struct {
	Mode   string `config:"mode"`
	APIKey string `config:"api-key,encrypted"`
}
```

```go
// Values the target already holds are its defaults: only keys present in the
// sources overwrite them.
params := &Amd2AppParams{Record: 1}
if err := cfg.Amd2Config.AppParams.Load(params); err != nil { ... }

// The very same sub-document, loaded into an unrelated struct:
second := &Amd2AppParamsSecond{}
if err := cfg.Amd2Config.AppParams.Load(second); err != nil { ... }
```

`Load` resolves the target exactly like a nested struct field is resolved, so
everything above keeps working inside a sub-config: `config` tags, nested
structs and lists, `required`, `encrypted` (the loader's crypto decrypts
`ENC(...)` values in the target too), per-field source precedence and merging
across sources. A `backend=` pin on the `SubConfig` field locks the target's
fields to the same sources, just as it does for a nested struct. Every encoder
(JSON, YAML, TOML) and backend is supported — the sub-document is decoded with
the encoder of the source it came from.

Two more rules:

- If the key is in none of the sources, the field stays `nil` and `Load` is a
  no-op that leaves the target untouched (a `nil` `*SubConfig` is safe to call).
  Tag the field `required` to turn a missing block into a load error.
- A `SubConfig` belongs to the snapshot it was loaded into. After a
  watcher-triggered reload take it from a fresh `store.Config()` and call `Load`
  again to see the new values.

## Encrypted values

Sensitive fields (passwords, API keys) can be stored encrypted in the config
source. Mark the field with the `encrypted` tag option and store the value in
the `ENC(<kid>:<base64>)` envelope, where `<kid>` names the key it was
encrypted with:

```go
type Settings struct {
	DBPass string `config:"db_pass,encrypted"`
}
```

```json
{ "db_pass": "ENC(prod-2026-07:4Yw3...base64...)" }
```

Keys live in a **key-ring file**: one `<kid>: <base64 32-byte key>` entry per
line, blank lines and `#` comments allowed, the **first entry is the active
key** (used for encryption; the others only decrypt):

```
# config.keyring
prod-2026-07: 4Yw3...base64...   # active
prod-2026-01: 9k2f...base64...
```

Configure a `crypto.Crypto` on the loader before calling `Load`. It wraps the
key ring and the ENC(...) envelope handling, hiding the cipher implementation
from the loader. `crypto.New` loads a key ring file and turns each raw key
into a `crypto.Decrypter` with the key parser you supply — the cipher is
pluggable, the `crypto/aesgcm` subpackage ships AES-256-GCM, but anything
implementing `crypto.Decrypter` (Vault, KMS, ...) can be a key:

```go
import (
	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

cr, err := crypto.New("config.keyring", func(key []byte) (crypto.Decrypter, error) {
	return aesgcm.New(key)
})
if err != nil { ... }
loader.SetCrypto(cr)
```

Rules:

- `encrypted` applies only to leaf `string` / `*string` fields (including
  named string types); on other field types the load reports an error, on
  struct/list fields the option is ignored.
- A tagged field whose value is **not** in `ENC(...)` form passes through as
  plaintext — the same field can be encrypted in production and plain in a
  local config.
- An `ENC(...)` value without a configured crypto, with an unknown key id, or
  one that fails to decrypt, surfaces as a per-field error via
  `store.Config()`.
- The feature is encoder- and backend-agnostic: it works with JSON, YAML and
  TOML files, env and Consul sources alike.

### Producing encrypted values

Use the `configcrypt` helper (alias it as
`go run github.com/Ak-Army/config/cmd/configcrypt`). It takes a command
(`create`, `update`, `encrypt`, `decrypt`), and every command needs the keyring
with `-key`:

```sh
# create a keyring holding one fresh 32-byte key
# (-kid defaults to kid-<YYYYMMDD>; the file is overwritten if it exists)
configcrypt create -key config.keyring -kid prod-2026-07

# encrypt a value with the active key
configcrypt encrypt -key config.keyring -in "s3cr3t"
# -> ENC(prod-2026-07:4Yw3...base64...)

# decrypt / inspect a value
configcrypt decrypt -key config.keyring -in 'ENC(prod-2026-07:4Yw3...)'

# decrypt every ENC(...) value of a whole config: preview on stdout,
# or rewrite the file in place with -write
configcrypt decrypt -key config.keyring -file config.json
```

Copy the printed `ENC(...)` envelope into the config file — `encrypt -file`
does not encrypt plain values, it re-keys the already encrypted ones (see
below). Programmatic encryption is available via `crypto.EncryptValue`.

### Key rotation

Every value names its key, so old and new keys can coexist while configs are
re-encrypted:

1. Add a new key to the keyring:

   ```sh
   configcrypt update -key config.keyring -kid prod-2026-07
   ```

   `update` **appends** the key, so move its line to the **top** of the file to
   make it the active one, keeping the old entry below it (it is still needed to
   decrypt the not yet re-encrypted values).
2. Deploy the keyring — services now decrypt both old and new values.
3. Re-encrypt each config with the active key; only `ENC(...)` values change,
   every other byte of the file stays untouched (works for JSON, YAML, TOML):

   ```sh
   configcrypt encrypt -key config.keyring -file config.json
   ```

   The file is rewritten in place (atomically); values already encrypted with
   the active key are left alone, so re-running it is a no-op.
4. Once no config references the old key id (`grep -r 'ENC(prod-2026-01:'`),
   remove its line from the keyring.

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

Resolution is **per field**: each field independently takes its value from the
**first registered source that has the key** (deterministic, in registration
order). Because of this, a nested `struct` can be assembled from several sources
at once — each of its fields is filled from whichever source provides it.

Pin a field to one or more sources with `backend=<name>` (repeat the option to
allow several); the field is then read only from those, still in registration
order. A pin on a nested `struct` or `[]struct` **locks all of its subfields**
to the same sources — a subfield's own `backend=` cannot widen or change it.

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
