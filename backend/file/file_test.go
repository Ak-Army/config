package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type FileTestSuite struct {
	suite.Suite
}

func TestFile(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}

func (s *FileTestSuite) TestReadPathNotSet() {
	if _, err := New().Read(); err == nil {
		s.T().Fatal("expected error when path is not set")
	}
}

func (s *FileTestSuite) TestRead() {
	path := filepath.Join(s.T().TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"key":"value"}`), 0o600); err != nil {
		s.T().Fatal(err)
	}
	c, err := New(WithPath(path)).Read()
	if err != nil {
		s.T().Fatal(err)
	}
	if _, ok := c.Data["key"]; !ok {
		s.T().Fatalf("expected key in data, got %v", c.Data)
	}
	if c.Source != "file" {
		s.T().Fatalf("want source file, got %q", c.Source)
	}
}

func (s *FileTestSuite) TestWatcherDisabledByDefault() {
	path := filepath.Join(s.T().TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		s.T().Fatal(err)
	}
	w, err := New(WithPath(path)).Watcher()
	if err != nil {
		s.T().Fatal(err)
	}
	if w != nil {
		s.T().Fatal("expected nil watcher when not enabled")
	}
}
