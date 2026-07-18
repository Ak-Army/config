package consul

import (
	"strings"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/suite"
)

type ConsulTestSuite struct {
	suite.Suite
}

func TestConsul(t *testing.T) {
	suite.Run(t, new(ConsulTestSuite))
}

func (s *ConsulTestSuite) TestReadNilClient() {
	_, err := New().Read()
	if err == nil || !strings.Contains(err.Error(), "client not set") {
		s.T().Fatalf("expected client-not-set error, got %v", err)
	}
}

func (s *ConsulTestSuite) TestReadKV() {
	c := New().(*consul)
	content, err := c.read(api.KVPairs{
		{Key: "host", Value: []byte(`"localhost"`)},
		{Key: "db/name", Value: []byte(`"cfg"`)},
	})
	if err != nil {
		s.T().Fatal(err)
	}
	// Values must survive verbatim: a []byte stored into the map would be
	// base64-encoded by the JSON encoder.
	var host string
	s.Require().NoError(content.Encoder.Decode(content.Data["host"], &host))
	s.Require().Equal("localhost", host)
	dbData, err := content.Encoder.DecodeData(content.Data["db"])
	s.Require().NoError(err)
	var dbName string
	s.Require().NoError(content.Encoder.Decode(dbData["name"], &dbName))
	s.Require().Equal("cfg", dbName)
}

// TestReadPathConflict verifies a key that is both a leaf and a prefix yields
// an error instead of a panic on the type assertion.
func (s *ConsulTestSuite) TestReadPathConflict() {
	c := New().(*consul)
	_, err := c.read(api.KVPairs{
		{Key: "a", Value: []byte(`"leaf"`)},
		{Key: "a/b", Value: []byte(`"child"`)},
	})
	if err == nil || !strings.Contains(err.Error(), "path conflict") {
		s.T().Fatalf("expected path-conflict error, got %v", err)
	}
}
