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
