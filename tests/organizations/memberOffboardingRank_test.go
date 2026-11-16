// tests/organizations/memberOffboardingRank_test.go
//
// Offboarding follows the rank rule of password regeneration: a caller must not
// offboard a member who outranks them (another manager, a platform
// administrator), and offboarding must not block an account that still belongs
// elsewhere: that membership is offboarded, the account stays usable.
package organizations_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/gin-gonic/gin"

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

	require.ErrorIs(t, err, services.ErrOutranksCaller)
	assertStillActiveMember(t, db, identity, orgID, "manager-2")
}

func TestOffboard_PlatformAdministratorTarget_RefusedToManagerAndOwner(t *testing.T) {
	for _, caller := range []string{"manager-1", "owner-1"} {
		t.Run(caller, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, _, identity := seedRankedClass(t, db)

			err := newOffboardingService(db, identity).Offboard(orgID, []string{"admin-1"}, caller)

			require.ErrorIs(t, err, services.ErrOutranksCaller)
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

func TestOffboard_BatchWithOneRefusedTarget_NothingWritten(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)

	err := newOffboardingService(db, identity).Offboard(orgID, []string{"student-1", "manager-2"}, "manager-1")

	require.ErrorIs(t, err, services.ErrOutranksCaller)
	assertStillActiveMember(t, db, identity, orgID, "student-1")
	assertStillActiveMember(t, db, identity, orgID, "manager-2")
}

func TestOffboard_BatchMixingBelongsElsewhereAndAlone_OnlyAloneForbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	seedMember(t, db, orgID, "student-2", models.OrgRoleMember)
	seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-2", models.OrgRoleMember)
	identity.users["student-2"] = identity.users["student-1"]

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-2", "student-1"}, "manager-1"))

	assertOffboarded(t, db, orgID, "student-1")
	assertOffboarded(t, db, orgID, "student-2")
	assert.NotNil(t, loadMember(t, db, orgID, "student-2").ScheduledErasureAt, "erasure is scheduled as before; the job refuses it while active elsewhere")
	assert.True(t, identity.forbidden["student-1"])
	assert.False(t, identity.forbidden["student-2"])
}

func TestOffboard_ForbidFailsMidBatch_OnlyAccountsThisCallForbadeAreUnforbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	seedMember(t, db, orgID, "student-2", models.OrgRoleMember)
	seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-2", models.OrgRoleMember)
	identity.users["student-2"] = identity.users["student-1"]
	identity.forbidden["student-2"] = true                  // blocked by someone else; this call does not touch it
	seedMember(t, db, orgID, "ghost", models.OrgRoleMember) // unknown to the identity provider: SetForbidden fails

	err := newOffboardingService(db, identity).Offboard(orgID, []string{"student-2", "student-1", "ghost"}, "manager-1")

	require.ErrorContains(t, err, "failed to forbid account ghost")
	for _, id := range []string{"student-1", "student-2", "ghost"} {
		assert.True(t, loadMember(t, db, orgID, id).IsActive, "%s's membership is rolled back", id)
	}
	assert.False(t, identity.forbidden["student-1"], "the account this call forbade is unforbidden")
	assert.True(t, identity.forbidden["student-2"], "an account this call did not forbid keeps its state")
}

func TestOffboard_Self_ManagerRefusedAsOutranking_OwnerRefusedAsOwner(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)

	require.ErrorIs(t, svc.Offboard(orgID, []string{"manager-1"}, "manager-1"), services.ErrOutranksCaller)
	require.ErrorIs(t, svc.Offboard(orgID, []string{"owner-1"}, "owner-1"), services.ErrCannotOffboardOwner)
	assertStillActiveMember(t, db, identity, orgID, "manager-1")
	assertStillActiveMember(t, db, identity, orgID, "owner-1")
}

func TestOffboard_PlatformAdministratorWithoutMembership_OffboardsAndForbidsOnlyTheAlone(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	installMockEnforcer(t).GetRolesForUserFunc = func(name string) ([]string, error) {
		if name == "platform-admin" {
			return []string{"member", "administrator"}, nil
		}
		return []string{"member"}, nil
	}
	seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "manager-2", models.OrgRoleMember)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1", "manager-2"}, "platform-admin"))

	assertOffboarded(t, db, orgID, "student-1")
	assertOffboarded(t, db, orgID, "manager-2")
	assert.True(t, identity.forbidden["student-1"])
	assert.False(t, identity.forbidden["manager-2"], "an administrator caller does not lift the belongs-elsewhere rule for forbidding")
}

func TestOffboard_WithoutTheAloneCheck_OffboardedButNotForbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := services.NewMemberOffboardingService(db, identity, paymentServices.NewBulkLicenseService(db), newDeletionService(db, identity), nil)

	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))

	assertOffboarded(t, db, orgID, "student-1")
	assert.False(t, identity.forbidden["student-1"])
}

func TestOffboardMembersHandler_OutrankingTargetIs403_PlainMemberIs204(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	ctrl := orgController.NewOrganizationController(nil, nil, newOffboardingService(db, identity), nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "manager-1"); c.Next() })
	router.POST("/organizations/:id/members/offboard", ctrl.OffboardMembers)
	post := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/members/offboard", strings.NewReader(`{"user_ids":["`+target+`"]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := post("manager-2")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assertStillActiveMember(t, db, identity, orgID, "manager-2")

	rec = post("student-1")
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assertOffboarded(t, db, orgID, "student-1")
}

// seedRunningTerminalsInOrgs gives userID one running terminal per organization
// and returns their session ids in the same order.
func seedRunningTerminalsInOrgs(t *testing.T, db *gorm.DB, userID string, orgIDs ...uuid.UUID) []string {
	t.Helper()
	key := &terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + userID, KeyName: "k", IsActive: true}
	key.ID = uuid.New()
	require.NoError(t, db.Create(key).Error)
	var sessions []string
	for i := range orgIDs {
		term := &terminalModels.Terminal{
			SessionID: "sess-" + userID + "-" + orgIDs[i].String()[:8], UserID: userID, State: terminalModels.StateRunning,
			ExpiresAt: time.Now().Add(2 * time.Hour), LastStartedAt: time.Now(), UserTerminalKeyID: key.ID, OrganizationID: &orgIDs[i],
		}
		term.ID = uuid.New()
		require.NoError(t, db.Create(term).Error)
		sessions = append(sessions, term.SessionID)
	}
	return sessions
}

func terminalState(t *testing.T, db *gorm.DB, sessionID string) terminalModels.TerminalState {
	t.Helper()
	var term terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&term).Error)
	return term.State
}

func TestOffboard_MemberWhoBelongsElsewhere_KeepsTheTerminalsOfTheOtherOrganization(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	otherOrgID := seedTeamOrg(t, db, "owner-2", new(10))
	seedMember(t, db, otherOrgID, "student-1", models.OrgRoleMember)
	sessions := seedRunningTerminalsInOrgs(t, db, "student-1", orgID, otherOrgID)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "manager-1"))

	assert.NotEqual(t, terminalModels.StateRunning, terminalState(t, db, sessions[0]), "this organization's terminal is terminated")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, sessions[1]),
		"the account still belongs elsewhere: its terminal in the other organization keeps running")
}

func TestOffboard_MemberOfThisOrganizationAlone_AllTerminalsTerminated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	otherOrgID := seedTeamOrg(t, db, "owner-2", new(10))
	sessions := seedRunningTerminalsInOrgs(t, db, "student-1", orgID, otherOrgID)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "manager-1"))

	require.True(t, identity.forbidden["student-1"])
	for _, s := range sessions {
		assert.NotEqual(t, terminalModels.StateRunning, terminalState(t, db, s), "a forbidden account keeps no running terminal")
	}
}
