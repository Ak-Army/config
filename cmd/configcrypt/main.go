// Command configcrypt produces, inspects and re-keys the ENC(<kid>:<base64>)
// values understood by the config loader's `encrypted` tag option.
//
// Usage:
//
//	configcrypt -genkey
//	configcrypt -key <keyring> [value]                encrypt with the active key
//	configcrypt -key <keyring> -d [value]             decrypt
//	configcrypt -key <keyring> -rekey [value]         re-encrypt with the active key
//	configcrypt -key <keyring> -rekey -in <file> [-write]
//
// The key ring file holds one `<kid>: <base64 32-byte key>` entry per line,
// the first entry being the active key. Single values are taken from the
// positional argument, or from stdin when no argument is given (avoids
// leaking secrets into the shell history). -rekey -in re-encrypts every
// ENC(...) value found in the file, leaving all other bytes untouched; the
// result goes to stdout unless -write rewrites the file in place.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Ak-Army/config/crypto"
	"github.com/Ak-Army/config/crypto/aesgcm"
)

var encPattern = regexp.MustCompile(`ENC\([^)]*\)`)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("configcrypt", flag.ContinueOnError)
	key := fs.String("key", "", "path to the key-ring file (`<kid>: <base64 key>` per line, first entry is active)")
	decryptMode := fs.Bool("d", false, "decrypt an ENC(...) value instead of encrypting")
	rekeyMode := fs.Bool("rekey", false, "re-encrypt ENC(...) values with the active key")
	in := fs.String("in", "", "with -rekey: config file whose ENC(...) values are re-encrypted")
	write := fs.Bool("write", false, "with -rekey -in: rewrite the file in place instead of printing to stdout")
	genKey := fs.Bool("genkey", false, "generate a fresh base64-encoded 32-byte key and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *genKey {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			return err
		}
		fmt.Fprintln(stdout, base64.StdEncoding.EncodeToString(k))
		return nil
	}

	if *key == "" {
		return errors.New("missing -key <keyring file>")
	}
	c, err := crypto.New(*key, func(key []byte) (crypto.Decrypter, error) {
		return aesgcm.New(key)
	})
	if err != nil {
		return err
	}

	if *rekeyMode && *in != "" {
		return rekeyFile(c, *in, *write, stdout, stderr)
	}

	value, err := readValue(fs.Args(), stdin)
	if err != nil {
		return err
	}

	switch {
	case *rekeyMode:
		rekeyed, err := c.RekeyValue(value)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, rekeyed)
	case *decryptMode:
		if !c.IsEncrypted(value) {
			return errors.New("value is not in ENC(...) format")
		}
		plain, err := c.DecryptValue(value)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, plain)
	default:
		encoded, err := c.EncryptValue(value)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, encoded)
	}
	return nil
}

// rekeyFile re-encrypts every ENC(...) value in the file with the active key,
// leaving every other byte unchanged.
func rekeyFile(c *crypto.Crypto, path string, write bool, stdout, stderr io.Writer) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rekeyed, skipped := 0, 0
	var gerr error
	out := encPattern.ReplaceAllFunc(content, func(match []byte) []byte {
		if gerr != nil {
			return match
		}
		newValue, err := c.RekeyValue(string(match))
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
	fmt.Fprintf(stderr, "%s: %d value(s) re-encrypted, %d already using the active key\n", path, rekeyed, skipped)
	if write {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, out, info.Mode())
	}
	_, err = stdout.Write(out)
	return err
}

func readValue(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return "", errors.New("no value given: pass it as an argument or on stdin")
	}
	return value, nil
}
