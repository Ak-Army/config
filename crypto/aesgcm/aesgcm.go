// Package aesgcm provides an AES-256-GCM cipher implementing the
// crypto.Encrypter and crypto.Decrypter interfaces.
package aesgcm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// AESGCM implements crypto.Encrypter and crypto.Decrypter using AES-256-GCM.
// The raw ciphertext layout is nonce || GCM ciphertext+tag.
type AESGCM struct {
	aead cipher.AEAD
}

// New creates an AESGCM from a raw 32-byte key.
func New(key []byte) (*AESGCM, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM requires a 32-byte key, got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: aead}, nil
}

// Encrypt encrypts the plaintext with a fresh random nonce and returns
// nonce || ciphertext+tag.
func (a *AESGCM) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt splits the nonce prefix off data and decrypts the rest.
func (a *AESGCM) Decrypt(data []byte) ([]byte, error) {
	if len(data) < a.aead.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short: %d bytes", len(data))
	}
	nonce, ciphertext := data[:a.aead.NonceSize()], data[a.aead.NonceSize():]
	return a.aead.Open(nil, nonce, ciphertext, nil)
}
