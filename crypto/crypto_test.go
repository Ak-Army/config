package crypto_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

type CryptoTestSuite struct {
	suite.Suite
}

func TestCrypto(t *testing.T) {
	suite.Run(t, new(CryptoTestSuite))
}

func testKey(firstByte byte) string {
	key := bytes.Repeat([]byte{0x42}, 32)
	key[0] = firstByte
	return base64.StdEncoding.EncodeToString(key)
}

func (s *CryptoTestSuite) writeRing(lines ...string) string {
	path := filepath.Join(s.T().TempDir(), "keyring")
	s.Require().NoError(os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

func (s *CryptoTestSuite) testCrypto() *crypto.Crypto {
	c, err := crypto.New(s.writeRing("v1: "+testKey(0)), aesKeyParser)
	s.Require().NoError(err)
	return c
}

func aesKeyParser(key []byte) (crypto.Decrypter, error) {
	return aesgcm.New(key)
}

func (s *CryptoTestSuite) TestEncodeDecodeValue() {
	c := s.testCrypto()
	encoded := c.EncodeValue([]byte("raw"))
	s.Equal("ENC(v1:cmF3)", encoded)
	s.True(c.IsEncrypted(encoded))
	s.False(c.IsEncrypted("plain"))
	s.False(c.IsEncrypted("ENC(missing-suffix"))
	s.False(c.IsEncrypted("missing-prefix)"))

	kid, decoded, err := c.DecodeValue(encoded)
	s.Require().NoError(err)
	s.Equal("v1", kid)
	s.Equal([]byte("raw"), decoded)

	_, _, err = c.DecodeValue("plain")
	s.Error(err)
	_, _, err = c.DecodeValue("ENC(cmF3)")
	s.Require().Error(err)
	s.Contains(err.Error(), "missing key id")
	_, _, err = c.DecodeValue("ENC(v1:not base64!!!)")
	s.Error(err)
}

func (s *CryptoTestSuite) TestEncryptDecryptValue() {
	c := s.testCrypto()

	encoded, err := c.EncryptValue("secret value")
	s.Require().NoError(err)
	s.True(c.IsEncrypted(encoded))
	kid, _, err := c.DecodeValue(encoded)
	s.Require().NoError(err)
	s.Equal("v1", kid)

	plain, err := c.DecryptValue(encoded)
	s.Require().NoError(err)
	s.Equal("secret value", plain)
}

func (s *CryptoTestSuite) TestDecryptValuePlainPassthrough() {
	c := s.testCrypto()
	plain, err := c.DecryptValue("plain value")
	s.Require().NoError(err)
	s.Equal("plain value", plain)
}

// TestDecryptValuePlainMarked: a value left marked for encryption must never
// load as the literal marker string.
func (s *CryptoTestSuite) TestDecryptValuePlainMarked() {
	c := s.testCrypto()

	_, err := c.DecryptValue("PLAIN(s3cr3t)")
	s.Require().Error(err)
	s.Contains(err.Error(), "still marked PLAIN(...)")

	var nilCrypto *crypto.Crypto
	_, err = nilCrypto.DecryptValue("PLAIN(s3cr3t)")
	s.Require().Error(err)
	s.Contains(err.Error(), "still marked PLAIN(...)")
}

func (s *CryptoTestSuite) TestIsPlainMarked() {
	c := s.testCrypto()

	s.True(c.IsPlainMarked("PLAIN(s3cr3t)"))
	s.True(c.IsPlainMarked("PLAIN()"))
	s.False(c.IsPlainMarked("PLAIN(s3cr3t"))
	s.False(c.IsPlainMarked("plain value"))
	s.False(c.IsPlainMarked("ENC(v1:cmF3)"))
}

func (s *CryptoTestSuite) TestDecryptValueUnknownKid() {
	c := s.testCrypto()
	_, err := c.DecryptValue("ENC(v2:cmF3)")
	s.Require().Error(err)
	s.Contains(err.Error(), "unknown key id 'v2'")
}

func (s *CryptoTestSuite) TestNilCrypto() {
	var c *crypto.Crypto

	plain, err := c.DecryptValue("plain value")
	s.Require().NoError(err)
	s.Equal("plain value", plain)

	_, err = c.DecryptValue("ENC(v1:cmF3)")
	s.Require().Error(err)
	s.Contains(err.Error(), "no decrypter configured")

	_, err = c.EncryptValue("value")
	s.Error(err)
}

func (s *CryptoTestSuite) TestDecryptValueBadPayload() {
	c := s.testCrypto()

	_, err := c.DecryptValue("ENC(v1:not base64!!!)")
	s.Error(err)

	// valid base64, but not a valid ciphertext for the key
	_, err = c.DecryptValue("ENC(v1:cmF3cmF3cmF3cmF3cmF3cmF3cmF3)")
	s.Require().Error(err)
	s.Contains(err.Error(), "decrypt")
}

func (s *CryptoTestSuite) TestRekeyValue() {
	oldCrypto, err := crypto.New(s.writeRing("v1: "+testKey(0)), aesKeyParser)
	s.Require().NoError(err)
	ring, err := crypto.New(s.writeRing("v2: "+testKey(1), "v1: "+testKey(0)), aesKeyParser)
	s.Require().NoError(err)

	encoded, err := oldCrypto.EncryptValue("secret value")
	s.Require().NoError(err)

	rekeyed, err := ring.RekeyValue(encoded)
	s.Require().NoError(err)
	s.NotEqual(encoded, rekeyed)
	kid, _, err := ring.DecodeValue(rekeyed)
	s.Require().NoError(err)
	s.Equal("v2", kid)

	plain, err := ring.DecryptValue(rekeyed)
	s.Require().NoError(err)
	s.Equal("secret value", plain)

	// idempotent: already on the active key
	again, err := ring.RekeyValue(rekeyed)
	s.Require().NoError(err)
	s.Equal(rekeyed, again)

	// plain values pass through unchanged
	plainThrough, err := ring.RekeyValue("plain value")
	s.Require().NoError(err)
	s.Equal("plain value", plainThrough)

	// unknown kid fails
	_, err = ring.RekeyValue("ENC(v9:cmF3)")
	s.Error(err)
}

func (s *CryptoTestSuite) TestNew() {
	path := s.writeRing(
		"# comment",
		"",
		"prod-2026-07: "+testKey(0)+"  # active",
		"prod-2026-01: "+testKey(1))

	c, err := crypto.New(path, aesKeyParser)
	s.Require().NoError(err)

	// the first entry is the active key: encryption stamps its kid
	encoded, err := c.EncryptValue("secret value")
	s.Require().NoError(err)
	kid, _, err := c.DecodeValue(encoded)
	s.Require().NoError(err)
	s.Equal("prod-2026-07", kid)

	// the second key decrypts too
	old, err := crypto.New(s.writeRing("prod-2026-01: "+testKey(1)), aesKeyParser)
	s.Require().NoError(err)
	oldValue, err := old.EncryptValue("old secret")
	s.Require().NoError(err)
	plain, err := c.DecryptValue(oldValue)
	s.Require().NoError(err)
	s.Equal("old secret", plain)
}

func (s *CryptoTestSuite) TestNewErrors() {
	key := testKey(0)

	_, err := crypto.New(filepath.Join(s.T().TempDir(), "missing"), aesKeyParser)
	s.Error(err)

	_, err = crypto.New(s.writeRing("# only comments"), aesKeyParser)
	s.Require().Error(err)
	s.Contains(err.Error(), "no keys found")

	_, err = crypto.New(s.writeRing("no-colon-line"), aesKeyParser)
	s.Error(err)

	_, err = crypto.New(s.writeRing(": "+key), aesKeyParser)
	s.Require().Error(err)
	s.Contains(err.Error(), "missing key id")

	_, err = crypto.New(s.writeRing("v1: "+key, "v1: "+key), aesKeyParser)
	s.Require().Error(err)
	s.Contains(err.Error(), "duplicate key id")

	_, err = crypto.New(s.writeRing("v1: not-base64!!!"), aesKeyParser)
	s.Error(err)

	_, err = crypto.New(s.writeRing("bad kid: "+key), aesKeyParser)
	s.Require().Error(err)
	s.Contains(err.Error(), "invalid key id")

	// the key parser decides what a valid key is
	_, err = crypto.New(s.writeRing("v1: cmF3"), aesKeyParser)
	s.Require().Error(err)
	s.Contains(err.Error(), "32-byte key")

	// a parser returning a nil Decrypter is rejected
	_, err = crypto.New(s.writeRing("v1: "+key), func([]byte) (crypto.Decrypter, error) {
		return nil, nil
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "no decrypter and no error")
}

type decryptOnly struct{}

func (decryptOnly) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

func (s *CryptoTestSuite) TestDecryptOnlyImplementation() {
	c, err := crypto.New(s.writeRing("v1: "+testKey(0)), func([]byte) (crypto.Decrypter, error) {
		return decryptOnly{}, nil
	})
	s.Require().NoError(err)

	plain, err := c.DecryptValue("ENC(v1:cmF3)")
	s.Require().NoError(err)
	s.Equal("raw", plain)

	_, err = c.EncryptValue("value")
	s.Require().Error(err)
	s.Contains(err.Error(), "no encrypter configured")
}
