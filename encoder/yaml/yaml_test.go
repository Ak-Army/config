package yaml

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type YamlTestSuite struct {
	suite.Suite
}

func TestYaml(t *testing.T) {
	suite.Run(t, new(YamlTestSuite))
}

func (s *YamlTestSuite) TestDecodeData() {
	enc := New()
	data, err := enc.DecodeData([]byte("a: 1\nb: two\n"))
	s.Require().NoError(err)
	var b string
	s.Require().NoError(enc.Decode(data["b"], &b))
	s.Require().Equal("two", b)
}

// TestDecodeDataYAML11Scalars ensures unquoted yes/no/on/off stay strings
// (YAML 1.2 semantics) instead of being resolved to booleans as yaml.v2 did.
func (s *YamlTestSuite) TestDecodeDataYAML11Scalars() {
	enc := New()
	data, err := enc.DecodeData([]byte("region: no\nenabled: true\n"))
	s.Require().NoError(err)
	var region string
	s.Require().NoError(enc.Decode(data["region"], &region))
	s.Require().Equal("no", region)
	var enabled bool
	s.Require().NoError(enc.Decode(data["enabled"], &enabled))
	s.Require().True(enabled)
}

// TestDecodeDataList covers the list decoding path, which previously wrote
// into a nil map and panicked.
func (s *YamlTestSuite) TestDecodeDataList() {
	enc := New()
	list, err := enc.DecodeDataList([]byte("- name: a\n  age: 1\n- name: b\n  age: 2\n"))
	s.Require().NoError(err)
	s.Require().Len(list, 2)
	var age int
	s.Require().NoError(enc.Decode(list[0]["age"], &age))
	s.Require().Equal(1, age)
}
