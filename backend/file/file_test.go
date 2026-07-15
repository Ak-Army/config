package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPathNotSet(t *testing.T) {
	if _, err := New().Read(); err == nil {
		t.Fatal("expected error when path is not set")
	}
}

func TestRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"key":"value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(WithPath(path)).Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Data["key"]; !ok {
		t.Fatalf("expected key in data, got %v", c.Data)
	}
	if c.Source != "file" {
		t.Fatalf("want source file, got %q", c.Source)
	}
}

func TestWatcherDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := New(WithPath(path)).Watcher()
	if err != nil {
		t.Fatal(err)
	}
	if w != nil {
		t.Fatal("expected nil watcher when not enabled")
	}
}
