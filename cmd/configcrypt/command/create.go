package command

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Ak-Army/cli"
)

func init() {
	cli.RootCommand().AddCommand("create", &create{})
}

type create struct {
	Base
	KID string `flag:"kid, key Id"`
}

func (c *create) Help() string {
	return `Usage: configcrypt create -key {path} [-kid {kid}]`
}

func (c *create) Synopsis() string {
	return "Create a new keyring file"
}

func (c *create) Run(_ context.Context) error {
	if dir := filepath.Dir(c.Key); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if c.KID == "" {
		c.KID = "kid-" + time.Now().Format("20060102")
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return err
	}

	entry := fmt.Sprintf("%s: %s\n", c.KID, base64.StdEncoding.EncodeToString(k))
	return os.WriteFile(c.Key, []byte(entry), 0o600)
}
