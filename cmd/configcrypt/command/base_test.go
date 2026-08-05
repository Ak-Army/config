package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

// commandSuite holds the setup shared by every command test: an isolated
// working directory plus helpers to build key rings and capture stdout.
type commandSuite struct {
	suite.Suite
	dir string
}

func (s *commandSuite) SetupTest() {
	s.dir = s.T().TempDir()
}

// writeKeyring writes a key ring file with one entry per key id, the first one
// becoming the active key.
func (s *commandSuite) writeKeyring(name string, kids ...string) string {
	s.T().Helper()
	lines := make([]string, 0, len(kids))
	for _, kid := range kids {
		lines = append(lines, kid+": "+testKeyMaterial(kid))
	}
	return s.writeFile(name, strings.Join(lines, "\n")+"\n", 0o600)
}

func (s *commandSuite) writeFile(name, content string, mode os.FileMode) string {
	s.T().Helper()
	path := filepath.Join(s.dir, name)
	s.Require().NoError(os.WriteFile(path, []byte(content), mode))
	return path
}

func (s *commandSuite) readFile(path string) string {
	s.T().Helper()
	content, err := os.ReadFile(path)
	s.Require().NoError(err)
	return string(content)
}

func (s *commandSuite) filePerm(path string) os.FileMode {
	s.T().Helper()
	info, err := os.Stat(path)
	s.Require().NoError(err)
	return info.Mode().Perm()
}

// captureStdout collects everything fn prints to os.Stdout.
func (s *commandSuite) captureStdout(fn func()) string {
	s.T().Helper()
	r, w, err := os.Pipe()
	s.Require().NoError(err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	collected := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		collected <- buf.String()
	}()

	fn()
	s.Require().NoError(w.Close())
	out := <-collected
	s.Require().NoError(r.Close())

	return out
}

// encryptWith encrypts plaintext with the active key of the given key ring.
func (s *commandSuite) encryptWith(keyring, plaintext string) string {
	s.T().Helper()
	c, err := crypto.New(keyring, aesKeyParser)
	s.Require().NoError(err)
	encoded, err := c.EncryptValue(plaintext)
	s.Require().NoError(err)
	return encoded
}

func (s *commandSuite) decryptWith(keyring, value string) string {
	s.T().Helper()
	c, err := crypto.New(keyring, aesKeyParser)
	s.Require().NoError(err)
	plain, err := c.DecryptValue(value)
	s.Require().NoError(err)
	return plain
}

// requireNoTempFiles asserts that writeFileAtomic left no half-written file behind.
func (s *commandSuite) requireNoTempFiles(dir string) {
	s.T().Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".configcrypt-*"))
	s.Require().NoError(err)
	s.Empty(matches)
}

// testKeyMaterial derives a deterministic 32 byte key from the key id, so the
// same kid yields the same key in every key ring file of a test.
func testKeyMaterial(kid string) string {
	sum := sha256.Sum256([]byte(kid))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func aesKeyParser(key []byte) (crypto.Decrypter, error) {
	return aesgcm.New(key)
}

type BaseTestSuite struct {
	commandSuite
}

func TestBase(t *testing.T) {
	suite.Run(t, new(BaseTestSuite))
}

func (s *BaseTestSuite) TestWriteFileAtomicSuccess() {
	path := s.writeFile("config.yml", "old content", 0o640)

	c := &Base{}
	s.Require().NoError(c.writeFileAtomic(path, []byte("new content")))

	s.Equal("new content", s.readFile(path))
	s.Equal(os.FileMode(0o640), s.filePerm(path))
}

func (s *BaseTestSuite) TestWriteFileAtomicKeepsNoTempFile() {
	path := s.writeFile("config.yml", "old content", 0o600)

	c := &Base{}
	s.Require().NoError(c.writeFileAtomic(path, []byte("new content")))

	s.requireNoTempFiles(s.dir)
}

func (s *BaseTestSuite) TestWriteFileAtomicMissingFile() {
	path := filepath.Join(s.dir, "missing.yml")

	c := &Base{}
	err := c.writeFileAtomic(path, []byte("new content"))

	s.Require().Error(err)
	s.True(os.IsNotExist(err))
	s.NoFileExists(path)
}

func (s *BaseTestSuite) TestWriteFileAtomicReadOnlyDir() {
	if os.Geteuid() == 0 {
		s.T().Skip("root ignores directory permissions")
	}
	dir := filepath.Join(s.dir, "readonly")
	s.Require().NoError(os.Mkdir(dir, 0o700))
	path := filepath.Join(dir, "config.yml")
	s.Require().NoError(os.WriteFile(path, []byte("old content"), 0o600))
	s.Require().NoError(os.Chmod(dir, 0o500))
	defer func() { s.Require().NoError(os.Chmod(dir, 0o700)) }()

	c := &Base{}
	err := c.writeFileAtomic(path, []byte("new content"))

	s.Require().Error(err)
	s.Equal("old content", s.readFile(path))
}

func (s *BaseTestSuite) TestWriteFileAtomicRenameError() {
	target := filepath.Join(s.dir, "a-directory")
	s.Require().NoError(os.Mkdir(target, 0o755))

	c := &Base{}
	err := c.writeFileAtomic(target, []byte("new content"))

	s.Require().Error(err)
	s.requireNoTempFiles(s.dir)
	s.DirExists(target)
}
