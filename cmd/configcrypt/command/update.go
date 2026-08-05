package command

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/Ak-Army/cli"
)

func init() {
	cli.RootCommand().AddCommand("update", &update{})
}

type update struct {
	Base
	KID string `flag:"kid, key Id"`
}

func (c *update) Help() string {
	return `Usage: configcrypt update -key {path} [-kid {kid}]`
}

func (c *update) Synopsis() string {
	return "Update keyring file with a newly created key"
}

func (c *update) Run(_ context.Context) error {
	if c.KID == "" {
		c.KID = "kid-" + time.Now().Format("20060102")
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return err
	}

	entry := fmt.Sprintf("%s: %s\n", c.KID, base64.StdEncoding.EncodeToString(k))
	f, err := os.OpenFile(c.Key, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(entry))
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}
