// File: internal/infra/security/encryption_service.go
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// EncryptionService provides symmetric encryption for sensitive payloads.
// Implementation uses AES-GCM (AEAD) with a randomly generated nonce per message.
// API surface is unchanged: NewEncryptionService(string), Encrypt(string), Decrypt(string).
type EncryptionService struct {
	gcm cipher.AEAD
}

// NewEncryptionService constructs an AES-GCM service.
// Key must be 16, 24, or 32 bytes (AES-128/192/256). If your env var isn't one of those
// lengths, switch to a compliant length.
func NewEncryptionService(key string) (*EncryptionService, error) {
	k := []byte(key)
	n := len(k)
	if n != 16 && n != 24 && n != 32 {
		return nil, fmt.Errorf("encryption key must be 16, 24, or 32 bytes; got %d", n)
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return &EncryptionService{gcm: gcm}, nil
}

// Encrypt returns base64-encoded ciphertext. Format: base64(nonce || ciphertext)

func (e *EncryptionService) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("rand nonce: %w", err)
	}
	ct := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt accepts output of Encrypt and returns the original plaintext.

func (e *EncryptionService) Decrypt(b64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	ns := e.gcm.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	pt, err := e.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open: %w", err)
	}
	return string(pt), nil
}
