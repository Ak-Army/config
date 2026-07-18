package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigCryptTestSuite struct {
	suite.Suite
}

func TestConfigCrypt(t *testing.T) {
	suite.Run(t, new(ConfigCryptTestSuite))
}

func (s *ConfigCryptTestSuite) genKey() string {
	var keyOut bytes.Buffer
	s.Require().NoError(run([]string{"-genkey"}, strings.NewReader(""), &keyOut, &bytes.Buffer{}))
	return strings.TrimSpace(keyOut.String())
}

func (s *ConfigCryptTestSuite) writeKeyring(content string) string {
	path := filepath.Join(s.T().TempDir(), "keyring")
	s.Require().NoError(os.WriteFile(path, []byte(content), 0o600))
	return path
}

func (s *ConfigCryptTestSuite) TestRunRoundTrip() {
	keyring := s.writeKeyring("v1: " + s.genKey() + "\n")

	var encOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", keyring, "secret value"}, strings.NewReader(""), &encOut, &bytes.Buffer{}))
	encoded := strings.TrimSpace(encOut.String())
	s.True(strings.HasPrefix(encoded, "ENC(v1:"))

	var decOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", keyring, "-d", encoded}, strings.NewReader(""), &decOut, &bytes.Buffer{}))
	s.Equal("secret value", strings.TrimSpace(decOut.String()))
}

func (s *ConfigCryptTestSuite) TestRunStdinValue() {
	keyring := s.writeKeyring("v1: " + s.genKey() + "\n")

	var encOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", keyring}, strings.NewReader("secret value\n"), &encOut, &bytes.Buffer{}))
	s.True(strings.HasPrefix(strings.TrimSpace(encOut.String()), "ENC(v1:"))
}

func (s *ConfigCryptTestSuite) TestRunRekeyValue() {
	oldKey, newKey := s.genKey(), s.genKey()
	oldRing := s.writeKeyring("v1: " + oldKey + "\n")
	newRing := s.writeKeyring("v2: " + newKey + "\nv1: " + oldKey + "\n")

	var encOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", oldRing, "secret value"}, strings.NewReader(""), &encOut, &bytes.Buffer{}))
	encoded := strings.TrimSpace(encOut.String())

	var rekeyOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", newRing, "-rekey", encoded}, strings.NewReader(""), &rekeyOut, &bytes.Buffer{}))
	rekeyed := strings.TrimSpace(rekeyOut.String())
	s.True(strings.HasPrefix(rekeyed, "ENC(v2:"))

	var decOut bytes.Buffer
	s.Require().NoError(run([]string{"-key", newRing, "-d", rekeyed}, strings.NewReader(""), &decOut, &bytes.Buffer{}))
	s.Equal("secret value", strings.TrimSpace(decOut.String()))
}

func (s *ConfigCryptTestSuite) TestRunRekeyFile() {
	oldKey, newKey := s.genKey(), s.genKey()
	oldRing := s.writeKeyring("v1: " + oldKey + "\n")
	newRing := s.writeKeyring("v2: " + newKey + "\nv1: " + oldKey + "\n")

	encrypt := func(value string) string {
		var out bytes.Buffer
		s.Require().NoError(run([]string{"-key", oldRing, value}, strings.NewReader(""), &out, &bytes.Buffer{}))
		return strings.TrimSpace(out.String())
	}
	configPath := filepath.Join(s.T().TempDir(), "config.json")
	content := `{"pass":"` + encrypt("pw") + `","plain":"untouched","api":"` + encrypt("key") + `"}`
	s.Require().NoError(os.WriteFile(configPath, []byte(content), 0o600))

	// stdout mode leaves the file unchanged
	var stdout, stderr bytes.Buffer
	s.Require().NoError(run([]string{"-key", newRing, "-rekey", "-in", configPath}, strings.NewReader(""), &stdout, &stderr))
	onDisk, err := os.ReadFile(configPath)
	s.Require().NoError(err)
	s.Equal(content, string(onDisk))
	s.Contains(stderr.String(), "2 value(s) re-encrypted")

	// -write rewrites in place; non-ENC parts stay byte-identical
	s.Require().NoError(run([]string{"-key", newRing, "-rekey", "-in", configPath, "-write"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}))
	rewritten, err := os.ReadFile(configPath)
	s.Require().NoError(err)
	s.Contains(string(rewritten), `"plain":"untouched"`)
	s.Contains(string(rewritten), "ENC(v2:")
	s.NotContains(string(rewritten), "ENC(v1:")

	// idempotent second run
	var stderr2 bytes.Buffer
	s.Require().NoError(run([]string{"-key", newRing, "-rekey", "-in", configPath, "-write"}, strings.NewReader(""), &bytes.Buffer{}, &stderr2))
	s.Contains(stderr2.String(), "0 value(s) re-encrypted, 2 already")
}

func (s *ConfigCryptTestSuite) TestRunErrors() {
	discard := func() (*bytes.Buffer, *bytes.Buffer) { return &bytes.Buffer{}, &bytes.Buffer{} }

	o, e := discard()
	s.Error(run([]string{"value"}, strings.NewReader(""), o, e), "missing -key")

	o, e = discard()
	s.Error(run([]string{"-key", "/nonexistent", "value"}, strings.NewReader(""), o, e))

	badRing := s.writeKeyring("v1: not-base64!!!\n")
	o, e = discard()
	s.Error(run([]string{"-key", badRing, "value"}, strings.NewReader(""), o, e))

	keyring := s.writeKeyring("v1: " + s.genKey() + "\n")
	o, e = discard()
	s.Error(run([]string{"-key", keyring}, strings.NewReader(""), o, e), "empty stdin")
	o, e = discard()
	s.Error(run([]string{"-key", keyring, "-d", "not-enc"}, strings.NewReader(""), o, e))
	o, e = discard()
	s.Error(run([]string{"-key", keyring, "-d", "ENC(cmF3)"}, strings.NewReader(""), o, e), "missing kid")
	o, e = discard()
	s.Error(run([]string{"-key", keyring, "-rekey", "ENC(v9:cmF3)"}, strings.NewReader(""), o, e), "unknown kid")
}

func (s *ConfigCryptTestSuite) TestBase64KeyOutput() {
	raw, err := base64.StdEncoding.DecodeString(s.genKey())
	s.Require().NoError(err)
	s.Len(raw, 32)
}
