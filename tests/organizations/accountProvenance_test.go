// tests/organizations/accountProvenance_test.go
//
// Only the organization that created an account controls it: resetting its
// password, blocking it by offboarding and erasing it. An account attached to
// the organization (a membership the organization did not create through the
// import) is just a member. Platform administrators keep every power.
// Refs #599.
package organizations_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/cron"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// markCreatedByOrganization marks userID's membership of orgID as the one whose
// organization created the account, as the import does.
func markCreatedByOrganization(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Update("account_created_by_organization", true).Error)
}

func createdByOrganization(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) bool {
	t.Helper()
	var created bool
	require.NoError(t, db.Raw("SELECT account_created_by_organization FROM organization_members WHERE organization_id = ? AND user_id = ?",
		orgID, userID).Scan(&created).Error)
	return created
}

// seedAttachedClassMember adds userID to seedRankedClass's organization and
// class as a membership the organization did not create, independently of how
// the shared seeds mark theirs.
func seedAttachedClassMember(t *testing.T, db *gorm.DB, identity *fakeIdentity, orgID, classID uuid.UUID, userID string) {
	t.Helper()
	seedMember(t, db, orgID, userID, models.OrgRoleMember)
	gm := &groupModels.GroupMember{GroupID: classID, UserID: userID, Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: true}
	require.NoError(t, db.Omit("Metadata").Create(gm).Error)
	identity.users[userID] = &casdoorsdk.User{Id: userID, Name: "user-" + userID, Email: userID + "@example.com", Owner: "soli"}
}

func TestImport_CreatedAccount_MembershipMarkedCreatedByOrganization(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity()
	orgID := seedTeamOrg(t, db, "owner-1", new(10))

	resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity), newDeletionService(db, identity)).
		ImportOrganizationData(orgID, "owner-1", usersCSV(t, "new@example.com,Ada,Lovelace,member\n"), nil, nil, false, false, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)
	require.Len(t, identity.created, 1)

	assert.True(t, createdByOrganization(t, db, orgID, identity.created[0].Id))
}

// A platform administrator passes every check, so the update reaches Enrol on a
// membership the organization did not create: that row keeps its provenance.
func TestImport_ExistingAccountEnrolled_NotMarkedCreated(t *testing.T) {
	installMockEnforcer(t).GetRolesForUserFunc = adminRoleFor("admin-1")
	db := newOffboardingDB(t)
	identity := newFakeIdentity("admin-1", "student-a")
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

	resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity), newDeletionService(db, identity)).
		ImportOrganizationData(orgID, "admin-1", usersCSV(t, "student-a@example.com,Ada,Lovelace,member\n"), nil, nil, false, true, "", true)
	require.NoError(t, err, "errors=%+v", resp.Errors)
	require.Equal(t, 1, resp.Summary.UsersUpdated, "warnings=%+v", resp.Warnings)

	assert.False(t, createdByOrganization(t, db, orgID, "student-a"))
}

func TestRegeneratePasswords_AttachedMember_SkippedNotCreatedHere(t *testing.T) {
	for _, caller := range []string{"manager-1", "owner-1"} {
		t.Run(caller, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, classID, identity := seedRankedClass(t, db)
			seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")

			resp := regenerate(t, db, identity, orgID, classID, caller, "attached-1")

			assertRefused(t, identity, resp, "attached-1", 1, "not created by this organization")
		})
	}
}

func TestRegeneratePasswords_AdministratorOnAttachedMember_Regenerated(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	// The service lets only owners and managers in; the administrator must hold one of those roles.
	require.NoError(t, db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, "admin-1").
		Update("role", models.OrgRoleManager).Error)

	seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")

	resp := regenerate(t, db, identity, orgID, classID, "admin-1", "attached-1")

	assertRegenerated(t, identity, resp, "attached-1")
	assert.Empty(t, resp.Errors)
}

func TestImportUpdateExisting_AttachedMember_SkippedNoPasswordWrite(t *testing.T) {
	for _, importer := range []string{"manager-1", "owner-1"} {
		t.Run(importer, func(t *testing.T) {
			installMockEnforcer(t)
			db := newOffboardingDB(t)
			identity := newFakeIdentity(importer, "student-a")
			identity.users["student-a"].FirstName = "Original"
			orgID := seedTeamOrg(t, db, "owner-1", new(10))
			if importer == "manager-1" {
				seedMember(t, db, orgID, "manager-1", models.OrgRoleManager)
			}
			seedMember(t, db, orgID, "student-a", models.OrgRoleMember)

			resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity), newDeletionService(db, identity)).
				ImportOrganizationData(orgID, importer, usersCSVWithColumns(t, "email,first_name,last_name,role,password",
					"student-a@example.com,Ada,Lovelace,member,Str0ng-Pass!\n"), nil, nil, false, true, "", true)
			require.NoError(t, err)

			assertImportRefused(t, identity, resp, "student-a", "not created by this organization")
			assert.True(t, loadMember(t, db, orgID, "student-a").IsActive, "the skipped row enrols nothing and changes nothing")
		})
	}
}

func TestOffboard_AttachedMember_DeactivatedNotBlockedNoErasureDate(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")
	otherOrgID := seedTeamOrg(t, db, "owner-2", new(10))
	sessions := seedRunningTerminalsInOrgs(t, db, "attached-1", orgID, otherOrgID)

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"attached-1"}, "manager-1"))

	m := loadMember(t, db, orgID, "attached-1")
	assert.False(t, m.IsActive)
	assert.NotNil(t, m.LeftAt)
	assert.Nil(t, m.ScheduledErasureAt, "an account this organization did not create is not its to erase")
	assert.False(t, m.AccountForbiddenByOffboarding)
	assert.False(t, identity.forbidden["attached-1"], "an account this organization did not create is not its to block")
	assert.NotEqual(t, terminalModels.StateRunning, terminalState(t, db, sessions[0]), "this organization's terminal is terminated")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, sessions[1]), "the account is not blocked: its other terminals keep running")
}

func TestOffboard_CreatedMember_StillBlockedAndScheduled(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	markCreatedByOrganization(t, db, orgID, "student-1")

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "manager-1"))

	assert.True(t, identity.forbidden["student-1"])
	assert.NotNil(t, loadMember(t, db, orgID, "student-1").ScheduledErasureAt)
}

func eraseAs(t *testing.T, db *gorm.DB, identity *fakeIdentity, orgID uuid.UUID, actor, target string) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := orgController.NewOrganizationController(nil, nil, newOffboardingService(db, identity), nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", actor); c.Next() })
	router.POST("/organizations/:id/members/:userId/erase", ctrl.EraseMember)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/members/"+target+"/erase", nil))
	return rec
}

func TestEraseNow_AttachedMember_RefusedNotCreatedByOrganization(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")
	// Offboarded before provenance existed: the row is offboarded with an erasure date.
	offboardedAt(t, db, orgID, "attached-1", time.Now().AddDate(0, 0, -20), time.Now().Add(-time.Hour))

	assert.Error(t, newDeletionService(db, identity).CheckDepartedMemberErasable(orgID, "attached-1"),
		"the members list reports the erasure as refused")
	rec := eraseAs(t, db, identity, orgID, "owner-1", "attached-1")

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Empty(t, identity.deleted, "an account this organization did not create is never erased by it")
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "attached-1"))
}

// The erasure date was set before provenance existed; the job still refuses it.
func TestMemberErasureJob_AttachedMemberWithLegacyDate_Skipped(t *testing.T) {
	db := newOffboardingDB(t)
	identity := newFakeIdentity("attached")
	now := time.Now()
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "attached", models.OrgRoleMember)
	offboardedAt(t, db, orgID, "attached", now.AddDate(0, 0, -20), now.Add(-time.Hour))

	report, err := cron.RunMemberErasure(db, newDeletionService(db, identity), now, 50)
	require.NoError(t, err)

	assert.Equal(t, 0, report.Erased)
	assert.Empty(t, identity.deleted, "an account this organization did not create is never erased by it")
	assert.Equal(t, int64(1), countMemberRows(t, db, orgID, "attached"))
	require.Len(t, report.Skipped, 1)
	assert.Equal(t, "attached", report.Skipped[0].UserID)
}

// An administrator's block carries no offboarding stamp: joining an
// organization through the generic create never lifts it.
func TestGenericMemberCreate_AdministratorBlock_NotLifted(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	identity := newFakeIdentity("owner-c", "student-1")
	orgC := seedTeamOrg(t, db, "owner-c", new(10))
	adminSetsForbidden(t, db, identity, "student-1", true)
	registerOrgHooks(t, db, identity)

	member := &models.OrganizationMember{OrganizationID: orgC, UserID: "student-1", Role: models.OrgRoleMember}
	ctx := &hooks.HookContext{EntityName: "OrganizationMember", HookType: hooks.BeforeCreate, NewEntity: member, UserID: "owner-c", UserRoles: platformMember}
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.NoError(t, db.Omit("Metadata").Create(member).Error)
	ctx.HookType = hooks.AfterCreate
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))

	assert.True(t, loadMember(t, db, orgC, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "the administrator's block is never lifted by joining an organization")
}
