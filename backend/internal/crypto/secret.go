// Package crypto provides authenticated symmetric encryption for small secrets
// stored at rest (currently the Google Drive refresh token persisted by the
// in-app storage settings). It uses AES-256-GCM with a random per-message nonce.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Secretbox encrypts and decrypts strings with a fixed 256-bit key.
type Secretbox struct {
	gcm cipher.AEAD
}

// New builds a Secretbox from a base64-encoded 32-byte key. Generate one with:
//
//	head -c 32 /dev/urandom | base64
func New(base64Key string) (*Secretbox, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode secret key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes (got %d) — generate one with `head -c 32 /dev/urandom | base64`", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Secretbox{gcm: gcm}, nil
}

// Encrypt returns base64(nonce || ciphertext) for the given plaintext.
func (s *Secretbox) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It fails if the data was tampered with or encrypted
// under a different key.
func (s *Secretbox) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plain, err := s.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
