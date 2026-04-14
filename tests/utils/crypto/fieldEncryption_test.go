package crypto_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/utils/crypto"
)

const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAA=\n-----END OPENSSH PRIVATE KEY-----"

// TestFieldEncryption_EncryptDecrypt_Roundtrip verifies that encrypting a
// value and then decrypting it returns the original plaintext, and that
// the encrypted form carries the expected "enc::v1:" prefix.
func TestFieldEncryption_EncryptDecrypt_Roundtrip(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_SECRET", "a-very-secret-key-for-testing-ok")

	encrypted, err := crypto.Encrypt(testPrivateKey)
	require.NoError(t, err, "Encrypt should not return an error when secret is set")
	assert.True(t, strings.HasPrefix(encrypted, "enc::v1:"), "Encrypted value must start with enc::v1:")

	decrypted, err := crypto.Decrypt(encrypted)
	require.NoError(t, err, "Decrypt should not return an error on a valid ciphertext")
	assert.Equal(t, testPrivateKey, decrypted, "Decrypted value must equal the original plaintext")
}

// TestFieldEncryption_Decrypt_LegacyPlaintext_ReturnsAsIs verifies that
// Decrypt is backward-compatible: values without the "enc::v1:" prefix are
// returned unchanged (no error, no transformation).
func TestFieldEncryption_Decrypt_LegacyPlaintext_ReturnsAsIs(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_SECRET", "a-very-secret-key-for-testing-ok")

	legacy := "some legacy plaintext"
	result, err := crypto.Decrypt(legacy)
	require.NoError(t, err, "Decrypt on a legacy value must not return an error")
	assert.Equal(t, legacy, result, "Decrypt must return the legacy plaintext unchanged")
}

// TestFieldEncryption_Decrypt_TamperedCiphertext_Fails verifies that AES-GCM
// authentication catches any modification of the ciphertext after encryption.
func TestFieldEncryption_Decrypt_TamperedCiphertext_Fails(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_SECRET", "a-very-secret-key-for-testing-ok")

	encrypted, err := crypto.Encrypt(testPrivateKey)
	require.NoError(t, err)

	// Tamper: flip the last character of the base64 payload.
	prefix := "enc::v1:"
	payload := encrypted[len(prefix):]
	tampered := payload[:len(payload)-1] + string(rune(payload[len(payload)-1]^1))
	tamperedCiphertext := prefix + tampered

	_, err = crypto.Decrypt(tamperedCiphertext)
	assert.Error(t, err, "Decrypt must fail when the ciphertext has been tampered with")
}

// TestFieldEncryption_Encrypt_MissingSecret_Fails verifies that Encrypt
// returns an error when the FIELD_ENCRYPTION_SECRET env var is not set.
func TestFieldEncryption_Encrypt_MissingSecret_Fails(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_SECRET", "")

	_, err := crypto.Encrypt("x")
	assert.Error(t, err, "Encrypt must fail when FIELD_ENCRYPTION_SECRET is empty")
}
