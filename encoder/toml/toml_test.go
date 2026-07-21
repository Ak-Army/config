package toml

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TomlTestSuite struct {
	suite.Suite
}

func TestToml(t *testing.T) {
	suite.Run(t, new(TomlTestSuite))
}

func (s *TomlTestSuite) TestDecodeData() {
	enc := New()
	data, err := enc.DecodeData([]byte("a = 1\nb = \"two\"\n"))
	s.Require().NoError(err)
	var b string
	s.Require().NoError(enc.Decode(data["b"], &b))
	s.Require().Equal("two", b)
}

// TestDecodeDataList covers arrays of tables. This path previously panicked
// on a nil map and could not decode scalar fields (innerToml had no
// UnmarshalJSON).
func (s *TomlTestSuite) TestDecodeDataList() {
	enc := New()
	data, err := enc.DecodeData([]byte("[[items]]\nname = \"a\"\nage = 1\n[[items]]\nname = \"b\"\nage = 2\n"))
	s.Require().NoError(err)
	list, err := enc.DecodeDataList(data["items"])
	s.Require().NoError(err)
	s.Require().Len(list, 2)
	var name string
	s.Require().NoError(enc.Decode(list[0]["name"], &name))
	var age int
	s.Require().NoError(enc.Decode(list[1]["age"], &age))
	s.Require().Equal("a", name)
	s.Require().Equal(2, age)
}

// TestDecodeDataRawMessage: json.RawMessage is the backend-neutral leaf
// representation (env, consul) and must be accepted alongside TOML source.
func (s *TomlTestSuite) TestDecodeDataRawMessage() {
	enc := New()
	data, err := enc.DecodeData(json.RawMessage(`{"a":"b","n":{"k":1}}`))
	s.Require().NoError(err)
	var a string
	s.Require().NoError(enc.Decode(data["a"], &a))
	s.Require().Equal("b", a)
	nested, err := enc.DecodeData(data["n"])
	s.Require().NoError(err)
	var k int
	s.Require().NoError(enc.Decode(nested["k"], &k))
	s.Require().Equal(1, k)
}

func (s *TomlTestSuite) TestDecodeDataListRawMessage() {
	enc := New()
	list, err := enc.DecodeDataList(json.RawMessage(`[{"name":"a"},{"name":"b"}]`))
	s.Require().NoError(err)
	s.Require().Len(list, 2)
	var name string
	s.Require().NoError(enc.Decode(list[1]["name"], &name))
	s.Require().Equal("b", name)
}

// TestDecodeValue covers the single-value parse path: a TOML table parses, while
// a bare scalar is an error because TOML has no bare-scalar document.
func (s *TomlTestSuite) TestDecodeValue() {
	enc := New()

	v, err := enc.DecodeValue([]byte("name = \"cfg\"\nport = 5432"))
	s.Require().NoError(err)
	b, err := json.Marshal(v)
	s.Require().NoError(err)
	s.Require().JSONEq(`{"name":"cfg","port":5432}`, string(b))

	_, err = enc.DecodeValue([]byte(`localhost`))
	s.Require().Error(err)
}
