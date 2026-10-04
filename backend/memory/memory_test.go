package memory_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config"
	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/backend/memory"
	"github.com/Ak-Army/config/encoder/yaml"
)

type MemoryTestSuite struct {
	suite.Suite
}

func TestMemory(t *testing.T) {
	suite.Run(t, new(MemoryTestSuite))
}

type settings struct {
	Name  string         `config:"name"`
	Inner inner          `config:"inner"`
	List  []string       `config:"list"`
	Map   map[string]int `config:"map"`
	Kept  int            `config:"kept"`
}

type inner struct {
	Count int `config:"count"`
}

type defaults struct{}

func (defaults) Default() *settings { return &settings{Kept: 7} }
func (defaults) Set(*settings)      {}

func (s *MemoryTestSuite) load(b backend.Backend) settings {
	l, err := config.NewLoader(context.Background(), b)
	s.Require().NoError(err)
	st := config.NewStore[settings](defaults{})
	s.Require().NoError(config.Load(l, st))
	c, err := st.Config()
	s.Require().NoError(err)
	return c
}

func (s *MemoryTestSuite) TestReadNotSet() {
	_, err := memory.New().Read()
	s.Error(err)
}

func (s *MemoryTestSuite) TestValue() {
	c := s.load(memory.New(memory.WithValue(map[string]interface{}{
		"name":  "x",
		"inner": map[string]interface{}{"count": 3},
		"list":  []string{"a", "b"},
		"map":   map[string]int{"k": 1},
	})))
	want := settings{Name: "x", Inner: inner{Count: 3}, List: []string{"a", "b"}, Map: map[string]int{"k": 1}, Kept: 7}
	s.True(reflect.DeepEqual(c, want), "got %+v", c)
}

func (s *MemoryTestSuite) TestDocumentWithEncoder() {
	c := s.load(memory.New(
		memory.WithOption(backend.WithEncoder(yaml.New())),
		memory.WithDocument([]byte("name: y\ninner:\n  count: 2\n")),
	))
	s.Equal("y", c.Name)
	s.Equal(2, c.Inner.Count)
	s.Equal(7, c.Kept)
}

func (s *MemoryTestSuite) TestName() {
	s.Equal("memory", memory.New().String())
	s.Equal("fixed", memory.New(memory.WithOption(backend.WithName("fixed"))).String())
	w, err := memory.New().Watcher()
	s.NoError(err)
	s.Nil(w)
}
