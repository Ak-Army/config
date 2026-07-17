package config

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/backend/env"
	"github.com/Ak-Army/config/backend/file"
	"github.com/Ak-Army/config/encoder/toml"
	"github.com/Ak-Army/config/encoder/yaml"
)

type ConfigTestSuite struct {
	suite.Suite
	files  []*os.File
	cancel context.CancelFunc
	ctx    context.Context
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (suite *ConfigTestSuite) SetupTest() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())
}

func (suite *ConfigTestSuite) TearDownTest() {
	suite.cancel()
	for _, f := range suite.files {
		suite.Nil(f.Close())
		suite.Nil(os.Remove(f.Name()))
	}
	suite.files = []*os.File{}
}

// snapshotHandler seeds a Store with a caller-supplied default for the tests.
// A nil def yields a zero-valued *T for every load.
type snapshotHandler[T any] struct {
	def func() *T
}

func (h snapshotHandler[T]) Default() *T {
	if h.def != nil {
		return h.def()
	}
	return new(T)
}

func (snapshotHandler[T]) Set(*T) {}

func (suite *ConfigTestSuite) TestLoad() {
	type nested struct {
		Int    int    `config:"int"`
		String string `config:"string"`
	}

	type testStruct struct {
		Bool            bool    `config:"bool"`
		Int             int     `config:"int"`
		Int8            int8    `config:"int8"`
		Int16           int16   `config:"int16"`
		Int32           int32   `config:"int32"`
		Int64           int64   `config:"int64"`
		Uint            uint    `config:"uint"`
		Uint8           uint8   `config:"uint8"`
		Uint16          uint16  `config:"uint16"`
		Uint32          uint32  `config:"uint32"`
		Uint64          uint64  `config:"uint64"`
		Float32         float32 `config:"float32"`
		Float64         float64 `config:"float64"`
		Ptr             *string `config:"ptr"`
		String          string  `config:"string"`
		Struct          nested
		StructPtrNil    *nested
		StructPtrNotNil *nested
		Ignored         string
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"bool":true}`)).Name(),
		)),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"int":%d,`, math.MaxInt64)+
				fmt.Sprintf(`"int8":%d,`, math.MaxInt8)+
				fmt.Sprintf(`"int16":%d,`, math.MaxInt16)+
				fmt.Sprintf(`"int32":%d,`, math.MaxInt32)+
				fmt.Sprintf(`"int64":%d`, math.MaxInt64)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"uint":%d,`, math.MaxUint32)+
				fmt.Sprintf(`"uint8":%d,`, math.MaxUint8)+
				fmt.Sprintf(`"uint16":%d,`, math.MaxUint16)+
				fmt.Sprintf(`"uint32":%d,`, math.MaxUint32)+
				fmt.Sprintf(`"uint64":%d`, math.MaxUint32)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"float32":%f,`, math.MaxFloat32)+
				fmt.Sprintf(`"float64":%f`, math.MaxFloat64)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{`+
				`"ptr": "ptr",`+
				`"string": "string"`+
				`}`)).Name(),
		)),
	)
	suite.Nil(err)
	ptr := "ptr"
	store := NewStore[testStruct](snapshotHandler[testStruct]{def: func() *testStruct {
		return &testStruct{StructPtrNotNil: new(nested)}
	}})
	err = Load(loader, store)
	suite.Nil(err, "Load got err")
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(testStruct{
		Bool:    true,
		Int:     math.MaxInt64,
		Int8:    math.MaxInt8,
		Int16:   math.MaxInt16,
		Int32:   math.MaxInt32,
		Int64:   math.MaxInt64,
		Uint:    math.MaxUint32,
		Uint8:   math.MaxUint8,
		Uint16:  math.MaxUint16,
		Uint32:  math.MaxUint32,
		Uint64:  math.MaxUint32,
		Float32: math.MaxFloat32,
		Float64: math.MaxFloat64,
		Ptr:     &ptr,
		String:  "string",
		Struct: nested{
			Int:    0,
			String: "",
		},
		StructPtrNotNil: &nested{
			Int:    0,
			String: "",
		},
	}, cfg)
}

func (suite *ConfigTestSuite) TestLoadRequired() {
	type test struct {
		Name string `config:"name,required"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	_, cerr := store.Config()
	suite.NotNil(cerr)
}

func (suite *ConfigTestSuite) TestLoadOmitempty() {
	type Test struct {
		Hunyi string `config:"name,omitempty"`
		Alma  string `config:"age,omitempty"`
	}
	type st struct {
		Name []Test `config:"name,omitempty"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"name":[{"name":"asd","age":10}]}`)).Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[st](nil)
	err = Load(loader, store)
	suite.Nil(err)
	_, cerr := store.Config()
	suite.Nil(cerr)
}

func (suite *ConfigTestSuite) TestLoadIgnored() {
	type test struct {
		Name string `config:"-"`
		Age  int    `config:"age"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"name":"name","age":10}`)).Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(10, cfg.Age)
	suite.Empty(cfg.Name)
}

func (suite *ConfigTestSuite) TestBackendTagOK() {
	type test struct {
		Hunyi string `config:"hunyi,backend=store"`
		Alma  string `config:"alma,required,backend=backendCalled"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"hunyi":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"alma":"aaaaa"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendNotCalled")),
		),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"alma":"nan"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendCalled")),
		),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)

	suite.Equal("nan", cfg.Alma)
	suite.Equal("megvan", cfg.Hunyi)
}

func (suite *ConfigTestSuite) TestBackendTagNOK() {
	type test struct {
		Hunyi string `config:"hunyi,backend=store"`
		Alma  string `config:"alma,required,backend=backendCalled"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"hunyi":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"alma":"aaaaa"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendNotCalled")),
		),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"affs":"nan"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendCalled")),
		),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	_, cerr := store.Config()
	suite.EqualError(cerr, "required key 'alma' for field 'Alma' not found")
}

func (suite *ConfigTestSuite) TestTagsBadRequired() {
	type test struct {
		Key string `config:"key,rrequiredd,backend=store"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"kkkk":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)

	suite.Equal("", cfg.Key)
}

func (suite *ConfigTestSuite) TestTagsBadBackendValue() {
	type test struct {
		Key string `config:"key,backend=stor"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"key":"value"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	_, cerr := store.Config()
	suite.NotNil(cerr)
}

func (suite *ConfigTestSuite) TestNested() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"int":10,"string":"string","key":"key","nested":{"key":"nested key"}}`)).Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (suite *ConfigTestSuite) TestNestedRequired() {
	type nested struct {
		Asd       string `config:"asd"`
		NestedKey string `config:"key,required"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"int":10,"string":"string","key":"key","nested":{"asd":"nested key"}}`)).Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Error(cerr, "required key 'key' for field 'NestedKey' not found")
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Asd:       "nested key",
			NestedKey: "",
		},
	}, cfg)
}

func (suite *ConfigTestSuite) TestNestedYaml() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`int: 10
string: string
key: key
nested:
  key: nested key`)).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	suite.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (suite *ConfigTestSuite) TestNestedToml() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`
int = 10
string = "string"
key = "key"
[nested]
  key = "nested key"
`)).Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	suite.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (suite *ConfigTestSuite) TestLoadEnv() {
	type test struct {
		Int    int    `config:"int"`
		String string `config:"string"`
		Key    string `config:"key"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		env.New(env.WithDefaults(
			suite.createFileForTest([]byte(`
STRING="string"
INT=10
KEY="key"
`)).Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
	}, cfg)
}

func (suite *ConfigTestSuite) TestLoadEnvWithStripPrefixes() {
	type test struct {
		Int    int    `config:"int"`
		String string `config:"string"`
		Key    string `config:"key"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		env.New(env.WithDefaults(
			suite.createFileForTest([]byte(`
AA_STRING="string"
AA_INT=10
AA_KEY="key"
BB_Key="aaaa"
`)).Name(),
		), env.WithStripPrefix("AA_")),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
	}, cfg)
}

func (suite *ConfigTestSuite) TestTagsBadTagsOrder() {
	type test struct {
		Key string `config:"backend=store,key"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"key":"value"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)

	suite.Equal("", cfg.Key)
}

func (suite *ConfigTestSuite) TestWatch() {
	type test struct {
		Name string `config:"name,required"`
		Age  int    `config:"age,required"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	f := suite.createFileForTest([]byte(`{"name":"name","age":10}`))
	err = loader.AddSource(
		file.New(file.WithPath(
			f.Name(),
		), file.WithWatchInterval(1*time.Second),
			file.WithOption(backend.WithWatcher())),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	_, cerr := store.Config()
	suite.Nil(cerr)
	f.Seek(0, 0)
	f.WriteString(`{"name":"name2","age":10}`)
	f.Sync()
	time.Sleep(4 * time.Second)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal("name2", cfg.Name)
}

func (suite *ConfigTestSuite) TestArray() {
	type nested2 struct {
		StringName string `config:"strings"`
	}
	type nested struct {
		NestedInt  int      `config:"ints"`
		IntPointer *int     `config:"intpointer"`
		Nested     *nested2 `config:"nes"`
	}
	type test struct {
		ModuleName *string  `config:"module,required"`
		Int        []nested `config:"int,required"`
	}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	f := suite.createFileForTest([]byte(`{"module":"name","int":[{"ints":10,"intpointer": 10,"nes":{"strings":"asd"}}, {"ints":11,"nes":{"strings":"qwe"}}]}`))
	err = loader.AddSource(
		file.New(file.WithPath(
			f.Name(),
		)),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal("asd", cfg.Int[0].Nested.StringName)
	suite.Equal("qwe", cfg.Int[1].Nested.StringName)
}

func (suite *ConfigTestSuite) TestArrayYaml() {
	type item struct {
		Name string `config:"name"`
		Age  int    `config:"age"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`items:
  - name: a
    age: 1
  - name: b
    age: 2
`)).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Len(cfg.Items, 2)
	suite.Equal("a", cfg.Items[0].Name)
	suite.Equal(1, cfg.Items[0].Age)
	suite.Equal("b", cfg.Items[1].Name)
	suite.Equal(2, cfg.Items[1].Age)
}

func (suite *ConfigTestSuite) TestArrayToml() {
	type item struct {
		Name string `config:"name"`
		Age  int    `config:"age"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`
[[items]]
name = "a"
age = 1
[[items]]
name = "b"
age = 2
`)).Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Len(cfg.Items, 2)
	suite.Equal("a", cfg.Items[0].Name)
	suite.Equal(1, cfg.Items[0].Age)
	suite.Equal("b", cfg.Items[1].Name)
	suite.Equal(2, cfg.Items[1].Age)
}

// TestPrecedence verifies that when several sources provide the same key and
// no backend is pinned, the first registered source wins deterministically
// (regardless of Go's random map iteration order).
func (suite *ConfigTestSuite) TestPrecedence() {
	type test struct {
		Key string `config:"key"`
	}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"key":"first"}`)).Name(),
		), file.WithOption(backend.WithName("first"))),
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"key":"second"}`)).Name(),
		), file.WithOption(backend.WithName("second"))),
	)
	suite.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	suite.Nil(err)
	cfg, cerr := store.Config()
	suite.Nil(cerr)
	suite.Equal("first", cfg.Key)
}

func (suite *ConfigTestSuite) createFileForTest(data []byte) *os.File {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	suite.Nil(err)
	_, err = fh.Write(data)
	suite.Nil(err)
	suite.files = append(suite.files, fh)
	return fh
}

/*
Run benchmarking with: go test -bench '.'
*/
func BenchmarkAddSourceJson(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`{"int":10,"string":"string","key":"key","nested":{"key":"nested key"}}`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.AddSource(
			file.New(file.WithPath(
				fh.Name(),
			)),
		)
		if err != nil {
			b.Fatal("Unable to add source")
		}
	}
}

func BenchmarkAddSourceYaml(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`int: 10
string: string
key: key
nested:
  key: nested key`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.AddSource(
			file.New(file.WithPath(
				fh.Name(),
			),
				file.WithOption(backend.WithEncoder(yaml.New())),
			))
		if err != nil {
			b.Fatal("Unable to add source")
		}
	}
}

func BenchmarkAddSourceToml(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`
int = 10
string = "string"
key = "key"
[nested]
  key = "nested key"
`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.AddSource(
			file.New(file.WithPath(
				fh.Name(),
			),
				file.WithOption(backend.WithEncoder(toml.New())),
			))
		if err != nil {
			b.Fatal("Unable to add source")
		}
	}
}

type benchNested struct {
	Key string `config:"key"`
}

type benchTest struct {
	Int    int          `config:"int"`
	String string       `config:"string"`
	Key    string       `config:"key"`
	Nested *benchNested `config:"nested"`
}

func benchStore() *Store[benchTest] {
	return NewStore[benchTest](snapshotHandler[benchTest]{def: func() *benchTest {
		return &benchTest{Nested: &benchNested{}}
	}})
}

func BenchmarkLoadJson(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`{"int":10,"string":"string","key":"key","nested":{"key":"nested key"}}`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	err = loader.AddSource(
		file.New(file.WithPath(
			fh.Name(),
		)),
	)
	if err != nil {
		b.Fatal("Unable to add source")
	}
	store := benchStore()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = Load(loader, store)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if _, cerr := store.Config(); cerr != nil {
			b.Fatal("Loading error")
		}
	}
}

func BenchmarkLoadYaml(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`
int: 10
string: string
key: key
nested:
  key: nested key
`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	err = loader.AddSource(
		file.New(file.WithPath(
			fh.Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	if err != nil {
		b.Fatal("Unable to add source")
	}
	store := benchStore()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = Load(loader, store)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if _, cerr := store.Config(); cerr != nil {
			b.Fatal("Loading error")
		}
	}
}

func BenchmarkLoadToml(b *testing.B) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	if err != nil {
		b.Fatalf("Unable to create file: %s", path)
	}
	_, err = fh.Write([]byte(`
int = 10
string = "string"
key = "key"
[nested]
  key = "nested key"
`))
	if err != nil {
		b.Fatalf("Unable to write file: %s", path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loader, err := NewLoader(ctx)
	if err != nil {
		b.Fatal("Unable to create loader")
	}
	err = loader.AddSource(
		file.New(file.WithPath(
			fh.Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	if err != nil {
		b.Fatal("Unable to add source")
	}
	store := benchStore()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = Load(loader, store)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if _, cerr := store.Config(); cerr != nil {
			b.Fatal("Loading error")
		}
	}
}
