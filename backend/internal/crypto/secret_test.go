package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	s, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	const secret = "1//0gP3-refresh-token-value"
	enc, err := s.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	if enc == secret {
		t.Fatal("ciphertext equals plaintext")
	}
	got, err := s.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("round-trip mismatch: got %q want %q", got, secret)
	}
}

func TestEncryptNonDeterministic(t *testing.T) {
	s, _ := New(newKey(t))
	a, _ := s.Encrypt("x")
	b, _ := s.Encrypt("x")
	if a == b {
		t.Fatal("expected distinct ciphertexts (random nonce)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	s1, _ := New(newKey(t))
	s2, _ := New(newKey(t))
	enc, _ := s1.Encrypt("secret")
	if _, err := s2.Decrypt(enc); err == nil {
		t.Fatal("expected decrypt under wrong key to fail")
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New("not-base64!!!"); err == nil {
		t.Fatal("expected error for non-base64 key")
	}
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := New(short); err == nil {
		t.Fatal("expected error for wrong-length key")
	}
}
