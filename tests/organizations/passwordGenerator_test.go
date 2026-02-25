package organizations_tests

import (
	"strings"
	"testing"

	orgUtils "soli/formations/src/organizations/utils"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSecurePassword_Length(t *testing.T) {
	password := orgUtils.GenerateSecurePassword(16)
	assert.Len(t, password, 16)
}

func TestGenerateSecurePassword_CustomLength(t *testing.T) {
	password := orgUtils.GenerateSecurePassword(32)
	assert.Len(t, password, 32)
}

func TestGenerateSecurePassword_MinimumLength(t *testing.T) {
	password := orgUtils.GenerateSecurePassword(2)
	assert.Len(t, password, 4, "Should enforce minimum length of 4")
}

func TestGenerateSecurePassword_CharsetCoverage(t *testing.T) {
	password := orgUtils.GenerateSecurePassword(16)

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, c := range password {
		switch {
		case strings.ContainsRune("abcdefghjkmnpqrstuvwxyz", c):
			hasLower = true
		case strings.ContainsRune("ABCDEFGHJKMNPQRSTUVWXYZ", c):
			hasUpper = true
		case strings.ContainsRune("23456789", c):
			hasDigit = true
		case strings.ContainsRune("!@#$%&*", c):
			hasSpecial = true
		}
	}

	assert.True(t, hasLower, "Password should contain at least one lowercase letter")
	assert.True(t, hasUpper, "Password should contain at least one uppercase letter")
	assert.True(t, hasDigit, "Password should contain at least one digit")
	assert.True(t, hasSpecial, "Password should contain at least one special character")
}

func TestGenerateSecurePassword_NoAmbiguousChars(t *testing.T) {
	ambiguous := "lIO01"

	// Generate many passwords to increase coverage
	for i := 0; i < 100; i++ {
		password := orgUtils.GenerateSecurePassword(16)
		for _, c := range password {
			assert.False(t, strings.ContainsRune(ambiguous, c),
				"Password should not contain ambiguous character '%c'", c)
		}
	}
}

func TestGenerateSecurePassword_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		password := orgUtils.GenerateSecurePassword(16)
		assert.False(t, seen[password], "Generated duplicate password: %s", password)
		seen[password] = true
	}
}
