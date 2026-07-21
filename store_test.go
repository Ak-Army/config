package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/backend/file"
)

type StoreTestSuite struct {
	suite.Suite
	files  []*os.File
	cancel context.CancelFunc
	ctx    context.Context
}

func TestStore(t *testing.T) {
	suite.Run(t, new(StoreTestSuite))
}

func (s *StoreTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *StoreTestSuite) TearDownTest() {
	s.cancel()
	for _, f := range s.files {
		s.Nil(f.Close())
		s.Nil(os.Remove(f.Name()))
	}
	s.files = []*os.File{}
}

func (s *StoreTestSuite) createFileForTest(data []byte) *os.File {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	s.Nil(err)
	_, err = fh.Write(data)
	s.Nil(err)
	s.files = append(s.files, fh)
	return fh
}

type storeConfig struct {
	Name    string        `config:"name"`
	Timeout time.Duration `config:"timeout"`
	Nested  *storeNested  `config:"nested"`
}

type storeNested struct {
	Value int `config:"value"`
}

// storeHandler implements Handler[storeConfig] for the tests: it seeds defaults
// and scales Timeout into seconds.
type storeHandler struct {
	name    string
	timeout time.Duration
}

func (h storeHandler) Default() *storeConfig {
	return &storeConfig{Name: h.name, Timeout: h.timeout, Nested: &storeNested{}}
}

func (storeHandler) Set(c *storeConfig) {
	c.Timeout *= time.Second
}

func (s *StoreTestSuite) TestStore() {
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"name":"cfg","timeout":5,"nested":{"value":7}}`)).Name(),
		)),
	)
	s.Nil(err)

	store := NewStore[storeConfig](storeHandler{})
	s.Nil(Load(loader, store))

	cfg, err := store.Config()
	s.Nil(err)
	s.Equal("cfg", cfg.Name)
	s.Equal(5*time.Second, cfg.Timeout)
	s.Equal(7, cfg.Nested.Value)
}

// TestStoreDefaults checks that the defaults are applied when a key is absent
// and that process runs even on a fresh (empty source) load.
func (s *StoreTestSuite) TestStoreDefaults() {
	loader, err := NewLoader(s.ctx)
	s.Nil(err)

	store := NewStore[storeConfig](storeHandler{name: "default", timeout: 2})
	s.Nil(Load(loader, store))

	cfg, err := store.Config()
	s.Nil(err)
	s.Equal("default", cfg.Name)
	s.Equal(2*time.Second, cfg.Timeout)
}

type storeRequiredConfig struct {
	Name string `config:"name,required"`
}

// storeRequiredHandler seeds a default Name so the tests can tell a
// defaults-only snapshot from a loaded one.
type storeRequiredHandler struct{}

func (storeRequiredHandler) Default() *storeRequiredConfig {
	return &storeRequiredConfig{Name: "default"}
}

func (storeRequiredHandler) Set(*storeRequiredConfig) {}

// TestFailedReloadKeepsConfig verifies the documented invariant that a bad
// reload never replaces a good snapshot: after a successful load, a reload
// missing a required key surfaces the error but keeps the last good config.
func (s *StoreTestSuite) TestFailedReloadKeepsConfig() {
	fh := s.createFileForTest([]byte(`{"name":"good"}`))
	src := file.New(file.WithPath(fh.Name()))
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	s.Nil(loader.AddSource(src))

	store := NewStore[storeRequiredConfig](storeRequiredHandler{})
	s.Nil(Load(loader, store))
	cfg, err := store.Config()
	s.Nil(err)
	s.Equal("good", cfg.Name)

	// Simulate a watcher-triggered reload after the required key disappeared.
	s.Nil(os.WriteFile(fh.Name(), []byte(`{}`), 0o644))
	content, err := src.Read()
	s.Nil(err)
	loader.maps[src] = content
	loader.load(store)

	cfg, err = store.Config()
	s.NotNil(err)
	s.Equal("good", cfg.Name, "failed reload must keep the last good config")
}

// TestFirstLoadErrorStoresDefaults verifies that a failed first load still
// exposes the handler defaults (plus whatever resolved) alongside the error.
func (s *StoreTestSuite) TestFirstLoadErrorStoresDefaults() {
	fh := s.createFileForTest([]byte(`{}`))
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	s.Nil(loader.AddSource(file.New(file.WithPath(fh.Name()))))

	store := NewStore[storeRequiredConfig](storeRequiredHandler{})
	s.Nil(Load(loader, store))

	cfg, err := store.Config()
	s.NotNil(err)
	s.Equal("default", cfg.Name)
}

// countingHandler counts Set invocations so the test can tell how many times a
// reload rebuilt the snapshot.
type countingHandler struct {
	sets *int
}

func (countingHandler) Default() *storeConfig { return &storeConfig{Nested: &storeNested{}} }

func (h countingHandler) Set(*storeConfig) { *h.sets++ }

// TestLoadTwiceRegistersOnce: a repeated Load with the same store must not
// register it twice, or every watcher tick would rebuild the snapshot and run
// Handler.Set once per duplicate.
func (s *StoreTestSuite) TestLoadTwiceRegistersOnce() {
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"name":"cfg"}`)).Name(),
		)),
	)
	s.Nil(err)

	sets := 0
	store := NewStore[storeConfig](countingHandler{sets: &sets})
	s.Nil(Load(loader, store))
	s.Nil(Load(loader, store))
	s.Len(loader.backendWatcher, 1, "the same store must be registered once")

	// Simulate one watcher tick: every registered watcher reloads once.
	sets = 0
	for _, w := range loader.backendWatcher {
		loader.load(w)
	}
	s.Equal(1, sets, "one watcher tick must run Handler.Set exactly once")
}

// TestParseCacheReused verifies that reloading the same snapshot type parses
// the struct only once: the cache keeps a single entry and later loads reuse
// it, while each reload still resolves into a fresh, correct snapshot.
func (s *StoreTestSuite) TestParseCacheReused() {
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"name":"first","timeout":1,"nested":{"value":1}}`)).Name(),
		)),
	)
	s.Nil(err)

	store := NewStore[storeConfig](storeHandler{})

	// Every Load returns a fresh *storeConfig from NewSnapshot (the watch
	// scenario), yet parseType must run only once for the type.
	s.Nil(Load(loader, store))
	s.Nil(Load(loader, store))
	loader.load(store)

	s.Len(loader.structCache, 1, "struct type should be parsed and cached exactly once")

	cfg, err := store.Config()
	s.Nil(err)
	s.Equal("first", cfg.Name)
	s.Equal(1, cfg.Nested.Value)
}
