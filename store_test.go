package config

import (
	"time"

	"github.com/Ak-Army/config/backend/file"
)

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

func (suite *ConfigTestSuite) TestStore() {
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"name":"cfg","timeout":5,"nested":{"value":7}}`)).Name(),
		)),
	)
	suite.Nil(err)

	store := NewStore[storeConfig](storeHandler{})
	suite.Nil(Load(loader, store))

	cfg, err := store.Config()
	suite.Nil(err)
	suite.Equal("cfg", cfg.Name)
	suite.Equal(5*time.Second, cfg.Timeout)
	suite.Equal(7, cfg.Nested.Value)
}

// TestStoreDefaults checks that the defaults are applied when a key is absent
// and that process runs even on a fresh (empty source) load.
func (suite *ConfigTestSuite) TestStoreDefaults() {
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)

	store := NewStore[storeConfig](storeHandler{name: "default", timeout: 2})
	suite.Nil(Load(loader, store))

	cfg, err := store.Config()
	suite.Nil(err)
	suite.Equal("default", cfg.Name)
	suite.Equal(2*time.Second, cfg.Timeout)
}

// TestParseCacheReused verifies that reloading the same snapshot type parses
// the struct only once: the cache keeps a single entry and later loads reuse
// it, while each reload still resolves into a fresh, correct snapshot.
func (suite *ConfigTestSuite) TestParseCacheReused() {
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"name":"first","timeout":1,"nested":{"value":1}}`)).Name(),
		)),
	)
	suite.Nil(err)

	store := NewStore[storeConfig](storeHandler{})

	// Every Load returns a fresh *storeConfig from NewSnapshot (the watch
	// scenario), yet parseType must run only once for the type.
	suite.Nil(Load(loader, store))
	suite.Nil(Load(loader, store))
	loader.load(store)

	suite.Len(loader.structCache, 1, "struct type should be parsed and cached exactly once")

	cfg, err := store.Config()
	suite.Nil(err)
	suite.Equal("first", cfg.Name)
	suite.Equal(1, cfg.Nested.Value)
}
