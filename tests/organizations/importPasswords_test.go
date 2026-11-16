// tests/organizations/importPasswords_test.go
//
// Passwords on import. A re-import of a class with "update existing" reset the
// password of every account already there: the loop generated a password for
// every row before knowing whether the account existed, and the update wrote it
// through the password column, which Casdoor stores raw. Twelve students got a
// credentials list that could not log in.
package organizations_tests

import (
	"bytes"
	"errors"
	"mime/multipart"
	"slices"
	"testing"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
	seedMember(t, db, orgID, "existing-1", models.OrgRoleMember)

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
	seedMember(t, db, orgID, "existing-1", models.OrgRoleMember)

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
	assert.NotEmpty(t, resp.Errors, "the row is reported")
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

func TestCsvImport_UpdateExisting_MemberOfAnotherOrganizationIsUntouched(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("elsewhere")
	identity.users["elsewhere"].FirstName = "Original"
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	otherOrgID := seedTeamOrg(t, db, "owner-2", new(10))
	seedMember(t, db, otherOrgID, "elsewhere", models.OrgRoleMember)

	importer := services.NewImportService(db, identity, offboarding)
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role", "elsewhere@example.com,Ada,Lovelace,member\n"),
		nil, nil, false, true, "", true)
	require.NoError(t, err)

	assert.Empty(t, identity.columns)
	assert.Equal(t, "Original", identity.users["elsewhere"].FirstName)
	assert.False(t, identity.users["elsewhere"].EmailVerified)
	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "elsewhere"), "the account is not enrolled")
	assert.Equal(t, int64(1), countMemberRows(t, db, otherOrgID, "elsewhere"))
	assert.Equal(t, 0, resp.Summary.UsersUpdated)
	assert.NotEmpty(t, resp.Errors)
}

func TestCsvImport_UpdateExisting_DryRunWritesNothing(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("outsider")
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))

	importer := services.NewImportService(db, identity, offboarding)
	_, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSVWithColumns(t, "email,first_name,last_name,role,password", "outsider@example.com,Ada,Lovelace,member,Str0ng-Pass!\n"),
		nil, nil, true, true, "", true)
	require.NoError(t, err)

	assert.Empty(t, identity.passwordsSet)
	assert.Empty(t, identity.columns)
	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "outsider"))
}

// importAsUpdate runs an update_existing import of one row (new name and a
// password) for the account target, with importer as the calling user. An empty
// importerRole seeds no membership row for the importer; platformRoles, when
// set, answers the enforcer's platform-role lookup.
func importAsUpdate(t *testing.T, importer string, importerRole models.OrganizationMemberRole, target string, targetRole models.OrganizationMemberRole, platformRoles ...func(string) ([]string, error)) (*fakeIdentity, int, int) {
	t.Helper()
	enforcer := installMockEnforcer(t)
	if len(platformRoles) > 0 {
		enforcer.GetRolesForUserFunc = platformRoles[0]
	}
	db := newOffboardingDB(t)
	identity := newFakeIdentity(importer, target)
	identity.users[target].FirstName = "Original"
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	for user, role := range map[string]models.OrganizationMemberRole{importer: importerRole, target: targetRole} {
		if user != "owner-1" && role != "" {
			seedMember(t, db, orgID, user, role)
		}
	}

	resp, err := services.NewImportService(db, identity, offboarding).ImportOrganizationData(orgID, importer,
		usersCSVWithColumns(t, "email,first_name,last_name,role,password", target+"@example.com,Ada,Lovelace,member,Str0ng-Pass!\n"),
		nil, nil, false, true, "", true)
	require.NoError(t, err)
	return identity, resp.Summary.UsersUpdated, len(resp.Errors)
}

func TestCsvImport_UpdateExisting_MemberOutrankingTheImporterIsUntouched(t *testing.T) {
	identity, updated, errs := importAsUpdate(t, "manager-1", models.OrgRoleManager, "owner-1", models.OrgRoleOwner)

	assert.Empty(t, identity.passwordsSet, "no password is applied to a member who outranks the importer")
	assert.Empty(t, identity.columns, "no column of a member who outranks the importer is written")
	assert.Equal(t, "Original", identity.users["owner-1"].FirstName)
	assert.Equal(t, 0, updated)
	assert.Equal(t, 1, errs, "the row is reported as refused")
}

func TestCsvImport_UpdateExisting_MemberAtOrBelowTheImporterRankIsUpdated(t *testing.T) {
	cases := []struct {
		name         string
		importer     string
		importerRole models.OrganizationMemberRole
		target       string
		targetRole   models.OrganizationMemberRole
	}{
		{"manager updates a member", "manager-1", models.OrgRoleManager, "student-a", models.OrgRoleMember},
		{"manager updates another manager", "manager-1", models.OrgRoleManager, "manager-2", models.OrgRoleManager},
		{"owner updates a manager", "owner-1", models.OrgRoleOwner, "manager-2", models.OrgRoleManager},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			identity, updated, errs := importAsUpdate(t, c.importer, c.importerRole, c.target, c.targetRole)

			assert.Equal(t, "Str0ng-Pass!", identity.passwordsSet["user-"+c.target])
			assert.Equal(t, "Ada", identity.users[c.target].FirstName)
			assert.Equal(t, 1, updated)
			assert.Equal(t, 0, errs)
		})
	}
}

func adminRoleFor(userID string) func(string) ([]string, error) {
	return func(name string) ([]string, error) {
		if name == userID {
			return []string{"administrator"}, nil
		}
		return []string{"member"}, nil
	}
}

func TestCsvImport_UpdateExisting_PlatformAdministratorWithoutMembershipUpdatesAnyMember(t *testing.T) {
	for _, target := range []struct {
		id   string
		role models.OrganizationMemberRole
	}{{"manager-2", models.OrgRoleManager}, {"owner-1", models.OrgRoleOwner}} {
		t.Run(target.id, func(t *testing.T) {
			identity, updated, errs := importAsUpdate(t, "admin-1", "", target.id, target.role, adminRoleFor("admin-1"))

			assert.Equal(t, "Str0ng-Pass!", identity.passwordsSet["user-"+target.id])
			assert.Equal(t, "Ada", identity.users[target.id].FirstName)
			assert.Equal(t, 1, updated)
			assert.Equal(t, 0, errs)
		})
	}
}

func TestCsvImport_UpdateExisting_ImporterWithoutMembershipOrAdministratorRoleUpdatesNoMember(t *testing.T) {
	lookupFails := func(string) ([]string, error) { return nil, errors.New("enforcer unavailable") }
	for _, c := range []struct {
		name          string
		platformRoles func(string) ([]string, error)
	}{{"plain member role", adminRoleFor("someone-else")}, {"role lookup fails", lookupFails}} {
		for _, target := range []struct {
			id   string
			role models.OrganizationMemberRole
		}{{"student-a", models.OrgRoleMember}, {"manager-2", models.OrgRoleManager}, {"owner-1", models.OrgRoleOwner}} {
			t.Run(c.name+"/"+target.id, func(t *testing.T) {
				identity, updated, errs := importAsUpdate(t, "outsider-1", "", target.id, target.role, c.platformRoles)

				assert.Empty(t, identity.passwordsSet)
				assert.Empty(t, identity.columns)
				assert.Equal(t, "Original", identity.users[target.id].FirstName)
				assert.Equal(t, 0, updated)
				assert.Equal(t, 1, errs)
			})
		}
	}
}

// The route admits managers and above only; at service level a member-rank
// importer is still bound by the same rule: equal rank applies, higher is refused.
func TestCsvImport_UpdateExisting_MemberRankImporterIsCappedAtMember(t *testing.T) {
	identity, updated, errs := importAsUpdate(t, "student-b", models.OrgRoleMember, "student-a", models.OrgRoleMember)
	assert.Equal(t, "Ada", identity.users["student-a"].FirstName)
	assert.Equal(t, 1, updated)
	assert.Equal(t, 0, errs)

	identity, updated, errs = importAsUpdate(t, "student-b", models.OrgRoleMember, "manager-2", models.OrgRoleManager)
	assert.Empty(t, identity.columns)
	assert.Equal(t, 0, updated)
	assert.Equal(t, 1, errs)
}

func TestCsvImport_UpdateExisting_NewAccountsAreCreatedWhateverTheImporterRank(t *testing.T) {
	for _, c := range []struct {
		importer      string
		importerRole  models.OrganizationMemberRole
		platformRoles func(string) ([]string, error)
	}{{"manager-1", models.OrgRoleManager, adminRoleFor("nobody")}, {"admin-1", "", adminRoleFor("admin-1")}} {
		t.Run(c.importer, func(t *testing.T) {
			enforcer := installMockEnforcer(t)
			enforcer.GetRolesForUserFunc = c.platformRoles
			db := newOffboardingDB(t)
			identity := newFakeIdentity(c.importer)
			orgID := seedTeamOrg(t, db, "owner-1", new(10))
			if c.importerRole != "" {
				seedMember(t, db, orgID, c.importer, c.importerRole)
			}

			resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity)).ImportOrganizationData(orgID, c.importer,
				usersCSV(t, "new@example.com,Grace,Hopper,member\n"), nil, nil, false, true, "", true)
			require.NoError(t, err, "errors=%+v", resp.Errors)

			require.Len(t, identity.created, 1)
			assert.Equal(t, "new@example.com", identity.created[0].Email)
			assert.Equal(t, int64(1), countMemberRows(t, db, orgID, identity.created[0].Id))
			assert.Empty(t, resp.Errors)
		})
	}
}

// platformRoles answers the platform-role lookup: administrator for the ids in
// admins, an error for failing, member for everyone else.
func platformRoles(failing string, admins ...string) func(string) ([]string, error) {
	return func(name string) ([]string, error) {
		if name == failing {
			return nil, errors.New("enforcer unavailable")
		}
		if slices.Contains(admins, name) {
			return []string{"administrator"}, nil
		}
		return []string{"member"}, nil
	}
}

func TestCsvImport_UpdateExisting_PlatformAdministratorMemberIsUntouched(t *testing.T) {
	for _, c := range []struct {
		name          string
		importer      string
		importerRole  models.OrganizationMemberRole
		platformRoles func(string) ([]string, error)
	}{
		{"owner importer", "owner-1", models.OrgRoleOwner, platformRoles("", "admin-2")},
		{"manager importer", "manager-1", models.OrgRoleManager, platformRoles("", "admin-2")},
		{"owner importer, target role lookup fails", "owner-1", models.OrgRoleOwner, platformRoles("admin-2")},
		{"manager importer, target role lookup fails", "manager-1", models.OrgRoleManager, platformRoles("admin-2")},
	} {
		t.Run(c.name, func(t *testing.T) {
			identity, updated, errs := importAsUpdate(t, c.importer, c.importerRole, "admin-2", models.OrgRoleMember, c.platformRoles)

			assert.Empty(t, identity.passwordsSet, "no password is applied to a platform administrator")
			assert.Empty(t, identity.columns, "no column of a platform administrator is written")
			assert.Equal(t, "Original", identity.users["admin-2"].FirstName)
			assert.Equal(t, 0, updated)
			assert.Equal(t, 1, errs, "the row is reported as refused")
		})
	}
}

func TestCsvImport_UpdateExisting_PlatformAdministratorImporterUpdatesAnAdministratorMember(t *testing.T) {
	identity, updated, errs := importAsUpdate(t, "admin-1", "", "admin-2", models.OrgRoleMember, platformRoles("", "admin-1", "admin-2"))

	assert.Equal(t, "Str0ng-Pass!", identity.passwordsSet["user-admin-2"])
	assert.Equal(t, "Ada", identity.users["admin-2"].FirstName)
	assert.Equal(t, 1, updated)
	assert.Equal(t, 0, errs)
}

func csvFile(t *testing.T, field, content string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile(field, field+".csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.File[field][0]
}

func groupMemberRows(t *testing.T, db *gorm.DB, userID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&groupModels.GroupMember{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

// A refused row must not reach a group through a memberships.csv row naming
// it for a group created in the same import. student-a, a member, is the
// control that the membership path runs.
func TestCsvImport_UpdateExisting_RefusedAccountJoinsNoGroup(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	// SQLite cannot bind the jsonb Metadata map that a created class carries.
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:omit_metadata", func(tx *gorm.DB) {
		tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
	}))
	identity := newFakeIdentity("outsider", "student-a")
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity)).ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "outsider@example.com,Ada,Lovelace,member\nstudent-a@example.com,Grace,Hopper,member\n"),
		csvFile(t, "groups", "group_name,display_name\nclass-b,Class B\n"),
		csvFile(t, "memberships", "user_email,group_name,role\noutsider@example.com,class-b,member\nstudent-a@example.com,class-b,member\n"),
		false, true, "", true)
	require.NoError(t, err)

	assert.Equal(t, int64(0), groupMemberRows(t, db, "outsider"), "the refused account joins no group")
	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "outsider"), "the refused account is not enrolled")
	assert.Equal(t, int64(1), groupMemberRows(t, db, "student-a"), "the member's row still reaches the group, errors=%+v", resp.Errors)
}
