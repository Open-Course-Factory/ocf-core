// tests/organizations/importUsername_test.go
//
// A class list is the teacher's data, not a username form. The import must
// derive an account name Casdoor accepts from whatever the cells hold:
// "ma_ben-abdallah@…"'s row was refused in production for consecutive
// separators the teacher never typed.
package organizations_tests

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCsvImport_DerivesUsernamesCasdoorAccepts(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1", usersCSV(t,
		"ma_ben-abdallah@orsysformation.fr,Ma,Ben-Abdallah,member\n"+
			"eloise@example.com,Éloïse,D'Angelo,member\n"+
			"dupont@example.com,,DUPONT,member\n"+
			"jm@example.com,Jean Marie,DE LA FONTAINE,member\n"), nil, nil, false, false, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)
	assert.Empty(t, resp.Errors)
	assert.Equal(t, 4, resp.Summary.UsersCreated)

	require.Len(t, identity.created, 4)
	seen := map[string]bool{}
	for _, u := range identity.created {
		assert.True(t, casdoor.IsValidUsername(u.Name), "username %q for %s must satisfy Casdoor's rule", u.Name, u.Email)
		assert.False(t, seen[u.Name], "usernames must be distinct")
		seen[u.Name] = true
	}
	assert.Contains(t, identity.created[1].Name, "eloise-d-angelo-")
	assert.Equal(t, "Éloïse D'Angelo", identity.created[1].DisplayName, "the display name keeps the real spelling")
}
