package consul

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ak-Army/xlog"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/backend"
	"github.com/Ak-Army/config/encoder"
	jsonenc "github.com/Ak-Army/config/encoder/json"
	"github.com/Ak-Army/config/encoder/toml"
	"github.com/Ak-Army/config/encoder/yaml"
)

const deliveryTimeout = time.Second

type ConsulTestSuite struct {
	suite.Suite
	logged []map[string]any
}

func TestConsul(t *testing.T) {
	suite.Run(t, new(ConsulTestSuite))
}

func (s *ConsulTestSuite) SetupTest() {
	s.logged = nil
}

func kvPair(key, value string, modifyIndex uint64) *api.KVPair {
	return &api.KVPair{Key: key, Value: []byte(value), ModifyIndex: modifyIndex}
}

// newLoggedConsul returns a consul backend whose log entries land in s.logged.
func (s *ConsulTestSuite) newLoggedConsul(opts ...Option) *consul {
	logger := xlog.New(xlog.Config{
		Level:          xlog.LevelDebug,
		DisablePooling: true,
		Output: xlog.OutputFunc(func(fields map[string]any) error {
			s.logged = append(s.logged, fields)
			return nil
		}),
	})
	ctx := xlog.NewContext(context.Background(), logger)
	return New(append(opts, WithOption(backend.WithContext(ctx)))...).(*consul)
}

func (s *ConsulTestSuite) mustRead(c *consul, kvs ...*api.KVPair) *backend.Content {
	s.T().Helper()
	content, err := c.read(kvs)
	s.Require().NoError(err)
	return content
}

func (s *ConsulTestSuite) stringAt(data encoder.Data, enc encoder.Encoder, key string) string {
	s.T().Helper()
	s.Require().Contains(data, key)
	var value string
	s.Require().NoError(enc.Decode(data[key], &value))
	return value
}

func (s *ConsulTestSuite) tableAt(content *backend.Content, key string) encoder.Data {
	s.T().Helper()
	s.Require().Contains(content.Data, key)
	table, err := content.Encoder.DecodeData(content.Data[key])
	s.Require().NoError(err)
	return table
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

// TestReadKVTomlScalarSkipped documents the TOML limitation: TOML has no
// bare-scalar document, so a scalar value under the TOML encoder is an unusable
// key that is skipped rather than a silently mis-parsed value.
func (s *ConsulTestSuite) TestReadKVTomlScalarSkipped() {
	c := s.newLoggedConsul(WithOption(backend.WithEncoder(toml.New())))
	content := s.mustRead(c, kvPair("host", `localhost`, 1))
	s.NotContains(content.Data, "host")
}

func (s *ConsulTestSuite) TestReadKVStripsPrefix() {
	c := s.newLoggedConsul(WithStripPrefix("vcc/"))
	content := s.mustRead(c, kvPair("vcc/", ``, 1), kvPair("vcc/host", `"localhost"`, 1))
	s.Equal("localhost", s.stringAt(content.Data, content.Encoder, "host"))
}

// TestReadKVMergesValueTableWithPathTable: a table nested in a value composes
// with a table stored at the deeper path pointing at the same node.
func (s *ConsulTestSuite) TestReadKVMergesValueTableWithPathTable() {
	content := s.mustRead(s.newLoggedConsul(),
		kvPair("db", `{"auth":{"user":"u"}}`, 1),
		kvPair("db/auth", `{"pass":"p"}`, 1),
	)
	auth := s.tableAt(&backend.Content{Data: s.tableAt(content, "db"), Encoder: content.Encoder}, "auth")
	s.Equal("u", s.stringAt(auth, content.Encoder, "user"))
	s.Equal("p", s.stringAt(auth, content.Encoder, "pass"))
}

func (s *ConsulTestSuite) TestWatcherDisabledByDefault() {
	w, err := New(WithPrefix("cfg/")).Watcher()
	s.NoError(err)
	s.Nil(w)
}

func (s *ConsulTestSuite) TestWatcherEnabledByOption() {
	w, err := New(WithPrefix("cfg/"), WithOption(backend.WithWatcher())).Watcher()
	s.NoError(err)
	s.NotNil(w)
}

func (s *ConsulTestSuite) TestReadKVSkipsUndecodableKey() {
	content := s.mustRead(s.newLoggedConsul(),
		kvPair("cica", `miau`, 1),
		kvPair("host", `"localhost"`, 1),
	)
	s.NotContains(content.Data, "cica")
	s.Equal("localhost", s.stringAt(content.Data, content.Encoder, "host"))
}

func (s *ConsulTestSuite) TestReadKVKeepsLastValidValue() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("host", `"localhost"`, 1))
	content := s.mustRead(c, kvPair("host", `"broken`, 2))
	s.Equal("localhost", s.stringAt(content.Data, content.Encoder, "host"))
}

func (s *ConsulTestSuite) TestReadKVKeepsLastValidValueWhileKeyStaysBroken() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("host", `"localhost"`, 1))
	s.mustRead(c, kvPair("host", `"broken`, 2))
	content := s.mustRead(c, kvPair("host", `"broken`, 2), kvPair("other", `1`, 3))
	s.Equal("localhost", s.stringAt(content.Data, content.Encoder, "host"))
}

func (s *ConsulTestSuite) TestReadKVForgetsLastValidValueOfDeletedKey() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("host", `"localhost"`, 1))
	s.mustRead(c, kvPair("other", `1`, 2))
	content := s.mustRead(c, kvPair("host", `"broken`, 3))
	s.NotContains(content.Data, "host")
}

// TestReadKVLastValidTableDropsMergedPaths: a deeper path key merges into the
// table of its parent key, which must not leak into the parent's last valid value.
func (s *ConsulTestSuite) TestReadKVLastValidTableDropsMergedPaths() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("db", `{"name":"cfg"}`, 1), kvPair("db/pass", `"secret"`, 1))
	db := s.tableAt(s.mustRead(c, kvPair("db", `{broken`, 2)), "db")
	s.Contains(db, "name")
	s.NotContains(db, "pass")
}

func (s *ConsulTestSuite) TestReadKVSkipsFolderKeySilently() {
	c := s.newLoggedConsul()
	content := s.mustRead(c, kvPair("db/", ``, 1), kvPair("db/name", `"cfg"`, 1))
	s.Equal("cfg", s.stringAt(s.tableAt(content, "db"), content.Encoder, "name"))
	s.Empty(s.logged)
}

func (s *ConsulTestSuite) TestReadKVReportsUnusableKey() {
	s.mustRead(s.newLoggedConsul(), kvPair("cica", `miau`, 7))
	s.Require().Len(s.logged, 1)
	s.Equal("Unable to use Consul value", s.logged[0][xlog.KeyMessage])
	s.Equal("cica", s.logged[0]["key"])
	s.Equal(false, s.logged[0]["lastValidKept"])
	s.Contains(s.logged[0], xlog.KeyError)
}

func (s *ConsulTestSuite) TestReadKVReportsLastValidKept() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("host", `"localhost"`, 1))
	s.mustRead(c, kvPair("host", `"broken`, 2))
	s.Require().Len(s.logged, 1)
	s.Equal(true, s.logged[0]["lastValidKept"])
}

func (s *ConsulTestSuite) TestReadKVReportsUnusableKeyOncePerVersion() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("cica", `miau`, 7))
	s.mustRead(c, kvPair("cica", `miau`, 7), kvPair("other", `1`, 8))
	s.Len(s.logged, 1)
}

func (s *ConsulTestSuite) TestReadKVReportsNewVersionOfUnusableKey() {
	c := s.newLoggedConsul()
	s.mustRead(c, kvPair("cica", `miau`, 7))
	s.mustRead(c, kvPair("cica", `vau`, 8))
	s.Len(s.logged, 2)
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

// TestHandleDropsWrongType: the watch handler must not deliver anything (and
// must not panic) on data that is not a KV list; the drop is logged so a stale
// config is diagnosable.
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

// TestHandleDeliversDespiteUnusableKey: an update with one unusable key is still
// delivered, so the other keys do not stay stale.
func (s *ConsulTestSuite) TestHandleDeliversDespiteUnusableKey() {
	w, err := newWatcher(s.newLoggedConsul(WithPrefix("cfg/")))
	s.Require().NoError(err)
	ww := w.(*watcher)
	go ww.handle(1, api.KVPairs{kvPair("a", `"leaf"`, 1), kvPair("a/b", `"child"`, 1)})
	select {
	case content := <-ww.ch:
		s.Equal("leaf", s.stringAt(content.Data, content.Encoder, "a"))
	case <-time.After(deliveryTimeout):
		s.Fail("an update with an unusable key must still be delivered")
	}
}

// TestReadPathConflictSkipsLaterKey verifies a key that is both a leaf and a
// prefix drops the later key (consul lists keys in lexicographic order)
// instead of panicking on the type assertion.
func (s *ConsulTestSuite) TestReadPathConflictSkipsLaterKey() {
	content := s.mustRead(s.newLoggedConsul(), kvPair("a", `"leaf"`, 1), kvPair("a/b", `"child"`, 1))
	s.Equal("leaf", s.stringAt(content.Data, content.Encoder, "a"))
	s.Len(s.logged, 1)
}
