package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"printmarshal/internal/secrets"
)

// Secret settings: small write-only values (a directory service-account
// password, say) kept in the settings table sealed with the same master key
// that protects device credentials. The plaintext never leaves the store
// except to the caller that asks for it, and GetSecretSetting is the only
// way back; nothing lists or exports it.

type sealedSetting struct {
	Sealed string `json:"sealed"` // base64 of the AES-GCM box; "" = cleared
}

func sealSetting(box *secrets.Box, plain string) (sealedSetting, error) {
	if box == nil {
		return sealedSetting{}, errors.New("secret settings need the encryption key")
	}
	if plain == "" {
		return sealedSetting{}, nil
	}
	enc, err := box.Seal([]byte(plain))
	if err != nil {
		return sealedSetting{}, fmt.Errorf("seal setting: %w", err)
	}
	return sealedSetting{Sealed: base64.StdEncoding.EncodeToString(enc)}, nil
}

func openSetting(box *secrets.Box, v sealedSetting) (string, error) {
	if v.Sealed == "" {
		return "", ErrNotFound
	}
	if box == nil {
		return "", errors.New("secret settings need the encryption key")
	}
	enc, err := base64.StdEncoding.DecodeString(v.Sealed)
	if err != nil {
		return "", fmt.Errorf("secret setting is not base64: %w", err)
	}
	plain, err := box.Open(enc)
	if err != nil {
		return "", fmt.Errorf("open setting: %w", err)
	}
	return string(plain), nil
}

// SetSecretSetting seals plain under key; "" clears it.
func (s *SQLite) SetSecretSetting(ctx context.Context, key, plain string) error {
	v, err := sealSetting(s.box, plain)
	if err != nil {
		return err
	}
	return s.SetSetting(ctx, key, v)
}

// GetSecretSetting returns the plaintext under key, or ErrNotFound.
func (s *SQLite) GetSecretSetting(ctx context.Context, key string) (string, error) {
	var v sealedSetting
	if err := s.GetSetting(ctx, key, &v); err != nil {
		return "", err
	}
	return openSetting(s.box, v)
}

// SetSecretSetting seals plain under key; "" clears it.
func (s *Postgres) SetSecretSetting(ctx context.Context, key, plain string) error {
	v, err := sealSetting(s.box, plain)
	if err != nil {
		return err
	}
	return s.SetSetting(ctx, key, v)
}

// GetSecretSetting returns the plaintext under key, or ErrNotFound.
func (s *Postgres) GetSecretSetting(ctx context.Context, key string) (string, error) {
	var v sealedSetting
	if err := s.GetSetting(ctx, key, &v); err != nil {
		return "", err
	}
	return openSetting(s.box, v)
}
