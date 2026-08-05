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

type encrypt struct {
	Base
	File string `flag:"file, file to encrypt"`
	In   string `flag:"in, string to encrypt"`
}

func (c *encrypt) Help() string {
	return `Usage: configcrypt encrypt -key {path}`
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
	content, err := os.ReadFile(c.File)
	if err != nil {
		return err
	}
	rekeyed, skipped := 0, 0
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
			skipped++
			return match
		}
		rekeyed++
		return []byte(newValue)
	})
	if gerr != nil {
		return gerr
	}
	return c.Base.writeFileAtomic(c.File, out)
}
