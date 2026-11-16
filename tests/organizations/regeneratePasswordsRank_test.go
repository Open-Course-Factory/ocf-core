// tests/organizations/regeneratePasswordsRank_test.go
//
// Password regeneration follows the same rank rule as role changes: a caller
// must not reset the password of an account that outranks them in the
// organization, nor of a platform administrator, nor of an account outside the
// organization. Equal rank is allowed, as it is for role changes.
package organizations_tests

import (
	"slices"
	"testing"
	"time"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRankedClass builds an organization owned by owner-1 with two managers, a
// member, a member who is a platform administrator, and an account that is in
// the class but holds no membership in the organization. Everyone is an active
// member of the class and known to the identity provider.
func seedRankedClass(t *testing.T, db *gorm.DB) (orgID, classID uuid.UUID, identity *fakeIdentity) {
	t.Helper()
	enforcer := installMockEnforcer(t)
	enforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		if name == "admin-1" {
			return []string{"member", "administrator"}, nil
		}
		return []string{"member"}, nil
	}

	orgID = seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "manager-1", models.OrgRoleManager)
	seedMember(t, db, orgID, "manager-2", models.OrgRoleManager)
	seedMember(t, db, orgID, "student-1", models.OrgRoleMember)
	seedMember(t, db, orgID, "admin-1", models.OrgRoleMember)

	classID = seedClassWithMember(t, db, orgID, "manager-1", "student-1")
	for _, id := range []string{"owner-1", "manager-1", "manager-2", "admin-1", "outsider-1"} {
		gm := &groupModels.GroupMember{GroupID: classID, UserID: id, Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: true}
		require.NoError(t, db.Omit("Metadata").Create(gm).Error)
	}

	identity = newFakeIdentity("owner-1", "manager-1", "manager-2", "student-1", "admin-1", "outsider-1")
	for _, u := range identity.users {
		u.Owner = "soli"
	}
	return orgID, classID, identity
}

func regenerate(t *testing.T, db *gorm.DB, identity *fakeIdentity, orgID, classID uuid.UUID, caller string, targets ...string) *dto.RegeneratePasswordsResponse {
	t.Helper()
	resp, err := services.NewOrganizationServiceWithIdentity(db, identity).
		RegenerateGroupMemberPasswords(orgID, classID, caller, targets)
	require.NoError(t, err)
	return resp
}

func assertRegenerated(t *testing.T, identity *fakeIdentity, resp *dto.RegeneratePasswordsResponse, userID string) {
	t.Helper()
	assert.NotEmpty(t, identity.passwordsSet["user-"+userID], "%s's password is regenerated", userID)
	assert.True(t, slices.ContainsFunc(resp.Credentials, func(c dto.UserCredential) bool { return c.Email == userID+"@example.com" }),
		"a credential is returned for %s", userID)
}

// assertRefused checks the target's password is untouched, no credential is
// returned for them, and the refusal is reported on their row.
func assertRefused(t *testing.T, identity *fakeIdentity, resp *dto.RegeneratePasswordsResponse, userID string, row int) {
	t.Helper()
	assert.Empty(t, identity.passwordsSet["user-"+userID], "%s's password must not change", userID)
	assert.False(t, slices.ContainsFunc(resp.Credentials, func(c dto.UserCredential) bool { return c.Email == userID+"@example.com" }),
		"no credential may be returned for %s", userID)
	assert.True(t, slices.ContainsFunc(resp.Errors, func(e dto.ImportError) bool { return e.Row == row && e.Code == dto.ErrCodeNotFound }),
		"the refusal of %s is reported on row %d, errors=%+v", userID, row, resp.Errors)
	assert.False(t, resp.Success)
}

func TestRegeneratePasswords_ManagerTargetsOrganizationOwner_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "owner-1")

	assertRefused(t, identity, resp, "owner-1", 1)
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_ManagerTargetsPlatformAdministrator_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "admin-1")

	assertRefused(t, identity, resp, "admin-1", 1)
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_TargetOutsideOrganization_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "outsider-1")

	assertRefused(t, identity, resp, "outsider-1", 1)
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_ManagerTargetsMember_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "student-1")

	assertRegenerated(t, identity, resp, "student-1")
	assert.Empty(t, resp.Errors)
}

func TestRegeneratePasswords_ManagerTargetsEqualRankManager_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "manager-2")

	assertRegenerated(t, identity, resp, "manager-2")
	assert.Empty(t, resp.Errors)
}

func TestRegeneratePasswords_OwnerTargetsManager_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "owner-1", "manager-1")

	assertRegenerated(t, identity, resp, "manager-1")
	assert.Empty(t, resp.Errors)
}

func TestRegeneratePasswords_MixedRequest_RegeneratesAllowedAndReportsRefused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "student-1", "owner-1")

	assertRegenerated(t, identity, resp, "student-1")
	assertRefused(t, identity, resp, "owner-1", 2)
	assert.Equal(t, 1, resp.Summary.Succeeded)
	assert.Equal(t, 1, resp.Summary.Failed)
}
