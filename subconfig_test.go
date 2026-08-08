package config

import (
	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/backend/file"
	"github.com/Ak-Army/config/encoder/toml"
	"github.com/Ak-Army/config/encoder/yaml"
)

// The sub-config tests share the ConfigTestSuite helpers (createFileForTest,
// testCrypto, encryptForTest) with the loader tests.

// appParams and appParamsSecond are the two unrelated shapes the same
// `app-params` sub-document is loaded into.
type appParams struct {
	Record   int    `config:"record"`
	Filepath string `config:"filepath"`
}

type appParamsSecond struct {
	Mode  string  `config:"mode"`
	Ratio float64 `config:"ratio"`
}

type amd2Config struct {
	Active    bool       `config:"active"`
	AppParams *SubConfig `config:"app-params"`
}

type subConfigTest struct {
	Amd2 *amd2Config `config:"amd2"`
}

// subConfigStore builds a store whose snapshot has the nested amd2 struct
// pre-allocated, mirroring how a Default() would supply it.
func (s *ConfigTestSuite) subConfigStore() *Store[subConfigTest] {
	s.T().Helper()
	return NewStore[subConfigTest](snapshotHandler[subConfigTest]{def: func() *subConfigTest {
		return &subConfigTest{Amd2: &amd2Config{}}
	}})
}

// loadSubConfig loads the given documents (one file backend each, in
// precedence order) and returns the resolved snapshot.
func (s *ConfigTestSuite) loadSubConfig(docs ...[]byte) subConfigTest {
	s.T().Helper()
	loader, err := NewLoader(s.ctx)
	s.Require().NoError(err)
	for _, doc := range docs {
		s.Require().NoError(loader.AddSource(
			file.New(file.WithPath(s.createFileForTest(doc).Name())),
		))
	}
	store := s.subConfigStore()
	s.Require().NoError(Load(loader, store))
	cfg, cerr := store.Config()
	s.Require().NoError(cerr)
	return cfg
}

// TestSubConfigTwoTargets: the same sub-document is loaded into two completely
// different structs, each picking up only the keys it declares.
func (s *ConfigTestSuite) TestSubConfigTwoTargets() {
	cfg := s.loadSubConfig([]byte(`{"amd2":{"active":true,"app-params":` +
		`{"record":3,"filepath":"/tmp/a.wav","mode":"fast","ratio":0.5}}}`))

	s.True(cfg.Amd2.Active)
	s.Require().NotNil(cfg.Amd2.AppParams)

	first := &appParams{}
	s.NoError(cfg.Amd2.AppParams.Load(first))
	s.Equal(&appParams{Record: 3, Filepath: "/tmp/a.wav"}, first)

	second := &appParamsSecond{}
	s.NoError(cfg.Amd2.AppParams.Load(second))
	s.Equal(&appParamsSecond{Mode: "fast", Ratio: 0.5}, second)
}

// TestSubConfigKeepsDefaults: keys the sub-document does not provide keep the
// value the target already holds.
func (s *ConfigTestSuite) TestSubConfigKeepsDefaults() {
	cfg := s.loadSubConfig([]byte(`{"amd2":{"app-params":{"record":3}}}`))

	params := &appParams{Record: 1, Filepath: "/default.wav"}
	s.NoError(cfg.Amd2.AppParams.Load(params))
	s.Equal(&appParams{Record: 3, Filepath: "/default.wav"}, params)
}

// TestSubConfigMissingKey: an absent key leaves the field nil, and loading from
// it is a no-op instead of a panic, so the target keeps its defaults.
func (s *ConfigTestSuite) TestSubConfigMissingKey() {
	cfg := s.loadSubConfig([]byte(`{"amd2":{"active":true}}`))

	s.Nil(cfg.Amd2.AppParams)
	params := &appParams{Record: 1}
	s.NoError(cfg.Amd2.AppParams.Load(params))
	s.Equal(&appParams{Record: 1}, params)
}

// TestSubConfigRequired: a missing sub-document is only an error when the field
// is tagged required.
func (s *ConfigTestSuite) TestSubConfigRequired() {
	type amd2 struct {
		AppParams *SubConfig `config:"app-params,required"`
	}
	type test struct {
		Amd2 *amd2 `config:"amd2"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"active":true}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Amd2: &amd2{}}
	}})
	s.Nil(Load(loader, store))
	_, cerr := store.Config()
	s.EqualError(cerr, "required key 'app-params' for field 'AppParams' not found")
}

// TestSubConfigTargetRequired: a required key missing from the target is
// reported by Load, while the keys that are present still land in the target.
func (s *ConfigTestSuite) TestSubConfigTargetRequired() {
	type params struct {
		Record int    `config:"record"`
		Mode   string `config:"mode,required"`
	}
	cfg := s.loadSubConfig([]byte(`{"amd2":{"app-params":{"record":3}}}`))

	target := &params{}
	s.EqualError(cfg.Amd2.AppParams.Load(target),
		"required key 'mode' for field 'Mode' not found")
	s.Equal(&params{Record: 3}, target)
}

// TestSubConfigEncrypted: ENC(...) values under a sub-document are decrypted
// with the loader's crypto, at any depth of the target.
func (s *ConfigTestSuite) TestSubConfigEncrypted() {
	type credentials struct {
		Token string `config:"token,encrypted"`
	}
	type params struct {
		Secret      string       `config:"secret,encrypted"`
		Plain       string       `config:"plain,encrypted"`
		Credentials *credentials `config:"credentials"`
	}

	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(cr)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":{` +
				`"secret":"` + s.encryptForTest(cr, "s3cr3t") + `",` +
				`"plain":"not encrypted",` +
				`"credentials":{"token":"` + s.encryptForTest(cr, "t0k3n") + `"}` +
				`}}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := s.subConfigStore()
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	target := &params{Credentials: &credentials{}}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal("s3cr3t", target.Secret)
	s.Equal("not encrypted", target.Plain)
	s.Equal("t0k3n", target.Credentials.Token)
}

// TestSubConfigEncryptedWrongKey: a value encrypted with another key is
// reported by Load and never lands in the target as an empty string.
func (s *ConfigTestSuite) TestSubConfigEncryptedWrongKey() {
	type params struct {
		Secret string `config:"secret,encrypted"`
	}

	other := s.testCrypto(1)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(s.testCrypto(0))
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":` +
				`{"secret":"` + s.encryptForTest(other, "s3cr3t") + `"}}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := s.subConfigStore()
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	target := &params{Secret: "untouched"}
	s.ErrorContains(cfg.Amd2.AppParams.Load(target), "field 'Secret'")
	s.Equal("untouched", target.Secret)
}

// TestSubConfigYaml: the sub-document is decoded with the encoder of the source
// it came from.
func (s *ConfigTestSuite) TestSubConfigYaml() {
	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(cr)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`amd2:
  active: true
  app-params:
    record: 3
    filepath: /tmp/a.wav
    secret: `+s.encryptForTest(cr, "s3cr3t"))).Name(),
		), file.WithOption(backend.WithEncoder(yaml.New()))),
	)
	s.Nil(err)
	store := s.subConfigStore()
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	type params struct {
		Record   int    `config:"record"`
		Filepath string `config:"filepath"`
		Secret   string `config:"secret,encrypted"`
	}
	target := &params{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(&params{Record: 3, Filepath: "/tmp/a.wav", Secret: "s3cr3t"}, target)
}

// TestSubConfigToml: same as TestSubConfigYaml, for the TOML encoder.
func (s *ConfigTestSuite) TestSubConfigToml() {
	cr := s.testCrypto(0)
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	loader.SetCrypto(cr)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`
[amd2]
active = true
[amd2.app-params]
record = 3
filepath = "/tmp/a.wav"
secret = "`+s.encryptForTest(cr, "s3cr3t")+`"
`)).Name(),
		), file.WithOption(backend.WithEncoder(toml.New()))),
	)
	s.Nil(err)
	store := s.subConfigStore()
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	type params struct {
		Record   int    `config:"record"`
		Filepath string `config:"filepath"`
		Secret   string `config:"secret,encrypted"`
	}
	target := &params{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(&params{Record: 3, Filepath: "/tmp/a.wav", Secret: "s3cr3t"}, target)
}

// TestSubConfigNestedTarget: nested structs and lists inside the target are
// resolved just like they are in a snapshot.
func (s *ConfigTestSuite) TestSubConfigNestedTarget() {
	type item struct {
		Name string `config:"name"`
		Prio int    `config:"prio"`
	}
	type nested struct {
		Key string `config:"key"`
	}
	type params struct {
		Nested *nested `config:"nested"`
		Items  []item  `config:"items"`
	}

	cfg := s.loadSubConfig([]byte(`{"amd2":{"app-params":{` +
		`"nested":{"key":"nested key"},` +
		`"items":[{"name":"a","prio":1},{"name":"b","prio":2}]}}}`))

	target := &params{Nested: &nested{}}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(&params{
		Nested: &nested{Key: "nested key"},
		Items:  []item{{Name: "a", Prio: 1}, {Name: "b", Prio: 2}},
	}, target)
}

// TestSubConfigMerge: every source holding the key contributes its own view, so
// the target is filled per field, the first source winning on a shared key.
func (s *ConfigTestSuite) TestSubConfigMerge() {
	cfg := s.loadSubConfig(
		[]byte(`{"amd2":{"app-params":{"record":3}}}`),
		[]byte(`{"amd2":{"app-params":{"record":9,"filepath":"/tmp/b.wav"}}}`),
	)

	target := &appParams{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(&appParams{Record: 3, Filepath: "/tmp/b.wav"}, target)
}

// TestSubConfigPinnedLocksTarget: a `backend=` pin on the SubConfig field locks
// the target's fields to the same source, a target field's own pin cannot
// change it.
func (s *ConfigTestSuite) TestSubConfigPinnedLocksTarget() {
	type amd2 struct {
		AppParams *SubConfig `config:"app-params,backend=a"`
	}
	type test struct {
		Amd2 *amd2 `config:"amd2"`
	}
	type params struct {
		Filepath string `config:"filepath,backend=b"` // own pin, must not widen
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":{"filepath":"fromA"}}}`)).Name(),
		), file.WithOption(backend.WithName("a"))),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":{"filepath":"fromB"}}}`)).Name(),
		), file.WithOption(backend.WithName("b"))),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Amd2: &amd2{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	target := &params{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal("fromA", target.Filepath)
}

// TestSubConfigValueField: a SubConfig declared by value behaves like the
// pointer form.
func (s *ConfigTestSuite) TestSubConfigValueField() {
	type amd2 struct {
		AppParams SubConfig `config:"app-params"`
	}
	type test struct {
		Amd2 *amd2 `config:"amd2"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":{"record":3}}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](snapshotHandler[test]{def: func() *test {
		return &test{Amd2: &amd2{}}
	}})
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)

	target := &appParams{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(3, target.Record)
}

// TestSubConfigInListElement: a SubConfig inside a []struct element captures
// that element's sub-document.
func (s *ConfigTestSuite) TestSubConfigInListElement() {
	type item struct {
		Name   string     `config:"name"`
		Params *SubConfig `config:"params"`
	}
	type test struct {
		Items []item `config:"items"`
	}

	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"items":[` +
				`{"name":"a","params":{"record":1}},` +
				`{"name":"b","params":{"record":2}}]}`)).Name(),
		)),
	)
	s.Nil(err)
	store := NewStore[test](nil)
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Nil(cerr)
	s.Require().Len(cfg.Items, 2)

	for i, want := range []int{1, 2} {
		target := &appParams{}
		s.NoError(cfg.Items[i].Params.Load(target))
		s.Equal(want, target.Record)
	}
}

// TestSubConfigNotADocument: a key holding a scalar instead of a document is
// reported by the load, and the sources that do hold a document still fill the
// sub-config.
func (s *ConfigTestSuite) TestSubConfigNotADocument() {
	loader, err := NewLoader(s.ctx)
	s.Nil(err)
	err = loader.AddSource(
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":5}}`)).Name(),
		)),
		file.New(file.WithPath(
			s.createFileForTest([]byte(`{"amd2":{"app-params":{"record":3}}}`)).Name(),
		)),
	)
	s.Nil(err)
	store := s.subConfigStore()
	s.Nil(Load(loader, store))
	cfg, cerr := store.Config()
	s.Error(cerr)

	target := &appParams{}
	s.NoError(cfg.Amd2.AppParams.Load(target))
	s.Equal(3, target.Record)
}

// TestSubConfigBadTarget: Load only accepts a usable pointer to struct.
func (s *ConfigTestSuite) TestSubConfigBadTarget() {
	cfg := s.loadSubConfig([]byte(`{"amd2":{"app-params":{"record":3}}}`))

	const want = "provided target must be a pointer to struct"
	s.EqualError(cfg.Amd2.AppParams.Load(appParams{}), want)
	s.EqualError(cfg.Amd2.AppParams.Load((*appParams)(nil)), want)
	s.EqualError(cfg.Amd2.AppParams.Load(new(string)), want)
	s.EqualError(cfg.Amd2.AppParams.Load(nil), want)
}
