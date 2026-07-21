package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder/toml"
	"github.com/Ak-Army/config/encoder/yaml"
)

type EnvTestSuite struct {
	suite.Suite
}

func TestEnv(t *testing.T) {
	suite.Run(t, new(EnvTestSuite))
}

func (s *EnvTestSuite) writeEnvFile(content string) string {
	s.T().Helper()
	path := filepath.Join(s.T().TempDir(), "defaults.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		s.T().Fatal(err)
	}
	return path
}

func (s *EnvTestSuite) TestReadStripPrefix() {
	path := s.writeEnvFile("AA_KEY=value\nBB_KEY=other\n")
	c, err := New(WithDefaults(path), WithStripPrefix("AA_")).Read()
	if err != nil {
		s.T().Fatal(err)
	}
	// Stripped key is lower-cased and stored both with "_" and "-".
	if _, ok := c.Data["key"]; !ok {
		s.T().Fatalf("expected stripped key %q in data, got %v", "key", c.Data)
	}
	// A value that does not match any prefix must be filtered out.
	if _, ok := c.Data["bb_key"]; ok {
		s.T().Fatalf("did not expect bb_key in data, got %v", c.Data)
	}
}

// TestReadReloadPicksUpChanges covers the watcher-triggered reload path: a
// changed defaults file must override the values set by the first Read,
// otherwise the reload is a silent no-op.
func (s *EnvTestSuite) TestReadReloadPicksUpChanges() {
	path := s.writeEnvFile("RELOAD_KEY=first\n")
	os.Unsetenv("RELOAD_KEY")
	defer os.Unsetenv("RELOAD_KEY")
	b := New(WithDefaults(path))
	c, err := b.Read()
	if err != nil {
		s.T().Fatal(err)
	}
	var v string
	s.Require().NoError(c.Encoder.Decode(c.Data["reload_key"], &v))
	s.Require().Equal("first", v)

	if err := os.WriteFile(path, []byte("RELOAD_KEY=second\n"), 0o600); err != nil {
		s.T().Fatal(err)
	}
	c, err = b.Read()
	if err != nil {
		s.T().Fatal(err)
	}
	s.Require().NoError(c.Encoder.Decode(c.Data["reload_key"], &v))
	s.Require().Equal("second", v)
}

// TestReadReloadRemovesDeletedKey: a key removed from the defaults file must
// disappear from the data on reload, instead of lingering (the merge builds a
// fresh map from os.Environ + the current file rather than mutating the env).
func (s *EnvTestSuite) TestReadReloadRemovesDeletedKey() {
	path := s.writeEnvFile("KEEP_KEY=k\nDROP_KEY=d\n")
	os.Unsetenv("KEEP_KEY")
	os.Unsetenv("DROP_KEY")
	defer os.Unsetenv("KEEP_KEY")
	defer os.Unsetenv("DROP_KEY")
	b := New(WithDefaults(path))
	c, err := b.Read()
	s.Require().NoError(err)
	_, ok := c.Data["drop_key"]
	s.Require().True(ok)

	if err := os.WriteFile(path, []byte("KEEP_KEY=k\n"), 0o600); err != nil {
		s.T().Fatal(err)
	}
	c, err = b.Read()
	s.Require().NoError(err)
	_, ok = c.Data["keep_key"]
	s.Require().True(ok)
	_, ok = c.Data["drop_key"]
	s.Require().False(ok, "removed default key must not linger after reload")
}

// TestRealEnvWinsOnFirstLoad: on the first load a real environment variable
// takes precedence over the defaults file.
func (s *EnvTestSuite) TestRealEnvWinsOnFirstLoad() {
	path := s.writeEnvFile("WIN_KEY=fromfile\n")
	os.Setenv("WIN_KEY", "fromenv")
	defer os.Unsetenv("WIN_KEY")
	c, err := New(WithDefaults(path)).Read()
	s.Require().NoError(err)
	var v string
	s.Require().NoError(c.Encoder.Decode(c.Data["win_key"], &v))
	s.Require().Equal("fromenv", v)
}

// TestDoesNotPolluteProcessEnv: reading the defaults file must not write its
// keys into the process environment.
func (s *EnvTestSuite) TestDoesNotPolluteProcessEnv() {
	path := s.writeEnvFile("NOPOLLUTE_KEY=value\n")
	os.Unsetenv("NOPOLLUTE_KEY")
	defer os.Unsetenv("NOPOLLUTE_KEY")
	_, err := New(WithDefaults(path)).Read()
	s.Require().NoError(err)
	_, ok := os.LookupEnv("NOPOLLUTE_KEY")
	s.Require().False(ok, "defaults file must not be written into the process environment")
}

func (s *EnvTestSuite) TestReadDashAlias() {
	path := s.writeEnvFile("MY_KEY=value\n")
	os.Unsetenv("MY_KEY")
	c, err := New(WithDefaults(path)).Read()
	if err != nil {
		s.T().Fatal(err)
	}
	if _, ok := c.Data["my-key"]; !ok {
		s.T().Fatalf("expected dash-aliased key my-key, got %v", c.Data)
	}
}

// TestReadWithYamlEncoder / TestReadWithTomlEncoder: env values must be
// decodable with the documented backend.WithEncoder option. Previously the
// backend stored the encoder's raw Encode output, which yaml/toml Decode
// rejected ("unknown data type []uint8" / a nil value).
func (s *EnvTestSuite) TestReadWithYamlEncoder() {
	path := s.writeEnvFile("YAMLENC_KEY=value\n")
	os.Unsetenv("YAMLENC_KEY")
	defer os.Unsetenv("YAMLENC_KEY")
	c, err := New(WithDefaults(path), WithOption(backend.WithEncoder(yaml.New()))).Read()
	if err != nil {
		s.T().Fatal(err)
	}
	var v string
	s.Require().NoError(c.Encoder.Decode(c.Data["yamlenc_key"], &v))
	s.Require().Equal("value", v)
}

func (s *EnvTestSuite) TestReadWithTomlEncoder() {
	path := s.writeEnvFile("TOMLENC_KEY=value\n")
	os.Unsetenv("TOMLENC_KEY")
	defer os.Unsetenv("TOMLENC_KEY")
	c, err := New(WithDefaults(path), WithOption(backend.WithEncoder(toml.New()))).Read()
	if err != nil {
		s.T().Fatal(err)
	}
	var v string
	s.Require().NoError(c.Encoder.Decode(c.Data["tomlenc_key"], &v))
	s.Require().Equal("value", v)
}
