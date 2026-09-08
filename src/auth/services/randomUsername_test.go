package services

import (
	"regexp"
	"testing"
)

func TestRandomUsername_HasAdjectiveNounDigitShape(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z]+_[a-z]+[0-9]$`)
	for i := 0; i < 100; i++ {
		if name := randomUsername(); !shape.MatchString(name) {
			t.Fatalf("unexpected username shape: %q", name)
		}
	}
}
