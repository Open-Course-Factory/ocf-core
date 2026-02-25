package utils

import (
	"crypto/rand"
	"math/big"
)

const (
	lowerChars   = "abcdefghjkmnpqrstuvwxyz"    // excludes l
	upperChars   = "ABCDEFGHJKMNPQRSTUVWXYZ"    // excludes I, O
	digitChars   = "23456789"                    // excludes 0, 1
	specialChars = "!@#$%&*"
	allChars     = lowerChars + upperChars + digitChars + specialChars
)

// GenerateSecurePassword generates a cryptographically secure password of the given length.
// The password is guaranteed to contain at least 1 uppercase, 1 lowercase, 1 digit, and 1 special character.
// Ambiguous characters (l, 1, I, 0, O) are excluded.
func GenerateSecurePassword(length int) string {
	if length < 4 {
		length = 4
	}

	password := make([]byte, length)

	// Guarantee at least one character from each required set
	password[0] = randomCharFrom(lowerChars)
	password[1] = randomCharFrom(upperChars)
	password[2] = randomCharFrom(digitChars)
	password[3] = randomCharFrom(specialChars)

	// Fill the rest with random characters from the full set
	for i := 4; i < length; i++ {
		password[i] = randomCharFrom(allChars)
	}

	// Shuffle using Fisher-Yates
	for i := length - 1; i > 0; i-- {
		j := randomInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

func randomCharFrom(charset string) byte {
	return charset[randomInt(len(charset))]
}

func randomInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return int(n.Int64())
}
