// tests/organizations/importPasswords_test.go
//
// Passwords on import. A re-import of a class with "update existing" reset the
// password of every account already there: the loop generated a password for
// every row before knowing whether the account existed, and the update wrote it
// through the password column, which Casdoor stores raw. Twelve students got a
// credentials list that could not log in.
package organizations_tests

import (
	"slices"
	"testing"

	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func columnsMention(columns [][]string, name string) bool {
	return slices.ContainsFunc(columns, func(c []string) bool { return slices.Contains(c, name) })
}

func TestCsvImport_ReimportLeavesAnExistingPasswordAlone(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	identity.users["existing-1"] = &casdoorsdk.User{Id: "existing-1", Owner: "soli", Name: "ada-lovelace-1", Email: "ada@example.com"}
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "ada@example.com,Ada,Lovelace,member\n"), nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	assert.Empty(t, identity.passwordsSet, "an existing account keeps its password when the row states none")
	assert.False(t, columnsMention(identity.columns, "password"), "the password must never travel as an update column")
	assert.Empty(t, resp.Credentials, "no credential is reported for an account whose password did not change")
	assert.Equal(t, 1, resp.Summary.UsersUpdated)
}

func TestCsvImport_ExplicitPasswordOnExistingAccountGoesThroughSetPassword(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	identity.users["existing-1"] = &casdoorsdk.User{Id: "existing-1", Owner: "soli", Name: "ada-lovelace-1", Email: "ada@example.com"}
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role,password", "ada@example.com,Ada,Lovelace,member,Str0ng-Pass!\n"), nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	assert.Equal(t, "Str0ng-Pass!", identity.passwordsSet["ada-lovelace-1"], "a stated password is applied through the hashing call")
	assert.False(t, columnsMention(identity.columns, "password"))
}

func TestCsvImport_CreatedAccountGetsAGeneratedPasswordAndACredential(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "new@example.com,Grace,Hopper,member\n"), nil, nil, false, false, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	require.Len(t, identity.created, 1)
	assert.NotEmpty(t, identity.created[0].Password, "a created account is given a password")
	assert.Equal(t, "true", identity.created[0].Properties["force_password_reset"])
	require.Len(t, resp.Credentials, 1)
	assert.Equal(t, identity.created[0].Password, resp.Credentials[0].Password, "the credential reported is the one set on the account")
	assert.Empty(t, identity.passwordsSet, "creation carries the password itself; no separate set-password call")
}

// update_existing applies to accounts of the importing organization only: an
// import must not modify an account outside the organization.
func TestCsvImport_UpdateExisting_AccountOutsideTheOrganizationIsUntouched(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("outsider")
	identity.users["outsider"].FirstName = "Original"
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role,password,force_reset", "outsider@example.com,Ada,Lovelace,member,Str0ng-Pass!,true\n"),
		nil, nil, false, true, "", true)
	require.NoError(t, err)

	assert.Empty(t, identity.passwordsSet, "no password is applied to an account outside the organization")
	assert.Empty(t, identity.columns, "no column of an account outside the organization is written")
	assert.Equal(t, "Original", identity.users["outsider"].FirstName)
	assert.False(t, identity.users["outsider"].EmailVerified)
	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "outsider"), "the account is not enrolled")
	assert.Equal(t, 0, resp.Summary.UsersUpdated)
	assert.True(t, len(resp.Errors) > 0 || resp.Summary.UsersSkipped == 1, "the row is reported, errors=%+v summary=%+v", resp.Errors, resp.Summary)
}

func TestCsvImport_UpdateExisting_ActiveMemberIsStillUpdated(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role,password", "student-a@example.com,Ada,Lovelace,member,Str0ng-Pass!\n"),
		nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	assert.Equal(t, "Str0ng-Pass!", identity.passwordsSet["user-student-a"])
	assert.Equal(t, "Ada", identity.users["student-a"].FirstName)
	assert.Len(t, identity.columns, 1)
	assert.Equal(t, 1, resp.Summary.UsersUpdated)
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "student-a"))
}
