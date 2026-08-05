package command

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/crypto"
)

type CreateTestSuite struct {
	commandSuite
}

func TestCreate(t *testing.T) {
	suite.Run(t, new(CreateTestSuite))
}

func (s *CreateTestSuite) TestRunNewKeyring() {
	path := filepath.Join(s.dir, "nested", "config.keyring")
	c := &create{Base: Base{Key: path}, KID: "v1"}

	s.Require().NoError(c.Run(context.Background()))

	s.Equal(os.FileMode(0o600), s.filePerm(path))
	kid, key, found := strings.Cut(strings.TrimSpace(s.readFile(path)), ": ")
	s.Require().True(found)
	s.Equal("v1", kid)
	raw, err := base64.StdEncoding.DecodeString(key)
	s.Require().NoError(err)
	s.Len(raw, 32)
}

func (s *CreateTestSuite) TestRunKeyringIsLoadable() {
	path := filepath.Join(s.dir, "config.keyring")
	c := &create{Base: Base{Key: path}, KID: "v1"}

	s.Require().NoError(c.Run(context.Background()))

	ring, err := crypto.New(path, aesKeyParser)
	s.Require().NoError(err)
	encoded, err := ring.EncryptValue("secret")
	s.Require().NoError(err)
	s.Equal("secret", s.decryptWith(path, encoded))
}

func (s *CreateTestSuite) TestRunDefaultKID() {
	path := filepath.Join(s.dir, "config.keyring")
	c := &create{Base: Base{Key: path}}

	s.Require().NoError(c.Run(context.Background()))

	s.Require().True(strings.HasPrefix(c.KID, "kid-"))
	_, err := time.Parse("20060102", strings.TrimPrefix(c.KID, "kid-"))
	s.Require().NoError(err)
	s.True(strings.HasPrefix(s.readFile(path), c.KID+": "))
}

func (s *CreateTestSuite) TestRunMkdirError() {
	blocker := s.writeFile("blocker", "not a directory", 0o600)
	c := &create{Base: Base{Key: filepath.Join(blocker, "config.keyring")}, KID: "v1"}

	err := c.Run(context.Background())

	s.Require().Error(err)
}

func (s *CreateTestSuite) TestHelp() {
	c := &create{}

	s.Contains(c.Help(), "configcrypt create")
	s.NotEmpty(c.Synopsis())
}
