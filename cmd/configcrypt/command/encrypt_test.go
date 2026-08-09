package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type EncryptTestSuite struct {
	commandSuite
}

func TestEncrypt(t *testing.T) {
	suite.Run(t, new(EncryptTestSuite))
}

func (s *EncryptTestSuite) TestRunString() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &encrypt{Base: Base{Key: ring}, In: "secret"}

	out := s.captureStdout(func() {
		s.Require().NoError(c.Run(context.Background()))
	})

	encoded := strings.TrimSpace(out)
	s.True(strings.HasPrefix(encoded, "ENC(v1:"))
	s.Equal("secret", s.decryptWith(ring, encoded))
}

func (s *EncryptTestSuite) TestRunFileRekey() {
	oldRing := s.writeKeyring("old.keyring", "v1")
	ring := s.writeKeyring("config.keyring", "v2", "v1")
	file := s.writeFile("config.yml",
		"password: "+s.encryptWith(oldRing, "secret")+"\nuser: admin\n", 0o640)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	s.Require().NoError(c.Run(context.Background()))

	content := s.readFile(file)
	matches := encPattern.FindAllString(content, -1)
	s.Require().Len(matches, 1)
	s.True(strings.HasPrefix(matches[0], "ENC(v2:"))
	s.Equal("secret", s.decryptWith(ring, matches[0]))
	s.Contains(content, "user: admin")
	s.Equal(os.FileMode(0o640), s.filePerm(file))
}

func (s *EncryptTestSuite) TestRunFileAlreadyActive() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := "password: " + s.encryptWith(ring, "secret") + "\n"
	file := s.writeFile("config.yml", original, 0o600)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	s.Require().NoError(c.Run(context.Background()))

	s.Equal(original, s.readFile(file))
}

func (s *EncryptTestSuite) TestRunFileUnknownKeyID() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := "password: ENC(nope:AAAA)\n"
	file := s.writeFile("config.yml", original, 0o600)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "ENC(nope:AAAA)")
	s.Contains(err.Error(), "unknown key id")
	s.Equal(original, s.readFile(file), "a failed run must not rewrite the file")
}

func (s *EncryptTestSuite) TestRunFileReportsFirstError() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml", "a: ENC(nope:AAAA)\nb: ENC(other:BBBB)\n", 0o600)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "ENC(nope:AAAA)")
	s.NotContains(err.Error(), "ENC(other:BBBB)")
}

// encryptFileForTest runs the encrypt command on file and returns its new
// content, keeping the summary line out of the test output.
func (s *EncryptTestSuite) encryptFileForTest(ring, file string) string {
	s.T().Helper()
	c := &encrypt{Base: Base{Key: ring}, File: file}
	s.captureStdout(func() {
		s.Require().NoError(c.Run(context.Background()))
	})
	return s.readFile(file)
}

// requireEncrypted asserts that content holds exactly one envelope, built with
// the ring's active key, and that it decrypts back to plaintext.
func (s *EncryptTestSuite) requireEncrypted(ring, content, plaintext string) {
	s.T().Helper()
	matches := encPattern.FindAllString(content, -1)
	s.Require().Len(matches, 1)
	s.Equal(plaintext, s.decryptWith(ring, matches[0]))
	s.NotContains(content, "PLAIN(")
}

func (s *EncryptTestSuite) TestRunFileMarkedJSON() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.json",
		"{\n  \"api-key\": \"PLAIN(s3cr3t)\",\n  \"user\": \"admin\"\n}\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	s.requireEncrypted(ring, content, "s3cr3t")
	s.Contains(content, `"user": "admin"`)
}

func (s *EncryptTestSuite) TestRunFileMarkedYaml() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml",
		"# comment\npassword: PLAIN(s3cr3t)   # inline comment\nuser: admin\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	s.requireEncrypted(ring, content, "s3cr3t")
	s.Contains(content, "# comment\n")
	s.Contains(content, "   # inline comment")
	s.Contains(content, "user: admin")
}

func (s *EncryptTestSuite) TestRunFileMarkedToml() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.toml",
		"[db]\npassword = \"PLAIN(s3cr3t)\"\nuser = \"admin\"\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	s.requireEncrypted(ring, content, "s3cr3t")
	s.Contains(content, "[db]\n")
	s.Contains(content, `user = "admin"`)
}

// TestRunFileMarkedMinified: several markers on one line, as in a minified
// JSON document, are encrypted one by one.
func (s *EncryptTestSuite) TestRunFileMarkedMinified() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.json",
		`{"a":"PLAIN(first)","b":"PLAIN(second)","c":"plain"}`, 0o600)

	content := s.encryptFileForTest(ring, file)

	matches := encPattern.FindAllString(content, -1)
	s.Require().Len(matches, 2)
	s.Equal("first", s.decryptWith(ring, matches[0]))
	s.Equal("second", s.decryptWith(ring, matches[1]))
	s.Contains(content, `"c":"plain"`)
}

// TestRunFileMarkedParentheses: the marker ends at the ')' that closes the
// config value, so the secret itself may contain parentheses.
func (s *EncryptTestSuite) TestRunFileMarkedParentheses() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml", "password: \"PLAIN(p@ss(w0rd))\"\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	s.requireEncrypted(ring, content, "p@ss(w0rd)")
}

func (s *EncryptTestSuite) TestRunFileMarkedEmptyValue() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml", "password: PLAIN()\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	s.requireEncrypted(ring, content, "")
}

// TestRunFileMarkedAndRekeyed: one run both encrypts the marked values and
// re-keys the ones encrypted with an older key.
func (s *EncryptTestSuite) TestRunFileMarkedAndRekeyed() {
	oldRing := s.writeKeyring("old.keyring", "v1")
	ring := s.writeKeyring("config.keyring", "v2", "v1")
	file := s.writeFile("config.yml",
		"old: "+s.encryptWith(oldRing, "old secret")+"\nnew: PLAIN(new secret)\n", 0o600)

	content := s.encryptFileForTest(ring, file)

	matches := encPattern.FindAllString(content, -1)
	s.Require().Len(matches, 2)
	for _, match := range matches {
		s.True(strings.HasPrefix(match, "ENC(v2:"), "every value uses the active key")
	}
	s.Equal("old secret", s.decryptWith(ring, matches[0]))
	s.Equal("new secret", s.decryptWith(ring, matches[1]))
}

func (s *EncryptTestSuite) TestRunFileMarkedIdempotent() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml", "password: PLAIN(s3cr3t)\n", 0o640)

	first := s.encryptFileForTest(ring, file)
	second := s.encryptFileForTest(ring, file)

	s.Equal(first, second, "a second run must not change the file")
	s.Equal(os.FileMode(0o640), s.filePerm(file))
	s.requireNoTempFiles(s.dir)
}

func (s *EncryptTestSuite) TestRunFileReportsCounts() {
	oldRing := s.writeKeyring("old.keyring", "v1")
	ring := s.writeKeyring("config.keyring", "v2", "v1")
	file := s.writeFile("config.yml",
		"a: PLAIN(one)\nb: PLAIN(two)\n"+
			"c: "+s.encryptWith(oldRing, "old")+"\n"+
			"d: "+s.encryptWith(ring, "current")+"\n", 0o600)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	out := s.captureStdout(func() {
		s.Require().NoError(c.Run(context.Background()))
	})

	s.Equal("encrypted 2, re-keyed 1, unchanged 1\n", out)
}

// TestRunFileMarkedFailureKeepsFile: the file is only rewritten when both
// passes succeed.
func (s *EncryptTestSuite) TestRunFileMarkedFailureKeepsFile() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := "a: PLAIN(s3cr3t)\nb: ENC(nope:AAAA)\n"
	file := s.writeFile("config.yml", original, 0o600)

	c := &encrypt{Base: Base{Key: ring}, File: file}
	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "unknown key id")
	s.Equal(original, s.readFile(file))
	s.requireNoTempFiles(s.dir)
}

func (s *EncryptTestSuite) TestRunMissingKeyring() {
	c := &encrypt{Base: Base{Key: filepath.Join(s.dir, "missing.keyring")}, In: "secret"}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.True(os.IsNotExist(err))
}

func (s *EncryptTestSuite) TestRunMissingFile() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &encrypt{Base: Base{Key: ring}, File: filepath.Join(s.dir, "missing.yml")}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.True(os.IsNotExist(err))
}

func (s *EncryptTestSuite) TestHelp() {
	c := &encrypt{}

	s.Contains(c.Help(), "configcrypt encrypt")
	s.NotEmpty(c.Synopsis())
}
