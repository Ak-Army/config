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

// TestDecodeDataNull mirrors the standard library: a JSON null decodes to an
// empty (non-nil) object without an error.
func TestDecodeDataNull(t *testing.T) {
	data, err := New().DecodeData([]byte(`null`))
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || len(data) != 0 {
		t.Fatalf("want empty non-nil map, got %#v", data)
	}
}

// TestDecodeDataNotObject checks that a non-object value is rejected.
func TestDecodeDataNotObject(t *testing.T) {
	if _, err := New().DecodeData([]byte(`"a string"`)); err == nil {
		t.Fatal("expected error for non-object value")
	}
}

// TestDecodeDataNested verifies raw nested values survive a round trip and stay
// decodable, which is what the loader relies on for nested structs.
func TestDecodeDataNested(t *testing.T) {
	data, err := New().DecodeData([]byte(`{"n":{"key":"v"},"i":10}`))
	if err != nil {
		t.Fatal(err)
	}
	inner, err := New().DecodeData(data["n"])
	if err != nil {
		t.Fatal(err)
	}
	var s string
	if err := New().Decode(inner["key"], &s); err != nil {
		t.Fatal(err)
	}
	if s != "v" {
		t.Fatalf("want v, got %q", s)
	}
	var i int
	if err := New().Decode(data["i"], &i); err != nil {
		t.Fatal(err)
	}
	if i != 10 {
		t.Fatalf("want 10, got %d", i)
	}
}

// TestDecodeDataListEmpty ensures an empty array yields an empty result.
func TestDecodeDataListEmpty(t *testing.T) {
	list, err := New().DecodeDataList([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want 0 items, got %d", len(list))
	}
}
