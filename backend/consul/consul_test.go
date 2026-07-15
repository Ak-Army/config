package consul

import (
	"strings"
	"testing"

	"github.com/hashicorp/consul/api"
)

func TestReadNilClient(t *testing.T) {
	_, err := New().Read()
	if err == nil || !strings.Contains(err.Error(), "client not set") {
		t.Fatalf("expected client-not-set error, got %v", err)
	}
}

func TestReadKV(t *testing.T) {
	c := New().(*consul)
	content, err := c.read(api.KVPairs{
		{Key: "host", Value: []byte(`"localhost"`)},
		{Key: "db/name", Value: []byte(`"cfg"`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := content.Data["host"]; !ok {
		t.Fatalf("expected host key, got %v", content.Data)
	}
	if _, ok := content.Data["db"]; !ok {
		t.Fatalf("expected nested db key, got %v", content.Data)
	}
}

// TestReadPathConflict verifies a key that is both a leaf and a prefix yields
// an error instead of a panic on the type assertion.
func TestReadPathConflict(t *testing.T) {
	c := New().(*consul)
	_, err := c.read(api.KVPairs{
		{Key: "a", Value: []byte(`"leaf"`)},
		{Key: "a/b", Value: []byte(`"child"`)},
	})
	if err == nil || !strings.Contains(err.Error(), "path conflict") {
		t.Fatalf("expected path-conflict error, got %v", err)
	}
}
