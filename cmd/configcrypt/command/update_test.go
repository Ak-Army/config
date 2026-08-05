package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type UpdateTestSuite struct {
	commandSuite
}

func TestUpdate(t *testing.T) {
	suite.Run(t, new(UpdateTestSuite))
}

func (s *UpdateTestSuite) TestRunAppendsKey() {
	ring := s.writeKeyring("config.keyring", "v1")
	original := s.readFile(ring)
	c := &update{Base: Base{Key: ring}, KID: "v2"}

	s.Require().NoError(c.Run(context.Background()))

	content := s.readFile(ring)
	s.True(strings.HasPrefix(content, original), "the existing entries must stay untouched")
	lines := strings.Split(strings.TrimSpace(content), "\n")
	s.Require().Len(lines, 2)
	s.True(strings.HasPrefix(lines[1], "v2: "))
}

func (s *UpdateTestSuite) TestRunKeepsActiveKey() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &update{Base: Base{Key: ring}, KID: "v2"}

	s.Require().NoError(c.Run(context.Background()))

	encoded := s.encryptWith(ring, "secret")
	s.True(strings.HasPrefix(encoded, "ENC(v1:"), "the first entry stays the active key")
	s.Equal("secret", s.decryptWith(ring, encoded))
}

func (s *UpdateTestSuite) TestRunDefaultKID() {
	ring := s.writeKeyring("config.keyring", "v1")
	c := &update{Base: Base{Key: ring}}

	s.Require().NoError(c.Run(context.Background()))

	s.Require().True(strings.HasPrefix(c.KID, "kid-"))
	_, err := time.Parse("20060102", strings.TrimPrefix(c.KID, "kid-"))
	s.Require().NoError(err)
	s.Contains(s.readFile(ring), "\n"+c.KID+": ")
}

func (s *UpdateTestSuite) TestRunMissingKeyring() {
	path := filepath.Join(s.dir, "missing.keyring")
	c := &update{Base: Base{Key: path}, KID: "v2"}

	err := c.Run(context.Background())

	s.Require().Error(err)
	s.NoFileExists(path)
}

func (s *UpdateTestSuite) TestHelp() {
	c := &update{}

	s.Contains(c.Help(), "configcrypt update")
	s.NotEmpty(c.Synopsis())
}
