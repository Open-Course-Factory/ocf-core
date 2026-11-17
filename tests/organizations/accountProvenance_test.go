// tests/organizations/accountProvenance_test.go
//
// Only the organization that created an account controls it: resetting its
// password, blocking it by offboarding and erasing it. An account attached to
// the organization (a membership the organization did not create through the
// import) is just a member. Platform administrators keep every power.
// Refs #599.
package organizations_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"soli/formations/src/cron"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/dto"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
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

	assert.Error(t, newDeletionService(db, identity).CheckDepartedMemberErasable(orgID, "attached-1", false),
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

// A platform administrator may do everything an organization may not: the
// route erases an attached member it offboarded before provenance existed.
func TestEraseNowHandler_AdministratorOnAttachedOffboardedMember_Erased(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")
	offboardedAt(t, db, orgID, "attached-1", time.Now().AddDate(0, 0, -20), time.Now().Add(-time.Hour))

	rec := eraseAs(t, db, identity, orgID, "admin-1", "attached-1")

	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"attached-1"}, identity.deleted)
}

// An administrator blocks an attached account that belongs to this
// organization alone, but schedules no erasure: the block has no end date and
// the nightly job never touches it.
func TestOffboard_AdministratorOnAttachedMemberAlone_BlockedNoErasureDate(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, classID, identity := seedRankedClass(t, db)
	seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")

	require.NoError(t, newOffboardingService(db, identity).Offboard(orgID, []string{"attached-1"}, "admin-1"))

	m := loadMember(t, db, orgID, "attached-1")
	assert.True(t, identity.forbidden["attached-1"], "the administrator's offboarding blocks the account")
	assert.True(t, m.AccountForbiddenByOffboarding, "a reinstate or a new membership lifts it")
	assert.Nil(t, m.ScheduledErasureAt, "no erasure is scheduled for an account the organization did not create")
	report, err := cron.RunMemberErasure(db, newDeletionService(db, identity), time.Now().AddDate(10, 0, 0), 50)
	require.NoError(t, err)
	assert.Equal(t, 0, report.Erased)
	assert.Empty(t, identity.deleted)
}

// A row offboarded before provenance existed carries the block stamp and an
// erasure date; reinstating it is unchanged: the block is lifted.
func TestReinstateHandler_LegacyAttachedRowWithBlockStamp_ReinstatedAndUnblocked(t *testing.T) {
	for _, actor := range []string{"manager-1", "owner-1"} {
		t.Run(actor, func(t *testing.T) {
			db := newOffboardingDB(t)
			orgID, classID, identity := seedRankedClass(t, db)
			seedAttachedClassMember(t, db, identity, orgID, classID, "attached-1")
			offboardedAt(t, db, orgID, "attached-1", time.Now().AddDate(0, 0, -2), time.Now().AddDate(0, 0, 8))
			require.NoError(t, db.Model(&models.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", orgID, "attached-1").
				Update("account_forbidden_by_offboarding", true).Error)
			blockAccount(t, db, identity, "attached-1")

			rec := reinstateAs(t, db, identity, orgID, actor, "attached-1")

			assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			m := loadMember(t, db, orgID, "attached-1")
			assert.True(t, m.IsActive)
			assert.Nil(t, m.ScheduledErasureAt)
			assert.False(t, m.AccountForbiddenByOffboarding)
			assert.False(t, identity.forbidden["attached-1"])
		})
	}
}

func TestOrganizationMemberGenericRoutes_ProvenanceNeitherExposedNorSettable(t *testing.T) {
	installMockEnforcer(t)
	service := ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganizationMember(service)
	ops, ok := service.GetEntityOps("OrganizationMember")
	require.True(t, ok)
	keys := []string{"account_created_by_organization", "AccountCreatedByOrganization", "accountCreatedByOrganization", "ACCOUNT_CREATED_BY_ORGANIZATION"}

	// PATCH: decoded the way the generic edit handler decodes the body.
	body := map[string]any{"is_active": true}
	for _, k := range keys {
		body[k] = true
	}
	edit := ops.NewEditDto()
	require.NoError(t, mapstructure.Decode(body, &edit))
	updates, err := ops.ConvertEditDtoToMap(edit)
	require.NoError(t, err)
	for k := range updates {
		assert.NotContains(t, strings.ToLower(k), "created_by", "PATCH must not reach the provenance column")
		assert.NotContains(t, strings.ToLower(k), "createdby", "PATCH must not reach the provenance column")
	}

	// POST.
	post := map[string]any{"organization_id": uuid.New(), "user_id": "u"}
	for _, k := range keys {
		post[k] = true
	}
	raw, _ := json.Marshal(post)
	var create dto.CreateOrganizationMemberInput
	require.NoError(t, json.Unmarshal(raw, &create))
	created, err := ops.ConvertDtoToModel(create)
	require.NoError(t, err)
	assert.False(t, created.(*models.OrganizationMember).AccountCreatedByOrganization)

	// Output.
	out, err := ops.ConvertModelToDto(&models.OrganizationMember{UserID: "u", AccountCreatedByOrganization: true})
	require.NoError(t, err)
	encoded, _ := json.Marshal(out)
	assert.NotContains(t, strings.ToLower(string(encoded)), "created_by_organization")
	assert.NotContains(t, strings.ToLower(string(encoded)), "createdbyorganization")
}

func TestProvenance_OffboardThenReinstate_StaysMarked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	markCreatedByOrganization(t, db, orgID, "student-1")
	svc := newOffboardingService(db, identity)

	require.NoError(t, svc.Offboard(orgID, []string{"student-1"}, "manager-1"))
	require.NoError(t, svc.Reinstate(orgID, "student-1", "manager-1"))

	assert.True(t, loadMember(t, db, orgID, "student-1").AccountCreatedByOrganization)
}

// genericCreateMember runs the generic POST /organization-members create: the
// real hooks around the insert.
func genericCreateMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, actor, userID string) *models.OrganizationMember {
	t.Helper()
	member := &models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrgRoleMember, IsActive: true, JoinedAt: time.Now()}
	ctx := &hooks.HookContext{EntityName: "OrganizationMember", HookType: hooks.BeforeCreate, NewEntity: member, UserID: actor, UserRoles: platformMember}
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	require.NoError(t, db.Omit("Metadata").Create(member).Error)
	ctx.HookType = hooks.AfterCreate
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx))
	return member
}

func TestProvenance_MemberRemovedThenReAdded_Unmarked(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	markCreatedByOrganization(t, db, orgID, "student-1")
	registerOrgHooks(t, db, identity)

	require.NoError(t, genericDelete(t, db, orgID, "owner-1", platformMember, "student-1"))
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", orgID, "student-1").Delete(&models.OrganizationMember{}).Error)
	genericCreateMember(t, db, orgID, "owner-1", "student-1")

	assert.False(t, loadMember(t, db, orgID, "student-1").AccountCreatedByOrganization, "a re-added account is an attachment")
}

// The organization's delete is a soft delete: the member rows stay, flag
// included, but no route of that organization reaches them any more.
func TestProvenance_CreatingOrganizationDeleted_RowKeepsFlagAndOrganizationActionsRefused(t *testing.T) {
	db := newOffboardingDB(t)
	orgID, _, identity := seedRankedClass(t, db)
	markCreatedByOrganization(t, db, orgID, "student-1")

	require.NoError(t, db.Delete(&models.Organization{}, "id = ?", orgID).Error)

	assert.True(t, loadMember(t, db, orgID, "student-1").AccountCreatedByOrganization)
	err := newOffboardingService(db, identity).Offboard(orgID, []string{"student-1"}, "owner-1")
	assert.ErrorContains(t, err, "organization not found")
	assert.False(t, identity.forbidden["student-1"])
}

// blockedByOffboardingOfB: organization B created student-1, who belongs to B
// alone, and offboarded them, so B blocked the account. Organization C exists.
func blockedByOffboardingOfB(t *testing.T) (db *gorm.DB, identity *fakeIdentity, orgB, orgC uuid.UUID) {
	t.Helper()
	installMockEnforcer(t)
	db = newOffboardingDB(t)
	identity = newFakeIdentity("owner-b", "owner-c", "student-1")
	orgB = seedTeamOrg(t, db, "owner-b", new(10))
	orgC = seedTeamOrg(t, db, "owner-c", new(10))
	seedMember(t, db, orgB, "student-1", models.OrgRoleMember)
	markCreatedByOrganization(t, db, orgB, "student-1")
	require.NoError(t, newOffboardingService(db, identity).Offboard(orgB, []string{"student-1"}, "owner-b"))
	require.True(t, identity.forbidden["student-1"])
	return db, identity, orgB, orgC
}

func TestGenericMemberCreate_InactiveMembership_DoesNotLift(t *testing.T) {
	db, identity, orgB, orgC := blockedByOffboardingOfB(t)
	registerOrgHooks(t, db, identity)

	member := &models.OrganizationMember{OrganizationID: orgC, UserID: "student-1", Role: models.OrgRoleMember, IsActive: false}
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(&hooks.HookContext{
		EntityName: "OrganizationMember", HookType: hooks.AfterCreate, NewEntity: member, UserID: "owner-c", UserRoles: platformMember,
	}))

	assert.True(t, identity.forbidden["student-1"], "an inactive membership does not make the account belong to C")
	assert.True(t, loadMember(t, db, orgB, "student-1").AccountForbiddenByOffboarding)
}

// The identity provider refuses the unblock: the membership is still created,
// the account stays blocked, and B's stamp stays so a later join or reinstate
// can still lift the block.
func TestGenericMemberCreate_LiftFails_MemberCreatedBlockAndStampKept(t *testing.T) {
	db, identity, orgB, orgC := blockedByOffboardingOfB(t)
	registerOrgHooks(t, db, identity)
	identity.affected = false

	genericCreateMember(t, db, orgC, "owner-c", "student-1")

	assert.True(t, loadMember(t, db, orgC, "student-1").IsActive)
	assert.True(t, identity.forbidden["student-1"], "a failed unblock leaves the account blocked")
	assert.True(t, loadMember(t, db, orgB, "student-1").AccountForbiddenByOffboarding,
		"the stamp is what lets a later join lift the block: it must survive a failed unblock")
}

// importNewRow imports one row for new@example.com as owner-1 into a fresh
// organization and returns the response with the organization's member count
// beside the owner.
func importNewRow(t *testing.T, identity *fakeIdentity) (*gorm.DB, uuid.UUID, *dto.ImportOrganizationDataResponse) {
	t.Helper()
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	resp, err := services.NewImportService(db, identity, newOffboardingService(db, identity), newDeletionService(db, identity)).
		ImportOrganizationData(orgID, "owner-1", usersCSV(t, "new@example.com,Ada,Lovelace,member\n"), nil, nil, false, false, "", true)
	// The import recovers a panic and then returns no response at all.
	require.NotNil(t, resp, "the import returned no response (a recovered panic), err=%v", err)
	return db, orgID, resp
}

func membersBesidesOwner(t *testing.T, db *gorm.DB, orgID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.OrganizationMember{}).Where("organization_id = ? AND user_id <> ?", orgID, "owner-1").Count(&n).Error)
	return n
}

// The provider could not say whether the email is taken: the row is an error,
// never a new account.
func TestImport_LookupByEmailFails_RowErrorNothingCreatedOrEnrolled(t *testing.T) {
	identity := newFakeIdentity()
	identity.lookupErrs = 1

	db, orgID, resp := importNewRow(t, identity)

	assert.Empty(t, identity.created, "no account is created on a failed lookup")
	assert.Equal(t, 0, resp.Summary.UsersCreated)
	assert.Len(t, resp.Errors, 1, "the row is reported as an error, errors=%+v", resp.Errors)
	assert.Equal(t, int64(0), membersBesidesOwner(t, db, orgID))
}

// Adding the account "succeeds" without creating it, and the email already
// belongs to another account: the import did not create that account, so it
// neither marks nor enrols it; the row is skipped with a warning.
func TestImport_AddUserCreatesNothingEmailHeldByAnotherAccount_NotMarkedNotEnrolled(t *testing.T) {
	identity := newFakeIdentity()
	identity.addUser = func(*casdoorsdk.User) error {
		identity.users["existing"] = &casdoorsdk.User{Id: "existing", Name: "someone-else", Email: "new@example.com"}
		return nil
	}

	db, orgID, resp := importNewRow(t, identity)

	assert.Equal(t, int64(0), countMemberRows(t, db, orgID, "existing"), "an account this import did not create is not enrolled")
	assert.Equal(t, 0, resp.Summary.UsersCreated)
	assert.NotEmpty(t, resp.Warnings, "the row is reported as skipped")
}

// Adding the account "succeeds" and the read-back finds nothing: a row error,
// not a crash.
func TestImport_ReadBackFindsNoAccount_RowError(t *testing.T) {
	identity := newFakeIdentity()
	identity.addUser = func(*casdoorsdk.User) error { return nil }

	db, orgID, resp := importNewRow(t, identity)

	assert.Len(t, resp.Errors, 1, "errors=%+v", resp.Errors)
	assert.Equal(t, int64(0), membersBesidesOwner(t, db, orgID))
}
