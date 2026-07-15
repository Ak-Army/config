package json

import "testing"

func TestEncodeDecode(t *testing.T) {
	enc := New()
	b, err := enc.Encode(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	data, err := enc.DecodeData(b)
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if err := enc.Decode(data["a"], &got); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

// TestDecodeDataListBytes covers the []byte branch, which previously wrote
// into a nil map and panicked.
func TestDecodeDataListBytes(t *testing.T) {
	enc := New()
	list, err := enc.DecodeDataList([]byte(`[{"name":"a","age":1},{"name":"b","age":2}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 items, got %d", len(list))
	}
	var name string
	if err := enc.Decode(list[1]["name"], &name); err != nil {
		t.Fatal(err)
	}
	if name != "b" {
		t.Fatalf("want b, got %q", name)
	}
}

func TestDecodeUnknownType(t *testing.T) {
	if err := New().Decode(42, new(int)); err == nil {
		t.Fatal("expected error for unknown data type")
	}
}
