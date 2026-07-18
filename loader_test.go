package config

import (
	"context"
	"encoding/base64"
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
	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
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

func (s *ConfigTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *ConfigTestSuite) TearDownTest() {
	s.cancel()
	for _, f := range s.files {
		s.Nil(f.Close())
		s.Nil(os.Remove(f.Name()))
	}
	s.files = []*os.File{}
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

func (s *ConfigTestSuite) TestLoad() {
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

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"bool":true}`)).Name(),
		)),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"int":%d,`, math.MaxInt64)+
				fmt.Sprintf(`"int8":%d,`, math.MaxInt8)+
				fmt.Sprintf(`"int16":%d,`, math.MaxInt16)+
				fmt.Sprintf(`"int32":%d,`, math.MaxInt32)+
				fmt.Sprintf(`"int64":%d`, math.MaxInt64)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"uint":%d,`, math.MaxUint32)+
				fmt.Sprintf(`"uint8":%d,`, math.MaxUint8)+
				fmt.Sprintf(`"uint16":%d,`, math.MaxUint16)+
				fmt.Sprintf(`"uint32":%d,`, math.MaxUint32)+
				fmt.Sprintf(`"uint64":%d`, math.MaxUint32)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{`+
				fmt.Sprintf(`"float32":%f,`, math.MaxFloat32)+
				fmt.Sprintf(`"float64":%f`, math.MaxFloat64)+
				`}`)).Name(),
		)),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{`+
				`"ptr": "ptr",`+
				`"string": "string"`+
				`}`)).Name(),
		)),
	)
	s.Nil(err)
	ptr := "ptr"
	store := NewStore[testStruct](snapshotHandler[testStruct]{def: func() *testStruct {
		return &testStruct{StructPtrNotNil: new(nested)}
	}})
	err = Load(loader, store)
	s.Nil(err, "Load got err")
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(testStruct{
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

func (s *ConfigTestSuite) TestLoadRequired() {
	type test struct {
		Name string `config:"name,required"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.NotNil(cerr)
}

func (s *ConfigTestSuite) TestLoadOmitempty() {
	type Test struct {
		Hunyi string `config:"name,omitempty"`
		Alma  string `config:"age,omitempty"`
	}
	type st struct {
		Name []Test `config:"name,omitempty"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"name":[{"name":"asd","age":10}]}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[st](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.Nil(cerr)
}

func (s *ConfigTestSuite) TestLoadIgnored() {
	type test struct {
		Name string `config:"-"`
		Age  int    `config:"age"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"name":"name","age":10}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(10, cfg.Age)
	s.Empty(cfg.Name)
}

func (s *ConfigTestSuite) TestBackendTagOK() {
	type test struct {
		Hunyi string `config:"hunyi,backend=store"`
		Alma  string `config:"alma,required,backend=backendCalled"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"hunyi":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"alma":"aaaaa"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendNotCalled")),
		),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"alma":"nan"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendCalled")),
		),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)

	s.Equal("nan", cfg.Alma)
	s.Equal("megvan", cfg.Hunyi)
}

func (s *ConfigTestSuite) TestBackendTagNOK() {
	type test struct {
		Hunyi string `config:"hunyi,backend=store"`
		Alma  string `config:"alma,required,backend=backendCalled"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"hunyi":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"alma":"aaaaa"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendNotCalled")),
		),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"affs":"nan"}`)).Name(),
		),
			file.WithOption(backend.WithName("backendCalled")),
		),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.EqualError(cerr, "required key 'alma' for field 'Alma' not found")
}

func (s *ConfigTestSuite) TestTagsBadRequired() {
	type test struct {
		Key string `config:"key,rrequiredd,backend=store"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"kkkk":"megvan"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)

	s.Equal("", cfg.Key)
}

func (s *ConfigTestSuite) TestTagsBadBackendValue() {
	type test struct {
		Key string `config:"key,backend=stor"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"value"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.NotNil(cerr)
}

func (s *ConfigTestSuite) TestNested() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"int":10,"string":"string","key":"key","nested":{"key":"nested key"}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (s *ConfigTestSuite) TestNestedRequired() {
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

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"int":10,"string":"string","key":"key","nested":{"asd":"nested key"}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Error(cerr, "required key 'key' for field 'NestedKey' not found")
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Asd:       "nested key",
			NestedKey: "",
		},
	}, cfg)
}

func (s *ConfigTestSuite) TestNestedYaml() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`int: 10
string: string
key: key
nested:
  key: nested key`)).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (s *ConfigTestSuite) TestNestedToml() {
	type nested struct {
		Key string `config:"key"`
	}

	type test struct {
		Int    int     `config:"int"`
		String string  `config:"string"`
		Key    string  `config:"key"`
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`
int = 10
string = "string"
key = "key"
[nested]
  key = "nested key"
`)).Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
		Nested: &nested{
			Key: "nested key",
		},
	}, cfg)
}

func (s *ConfigTestSuite) TestLoadEnv() {
	type test struct {
		Int    int    `config:"int"`
		String string `config:"string"`
		Key    string `config:"key"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		env.New(env.WithDefaults(
			s.createFileForTest([]byte(`
STRING="string"
INT=10
KEY="key"
`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
	}, cfg)
}

func (s *ConfigTestSuite) TestLoadEnvWithStripPrefixes() {
	type test struct {
		Int    int    `config:"int"`
		String string `config:"string"`
		Key    string `config:"key"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		env.New(env.WithDefaults(
			s.createFileForTest([]byte(`
AA_STRING="string"
AA_INT=10
AA_KEY="key"
BB_Key="aaaa"
`)).Name(),
		), env.WithStripPrefix("AA_")),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal(test{
		Int:    10,
		String: "string",
		Key:    "key",
	}, cfg)
}

func (s *ConfigTestSuite) TestTagsBadTagsOrder() {
	type test struct {
		Key string `config:"backend=store,key"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"value"}`)).Name(),
		),
			file.WithOption(backend.WithName("store")),
		),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)

	s.Equal("", cfg.Key)
}

func (s *ConfigTestSuite) TestWatch() {
	type test struct {
		Name string `config:"name,required"`
		Age  int    `config:"age,required"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	f := s.createFileForTest([]byte(`{"name":"name","age":10}`))
	err = loader.AddSource(
		file.New(file.WithPath(
			f.Name(),
		), file.WithWatchInterval(1*time.Second),
			file.WithOption(backend.WithWatcher())),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.Nil(cerr)
	f.Seek(0, 0)
	f.WriteString(`{"name":"name2","age":10}`)
	f.Sync()
	time.Sleep(4 * time.Second)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("name2", cfg.Name)
}

func (s *ConfigTestSuite) TestArray() {
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

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	f := s.createFileForTest([]byte(`{"module":"name","int":[{"ints":10,"intpointer": 10,"nes":{"strings":"asd"}}, {"ints":11,"nes":{"strings":"qwe"}}]}`))
	err = loader.AddSource(
		file.New(file.WithPath(
			f.Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("asd", cfg.Int[0].Nested.StringName)
	s.Equal("qwe", cfg.Int[1].Nested.StringName)
}

func (s *ConfigTestSuite) TestArrayYaml() {
	type item struct {
		Name string `config:"name"`
		Age  int    `config:"age"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`items:
  - name: a
    age: 1
  - name: b
    age: 2
`)).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Len(cfg.Items, 2)
	s.Equal("a", cfg.Items[0].Name)
	s.Equal(1, cfg.Items[0].Age)
	s.Equal("b", cfg.Items[1].Name)
	s.Equal(2, cfg.Items[1].Age)
}

func (s *ConfigTestSuite) TestArrayToml() {
	type item struct {
		Name string `config:"name"`
		Age  int    `config:"age"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`
[[items]]
name = "a"
age = 1
[[items]]
name = "b"
age = 2
`)).Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Len(cfg.Items, 2)
	s.Equal("a", cfg.Items[0].Name)
	s.Equal(1, cfg.Items[0].Age)
	s.Equal("b", cfg.Items[1].Name)
	s.Equal(2, cfg.Items[1].Age)
}

// TestPrecedence verifies that when several sources provide the same key and
// no backend is pinned, the first registered source wins deterministically
// (regardless of Go's random map iteration order).
func (s *ConfigTestSuite) TestPrecedence() {
	type test struct {
		Key string `config:"key"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"first"}`)).Name(),
		), file.WithOption(backend.WithName("first"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"second"}`)).Name(),
		), file.WithOption(backend.WithName("second"))),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("first", cfg.Key)
}

// testCrypto returns a deterministic-key crypto for the encrypted-value tests.
func (s *ConfigTestSuite) testCrypto(firstByte byte) *crypto.Crypto {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	key[0] = firstByte
	ring := s.createFileForTest([]byte("test: " + base64.StdEncoding.EncodeToString(key) + "\n"))
	c, err := crypto.New(ring.Name(), func(key []byte) (crypto.Decrypter, error) {
		return aesgcm.New(key)
	})
	s.Require().NoError(err)
	return c
}

func (s *ConfigTestSuite) encryptForTest(c *crypto.Crypto, plaintext string) string {
	encoded, err := c.EncryptValue(plaintext)
	s.Require().NoError(err)
	return encoded
}

func (s *ConfigTestSuite) TestLoadEncrypted() {
	type nested struct {
		Secret string `config:"secret,encrypted"`
	}
	type test struct {
		Password  string  `config:"password,required,encrypted"`
		Plain     string  `config:"plain,encrypted"`
		PtrSecret *string `config:"ptr_secret,encrypted"`
		Nested    *nested `config:"nested"`
	}

	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(cr)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{` +
				`"password":"` + s.encryptForTest(cr, "s3cr3t") + `",` +
				`"plain":"not encrypted",` +
				`"ptr_secret":"` + s.encryptForTest(cr, "ptr secret") + `",` +
				`"nested":{"secret":"` + s.encryptForTest(cr, "nested secret") + `"}` +
				`}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("s3cr3t", cfg.Password)
	s.Equal("not encrypted", cfg.Plain)
	s.Require().NotNil(cfg.PtrSecret)
	s.Equal("ptr secret", *cfg.PtrSecret)
	s.Equal("nested secret", cfg.Nested.Secret)
}

func (s *ConfigTestSuite) TestLoadEncryptedYaml() {
	type test struct {
		Password string `config:"password,encrypted"`
	}

	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(cr)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`password: `+s.encryptForTest(cr, "s3cr3t"))).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("s3cr3t", cfg.Password)
}

func (s *ConfigTestSuite) TestLoadEncryptedNoDecrypter() {
	type test struct {
		Password string `config:"password,encrypted"`
	}

	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"password":"` + s.encryptForTest(cr, "s3cr3t") + `"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.Require().NotNil(cerr)
	s.Contains(cerr.Error(), "no decrypter configured")
}

func (s *ConfigTestSuite) TestLoadEncryptedWrongKey() {
	type test struct {
		Password string `config:"password,encrypted"`
	}

	cr := s.testCrypto(0)
	other := s.testCrypto(1)

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(other)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"password":"` + s.encryptForTest(cr, "s3cr3t") + `"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.Require().NotNil(cerr)
	s.Contains(cerr.Error(), "decrypt")
}

func (s *ConfigTestSuite) TestLoadEncryptedNonStringField() {
	type test struct {
		Age int `config:"age,encrypted"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(s.testCrypto(0))
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"age":"10"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	err = Load(loader, store)
	s.Nil(err)
	_, cerr := store.Config()
	s.Require().NotNil(cerr)
	s.Contains(cerr.Error(), "requires a string or *string field")
}

func (s *ConfigTestSuite) createFileForTest(data []byte) *os.File {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("file.%d", time.Now().UnixNano()))
	fh, err := os.Create(path)
	s.Nil(err)
	_, err = fh.Write(data)
	s.Nil(err)
	s.files = append(s.files, fh)
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
