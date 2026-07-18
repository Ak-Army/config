package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
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
