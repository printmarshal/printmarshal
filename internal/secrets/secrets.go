// Package secrets encrypts credential material at rest (spec §11) with
// AES-256-GCM under a key supplied by the operator or generated once into the
// data directory.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Box encrypts and decrypts small blobs.
type Box struct{ aead cipher.AEAD }

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("secrets: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// LoadOrCreateKey returns the key from env (base64), else from keyFile,
// creating the file with a fresh random key if it does not exist.
func LoadOrCreateKey(env string, keyFile string) ([]byte, error) {
	if env != "" {
		k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env))
		if err != nil {
			return nil, fmt.Errorf("secrets: key env is not base64: %w", err)
		}
		return k, nil
	}
	b, err := os.ReadFile(keyFile)
	if err == nil {
		return decodeKey(b)
	}
	if !errors.Is(err, os.ErrNotExist) {
		// An existing but unreadable key must stop startup: generating a
		// new one here would silently orphan every stored secret (code
		// review H-015).
		return nil, fmt.Errorf("secrets: read key file %s: %w", keyFile, err)
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return nil, err
	}
	// Exclusive create: a concurrent first start that wins the race keeps its
	// key and this process reads it back instead of overwriting it.
	f, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("secrets: read key file %s: %w", keyFile, err)
		}
		return decodeKey(b)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write([]byte(base64.StdEncoding.EncodeToString(k) + "\n")); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return k, nil
}

func decodeKey(b []byte) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("secrets: key file is not base64: %w", err)
	}
	if len(k) == 0 {
		return nil, errors.New("secrets: key file is empty")
	}
	return k, nil
}

// Seal encrypts plaintext; output is nonce||ciphertext.
func (b *Box) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, b.aead.Seal(nil, nonce, plain, nil)...), nil
}

// Open decrypts a Seal output.
func (b *Box) Open(data []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("secrets: ciphertext too short")
	}
	return b.aead.Open(nil, data[:ns], data[ns:], nil)
}
