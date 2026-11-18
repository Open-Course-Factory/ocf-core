// tests/organizations/regeneratePasswordsRank_test.go
//
// Password regeneration is limited to accounts the organization alone answers
// for: an owner or a platform administrator may reset any active member of this
// organization; any other caller only a plain member. Never a platform
// administrator (unless the caller is one), an account outside the organization,
// an offboarded member, or an account that also belongs elsewhere (another team
// organization, an organization it owns, a paid personal subscription).
package organizations_tests

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	authServices "soli/formations/src/auth/services"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	paymentModels "soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRankedClass builds an organization owned by owner-1 with two managers, a
// member, a member who is a platform administrator, and an account that is in
// the class but holds no membership in the organization. Everyone is an active
// member of the class and known to the identity provider. The organization
// created every member's account.
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
	for _, id := range []string{"manager-1", "manager-2", "student-1", "admin-1"} {
		markCreatedByOrganization(t, db, orgID, id)
	}

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
	resp, err := services.NewOrganizationServiceWithIdentity(db, identity, newDeletionService(db, identity)).
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
// returned for them, and the refusal is reported on their row with a message
// containing reason: every refusal shares one error code.
func assertRefused(t *testing.T, identity *fakeIdentity, resp *dto.RegeneratePasswordsResponse, userID string, row int, reason string) {
	t.Helper()
	assert.Empty(t, identity.passwordsSet["user-"+userID], "%s's password must not change", userID)
	assert.False(t, slices.ContainsFunc(resp.Credentials, func(c dto.UserCredential) bool { return c.Email == userID+"@example.com" }),
		"no credential may be returned for %s", userID)
	assert.True(t, slices.ContainsFunc(resp.Errors, func(e dto.ImportError) bool {
		return e.Row == row && e.Code == dto.ErrCodeNotFound && strings.Contains(e.Message, reason)
	}), "the refusal of %s is reported on row %d with %q, errors=%+v", userID, row, reason, resp.Errors)
	assert.False(t, resp.Success)
}

func TestRegeneratePasswords_ManagerTargetsOrganizationOwner_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "owner-1")

	assertRefused(t, identity, resp, "owner-1", 1, "outranks")
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_ManagerTargetsPlatformAdministrator_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "admin-1")

	assertRefused(t, identity, resp, "admin-1", 1, "outranks")
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_TargetOutsideOrganization_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "outsider-1")

	assertRefused(t, identity, resp, "outsider-1", 1, "not a member")
	assert.Equal(t, 0, resp.Summary.Succeeded)
}

func TestRegeneratePasswords_ManagerTargetsMember_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "student-1")

	assertRegenerated(t, identity, resp, "student-1")
	assert.Empty(t, resp.Errors)
}

func TestRegeneratePasswords_ManagerTargetsAnotherManager_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "manager-2")

	assertRefused(t, identity, resp, "manager-2", 1, "outranks")
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
	assertRefused(t, identity, resp, "owner-1", 2, "outranks")
	assert.Equal(t, 1, resp.Summary.Succeeded)
	assert.Equal(t, 1, resp.Summary.Failed)
}

// failRolesLookupFor makes the platform-role lookup fail for the given user,
// keeping admin-1 an administrator.
func failRolesLookupFor(userID string) {
	casdoor.Enforcer.(*mocks.MockEnforcer).GetRolesForUserFunc = func(name string) ([]string, error) {
		switch name {
		case userID:
			return nil, errors.New("role store unavailable")
		case "admin-1":
			return []string{"member", "administrator"}, nil
		}
		return []string{"member"}, nil
	}
}

// A manager is not a plain member, so the rule covers their own account too.
func TestRegeneratePasswords_ManagerTargetsThemselves_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "manager-1")

	assertRefused(t, identity, resp, "manager-1", 1, "outranks")
}

func TestRegeneratePasswords_OwnerTargetsPlatformAdministrator_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "owner-1", "admin-1")

	assertRefused(t, identity, resp, "admin-1", 1, "outranks")
}

func TestRegeneratePasswords_PlatformAdministratorManagerTargetsOwnerAndAdministrator_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	// The service lets only owners and managers in; the administrator must hold one of those roles.
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "admin-1").
		Update("role", models.OrgRoleManager).Error)

	resp := regenerate(t, db, identity, orgID, classID, "admin-1", "owner-1", "admin-1")

	assertRegenerated(t, identity, resp, "owner-1")
	assertRegenerated(t, identity, resp, "admin-1")
	assert.Empty(t, resp.Errors)
}

func TestRegeneratePasswords_TargetRolesLookupFails_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	failRolesLookupFor("student-1")

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "student-1")

	assertRefused(t, identity, resp, "student-1", 1, "outranks")
}

func TestRegeneratePasswords_CallerRolesLookupFails_CapStillApplies(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	failRolesLookupFor("manager-1")

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "student-1", "owner-1")

	assertRegenerated(t, identity, resp, "student-1")
	assertRefused(t, identity, resp, "owner-1", 2, "outranks")
}

func TestRegeneratePasswords_EveryTargetRefused_WellFormedResponseAndNoWrites(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)

	resp := regenerate(t, db, identity, orgID, classID, "manager-1", "owner-1", "admin-1", "outsider-1")

	assertRefused(t, identity, resp, "owner-1", 1, "outranks")
	assertRefused(t, identity, resp, "admin-1", 2, "outranks")
	assertRefused(t, identity, resp, "outsider-1", 3, "not a member")
	assert.Empty(t, resp.Credentials)
	assert.Empty(t, identity.passwordsSet, "no password is set")
	assert.Empty(t, identity.columns, "no force_password_reset write")
	assert.Equal(t, dto.RegeneratePasswordsSummary{Total: 3, Succeeded: 0, Failed: 3}, resp.Summary)
}

// student-1 is a plain member whose account this organization created; each
// case also ties them to something else, seeded the way the erasure tests do.
// Being active in another team organization leaves the account to its creator;
// anything the person owns or pays for makes it theirs. The rule binds the
// owner as well as managers.
func TestRegeneratePasswords_MemberWhoBelongsElsewhere_RefusedToManagerAndOwner(t *testing.T) {
	for _, c := range []struct {
		name       string
		seed       func(t *testing.T, db *gorm.DB)
		regenerate bool
	}{
		{"active member of another team organization", func(t *testing.T, db *gorm.DB) {
			seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-1", models.OrgRoleMember)
		}, true},
		{"owner of another organization", func(t *testing.T, db *gorm.DB) {
			otherOrgID := seedTeamOrg(t, db, "student-1", new(10))
			// Ownership alone: no membership row in the other organization.
			require.NoError(t, db.Where("organization_id = ? AND user_id = ?", otherOrgID, "student-1").Delete(&models.OrganizationMember{}).Error)
		}, false},
		{"paid personal subscription", func(t *testing.T, db *gorm.DB) {
			seedPaidPersonalSubscription(t, db, "student-1")
		}, false},
		{"owner of a group with no organization", func(t *testing.T, db *gorm.DB) {
			seedOwnedClass(t, db, "student-1", nil)
		}, false},
		{"owner of a group of another organization", func(t *testing.T, db *gorm.DB) {
			otherOrgID := seedTeamOrg(t, db, "owner-2", new(10))
			seedOwnedClass(t, db, "student-1", &otherOrgID)
		}, false},
	} {
		for _, caller := range []string{"manager-1", "owner-1"} {
			t.Run(c.name+"/"+caller, func(t *testing.T) {
				db := newOffboardingDB(t)
				orgID, classID, identity := seedRankedClass(t, db)
				c.seed(t, db)

				resp := regenerate(t, db, identity, orgID, classID, caller, "student-1")

				if c.regenerate {
					assertRegenerated(t, identity, resp, "student-1")
				} else {
					assertRefused(t, identity, resp, "student-1", 1, "elsewhere")
				}
			})
		}
	}
}

func seedOwnedClass(t *testing.T, db *gorm.DB, ownerID string, orgID *uuid.UUID) {
	t.Helper()
	class := &groupModels.ClassGroup{Name: "owned-" + uuid.NewString()[:8], DisplayName: "Owned", OwnerUserID: ownerID, OrganizationID: orgID}
	class.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(class).Error)
}

func TestRegeneratePasswords_PlatformAdministratorTargetsMemberWhoBelongsElsewhere_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	// The service lets only owners and managers in; the administrator must hold one of those roles.
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "admin-1").
		Update("role", models.OrgRoleManager).Error)
	seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-1", models.OrgRoleMember)

	resp := regenerate(t, db, identity, orgID, classID, "admin-1", "student-1")

	assertRegenerated(t, identity, resp, "student-1")
	assert.Empty(t, resp.Errors)
}

// Owning this organization's own classes is not belonging elsewhere for the
// password rule, but erasure still waits for those classes to change hands.
func TestRegeneratePasswords_OwnerTargetsManagerOwningAClassOfThisOrganization_RegeneratedButNotErasable(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db) // manager-1 owns classID, a class of orgID

	resp := regenerate(t, db, identity, orgID, classID, "owner-1", "manager-1")

	assertRegenerated(t, identity, resp, "manager-1")
	assert.ErrorIs(t, newDeletionService(db, identity).CheckDepartedMemberErasable(orgID, "manager-1", false), authServices.ErrOwnsGroups)
}

// Without the belongs-elsewhere check only a platform administrator may proceed.
func TestRegeneratePasswords_WithoutTheAloneCheck_RefusedToOwnerAllowedToAdministrator(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "admin-1").
		Update("role", models.OrgRoleManager).Error)
	svc := services.NewOrganizationServiceWithIdentity(db, identity, nil)

	resp, err := svc.RegenerateGroupMemberPasswords(orgID, classID, "owner-1", []string{"student-1"})
	require.NoError(t, err)
	assertRefused(t, identity, resp, "student-1", 1, "elsewhere")

	resp, err = svc.RegenerateGroupMemberPasswords(orgID, classID, "admin-1", []string{"student-1"})
	require.NoError(t, err)
	assertRegenerated(t, identity, resp, "student-1")
}

func seedPaidPersonalSubscription(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{Name: "Personal plan " + uuid.NewString()[:8], PriceAmount: 1000, IsActive: true}
	plan.ID = uuid.New()
	require.NoError(t, db.Create(plan).Error)
	sub := &paymentModels.UserSubscription{UserID: userID, SubscriptionType: "personal", SubscriptionPlanID: plan.ID, Status: "active"}
	sub.ID = uuid.New()
	require.NoError(t, db.Create(sub).Error)
}

func TestRegeneratePasswords_OffboardedMemberStillInTheGroup_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	offboardedAt(t, db, orgID, "student-1", time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 9))

	resp := regenerate(t, db, identity, orgID, classID, "owner-1", "student-1")

	assertRefused(t, identity, resp, "student-1", 1, "offboarded")
}
