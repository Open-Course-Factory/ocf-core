// tests/auth/casdoorUsername_test.go
//
// The bulk import derived a Casdoor username straight from the class list's
// names, and Casdoor refused "ma_ben-abdallah@…"'s row with a message about
// consecutive separators the teacher could do nothing about. UsernameFrom owns
// the rule; these tests pin it to what Casdoor accepts, not to sample outputs.
package auth_tests

import (
	"testing"

	"soli/formations/src/auth/casdoor"

	"github.com/stretchr/testify/assert"
)

func TestUsernameFrom_ProducesWhatCasdoorAccepts(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  string
	}{
		{"plain", []string{"Marie", "Dupont"}, "marie-dupont"},
		{"hyphen and underscore in the name", []string{"Ma", "Ben-Abdallah"}, "ma-ben-abdallah"},
		{"underscore then hyphen", []string{"ma_", "-ben"}, "ma-ben"},
		{"accents", []string{"Éloïse", "Đặng"}, "eloise-ang"},
		{"apostrophe", []string{"Jean", "D'Angelo"}, "jean-d-angelo"},
		{"spaces inside a part", []string{"Jean Marie", "DE LA FONTAINE"}, "jean-marie-de-la-fontaine"},
		{"empty first name", []string{"", "Dupont"}, "dupont"},
		{"leading and trailing junk", []string{"  -Marie-", "Dupont_ "}, "marie-dupont"},
		{"nothing usable", []string{"", "-_-"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := casdoor.UsernameFrom(tc.parts...)
			assert.Equal(t, tc.want, got)
			if got != "" {
				assert.True(t, casdoor.IsValidUsername(got), "%q must satisfy Casdoor's rule", got)
			}
		})
	}
}

func TestIsValidUsername_MirrorsCasdoorsRule(t *testing.T) {
	assert.True(t, casdoor.IsValidUsername("ma-ben-abdallah-1757"))
	assert.True(t, casdoor.IsValidUsername("A_b-c9"))
	assert.False(t, casdoor.IsValidUsername(""))
	assert.False(t, casdoor.IsValidUsername("-abc"))
	assert.False(t, casdoor.IsValidUsername("abc_"))
	assert.False(t, casdoor.IsValidUsername("a--b"))
	assert.False(t, casdoor.IsValidUsername("a_-b"))
	assert.False(t, casdoor.IsValidUsername("Éloïse"))
	assert.False(t, casdoor.IsValidUsername("jean dupont"))
}
