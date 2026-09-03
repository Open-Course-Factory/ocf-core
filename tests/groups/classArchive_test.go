// tests/groups/classArchive_test.go
//
// Contract for class archiving (#491): ClassGroup opts into the framework's
// archiving capability, `is_active` is replaced by `archived_at`, and an
// archived class grants nothing — while everything it produced stays readable.
package groups_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	config "soli/formations/src/configuration"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/cron"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/entityManagement/swagger"
	"soli/formations/src/groups/dto"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	groupRoutes "soli/formations/src/groups/routes"
	"soli/formations/src/initialization"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioModels "soli/formations/src/scenarios/models"
	scenarioRoutes "soli/formations/src/scenarios/routes"
	scenarioServices "soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const classGroupsPath = "/api/v1/class-groups"

// identity is the caller the permissive auth middleware injects; tests swap it
// between requests to act as different people on one engine.
type identity struct {
	userID string
	roles  []string
}

type classArchiveEnv struct {
	db     *gorm.DB
	engine *gin.Engine
	caller *identity
}

// classArchiveDB is a private SQLite database holding every table the archived
// class rules touch: the class and its roster, the organization it belongs to,
// the scenarios assigned to it, the terminals its learners run and the plans
// the launch route resolves.
func classArchiveDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&paymentModels.SubscriptionPlan{},
		&paymentModels.UserSubscription{},
		&paymentModels.SubscriptionBatch{},
		&paymentModels.OrganizationSubscription{},
		&paymentModels.OrganizationRolePlan{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&scenarioModels.Scenario{},
		&scenarioModels.ScenarioStep{},
		&scenarioModels.ScenarioInstanceType{},
		&scenarioModels.ScenarioAssignment{},
		&scenarioModels.ScenarioSession{},
		&scenarioModels.ScenarioStepProgress{},
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
	))
	return db
}

// setupClassArchiveEnv registers ClassGroup, GroupMember and ScenarioAssignment
// in the GLOBAL registration service with the real group hooks, and mounts the
// generated routes (CRUD, archive/unarchive, archive-preview) on a gin engine.
func setupClassArchiveEnv(t *testing.T) *classArchiveEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = authMocks.NewMockEnforcer()
	access.RouteRegistry.Reset()
	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.DisableAllHooks(false)
	t.Cleanup(func() {
		casdoor.Enforcer = origEnforcer
		hooks.GlobalHookRegistry.ClearAllHooks()
		access.RouteRegistry.Reset()
		for _, name := range []string{"ClassGroup", "GroupMember", "ScenarioAssignment"} {
			ems.GlobalEntityRegistrationService.UnregisterEntity(name)
		}
	})

	db := classArchiveDB(t)
	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)
	groupRegistration.RegisterGroupMember(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioAssignment(ems.GlobalEntityRegistrationService)
	groupHooks.InitGroupHooks(db)

	caller := &identity{userID: "nobody", roles: []string{access.RoleMember}}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("userId", caller.userID)
		c.Set("userRoles", caller.roles)
		c.Next()
	})
	permissive := func(c *gin.Context) { c.Next() }
	swagger.NewSwaggerRouteGenerator(db).RegisterDocumentedRoutes(engine.Group("/api/v1"), permissive, permissive)

	return &classArchiveEnv{db: db, engine: engine, caller: caller}
}

func (env *classArchiveEnv) as(userID string, roles ...string) {
	if len(roles) == 0 {
		roles = []string{access.RoleMember}
	}
	env.caller.userID = userID
	env.caller.roles = roles
}

func (env *classArchiveEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.engine.ServeHTTP(rec, req)
	return rec
}

func (env *classArchiveEnv) archive(t *testing.T, groupID uuid.UUID) {
	t.Helper()
	rec := env.do(http.MethodPost, classGroupsPath+"/"+groupID.String()+"/archive", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func seedOrganization(t *testing.T, db *gorm.DB, owner string) orgModels.Organization {
	t.Helper()
	org := orgModels.Organization{
		Name: "org-" + uuid.NewString()[:8], DisplayName: "Org", OwnerUserID: owner,
		OrganizationType: orgModels.OrgTypeTeam, IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata", "AllowedBackends", "Members", "Groups").Create(&org).Error)
	return org
}

func seedOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role orgModels.OrganizationMemberRole, active bool) {
	t.Helper()
	member := orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: userID, Role: role, JoinedAt: time.Now(), IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)
	// Create rewrites an explicit false through the gorm default:true tag, so a
	// stood-down membership is written in a second step.
	if !active {
		require.NoError(t, db.Model(&member).Update("is_active", false).Error)
	}
}

func seedClass(t *testing.T, db *gorm.DB, name, owner string, orgID *uuid.UUID) groupModels.ClassGroup {
	t.Helper()
	group := groupModels.ClassGroup{
		Name: name, DisplayName: name, OwnerUserID: owner, OrganizationID: orgID, MaxMembers: 50,
	}
	require.NoError(t, db.Omit("Metadata", "Members", "SubGroups", "ParentGroup").Create(&group).Error)
	return group
}

func seedClassMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) {
	t.Helper()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: userID, Role: role, JoinedAt: time.Now(), IsActive: true, InvitedBy: "seed",
	}).Error)
}

func seedPrivateScenario(t *testing.T, db *gorm.DB) scenarioModels.Scenario {
	t.Helper()
	scenario := scenarioModels.Scenario{
		Name: "private-" + uuid.NewString()[:8], Title: "Private", InstanceType: "M", CreatedByID: "author", IsPublic: false,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&scenarioModels.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Step"}).Error)
	return scenario
}

func seedAssignment(t *testing.T, db *gorm.DB, scenarioID, groupID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.Create(&scenarioModels.ScenarioAssignment{
		ScenarioID: scenarioID, GroupID: &groupID, Scope: "group", CreatedByID: "teacher", IsActive: true,
	}).Error)
}

func seedTerminal(t *testing.T, db *gorm.DB, userID string) terminalModels.Terminal {
	t.Helper()
	key := terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + userID, KeyName: "k", IsActive: true, MaxSessions: 5}
	require.NoError(t, db.Create(&key).Error)
	terminal := terminalModels.Terminal{
		SessionID: "session-" + uuid.NewString(), UserID: userID, State: terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour), UserTerminalKeyID: key.ID, InstanceType: "ubuntu", MachineSize: "M",
	}
	require.NoError(t, db.Create(&terminal).Error)
	return terminal
}

func reloadClass(t *testing.T, db *gorm.DB, id uuid.UUID) groupModels.ClassGroup {
	t.Helper()
	var row groupModels.ClassGroup
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	return row
}

// afterArchiveWitness records every AfterArchive context the ClassGroup hooks
// deliver, so a test can prove an archive went THROUGH the hooks.
type afterArchiveWitness struct{ seen []uuid.UUID }

func (h *afterArchiveWitness) GetName() string                { return "class_archive_witness" }
func (h *afterArchiveWitness) GetEntityName() string          { return "ClassGroup" }
func (h *afterArchiveWitness) GetHookTypes() []hooks.HookType { return []hooks.HookType{hooks.AfterArchive} }
func (h *afterArchiveWitness) IsEnabled() bool                { return true }
func (h *afterArchiveWitness) GetPriority() int               { return 100 }
func (h *afterArchiveWitness) Execute(ctx *hooks.HookContext) error {
	if group, ok := ctx.NewEntity.(*groupModels.ClassGroup); ok {
		h.seen = append(h.seen, group.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Who may archive
// ---------------------------------------------------------------------------

func TestArchiveClass_AllowedForGroupOwner_ManagerAndOrgManager_RefusedForAMember(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org := seedOrganization(t, env.db, "org-owner")
	seedOrgMember(t, env.db, org.ID, "org-manager", orgModels.OrgRoleManager, true)
	group := seedClass(t, env.db, "promo", "teacher", &org.ID)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	seedClassMember(t, env.db, group.ID, "assistant", groupModels.GroupMemberRoleManager)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)

	archivePath := classGroupsPath + "/" + group.ID.String() + "/archive"
	unarchivePath := classGroupsPath + "/" + group.ID.String() + "/unarchive"

	assert.NotContains(t, ems.GlobalEntityRegistrationService.ArchivableEntitiesWithoutBeforeArchiveHook(), "ClassGroup",
		"the write-authorization hook must guard BeforeArchive, or the startup check would warn")

	env.as("student")
	rec := env.do(http.MethodPost, archivePath, nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Nil(t, reloadClass(t, env.db, group.ID).ArchivedAt, "a refused archive must leave the row untouched")

	for _, allowed := range []string{"teacher", "assistant", "org-manager"} {
		env.as(allowed)
		rec = env.do(http.MethodPost, archivePath, nil)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", allowed, rec.Body.String())
		var out dto.GroupOutput
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		require.NotNil(t, out.ArchivedAt, "%s: response must carry archived_at", allowed)
		assert.False(t, out.IsActive, "%s: the derived is_active must follow archived_at", allowed)
		require.NotNil(t, reloadClass(t, env.db, group.ID).ArchivedAt)

		env.as("student")
		rec = env.do(http.MethodPost, unarchivePath, nil)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

		env.as(allowed)
		rec = env.do(http.MethodPost, unarchivePath, nil)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", allowed, rec.Body.String())
		assert.Nil(t, reloadClass(t, env.db, group.ID).ArchivedAt, "%s: unarchive must clear the stamp", allowed)
	}
}

// ---------------------------------------------------------------------------
// What an archived class refuses
// ---------------------------------------------------------------------------

func TestArchivedClass_RefusesANewMember(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	env.as("teacher")
	env.archive(t, group.ID)

	rec := env.do(http.MethodPost, "/api/v1/group-members", dto.CreateGroupMemberInput{
		GroupID: group.ID, UserID: "late-student", Role: groupModels.GroupMemberRoleMember,
	})
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), groupModels.ErrClassArchived.Error())

	var count int64
	env.db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ?", group.ID, "late-student").Count(&count)
	assert.Zero(t, count, "the refused membership must not be written")
}

func TestArchivedClass_RefusesANewAssignment(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	scenario := seedPrivateScenario(t, env.db)
	env.as("teacher")
	env.archive(t, group.ID)

	rec := env.do(http.MethodPost, "/api/v1/scenario-assignments", map[string]any{
		"scenario_id": scenario.ID.String(), "group_id": group.ID.String(), "scope": "group",
	})
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), groupModels.ErrClassArchived.Error())

	var count int64
	env.db.Model(&scenarioModels.ScenarioAssignment{}).Where("group_id = ?", group.ID).Count(&count)
	assert.Zero(t, count, "the refused assignment must not be written")
}

func TestArchivedClass_RefusesBulkStart(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)
	scenario := seedPrivateScenario(t, env.db)
	env.as("teacher")
	env.archive(t, group.ID)

	svc := scenarioServices.NewTeacherDashboardService(env.db, nil, nil)
	_, err := svc.BulkStartScenario(group.ID, scenario.ID, scenarioServices.ScenarioProvisioning{}, 60, "teacher")
	require.ErrorIs(t, err, groupModels.ErrClassArchived)

	var sessions int64
	env.db.Model(&scenarioModels.ScenarioSession{}).Count(&sessions)
	assert.Zero(t, sessions, "no session may be created for an archived class")
}

func TestArchivedClass_AssignmentNoLongerAuthorisesALaunch(t *testing.T) {
	env := setupClassArchiveEnv(t)
	// A tt-backend nobody answers on: the launch must be refused BEFORE any
	// provisioning call, so this address is never contacted on the archived
	// path. On the open path it fails the request later, which is not a 403.
	deadBackend := httptest.NewServer(http.NotFoundHandler())
	deadBackend.Close()
	t.Setenv("TERMINAL_TRAINER_URL", deadBackend.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)
	scenario := seedPrivateScenario(t, env.db)
	seedAssignment(t, env.db, scenario.ID, group.ID)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "student")
		c.Set("userRoles", []string{access.RoleMember})
		c.Next()
	})
	router.POST("/api/v1/scenario-sessions/launch", scenarioRoutes.NewScenarioLaunchController(env.db).LaunchScenario)
	launch := func() *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"scenario_id": scenario.ID.String()})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/launch", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := launch()
	require.NotEqual(t, http.StatusForbidden, rec.Code, "precondition: the assignment on the open class authorises the learner: %s", rec.Body.String())

	env.as("teacher")
	env.archive(t, group.ID)

	rec = launch()
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// ---------------------------------------------------------------------------
// What an archived class still shows
// ---------------------------------------------------------------------------

func TestArchivedClass_TeacherStillReadsRosterAndSessions(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)
	scenario := seedPrivateScenario(t, env.db)
	seedAssignment(t, env.db, scenario.ID, group.ID)
	require.NoError(t, env.db.Create(&scenarioModels.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student", Status: "completed", StartedAt: time.Now(),
	}).Error)
	env.as("teacher")
	env.archive(t, group.ID)

	rec := env.do(http.MethodGet, classGroupsPath+"/"+group.ID.String(), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out dto.GroupOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotNil(t, out.ArchivedAt)
	require.NotNil(t, out.Members, "the roster of an archived class is evidence and stays readable")
	assert.Len(t, *out.Members, 2)

	progress, err := scenarioServices.NewTeacherDashboardService(env.db, nil, nil).GetGroupAssignmentsProgress(group.ID)
	require.NoError(t, err)
	assert.Len(t, progress, 1, "the teacher progress view of an archived class still answers")
}

func TestArchivedClass_StaysOnTheTeacherConsoleFlagged(t *testing.T) {
	env := setupClassArchiveEnv(t)
	open := seedClass(t, env.db, "a-open", "teacher", nil)
	archived := seedClass(t, env.db, "b-archived", "teacher", nil)
	later := seedClass(t, env.db, "c-open", "teacher", nil)
	env.as("teacher")
	env.archive(t, archived.ID)

	items, err := scenarioServices.NewTeacherDashboardService(env.db, nil, nil).GetManagedGroupsOverview("teacher")
	require.NoError(t, err)
	require.Len(t, items, 3, "an archived class is listed, not hidden")

	assert.Equal(t, []uuid.UUID{open.ID, later.ID, archived.ID},
		[]uuid.UUID{items[0].GroupID, items[1].GroupID, items[2].GroupID},
		"open classes first, by display name, archived ones after")
	assert.Nil(t, items[0].ArchivedAt)
	require.NotNil(t, items[2].ArchivedAt)
	assert.False(t, items[2].IsActive, "the transitional is_active follows archived_at")
}

// ---------------------------------------------------------------------------
// What an archived class no longer grants
// ---------------------------------------------------------------------------

func TestArchivedClass_GrantsNoSupervisionAccess(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)
	terminal := seedTerminal(t, env.db, "student")

	svc := terminalServices.NewTerminalTrainerService(env.db)
	allowed, err := svc.HasTerminalAccess(terminal.ID.String(), "teacher")
	require.NoError(t, err)
	require.True(t, allowed, "precondition: the owner of an open class supervises its learners")

	env.as("teacher")
	env.archive(t, group.ID)

	allowed, err = svc.HasTerminalAccess(terminal.ID.String(), "teacher")
	require.NoError(t, err)
	assert.False(t, allowed, "an archived class grants no supervision")
}

func TestArchivedClass_GrantsNoConsentPolicy(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	handled := true
	require.NoError(t, env.db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).
		Update("recording_consent_handled", &handled).Error)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)

	svc := terminalServices.NewTerminalTrainerService(env.db)
	consent, source, err := svc.GetUserConsentStatus("student")
	require.NoError(t, err)
	require.True(t, consent, "precondition: the open class carries the consent policy")
	require.Equal(t, "group", source)

	env.as("teacher")
	env.archive(t, group.ID)

	consent, _, err = svc.GetUserConsentStatus("student")
	require.NoError(t, err)
	assert.False(t, consent, "an archived class no longer answers for the learner's consent")
}

// ---------------------------------------------------------------------------
// Archive preview
// ---------------------------------------------------------------------------

func TestArchivePreview_ListsMembersWithTheirOtherActiveClassCount(t *testing.T) {
	env := setupClassArchiveEnv(t)
	origUsers := groupRoutes.ListCasdoorUsers
	groupRoutes.ListCasdoorUsers = func() ([]*casdoorsdk.User, error) {
		return []*casdoorsdk.User{
			{Id: "alice", Name: "alice", DisplayName: "Alice", Email: "alice@example.org"},
			{Id: "bob", Name: "bob", DisplayName: "Bob", Email: "bob@example.org"},
			{Id: "carol", Name: "carol", DisplayName: "Carol", Email: "carol@example.org"},
		}, nil
	}
	t.Cleanup(func() { groupRoutes.ListCasdoorUsers = origUsers })

	org := seedOrganization(t, env.db, "org-owner")
	require.NoError(t, env.db.Model(&org).Update("retention_days", 30).Error)
	seedOrgMember(t, env.db, org.ID, "alice", orgModels.OrgRoleMember, true)
	seedOrgMember(t, env.db, org.ID, "bob", orgModels.OrgRoleMember, false)
	seedOrgMember(t, env.db, org.ID, "carol", orgModels.OrgRoleMember, false)
	require.NoError(t, env.db.Model(&orgModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", org.ID, "carol").Update("left_at", time.Now()).Error)
	closing := seedClass(t, env.db, "closing", "teacher", &org.ID)
	other := seedClass(t, env.db, "other", "teacher", &org.ID)
	archivedOther := seedClass(t, env.db, "archived-other", "teacher", &org.ID)
	elsewhere := seedClass(t, env.db, "elsewhere", "teacher", nil)
	seedClassMember(t, env.db, closing.ID, "alice", groupModels.GroupMemberRoleMember)
	seedClassMember(t, env.db, closing.ID, "bob", groupModels.GroupMemberRoleMember)
	seedClassMember(t, env.db, closing.ID, "carol", groupModels.GroupMemberRoleMember)
	// alice continues in `other`; her archived class and the class outside the
	// org must not count.
	seedClassMember(t, env.db, other.ID, "alice", groupModels.GroupMemberRoleMember)
	seedClassMember(t, env.db, archivedOther.ID, "alice", groupModels.GroupMemberRoleMember)
	seedClassMember(t, env.db, elsewhere.ID, "alice", groupModels.GroupMemberRoleMember)
	seedClassMember(t, env.db, elsewhere.ID, "bob", groupModels.GroupMemberRoleMember)
	env.as("teacher")
	env.archive(t, archivedOther.ID)

	previewPath := classGroupsPath + "/" + closing.ID.String() + "/archive-preview"

	env.as("stranger")
	rec := env.do(http.MethodGet, previewPath, nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, "only someone who may archive the class may preview it")

	env.as("teacher")
	rec = env.do(http.MethodGet, previewPath, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var preview dto.ArchivePreviewOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.Len(t, preview.Members, 3)
	assert.Equal(t, 30, preview.RetentionDays, "the organization's own retention period")

	byUser := map[string]dto.ArchivePreviewMember{}
	for _, m := range preview.Members {
		byUser[m.UserID] = m
	}
	assert.Equal(t, 1, byUser["alice"].OtherActiveClassesInOrg)
	assert.Equal(t, "alice@example.org", byUser["alice"].Email)
	assert.Equal(t, "Alice", byUser["alice"].DisplayName)
	assert.Equal(t, "member", byUser["alice"].Role)
	assert.Equal(t, "active", byUser["alice"].OrgMemberState)
	assert.Equal(t, 0, byUser["bob"].OtherActiveClassesInOrg)
	assert.Equal(t, "removed", byUser["bob"].OrgMemberState)
	assert.Equal(t, "offboarded", byUser["carol"].OrgMemberState, "a stood-down row with left_at is offboarded, not merely removed")
}

func TestArchivePreview_PersonalClassReportsThePlatformRetentionDefault(t *testing.T) {
	env := setupClassArchiveEnv(t)
	origUsers := groupRoutes.ListCasdoorUsers
	groupRoutes.ListCasdoorUsers = func() ([]*casdoorsdk.User, error) { return nil, nil }
	t.Cleanup(func() { groupRoutes.ListCasdoorUsers = origUsers })

	personal := seedClass(t, env.db, "personal", "teacher", nil)
	seedClassMember(t, env.db, personal.ID, "alice", groupModels.GroupMemberRoleMember)

	env.as("teacher")
	rec := env.do(http.MethodGet, classGroupsPath+"/"+personal.ID.String()+"/archive-preview", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var preview dto.ArchivePreviewOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	assert.Equal(t, config.DefaultRetentionDays(), preview.RetentionDays, "no organization means the platform default")
	require.Len(t, preview.Members, 1)
	assert.Equal(t, "none", preview.Members[0].OrgMemberState)
}

// ---------------------------------------------------------------------------
// Auto-archive on expiry
// ---------------------------------------------------------------------------

func TestClassAutoArchive_ArchivesExpiredClassesThroughTheHooks(t *testing.T) {
	env := setupClassArchiveEnv(t)
	witness := &afterArchiveWitness{}
	require.NoError(t, hooks.GlobalHookRegistry.RegisterHook(witness))

	yesterday := time.Now().Add(-24 * time.Hour)
	tomorrow := time.Now().Add(24 * time.Hour)
	expired := seedClass(t, env.db, "expired", "teacher", nil)
	require.NoError(t, env.db.Model(&groupModels.ClassGroup{}).Where("id = ?", expired.ID).Update("expires_at", yesterday).Error)
	running := seedClass(t, env.db, "running", "teacher", nil)
	require.NoError(t, env.db.Model(&groupModels.ClassGroup{}).Where("id = ?", running.ID).Update("expires_at", tomorrow).Error)
	open := seedClass(t, env.db, "open-ended", "teacher", nil)
	alreadyArchived := seedClass(t, env.db, "already", "teacher", nil)
	require.NoError(t, env.db.Model(&groupModels.ClassGroup{}).Where("id = ?", alreadyArchived.ID).
		Updates(map[string]any{"expires_at": yesterday, "archived_at": yesterday}).Error)

	cron.ArchiveExpiredClasses(env.db)

	require.NotNil(t, reloadClass(t, env.db, expired.ID).ArchivedAt, "an expired class is archived within the hour")
	assert.Nil(t, reloadClass(t, env.db, running.ID).ArchivedAt)
	assert.Nil(t, reloadClass(t, env.db, open.ID).ArchivedAt, "no expiry means no auto-archive")
	assert.Equal(t, yesterday.Unix(), reloadClass(t, env.db, alreadyArchived.ID).ArchivedAt.Unix(), "an archived class keeps its original stamp")
	assert.Equal(t, []uuid.UUID{expired.ID}, witness.seen, "the archive must go through the ClassGroup hooks")
}

// ---------------------------------------------------------------------------
// Legacy data
// ---------------------------------------------------------------------------

func TestClassMigration_LegacyInactiveClassesBecomeArchived(t *testing.T) {
	db := classArchiveDB(t)
	// The production table still carries the orphaned is_active column.
	require.NoError(t, db.Exec("ALTER TABLE class_groups ADD COLUMN is_active boolean DEFAULT true").Error)

	legacyArchived := seedClass(t, db, "legacy-archived", "teacher", nil)
	legacyOpen := seedClass(t, db, "legacy-open", "teacher", nil)
	stamped := seedClass(t, db, "already-stamped", "teacher", nil)
	lastWeek := time.Now().Add(-7 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, db.Exec("UPDATE class_groups SET is_active = false, updated_at = ? WHERE id = ?", lastWeek, legacyArchived.ID).Error)
	require.NoError(t, db.Exec("UPDATE class_groups SET is_active = false, archived_at = ? WHERE id = ?", lastWeek.Add(time.Hour), stamped.ID).Error)

	initialization.MigrateInactiveClassesToArchived(db)

	migrated := reloadClass(t, db, legacyArchived.ID)
	require.NotNil(t, migrated.ArchivedAt)
	assert.Equal(t, lastWeek.Unix(), migrated.ArchivedAt.Unix(), "the stamp is the row's last update, the best evidence of when it was archived")
	assert.False(t, dto.GroupModelToGroupOutput(&migrated).IsActive, "the derived is_active reports it archived")
	assert.Nil(t, reloadClass(t, db, legacyOpen.ID).ArchivedAt)
	assert.Equal(t, lastWeek.Add(time.Hour).Unix(), reloadClass(t, db, stamped.ID).ArchivedAt.Unix(), "an existing stamp is never overwritten")

	// Running again is harmless, and so is running on a database without the column.
	initialization.MigrateInactiveClassesToArchived(db)
	require.NoError(t, db.Exec("ALTER TABLE class_groups DROP COLUMN is_active").Error)
	initialization.MigrateInactiveClassesToArchived(db)
}
