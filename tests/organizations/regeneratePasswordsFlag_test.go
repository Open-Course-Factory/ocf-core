// tests/organizations/regeneratePasswordsFlag_test.go
//
// #506: RegenerateGroupMemberPasswords set force_password_reset with a bare
// casdoorsdk.UpdateUser. A bare UpdateUser applies Casdoor's default column
// whitelist and silently drops columns such as email_verified — the class of
// bug that locked 36 accounts out of billing. The flag must travel through
// UpdateUserForColumns with an explicit column list, and a write Casdoor reports
// as not persisted must be reported, not swallowed.
package organizations_tests

import (
	"testing"

	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedClassForPasswordRegeneration(t *testing.T, db *gorm.DB, identity *fakeIdentity) (orgID, classID uuid.UUID) {
	t.Helper()
	installMockEnforcer(t)
	orgID = seedTeamOrg(t, db, "owner-1", intPtr(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)
	classID = seedClassWithMember(t, db, orgID, "owner-1", "student-1")
	identity.users["student-1"] = &casdoorsdk.User{Id: "student-1", Owner: "soli", Name: "student-one", Email: "student@example.com", DisplayName: "Student One"}
	return orgID, classID
}

func TestRegeneratePasswords_SetsForcePasswordResetThroughAnExplicitColumnList(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	orgID, classID := seedClassForPasswordRegeneration(t, db, identity)

	resp, err := services.NewOrganizationServiceWithIdentity(db, identity).
		RegenerateGroupMemberPasswords(orgID, classID, "owner-1", []string{"student-1"})
	require.NoError(t, err)
	require.Empty(t, resp.Errors, "errors=%+v", resp.Errors)

	require.NotEmpty(t, identity.passwordsSet["student-one"], "the password goes through the hashing set-password call")
	assert.False(t, columnsMention(identity.columns, "password"), "the password must never travel as an update column")
	require.Len(t, identity.columns, 1, "one column-scoped update for the flag, never a bare UpdateUser")
	assert.Equal(t, []string{"properties"}, identity.columns[0])
	assert.Equal(t, "true", identity.users["student-1"].Properties["force_password_reset"])

	require.Len(t, resp.Credentials, 1)
	assert.Equal(t, identity.passwordsSet["student-one"], resp.Credentials[0].Password)
	assert.Equal(t, 1, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_ReportsAFlagWriteCasdoorDidNotPersist(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	identity.affected = false
	orgID, classID := seedClassForPasswordRegeneration(t, db, identity)

	resp, err := services.NewOrganizationServiceWithIdentity(db, identity).
		RegenerateGroupMemberPasswords(orgID, classID, "owner-1", []string{"student-1"})
	require.NoError(t, err)

	// The password did change, so the teacher must still get the credential…
	require.Len(t, resp.Credentials, 1, "the new password is real even if the flag is not")
	// …and must be told the learner will not be forced to change it.
	require.Len(t, resp.Errors, 1)
	assert.Contains(t, resp.Errors[0].Message, "force_password_reset")
	assert.Contains(t, resp.Errors[0].Message, "student-1", "the report names the affected user")
	assert.False(t, resp.Success)
}
