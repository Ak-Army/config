package consul

import (
	"strings"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
	jsonenc "github.com/Ak-Army/config/encoder/json"
	"github.com/Ak-Army/config/encoder/toml"
	"github.com/Ak-Army/config/encoder/yaml"
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

// TestReadKVAllEncoders: each consul value is a document in the configured
// encoder's own format (not JSON), so it is fed here in the native syntax of
// each encoder and must resolve to the same tree.
func (s *ConsulTestSuite) TestReadKVAllEncoders() {
	cases := map[string]struct {
		enc encoder.Encoder
		kv  api.KVPairs
	}{
		// A nested-struct value expressed in each format's native table syntax.
		"json": {jsonenc.New(), api.KVPairs{{Key: "db", Value: []byte(`{"name":"cfg"}`)}}},
		"yaml": {yaml.New(), api.KVPairs{{Key: "db", Value: []byte("name: cfg")}}},
		"toml": {toml.New(), api.KVPairs{{Key: "db", Value: []byte(`name = "cfg"`)}}},
	}
	for name, tc := range cases {
		c := New(WithOption(backend.WithEncoder(tc.enc))).(*consul)
		content, err := c.read(tc.kv)
		s.Require().NoError(err, name)
		dbData, err := content.Encoder.DecodeData(content.Data["db"])
		s.Require().NoError(err, name)
		var dbName string
		s.Require().NoError(content.Encoder.Decode(dbData["name"], &dbName), name)
		s.Require().Equal("cfg", dbName, name)
	}
}

// TestReadKVValueDocumentMerge covers the "any node + merge" model: a value that
// is a whole table document composes with a deeper path key targeting inside it.
func (s *ConsulTestSuite) TestReadKVValueDocumentMerge() {
	c := New(WithOption(backend.WithEncoder(toml.New()))).(*consul)
	content, err := c.read(api.KVPairs{
		// A table document stored at db, plus a scalar leaf nested via the path.
		{Key: "db", Value: []byte("host = \"h\"\nport = 5432")},
		{Key: "db/pass", Value: []byte(`pass = "secret"`)},
	})
	s.Require().NoError(err)
	dbData, err := content.Encoder.DecodeData(content.Data["db"])
	s.Require().NoError(err)
	var host string
	s.Require().NoError(content.Encoder.Decode(dbData["host"], &host))
	s.Require().Equal("h", host)
	var port int
	s.Require().NoError(content.Encoder.Decode(dbData["port"], &port))
	s.Require().Equal(5432, port)
	passData, err := content.Encoder.DecodeData(dbData["pass"])
	s.Require().NoError(err)
	var pass string
	s.Require().NoError(content.Encoder.Decode(passData["pass"], &pass))
	s.Require().Equal("secret", pass)
}

// TestReadKVTomlScalarError documents the TOML limitation: TOML has no
// bare-scalar document, so a scalar value under the TOML encoder is a clear
// error rather than a silently mis-parsed value.
func (s *ConsulTestSuite) TestReadKVTomlScalarError() {
	c := New(WithOption(backend.WithEncoder(toml.New()))).(*consul)
	_, err := c.read(api.KVPairs{
		{Key: "host", Value: []byte(`localhost`)},
	})
	if err == nil || !strings.Contains(err.Error(), `consul value at "host"`) {
		s.T().Fatalf("expected a value-parse error for a bare TOML scalar, got %v", err)
	}
}

// TestReadKVDeepNesting covers the subtree-marshal path (nested maps below the
// top level must keep their RawMessage leaves verbatim).
func (s *ConsulTestSuite) TestReadKVDeepNesting() {
	c := New().(*consul)
	content, err := c.read(api.KVPairs{
		{Key: "a/b/c/value", Value: []byte(`42`)},
		{Key: "a/b/other", Value: []byte(`"x"`)},
	})
	s.Require().NoError(err)
	bData, err := content.Encoder.DecodeData(content.Data["a"])
	s.Require().NoError(err)
	cData, err := content.Encoder.DecodeData(bData["b"])
	s.Require().NoError(err)
	var other string
	s.Require().NoError(content.Encoder.Decode(cData["other"], &other))
	s.Require().Equal("x", other)
	leafData, err := content.Encoder.DecodeData(cData["c"])
	s.Require().NoError(err)
	var value int
	s.Require().NoError(content.Encoder.Decode(leafData["value"], &value))
	s.Require().Equal(42, value)
}

// TestHandleDropsWrongType / TestHandleReadError: the watch handler must not
// deliver anything (and must not panic) on undecodable updates; the drop is
// logged so a stale config is diagnosable.
func (s *ConsulTestSuite) TestHandleDropsWrongType() {
	w, err := newWatcher(New(WithPrefix("cfg/")).(*consul))
	s.Require().NoError(err)
	ww := w.(*watcher)
	done := make(chan struct{})
	go func() {
		ww.handle(1, "not a KVPairs")
		close(done)
	}()
	select {
	case c := <-ww.ch:
		s.T().Fatalf("wrong-type data must not be delivered, got %+v", c)
	case <-done:
	}
}

func (s *ConsulTestSuite) TestHandleReadError() {
	w, err := newWatcher(New(WithPrefix("cfg/")).(*consul))
	s.Require().NoError(err)
	ww := w.(*watcher)
	done := make(chan struct{})
	go func() {
		// A leaf-and-prefix conflict makes read() fail.
		ww.handle(1, api.KVPairs{
			{Key: "a", Value: []byte(`"leaf"`)},
			{Key: "a/b", Value: []byte(`"child"`)},
		})
		close(done)
	}()
	select {
	case c := <-ww.ch:
		s.T().Fatalf("failed read must not be delivered, got %+v", c)
	case <-done:
	}
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
