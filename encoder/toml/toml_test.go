package toml

import "testing"

func TestDecodeData(t *testing.T) {
	enc := New()
	data, err := enc.DecodeData([]byte("a = 1\nb = \"two\"\n"))
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

// TestDecodeDataList covers arrays of tables. This path previously panicked
// on a nil map and could not decode scalar fields (innerToml had no
// UnmarshalJSON).
func TestDecodeDataList(t *testing.T) {
	enc := New()
	data, err := enc.DecodeData([]byte("[[items]]\nname = \"a\"\nage = 1\n[[items]]\nname = \"b\"\nage = 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := enc.DecodeDataList(data["items"])
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 items, got %d", len(list))
	}
	var name string
	if err := enc.Decode(list[0]["name"], &name); err != nil {
		t.Fatal(err)
	}
	var age int
	if err := enc.Decode(list[1]["age"], &age); err != nil {
		t.Fatal(err)
	}
	if name != "a" || age != 2 {
		t.Fatalf("want a/2, got %q/%d", name, age)
	}
}
