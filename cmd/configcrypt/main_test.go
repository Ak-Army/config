package main

import (
	"os"
	"testing"

	"github.com/Ak-Army/cli"
	"github.com/Ak-Army/xlog"
	"github.com/stretchr/testify/suite"
)

type MainTestSuite struct {
	suite.Suite
	origArgs []string
}

func TestMain(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) SetupTest() {
	s.origArgs = os.Args
}

func (s *MainTestSuite) TearDownTest() {
	os.Args = s.origArgs
	xlog.SetLogger(xlog.NopLogger)
}

func (s *MainTestSuite) TestInitLoggerInfoLevel() {
	os.Args = []string{"configcrypt"}
	l := initLogger()
	s.Require().NotNil(l)
}

func (s *MainTestSuite) TestInitLoggerVerboseLevel() {
	os.Args = []string{"configcrypt", "-v"}
	l := initLogger()
	s.Require().NotNil(l)
}

func (s *MainTestSuite) TestInitLoggerSetsGlobalLogger() {
	os.Args = []string{"configcrypt"}
	l := initLogger()
	s.Require().NotNil(l)
	s.Same(l, xlog.GetLogger())
}

func (s *MainTestSuite) TestSubCommandsRegistered() {
	subCommands := cli.RootCommand().SubCommands()

	s.Contains(subCommands, "create")
	s.Contains(subCommands, "update")
	s.Contains(subCommands, "encrypt")
	s.Contains(subCommands, "decrypt")
}
