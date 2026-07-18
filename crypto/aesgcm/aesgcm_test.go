package aesgcm

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"
)

type AESGCMTestSuite struct {
	suite.Suite
}

func TestAESGCM(t *testing.T) {
	suite.Run(t, new(AESGCMTestSuite))
}

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

func (s *AESGCMTestSuite) TestRoundTrip() {
	a, err := New(testKey())
	s.Require().NoError(err)

	ciphertext, err := a.Encrypt([]byte("secret value"))
	s.Require().NoError(err)
	s.NotEqual([]byte("secret value"), ciphertext)

	plain, err := a.Decrypt(ciphertext)
	s.Require().NoError(err)
	s.Equal("secret value", string(plain))
}

func (s *AESGCMTestSuite) TestTamperedCiphertext() {
	a, err := New(testKey())
	s.Require().NoError(err)

	ciphertext, err := a.Encrypt([]byte("secret value"))
	s.Require().NoError(err)
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = a.Decrypt(ciphertext)
	s.Error(err)
}

func (s *AESGCMTestSuite) TestShortPayload() {
	a, err := New(testKey())
	s.Require().NoError(err)

	_, err = a.Decrypt([]byte("short"))
	s.Error(err)
}

func (s *AESGCMTestSuite) TestBadKeyLength() {
	_, err := New([]byte("too short"))
	s.Error(err)
}
