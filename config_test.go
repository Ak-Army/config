package config

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
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

	s := &testStruct{}
	s.StructPtrNotNil = new(nested)
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
	c := &config{
		structs: s,
	}
	err = loader.Load(c)
	suite.Nil(err, "Load got err")
	suite.Equal(&testStruct{
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
			Int:    math.MaxInt64,
			String: "string",
		},
		StructPtrNotNil: &nested{
			Int:    math.MaxInt64,
			String: "string",
		},
	}, s)
}

func (suite *ConfigTestSuite) TestLoadRequired() {
	s := &struct {
		Name string `config:"name,required"`
	}{}
	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	c := &config{
		structs: s,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.NotNil(c.err)
}

func (suite *ConfigTestSuite) TestLoadIgnored() {
	s := &struct {
		Name string `config:"-"`
		Age  int    `config:"age"`
	}{}

	loader, err := NewLoader(suite.ctx)
	suite.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			suite.createFileForTest([]byte(`{"name":"name","age":10}`)).Name(),
		)),
	)
	suite.Nil(err)
	c := &config{
		structs: s,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Equal(10, s.Age)
	suite.Empty(s.Name)
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)

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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.EqualError(c.err, "required key 'alma' for field 'Alma' not found")
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)

	suite.Equal("", cfg.Key)
}

func (suite *ConfigTestSuite) TestTagsBadBadBackendValue() {
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.NotNil(c.err)
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	suite.Equal(&test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: nst,
	}, cfg)
	suite.Equal(&nested{
		Key: "nested key",
	}, nst)
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	suite.Equal(&test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: nst,
	}, cfg)
	suite.Equal(&nested{
		Key: "nested key",
	}, nst)
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	suite.Equal(&test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: nst,
	}, cfg)
	suite.Equal(&nested{
		Key: "nested key",
	}, nst)
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	suite.Equal(&test{
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	suite.Equal(&test{
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
	cfg := &test{}
	c := &config{
		structs: cfg,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)

	suite.Equal("", cfg.Key)
}

func (suite *ConfigTestSuite) TestWatch() {
	s := &struct {
		Name string `config:"name,required"`
		Age  int    `config:"age,required"`
	}{}
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
	c := &config{
		structs: s,
	}
	err = loader.Load(c)
	suite.Nil(err)
	suite.Nil(c.err)
	f.Seek(0, 0)
	f.WriteString(`{"name":"name2","age":10}`)
	f.Sync()
	time.Sleep(4 * time.Second)
	c.Lock()
	defer c.Unlock()
	suite.Nil(c.err)
	suite.Equal("name2", s.Name)
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

type config struct {
	sync.Mutex
	structs interface{}
	err     error
}

func (c *config) NewSnapshot() interface{} {
	return c.structs
}

func (c *config) SetSnapshot(i interface{}, err error) {
	c.Lock()
	defer c.Unlock()
	c.structs = i
	c.err = err
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

func BenchmarkLoadJson(b *testing.B) {
	type nested struct {
		Key string `config:"key"`
	}
	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.Load(c)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if c.err != nil {
			b.Fatal("Loading error")
		}
	}
}

func BenchmarkLoadYaml(b *testing.B) {
	type nested struct {
		Key string `config:"key"`
	}
	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.Load(c)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if c.err != nil {
			b.Fatal("Loading error")
		}
	}
}

func BenchmarkLoadToml(b *testing.B) {
	type nested struct {
		Key string `config:"key"`
	}
	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}
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
	nst := &nested{}
	cfg := &test{
		Nested: nst,
	}
	c := &config{
		structs: cfg,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err = loader.Load(c)
		if err != nil {
			b.Fatal("Unable to load")
		}
		if c.err != nil {
			b.Fatal("Loading error")
		}
	}
}
