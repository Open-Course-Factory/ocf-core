package casdoor

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Casdoor accepts a username made of letters, digits, hyphens and underscores,
// with no consecutive separators and no separator at either end. Anything else
// is refused at account creation with a message the importing teacher cannot
// act on, since the name came from their class list.
//
// UsernameFrom is the one place that rule is applied when OCF derives a
// username from a person's name: accents are stripped, letters lowered, every
// run of other characters becomes a single hyphen, and the ends are trimmed.
// It returns "" when nothing usable remains, so the caller can fall back.
func UsernameFrom(parts ...string) string {
	joined := strings.Join(parts, " ")
	ascii, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), joined)
	if err != nil {
		ascii = joined
	}

	var b strings.Builder
	pendingSeparator := false
	for _, r := range strings.ToLower(ascii) {
		isAlnum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if !isAlnum {
			pendingSeparator = b.Len() > 0
			continue
		}
		if pendingSeparator {
			b.WriteByte('-')
			pendingSeparator = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsValidUsername reports whether Casdoor would accept the username. It exists
// so tests can pin UsernameFrom to the rule rather than to a sample of outputs.
func IsValidUsername(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		isAlnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		isSep := r == '-' || r == '_'
		if !isAlnum && !isSep {
			return false
		}
		if isSep && (i == 0 || i == len(name)-1 || name[i-1] == '-' || name[i-1] == '_') {
			return false
		}
	}
	return true
}
