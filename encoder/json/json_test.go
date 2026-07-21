package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
)

type JsonTestSuite struct {
	suite.Suite
}

func TestJson(t *testing.T) {
	suite.Run(t, new(JsonTestSuite))
}

func (s *JsonTestSuite) TestEncodeDecode() {
	enc := New()
	b, err := enc.Encode(map[string]int{"a": 1})
	s.Require().NoError(err)
	data, err := enc.DecodeData(b)
	s.Require().NoError(err)
	var got int
	s.Require().NoError(enc.Decode(data["a"], &got))
	s.Require().Equal(1, got)
}

// TestDecodeDataListBytes covers the []byte branch, which previously wrote
// into a nil map and panicked.
func (s *JsonTestSuite) TestDecodeDataListBytes() {
	enc := New()
	list, err := enc.DecodeDataList([]byte(`[{"name":"a","age":1},{"name":"b","age":2}]`))
	s.Require().NoError(err)
	s.Require().Len(list, 2)
	var name string
	s.Require().NoError(enc.Decode(list[1]["name"], &name))
	s.Require().Equal("b", name)
}

func (s *JsonTestSuite) TestDecodeUnknownType() {
	s.Require().Error(New().Decode(42, new(int)), "expected error for unknown data type")
}

// TestDecodeDataNull mirrors the standard library: a JSON null decodes to an
// empty (non-nil) object without an error.
func (s *JsonTestSuite) TestDecodeDataNull() {
	data, err := New().DecodeData([]byte(`null`))
	s.Require().NoError(err)
	s.Require().NotNil(data)
	s.Require().Len(data, 0)
}

// TestDecodeDataEmptyKey ensures an empty-string key neither terminates the
// object scan early nor drops the keys after it.
func (s *JsonTestSuite) TestDecodeDataEmptyKey() {
	data, err := New().DecodeData([]byte(`{"":1,"a":2}`))
	s.Require().NoError(err)
	s.Require().Len(data, 2)
	var a int
	s.Require().NoError(New().Decode(data["a"], &a))
	s.Require().Equal(2, a)
}

// TestDecodeDataNotObject checks that a non-object value is rejected.
func (s *JsonTestSuite) TestDecodeDataNotObject() {
	_, err := New().DecodeData([]byte(`"a string"`))
	s.Require().Error(err, "expected error for non-object value")
}

// TestDecodeDataNested verifies raw nested values survive a round trip and stay
// decodable, which is what the loader relies on for nested structs.
func (s *JsonTestSuite) TestDecodeDataNested() {
	data, err := New().DecodeData([]byte(`{"n":{"key":"v"},"i":10}`))
	s.Require().NoError(err)
	inner, err := New().DecodeData(data["n"])
	s.Require().NoError(err)
	var str string
	s.Require().NoError(New().Decode(inner["key"], &str))
	s.Require().Equal("v", str)
	var i int
	s.Require().NoError(New().Decode(data["i"], &i))
	s.Require().Equal(10, i)
}

// TestDecodeDataListEmpty ensures an empty array yields an empty result.
func (s *JsonTestSuite) TestDecodeDataListEmpty() {
	list, err := New().DecodeDataList([]byte(`[]`))
	s.Require().NoError(err)
	s.Require().Len(list, 0)
}

// TestDecodeValue covers the single-value parse path: a scalar, an object and a
// large integer that must survive re-marshalling without float64 precision loss.
func (s *JsonTestSuite) TestDecodeValue() {
	enc := New()

	v, err := enc.DecodeValue([]byte(`"localhost"`))
	s.Require().NoError(err)
	s.Require().Equal("localhost", v)

	v, err = enc.DecodeValue([]byte(`{"name":"cfg","port":5432}`))
	s.Require().NoError(err)
	b, err := json.Marshal(v)
	s.Require().NoError(err)
	s.Require().JSONEq(`{"name":"cfg","port":5432}`, string(b))

	v, err = enc.DecodeValue([]byte(`123456789012345678`))
	s.Require().NoError(err)
	b, err = json.Marshal(v)
	s.Require().NoError(err)
	s.Require().Equal("123456789012345678", string(b))
}
