// tests/organizations/memberOffboardingRank_test.go
//
// Offboarding follows the rank rule of password regeneration: a caller must not
// offboard a member who outranks them (another manager, a platform
// administrator), and offboarding must not block an account that still belongs
// elsewhere: that membership is offboarded, the account stays usable.
package organizations_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adminUsersRoutes "soli/formations/src/admin/routes/adminUsersRoutes"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/dto"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
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
		m := loadMember(t, db, orgID, id)
		assert.True(t, m.IsActive, "%s's membership is rolled back", id)
		assert.False(t, m.AccountForbiddenByOffboarding, "%s keeps no block stamp from a rolled-back offboarding", id)
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

// reinstateAs posts the reinstate route for target as actor, through the real controller.
func reinstateAs(t *testing.T, db *gorm.DB, identity *fakeIdentity, orgID uuid.UUID, actor, target string) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := orgController.NewOrganizationController(nil, nil, newOffboardingService(db, identity), nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", actor); c.Next() })
	router.POST("/organizations/:id/members/:userId/reinstate", ctrl.ReinstateMember)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/members/"+target+"/reinstate", nil))
	return rec
}

// blockAccount blocks userID the way the administrator action does: the
// identity provider's flag and the local mirror the auth middleware reads.
func blockAccount(t *testing.T, db *gorm.DB, identity *fakeIdentity, userID string) {
	t.Helper()
	identity.forbidden[userID] = true
	require.NoError(t, authModels.SetAccountBlocked(db, userID, true))
}

func TestReinstateHandler_ActiveMember_ConflictAndAccountUntouched(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	blockAccount(t, db, identity, "student-1")

	rec := reinstateAs(t, db, identity, orgID, "manager-1", "student-1")

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), services.ErrMemberNotOffboarded.Error())
	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "a member who was not offboarded keeps the account's state")
}

func TestReinstateHandler_ManagerTargetsOffboardedManager_ForbiddenAndNothingChanged(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"manager-2"}, "owner-1"))
	require.True(t, identity.forbidden["manager-2"])

	rec := reinstateAs(t, db, identity, orgID, "manager-1", "manager-2")

	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assertOffboarded(t, db, orgID, "manager-2")
	assert.True(t, identity.forbidden["manager-2"])
}

func TestReinstateHandler_AccountBlockedBeforeOffboarding_MembershipActiveAccountStillBlocked(t *testing.T) {
	for _, actor := range []string{"manager-1", "owner-1"} {
		t.Run(actor, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, _, identity := seedRankedClass(t, db)
			blockAccount(t, db, identity, "student-1")
			require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, actor))

			rec := reinstateAs(t, db, identity, orgID, actor, "student-1")

			assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			m := loadMember(t, db, orgID, "student-1")
			assert.True(t, m.IsActive)
			assert.Nil(t, m.LeftAt)
			assert.True(t, identity.forbidden["student-1"], "a block this organization did not set is not its to lift")
		})
	}
}

func TestReinstateHandler_AccountForbiddenByThisOffboarding_ReinstatedAndUnforbidden(t *testing.T) {
	for _, actor := range []string{"manager-1", "owner-1"} {
		t.Run(actor, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, _, identity := seedRankedClass(t, db)
			require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "manager-1"))
			require.True(t, identity.forbidden["student-1"])

			rec := reinstateAs(t, db, identity, orgID, actor, "student-1")

			assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			assertStillActiveMember(t, db, identity, orgID, "student-1")
		})
	}
}

func TestCsvImport_ReenrolsMemberBlockedBeforeOffboarding_AccountStillBlocked(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("student-a")
	offboarding := newOffboardingService(db, identity)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)
	blockAccount(t, db, identity, "student-a")
	require.NoError(t, offboarding.Offboard(orgID, []string{"student-a"}, "owner-1"))

	importer := services.NewImportService(db, identity, offboarding, newDeletionService(db, identity))
	resp, err := importer.ImportOrganizationData(orgID, "owner-1",
		usersCSV(t, "student-a@example.com,Ada,Lovelace,member\n"), nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)

	assert.True(t, loadMember(t, db, orgID, "student-a").IsActive, "re-importing still reinstates the membership")
	assert.True(t, identity.forbidden["student-a"], "a block this organization did not set is not its to lift")
}

func TestOffboardReinstateCycle_BlockStampFollowsEachCycle_AccountEndsForbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)

	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	assert.True(t, loadMember(t, db, orgID, "student-1").AccountForbiddenByOffboarding)
	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))
	assert.False(t, loadMember(t, db, orgID, "student-1").AccountForbiddenByOffboarding)
	assert.False(t, identity.forbidden["student-1"])
	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").AccountForbiddenByOffboarding, "the second offboarding owns the block again")
	assert.True(t, identity.forbidden["student-1"])
}

func TestReinstate_MemberWhoBelongsElsewhere_AccountStateUnchanged(t *testing.T) {
	for _, blockedBefore := range []bool{false, true} {
		t.Run(map[bool]string{false: "not blocked", true: "blocked by someone else"}[blockedBefore], func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, _, identity := seedRankedClass(t, db)
			seedMember(t, db, seedTeamOrg(t, db, "owner-2", new(10)), "student-1", models.OrgRoleMember)
			if blockedBefore {
				blockAccount(t, db, identity, "student-1")
			}
			svc := newOffboardingService(db, identity)
			require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
			require.False(t, loadMember(t, db, orgID, "student-1").AccountForbiddenByOffboarding)

			require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

			assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
			assert.Equal(t, blockedBefore, identity.forbidden["student-1"])
		})
	}
}

func TestOffboardBatch_OneAlreadyBlocked_OnlyTheOtherStampedAndLaterUnforbidden(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	seedMember(t, db, orgID, "student-2", models.OrgRoleMember)
	identity.users["student-2"] = identity.users["student-1"]
	blockAccount(t, db, identity, "student-2")
	svc := newOffboardingService(db, identity)

	require.NoError(t, svc.Offboard(orgID, []string{"student-1", "student-2"}, "manager-1"))
	assert.True(t, loadMember(t, db, orgID, "student-1").AccountForbiddenByOffboarding)
	assert.False(t, loadMember(t, db, orgID, "student-2").AccountForbiddenByOffboarding)

	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))
	require.NoError(t, svc.Reinstate(orgID, "student-2", "manager-1"))

	assert.False(t, identity.forbidden["student-1"], "the block this offboarding set is lifted")
	assert.True(t, identity.forbidden["student-2"], "the block set before is kept")
}

func TestEnrol_RemovedMemberNeverOffboarded_ReactivatedAndBlockUntouched(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "student-1").Update("is_active", false).Error)
	blockAccount(t, db, identity, "student-1")

	require.NoError(t, newOffboardingService(db, identity).Enrol(orgID, "student-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "re-adding a removed member does not lift a block it never set")
}

// adminSetsForbidden runs the real administrator action PUT /admin/users/:id/forbidden.
// Its identity-provider seam does what the real client does: the provider flag
// and the local mirror the auth middleware reads.
func adminSetsForbidden(t *testing.T, db *gorm.DB, identity *fakeIdentity, userID string, forbidden bool) {
	t.Helper()
	origForbidden, origKey := adminUsersRoutes.SetAccountForbidden, adminUsersRoutes.DisableUserTerminalKey
	t.Cleanup(func() {
		adminUsersRoutes.SetAccountForbidden, adminUsersRoutes.DisableUserTerminalKey = origForbidden, origKey
	})
	adminUsersRoutes.SetAccountForbidden = func(id string, f bool) error {
		if err := identity.SetForbidden(id, f); err != nil {
			return err
		}
		return authModels.SetAccountBlocked(db, id, f)
	}
	adminUsersRoutes.DisableUserTerminalKey = func(*gorm.DB, string) error { return nil }

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "platform-admin")
		c.Set("userRoles", []string{"member", "administrator"})
		c.Next()
	})
	router.PUT("/admin/users/:id/forbidden", adminUsersRoutes.NewSetForbiddenHandler(db))
	body := `{"forbidden":false}`
	if forbidden {
		body = `{"forbidden":true}`
	}
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+userID+"/forbidden", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestReinstate_AdministratorBlockedAfterOffboarding_MembershipActiveAccountStillBlocked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)
	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	adminSetsForbidden(t, db, identity, "student-1", true)

	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "the administrator's block is not the organization's to lift")
}

func TestReinstate_AdministratorUnblockedAfterOffboarding_ReinstatesAndStaysUnblocked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)
	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	adminSetsForbidden(t, db, identity, "student-1", false)
	require.False(t, identity.forbidden["student-1"])

	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.False(t, identity.forbidden["student-1"])
}

func TestOrganizationMemberGenericRoutes_BlockStampNeitherExposedNorSettable(t *testing.T) {
	installMockEnforcer(t)
	service := ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganizationMember(service)
	ops, ok := service.GetEntityOps("OrganizationMember")
	require.True(t, ok)
	body := map[string]any{"is_active": true, "account_forbidden_by_offboarding": true, "AccountForbiddenByOffboarding": true}

	// PATCH: decoded the way the generic edit handler decodes the body.
	edit := ops.NewEditDto()
	require.NoError(t, mapstructure.Decode(body, &edit))
	updates, err := ops.ConvertEditDtoToMap(edit)
	require.NoError(t, err)
	assert.NotContains(t, updates, "account_forbidden_by_offboarding")
	assert.NotContains(t, updates, "AccountForbiddenByOffboarding")

	// POST.
	raw, _ := json.Marshal(map[string]any{"organization_id": uuid.New(), "user_id": "u", "account_forbidden_by_offboarding": true, "AccountForbiddenByOffboarding": true})
	var create dto.CreateOrganizationMemberInput
	require.NoError(t, json.Unmarshal(raw, &create))
	created, err := ops.ConvertDtoToModel(create)
	require.NoError(t, err)
	assert.False(t, created.(*models.OrganizationMember).AccountForbiddenByOffboarding)

	// Output.
	out, err := ops.ConvertModelToDto(&models.OrganizationMember{UserID: "u", AccountForbiddenByOffboarding: true})
	require.NoError(t, err)
	encoded, _ := json.Marshal(out)
	assert.NotContains(t, strings.ToLower(string(encoded)), "forbidden")
	model, _ := json.Marshal(models.OrganizationMember{AccountForbiddenByOffboarding: true})
	assert.NotContains(t, strings.ToLower(string(model)), "forbidden", "the model itself never serializes it")
}

func TestOffboard_AlreadyOffboardedMemberOffboardedAgain_ReinstateStillUnblocks(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)
	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	first := loadMember(t, db, orgID, "student-1")
	require.True(t, identity.forbidden["student-1"])
	// The second call sees the account as blocked, as production does through the local mirror.
	require.NoError(t, authModels.SetAccountBlocked(db, "student-1", true))
	time.Sleep(20 * time.Millisecond)

	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	second := loadMember(t, db, orgID, "student-1")
	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.False(t, identity.forbidden["student-1"], "a repeated offboarding keeps the block this organization set: reinstating lifts it")
	require.NotNil(t, second.ScheduledErasureAt)
	assert.Equal(t, first.ScheduledErasureAt.UnixMilli(), second.ScheduledErasureAt.UnixMilli(), "a repeated offboarding does not push the erasure later")
}

func TestReinstateHandler_OwnerReinstatesOffboardedManager_ReinstatedAndUnblocked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"manager-2"}, "owner-1"))
	require.True(t, identity.forbidden["manager-2"])

	rec := reinstateAs(t, db, identity, orgID, "owner-1", "manager-2")

	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assertStillActiveMember(t, db, identity, orgID, "manager-2")
}

func TestReinstate_AdministratorUnblocksThenBlocksAfterOffboarding_AccountStillBlocked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	svc := newOffboardingService(db, identity)
	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	adminSetsForbidden(t, db, identity, "student-1", false)
	adminSetsForbidden(t, db, identity, "student-1", true)

	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "the administrator's block is not the organization's to lift")
}
