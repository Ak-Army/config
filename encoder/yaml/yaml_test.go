package yaml

import "testing"

func TestDecodeData(t *testing.T) {
	enc := New()
	data, err := enc.DecodeData([]byte("a: 1\nb: two\n"))
	if err != nil {
		t.Fatal(err)
	}
	var b string
	if err := enc.Decode(data["b"], &b); err != nil {
		t.Fatal(err)
	}
	if b != "two" {
		t.Fatalf("want two, got %q", b)
	}
}

// TestDecodeDataList covers the list decoding path, which previously wrote
// into a nil map and panicked.
func TestDecodeDataList(t *testing.T) {
	enc := New()
	list, err := enc.DecodeDataList([]byte("- name: a\n  age: 1\n- name: b\n  age: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 items, got %d", len(list))
	}
	var age int
	if err := enc.Decode(list[0]["age"], &age); err != nil {
		t.Fatal(err)
	}
	if age != 1 {
		t.Fatalf("want 1, got %d", age)
	}
}
