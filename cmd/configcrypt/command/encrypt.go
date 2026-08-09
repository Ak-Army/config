package command

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/Ak-Army/cli"

	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

func init() {
	cli.RootCommand().AddCommand("encrypt", &encrypt{})
}

var encPattern = regexp.MustCompile(`ENC\([^)]*\)`)

// plainPattern matches a PLAIN(<value>) marker up to the ')' that also ends the
// config value it sits in (the next character closes the value, or the line
// does). Anchoring on that context keeps a secret containing ')' intact and
// still allows several markers on one line, as in a minified JSON document.
var plainPattern = regexp.MustCompile(`(?m)PLAIN\((.*?)\)(["',\s}\]]|$)`)

type encrypt struct {
	Base
	File string `flag:"file, file to encrypt"`
	In   string `flag:"in, string to encrypt"`
}

func (c *encrypt) Help() string {
	return `Usage: configcrypt encrypt -key {path} (-in {value} | -file {path})

Encrypts a single value with the active key (-in), or rewrites a whole config
in place (-file): every PLAIN(<value>) marker becomes an encrypted envelope and
every existing ENC(...) value is re-keyed with the active key.`
}

func (c *encrypt) Synopsis() string {
	return "Encrypt a file or string"
}

func (c *encrypt) Run(_ context.Context) error {
	cryp, err := crypto.New(c.Key, func(key []byte) (crypto.Decrypter, error) {
		return aesgcm.New(key)
	})
	if err != nil {
		return err
	}
	if c.In != "" {
		encoded, err := cryp.EncryptValue(c.In)
		if err != nil {
			return err
		}
		fmt.Println(encoded)
		return nil
	}
	return c.encryptFile(cryp)
}

// encryptFile rewrites the config in place: the already encrypted values are
// re-keyed first, then the marked ones are encrypted — that order keeps the
// freshly built envelopes out of the re-key counters. The file is only written
// when both passes succeed, so a failure never leaves it half converted.
func (c *encrypt) encryptFile(cryp *crypto.Crypto) error {
	content, err := os.ReadFile(c.File)
	if err != nil {
		return err
	}
	out, rekeyed, unchanged, err := rekeyEncrypted(cryp, content)
	if err != nil {
		return err
	}
	out, encrypted, err := encryptMarked(cryp, out)
	if err != nil {
		return err
	}
	if err := c.Base.writeFileAtomic(c.File, out); err != nil {
		return err
	}
	fmt.Printf("encrypted %d, re-keyed %d, unchanged %d\n", encrypted, rekeyed, unchanged)
	return nil
}

// encryptMarked replaces every PLAIN(...) marker with an envelope encrypted
// with the active key, keeping the character that closed the marked value.
func encryptMarked(cryp *crypto.Crypto, content []byte) ([]byte, int, error) {
	encrypted := 0
	var gerr error
	out := plainPattern.ReplaceAllFunc(content, func(match []byte) []byte {
		if gerr != nil {
			return match
		}
		parts := plainPattern.FindSubmatch(match)
		encoded, err := cryp.EncryptValue(string(parts[1]))
		if err != nil {
			gerr = fmt.Errorf("%s: %s", match, err)
			return match
		}
		encrypted++
		return append([]byte(encoded), parts[2]...)
	})
	if gerr != nil {
		return nil, 0, gerr
	}
	return out, encrypted, nil
}

// rekeyEncrypted re-encrypts every ENC(...) value with the active key. Values
// already encrypted with it are left untouched and counted as unchanged.
func rekeyEncrypted(cryp *crypto.Crypto, content []byte) ([]byte, int, int, error) {
	rekeyed, unchanged := 0, 0
	var gerr error
	out := encPattern.ReplaceAllFunc(content, func(match []byte) []byte {
		if gerr != nil {
			return match
		}
		newValue, err := cryp.RekeyValue(string(match))
		if err != nil {
			gerr = fmt.Errorf("%s: %s", match, err)
			return match
		}
		if newValue == string(match) {
			unchanged++
			return match
		}
		rekeyed++
		return []byte(newValue)
	})
	if gerr != nil {
		return nil, 0, 0, gerr
	}
	return out, rekeyed, unchanged, nil
}
