package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const prefix = "enc::v1:"

// Encrypt encrypts plaintext using AES-256-GCM with a key derived from the
// FIELD_ENCRYPTION_SECRET environment variable.
//
// If plaintext is empty, Encrypt returns ("", nil) without reading the env var.
// If FIELD_ENCRYPTION_SECRET is unset or empty, Encrypt returns an error.
// The returned ciphertext is base64-encoded and prefixed with "enc::v1:".
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	secret := os.Getenv("FIELD_ENCRYPTION_SECRET")
	if secret == "" {
		return "", errors.New("FIELD_ENCRYPTION_SECRET is not set")
	}

	key := deriveKey(secret)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return prefix + encoded, nil
}

// Decrypt decrypts a value produced by Encrypt.
//
// If value is empty, Decrypt returns ("", nil).
// If value does NOT start with "enc::v1:", it is treated as legacy plaintext
// and returned unchanged (no error).
// On any AES-GCM authentication failure (tamper, wrong key), an error is returned.
func Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	if !strings.HasPrefix(value, prefix) {
		// Legacy plaintext — pass through for backward compatibility.
		return value, nil
	}

	secret := os.Getenv("FIELD_ENCRYPTION_SECRET")
	if secret == "" {
		return "", errors.New("FIELD_ENCRYPTION_SECRET is not set")
	}

	key := deriveKey(secret)

	encoded := value[len(prefix):]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// deriveKey returns a 32-byte AES-256 key derived from the given secret via SHA-256.
func deriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
