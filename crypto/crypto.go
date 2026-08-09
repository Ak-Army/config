// Package crypto provides the encrypted-value support for config. The Crypto
// struct wraps a key ring of Decrypter implementations (e.g. crypto/aesgcm)
// and handles the ENC(<kid>:<base64>) envelope used to mark encrypted values
// in config sources, hiding the cipher implementation from the loader.
//
// Every envelope carries a key id (kid), so values can always be decrypted
// with the right key and keys can be rotated: add the new key to the ring as
// the active one, re-encrypt the configs (see cmd/configcrypt encrypt -file),
// then drop the old key.
//
// The same command turns plaintext values of an existing config into envelopes:
// wrap them in the PLAIN(...) marker and every marked value is encrypted with
// the active key. The marker is transient — a value left marked is rejected by
// DecryptValue instead of being loaded as the literal marker string.
package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	prefix = "ENC("
	suffix = ")"
	// plainPrefix marks a value that is still plaintext and waiting to be
	// encrypted by cmd/configcrypt (encrypt -file). It is a transient state: a
	// marked value must never reach a running service, so DecryptValue rejects
	// it instead of loading the marker as the value.
	plainPrefix = "PLAIN("
)

var kidPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Decrypter decrypts a raw ciphertext produced by a matching Encrypter.
type Decrypter interface {
	Decrypt(ciphertext []byte) ([]byte, error)
}

// Encrypter encrypts a plaintext into a raw ciphertext.
type Encrypter interface {
	Encrypt(plaintext []byte) ([]byte, error)
}

// KeyParser turns the raw (base64-decoded) key material of a key-ring entry
// into a Decrypter, e.g. func(key []byte) (Decrypter, error) { return aesgcm.New(key) }.
type KeyParser func(key []byte) (Decrypter, error)

// Crypto wraps a key ring and the ENC(kid:...) envelope handling. Values are
// always encrypted with the active key; decryption picks the key named by the
// envelope's kid. A nil *Crypto is valid: DecryptValue passes plain values
// through and only errors when an actually encrypted value is encountered.
type Crypto struct {
	active    string
	keys      map[string]Decrypter
	encrypter Encrypter
}

// New builds a Crypto from a key-ring file, turning each key into a Decrypter
// with parseKey. The format is one `<kid>: <base64 key>` entry per line; blank
// lines and `#` comments are allowed, key ids must match [A-Za-z0-9._-]+, and
// the first entry is the active key (used for encryption, the others only
// decrypt):
//
//	# config.keyring
//	prod-2026-07: 4Yw3...   # active
//	prod-2026-01: 9k2f...
//
// When the active key's Decrypter also satisfies Encrypter (as crypto/aesgcm
// does), EncryptValue and RekeyValue work too.
func New(path string, parseKey KeyParser) (*Crypto, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Crypto{keys: make(map[string]Decrypter)}
	for i, line := range strings.Split(string(content), "\n") {
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := c.addEntry(line, parseKey); err != nil {
			return nil, fmt.Errorf("line %d: %s", i+1, err)
		}
	}
	if len(c.keys) == 0 {
		return nil, fmt.Errorf("no keys found in %s", path)
	}
	return c, nil
}

// addEntry adds one key-ring entry to the ring. The first key added becomes
// the active one.
func (c *Crypto) addEntry(line string, parseKey KeyParser) error {
	kid, key, err := c.parseEntry(line)
	if err != nil {
		return err
	}
	if _, ok := c.keys[kid]; ok {
		return fmt.Errorf("duplicate key id '%s'", kid)
	}
	d, err := parseKey(key)
	if err != nil {
		return fmt.Errorf("key '%s': %s", kid, err)
	}
	if d == nil {
		return fmt.Errorf("key '%s': the key parser returned no decrypter and no error", kid)
	}
	if c.active == "" {
		c.active = kid
		if e, ok := d.(Encrypter); ok {
			c.encrypter = e
		}
	}
	c.keys[kid] = d
	return nil
}

// parseEntry splits a key-ring entry into its key id and raw key material.
func (c *Crypto) parseEntry(line string) (string, []byte, error) {
	kid, val, found := strings.Cut(line, ":")
	if !found {
		return "", nil, errors.New("expected '<kid>: <base64 key>'")
	}
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return "", nil, errors.New("missing key id, expected '<kid>: <base64 key>'")
	}
	if !kidPattern.MatchString(kid) {
		return "", nil, fmt.Errorf("invalid key id '%s'", kid)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(val))
	if err != nil {
		return "", nil, fmt.Errorf("key '%s': invalid base64 key: %s", kid, err)
	}
	return kid, key, nil
}

// IsEncrypted reports whether s is wrapped in the ENC(...) envelope.
func (c *Crypto) IsEncrypted(s string) bool {
	return strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)
}

// IsPlainMarked reports whether s is a plaintext value marked for encryption
// with the PLAIN(...) marker.
func (c *Crypto) IsPlainMarked(s string) bool {
	return strings.HasPrefix(s, plainPrefix) && strings.HasSuffix(s, suffix)
}

// EncodeValue wraps a raw ciphertext into the ENC(<kid>:<base64>) envelope
// using the active key id.
func (c *Crypto) EncodeValue(ciphertext []byte) string {
	return prefix + c.active + ":" + base64.StdEncoding.EncodeToString(ciphertext) + suffix
}

// DecodeValue strips the ENC(...) envelope and returns the key id and the
// base64-decoded ciphertext.
func (c *Crypto) DecodeValue(s string) (string, []byte, error) {
	if !c.IsEncrypted(s) {
		return "", nil, errors.New("value is not in ENC(...) format")
	}
	payload := s[len(prefix) : len(s)-len(suffix)]
	idx := strings.Index(payload, ":")
	if idx < 1 {
		return "", nil, errors.New("missing key id, expected ENC(<kid>:<base64>) format")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload[idx+1:])
	if err != nil {
		return "", nil, err
	}
	return payload[:idx], ciphertext, nil
}

// DecryptValue resolves a possibly encrypted value: a plain value (no ENC(...)
// envelope) passes through unchanged, an encrypted one is decrypted with the
// key named by its kid, and one still marked PLAIN(...) is rejected. Safe to
// call on a nil *Crypto — it then only errors when the value is actually
// encrypted.
func (c *Crypto) DecryptValue(s string) (string, error) {
	if !c.IsEncrypted(s) {
		// A still marked value would otherwise load as the literal
		// "PLAIN(secret)" string — fail loudly instead.
		if c.IsPlainMarked(s) {
			return "", errors.New("value is still marked PLAIN(...), " +
				"encrypt it with 'configcrypt encrypt -key <keyring> -file <config>'")
		}
		return s, nil
	}
	if c == nil || len(c.keys) == 0 {
		return "", errors.New("encrypted value found but no decrypter configured")
	}
	kid, ciphertext, err := c.DecodeValue(s)
	if err != nil {
		return "", err
	}
	d, ok := c.keys[kid]
	if !ok {
		return "", fmt.Errorf("unknown key id '%s'", kid)
	}
	plaintext, err := d.Decrypt(ciphertext)
	if err != nil {
		return "", errors.New("decrypt: " + err.Error())
	}
	return string(plaintext), nil
}

// EncryptValue encrypts a plaintext with the active key and wraps it into
// ENC(<kid>:<base64>) form.
func (c *Crypto) EncryptValue(plaintext string) (string, error) {
	if c == nil || c.encrypter == nil {
		return "", errors.New("no encrypter configured")
	}
	ciphertext, err := c.encrypter.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return c.EncodeValue(ciphertext), nil
}

// RekeyValue re-encrypts an encrypted value with the active key. It is
// idempotent: a value already encrypted with the active key id, or a plain
// value, is returned unchanged.
func (c *Crypto) RekeyValue(s string) (string, error) {
	if !c.IsEncrypted(s) {
		return s, nil
	}
	kid, _, err := c.DecodeValue(s)
	if err != nil {
		return "", err
	}
	if c != nil && kid == c.active {
		return s, nil
	}
	plaintext, err := c.DecryptValue(s)
	if err != nil {
		return "", err
	}
	return c.EncryptValue(plaintext)
}
