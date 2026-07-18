package toml

import (
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
