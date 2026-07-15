package env

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "defaults.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadStripPrefix(t *testing.T) {
	path := writeEnvFile(t, "AA_KEY=value\nBB_KEY=other\n")
	c, err := New(WithDefaults(path), WithStripPrefix("AA_")).Read()
	if err != nil {
		t.Fatal(err)
	}
	// Stripped key is lower-cased and stored both with "_" and "-".
	if _, ok := c.Data["key"]; !ok {
		t.Fatalf("expected stripped key %q in data, got %v", "key", c.Data)
	}
	// A value that does not match any prefix must be filtered out.
	if _, ok := c.Data["bb_key"]; ok {
		t.Fatalf("did not expect bb_key in data, got %v", c.Data)
	}
}

func TestReadDashAlias(t *testing.T) {
	path := writeEnvFile(t, "MY_KEY=value\n")
	os.Unsetenv("MY_KEY")
	c, err := New(WithDefaults(path)).Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Data["my-key"]; !ok {
		t.Fatalf("expected dash-aliased key my-key, got %v", c.Data)
	}
}
