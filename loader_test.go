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

// TestNestedWithUnexportedField: a nested struct whose first field is
// unexported used to panic (reflect.Set on an unexported field), because
// subfields were written back by their slice position instead of spec.index.
func (s *ConfigTestSuite) TestNestedWithUnexportedField() {
	type inner struct {
		hidden string
		A      string `config:"a"`
		B      int    `config:"b"`
	}
	type test struct {
		Nested inner  `config:"nested"`
		Ptr    *inner `config:"ptr"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"a":"x","b":1},"ptr":{"a":"y","b":2}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("x", cfg.Nested.A)
	s.Equal(1, cfg.Nested.B)
	s.Require().NotNil(cfg.Ptr)
	s.Equal("y", cfg.Ptr.A)
	s.Equal(2, cfg.Ptr.B)
	_ = inner{}.hidden
}

// TestNestedWithIgnoredField: a config:"-" scalar between two tagged fields
// used to shift the write-back target, silently landing values in the wrong
// (same-typed) field.
func (s *ConfigTestSuite) TestNestedWithIgnoredField() {
	type inner struct {
		Skip string `config:"-"`
		A    string `config:"a"`
	}
	type test struct {
		Nested inner `config:"nested"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"a":"value"}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("value", cfg.Nested.A)
	s.Equal("", cfg.Nested.Skip)
}

// TestArrayWithUnexportedField: the same positional-index bug in the []struct
// list path.
func (s *ConfigTestSuite) TestArrayWithUnexportedField() {
	type item struct {
		hidden string
		Name   string `config:"name"`
		Age    int    `config:"age"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"items":[{"name":"a","age":1},{"name":"b","age":2}]}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Require().Len(cfg.Items, 2)
	s.Equal("a", cfg.Items[0].Name)
	s.Equal(1, cfg.Items[0].Age)
	s.Equal("b", cfg.Items[1].Name)
	s.Equal(2, cfg.Items[1].Age)
	_ = item{}.hidden
}

// TestFlattenStruct: values loaded into a config:"-" struct-by-value field
// used to be silently discarded (bound to a scratch copy never written back).
func (s *ConfigTestSuite) TestFlattenStruct() {
	type inner struct {
		Host string `config:"host"`
		Port int    `config:"port"`
	}
	type test struct {
		DB   inner  `config:"-"`
		Name string `config:"name"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"host":"localhost","port":5432,"name":"svc"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("localhost", cfg.DB.Host)
	s.Equal(5432, cfg.DB.Port)
	s.Equal("svc", cfg.Name)
}

// TestFlattenPtr: same for a pre-allocated config:"-" *struct field.
func (s *ConfigTestSuite) TestFlattenPtr() {
	type inner struct {
		Host string `config:"host"`
	}
	type test struct {
		DB *inner `config:"-"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"host":"localhost"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{DB: &inner{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Require().NotNil(cfg.DB)
	s.Equal("localhost", cfg.DB.Host)
}

// TestFlattenPtrNil: a nil config:"-" *struct field is allocated eagerly at
// bind time, so the promoted children have a real target to load into.
func (s *ConfigTestSuite) TestFlattenPtrNil() {
	type inner struct {
		Host string `config:"host"`
	}
	type test struct {
		DB *inner `config:"-"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"host":"localhost"}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Require().NotNil(cfg.DB)
	s.Equal("localhost", cfg.DB.Host)
}

// TestFlattenInsideNested: a config:"-" struct field inside a keyed nested
// struct splices extra entries into subFields; with positional write-back this
// could index past the struct's fields and panic.
func (s *ConfigTestSuite) TestFlattenInsideNested() {
	type creds struct {
		User string `config:"user"`
		Pass string `config:"pass"`
	}
	type inner struct {
		Creds creds  `config:"-"`
		Host  string `config:"host"`
	}
	type test struct {
		DB inner `config:"db"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"db":{"user":"u","pass":"p","host":"h"}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("u", cfg.DB.Creds.User)
	s.Equal("p", cfg.DB.Creds.Pass)
	s.Equal("h", cfg.DB.Host)
}

// TestDeepNestedRequired: a required field two levels below the top-level field
// must be validated, not silently ignored.
func (s *ConfigTestSuite) TestDeepNestedRequired() {
	type deep struct {
		Need string `config:"need,required"`
	}
	type mid struct {
		Deep deep `config:"deep"`
	}
	type test struct {
		Mid mid `config:"mid"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"mid":{"deep":{}}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	_, cerr := store.Config()
	s.EqualError(cerr, "required key 'need' for field 'Need' not found")
}

// TestListElementNestedRequired: a required field inside a struct nested within
// a list element must be validated per element.
func (s *ConfigTestSuite) TestListElementNestedRequired() {
	type inner struct {
		Need string `config:"need,required"`
	}
	type item struct {
		Inner inner `config:"inner"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"items":[{"inner":{}}]}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	_, cerr := store.Config()
	// A required key missing inside a list element surfaces during decode, so
	// it arrives via the aggregated "data loading errors" path.
	s.ErrorContains(cerr, "required key 'need' for field 'Need' not found")
}

// TestEmptyListRequiredSubfield: an empty list whose element type has a required
// field must not error — there is no element to require anything of.
func (s *ConfigTestSuite) TestEmptyListRequiredSubfield() {
	type item struct {
		Need string `config:"need,required"`
	}
	type test struct {
		Items []item `config:"items"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"items":[]}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Empty(cfg.Items)
}

// TestNestedRequiredPreservesPartial: a missing required subfield surfaces an
// error but must not discard the sibling values that did load (validation runs
// after write-back). Covers the by-value nested struct case.
func (s *ConfigTestSuite) TestNestedRequiredPreservesPartial() {
	type nested struct {
		Asd       string `config:"asd"`
		NestedKey string `config:"key,required"`
	}
	type test struct {
		Nested nested `config:"nested"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"asd":"partial"}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.EqualError(cerr, "required key 'key' for field 'NestedKey' not found")
	s.Equal("partial", cfg.Nested.Asd)
}

// TestAddSourceReloadsStores: AddSource must re-resolve already-registered
// stores so a source added after Load takes effect immediately.
func (s *ConfigTestSuite) TestAddSourceReloadsStores() {
	type test struct {
		Key string `config:"key"`
	}
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Empty(cfg.Key)

	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"value"}`)).Name(),
		)),
	)
	s.Nil(err)
	cfg, cerr = store.Config()
	s.Nil(cerr)
	s.Equal("value", cfg.Key)
}

// TestNestedBackendTag: a `backend=` pin on a nested subfield is honoured, so
// the value comes from the pinned backend even when an earlier-registered
// backend also provides it.
func (s *ConfigTestSuite) TestNestedBackendTag() {
	type nested struct {
		Key string `config:"key,backend=b"`
	}
	type test struct {
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"key":"fromA"}}`)).Name(),
		), file.WithOption(backend.WithName("a"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"key":"fromB"}}`)).Name(),
		), file.WithOption(backend.WithName("b"))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("fromB", cfg.Nested.Key)
}

// TestNestedStructMerge: without pins, a nested struct's subfields are filled
// per field from whichever backend provides them, so a struct can be assembled
// from several backends at once.
func (s *ConfigTestSuite) TestNestedStructMerge() {
	type nested struct {
		X string `config:"x"`
		Y string `config:"y"`
	}
	type test struct {
		Nested *nested `config:"nested"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"x":"fromA"}}`)).Name(),
		), file.WithOption(backend.WithName("a"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"y":"fromB"}}`)).Name(),
		), file.WithOption(backend.WithName("b"))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("fromA", cfg.Nested.X)
	s.Equal("fromB", cfg.Nested.Y)
}

// TestBackendTagMultiple: a field may pin several backends; data is read from
// the first of them (in registration order) that provides the key, and other
// backends are excluded.
func (s *ConfigTestSuite) TestBackendTagMultiple() {
	type test struct {
		Key string `config:"key,backend=b,backend=c"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"fromA"}`)).Name(),
		), file.WithOption(backend.WithName("a"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"fromB"}`)).Name(),
		), file.WithOption(backend.WithName("b"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"key":"fromC"}`)).Name(),
		), file.WithOption(backend.WithName("c"))),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("fromB", cfg.Key)
}

// TestNestedPinnedLocksSubfields: a `backend=` pin on a nested struct locks all
// of its subfields to the same backend; a subfield's own pin cannot change it.
func (s *ConfigTestSuite) TestNestedPinnedLocksSubfields() {
	type nested struct {
		Key string `config:"key,backend=b"`
	}
	type test struct {
		Nested *nested `config:"nested,backend=a"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"key":"fromA"}}`)).Name(),
		), file.WithOption(backend.WithName("a"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"nested":{"key":"fromB"}}`)).Name(),
		), file.WithOption(backend.WithName("b"))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Nested: &nested{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Equal("fromA", cfg.Nested.Key)
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
