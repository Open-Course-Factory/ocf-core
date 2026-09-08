package casdoor

import (
	"regexp"
	"strings"

	"soli/formations/src/utils"
)

var (
	nonAlnum      = regexp.MustCompile("[^a-z0-9]+")
	validUsername = regexp.MustCompile(`^[A-Za-z0-9]+([-_][A-Za-z0-9]+)*$`)
)

// UsernameFrom lowercases and de-accents the parts and joins every run of
// anything else with one hyphen: "Élodie  DUPONT-Martin" → "elodie-dupont-martin".
func UsernameFrom(parts ...string) string {
	ascii := strings.ToLower(utils.RemoveAccents(strings.Join(parts, " ")))
	return strings.Trim(nonAlnum.ReplaceAllString(ascii, "-"), "-")
}

// IsValidUsername accepts what casdoor accepts: alphanumerics with single
// "-" or "_" separators, none at either end.
func IsValidUsername(name string) bool {
	return validUsername.MatchString(name)
}
