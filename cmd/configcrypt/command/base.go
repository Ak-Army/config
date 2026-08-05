package command

import (
	"os"
	"path/filepath"
)

type Base struct {
	Key string `flag:"key, required, key to the keyring file"`
}

func (c *Base) writeFileAtomic(file string, out []byte) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".configcrypt-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, file); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
