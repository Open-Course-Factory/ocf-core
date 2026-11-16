// tests/organizations/memberOffboardingRank_test.go
//
// Offboarding follows the rank rule of password regeneration: a caller must not
// offboard a member who outranks them (another manager, a platform
// administrator), and offboarding must not block an account that still belongs
// elsewhere: that membership is offboarded, the account stays usable.
package organizations_tests

import (
	"testing"

	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func assertStillActiveMember(t *testing.T, db *gorm.DB, identity *fakeIdentity, orgID uuid.UUID, userID string) {
	t.Helper()
	m := loadMember(t, db, orgID, userID)
	assert.True(t, m.IsActive, "%s's membership must stay active", userID)
	assert.Nil(t, m.LeftAt)
	assert.Nil(t, m.ScheduledErasureAt)
	assert.False(t, identity.forbidden[userID], "%s's account must not be forbidden", userID)
}

func assertOffboarded(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	m := loadMember(t, db, orgID, userID)
	assert.False(t, m.IsActive, "%s's membership must be offboarded", userID)
	assert.True(t, m.IsOffboarded())
}

func TestOffboard_ManagerTargetsAnotherManager_Refused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)

	err := newOffboardingService(db, identity).Offboard(orgID, []string{"manager-2"}, "manager-1")

	require.Error(t, err)
	assertStillActiveMember(t, db, identity, orgID, "manager-2")
}

func TestOffboard_PlatformAdministratorTarget_RefusedToManagerAndOwner(t *testing.T) {
	for _, caller := range []string{"manager-1", "owner-1"} {
		t.Run(caller, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, _, identity := seedRankedClass(t, db)

			err := newOffboardingService(db, identity).Offboard(orgID, []string{"admin-1"}, caller)

			require.Error(t, err)
			assertStillActiveMember(t, db, identity, orgID, "admin-1")
		})
	}
}

func TestOffboard_MemberWhoBelongsElsewhere_OffboardedButAccountNotForbidden(t *testing.T) {
	for _, c := range []struct {
		name string
		seed func(t *testing.T, db *gorm.DB)
	}{
		{"active member of another team organization", func(t *testing.T, db *gorm.DB) {
			seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-1", models.OrgRoleMember)
		}},
		{"paid personal subscription", func(t *testing.T, db *gorm.DB) {
			seedPaidPersonalSubscription(t, db, "student-1")
		}},
	} {
		for _, caller := range []string{"manager-1", "owner-1"} {
			t.Run(c.name+"/"+caller, func(t *testing.T) {
				db := newOffboardingDB(t)
				orgID, _, identity := seedRankedClass(t, db)
				c.seed(t, db)

				require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, caller))

				assertOffboarded(t, db, orgID, "student-1")
				assert.False(t, identity.forbidden["student-1"],
					"the account still belongs elsewhere: it must keep signing in")
			})
		}
	}
}

func TestOffboard_ManagerTargetsMemberOfThisOrganizationAlone_OffboardedAndForbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "manager-1"))

	assertOffboarded(t, db, orgID, "student-1")
	assert.True(t, identity.forbidden["student-1"])
}

func TestOffboard_OwnerTargetsManager_Offboarded(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"manager-2"}, "owner-1"))

	assertOffboarded(t, db, orgID, "manager-2")
	assert.True(t, identity.forbidden["manager-2"])
}

func TestOffboard_PlatformAdministratorCaller_NotCapped(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"manager-2"}, "admin-1"))

	assertOffboarded(t, db, orgID, "manager-2")
}
