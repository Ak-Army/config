package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DecryptTestSuite struct {
	commandSuite
}

func TestDecrypt(t *testing.T) {
	suite.Run(t, new(DecryptTestSuite))
}

func (s *DecryptTestSuite) TestRunString() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &decrypt{Base: Base{Key: ring}, In: s.encryptWith(ring, "secret")}

	out := s.captureStdout(func() {
		s.Require().NoError(c.Run(context.Background()))
	})

	s.Equal("secret", strings.TrimSpace(out))
}

func (s *DecryptTestSuite) TestRunStringNotEncrypted() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &decrypt{Base: Base{Key: ring}, In: "plain"}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "value is not in ENC(...) format")
}

func (s *DecryptTestSuite) TestRunFileToStdout() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := "password: " + s.encryptWith(ring, "secret") + "\nuser: admin\n"
	file := s.writeFile("config.yml", original, 0o600)

	c := &decrypt{Base: Base{Key: ring}, File: file}
	out := s.captureStdout(func() {
		s.Require().NoError(c.Run(context.Background()))
	})

	s.Contains(out, "password: secret")
	s.Contains(out, "user: admin")
	s.Equal(original, s.readFile(file), "without -write the file stays encrypted")
}

func (s *DecryptTestSuite) TestRunFileInPlace() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml",
		"password: "+s.encryptWith(ring, "secret")+"\nuser: admin\n", 0o640)

	c := &decrypt{Base: Base{Key: ring}, File: file, Write: true}
	s.Require().NoError(c.Run(context.Background()))

	s.Equal("password: secret\nuser: admin\n", s.readFile(file))
	s.Equal(os.FileMode(0o640), s.filePerm(file))
}

func (s *DecryptTestSuite) TestRunFileUnknownKeyID() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := "password: ENC(nope:AAAA)\n"
	file := s.writeFile("config.yml", original, 0o600)

	c := &decrypt{Base: Base{Key: ring}, File: file, Write: true}
	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "ENC(nope:AAAA)")
	s.Contains(err.Error(), "unknown key id")
	s.Equal(original, s.readFile(file), "a failed run must not rewrite the file")
}

func (s *DecryptTestSuite) TestRunFileReportsFirstError() {
	ring := s.writeKeyring("config.keyring", "v1")
	file := s.writeFile("config.yml", "a: ENC(nope:AAAA)\nb: ENC(other:BBBB)\n", 0o600)

	c := &decrypt{Base: Base{Key: ring}, File: file, Write: true}
	err := c.Run(context.Background())

	s.Require().Error(err)
	s.Contains(err.Error(), "ENC(nope:AAAA)")
	s.NotContains(err.Error(), "ENC(other:BBBB)")
}

func (s *DecryptTestSuite) TestRunMissingKeyring() {
	c := &decrypt{Base: Base{Key: filepath.Join(s.dir, "missing.keyring")}, In: "ENC(v1:AAAA)"}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.True(os.IsNotExist(err))
}

func (s *DecryptTestSuite) TestRunMissingFile() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &decrypt{Base: Base{Key: ring}, File: filepath.Join(s.dir, "missing.yml")}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.True(os.IsNotExist(err))
}

func (s *DecryptTestSuite) TestHelp() {
	c := &decrypt{}

	s.Contains(c.Help(), "configcrypt decrypt")
	s.NotEmpty(c.Synopsis())
}
