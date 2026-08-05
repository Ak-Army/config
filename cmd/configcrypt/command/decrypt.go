package command

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Ak-Army/cli"

	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

func init() {
	cli.RootCommand().AddCommand("decrypt", &decrypt{})
}

type decrypt struct {
	Base
	File  string `flag:"file, file to decrypt"`
	In    string `flag:"in, string to decrypt"`
	Write bool   `flag:"write, decrypt and write file inplace"`
}

func (c *decrypt) Help() string {
	return `Usage: configcrypt decrypt -key {path} `
}

func (c *decrypt) Synopsis() string {
	return "Decrypt a file or string"
}

func (c *decrypt) Run(_ context.Context) error {
	cryp, err := crypto.New(c.Key, func(key []byte) (crypto.Decrypter, error) {
		return aesgcm.New(key)
	})
	if err != nil {
		return err
	}
	if c.In != "" {
		if !cryp.IsEncrypted(c.In) {
			return errors.New("value is not in ENC(...) format")
		}
		plain, err := cryp.DecryptValue(c.In)
		if err != nil {
			return err
		}
		fmt.Println(plain)
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
		newValue, err := cryp.DecryptValue(string(match))
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
	if c.Write {
		return c.Base.writeFileAtomic(c.File, out)
	}
	fmt.Println(string(out))
	return nil
}
