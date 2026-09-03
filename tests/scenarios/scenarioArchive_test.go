// tests/scenarios/scenarioArchive_test.go
//
// Archiving retires a scenario whose class results must stay readable. The
// promise has two halves and both are pinned here:
//
//   - nothing new — an archived scenario disappears from the assignment
//     picker, refuses new assignments, and refuses every launch path;
//   - nothing lost — the row survives, so past sessions keep resolving their
//     scenario, and a run already in flight when the archive happens is left
//     alone to finish.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/swagger"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

// setupArchiveRouter wires the platform archive/unarchive routes as production
// registers them, behind the same Layer 2 enforcement.
// setupArchiveRouter mounts the Scenario entity the way production does —
// generated CRUD and archive/unarchive actions, live scenario hooks, Layer 2
// enforcement — so the tests below exercise the framework's archive route and
// the ScenarioArchiveAuthorizationHook, not a hand-written controller.
func setupArchiveRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()

	mockEnforcer := mocks.NewMockEnforcer()
	mockEnforcer.EnforceFunc = func(params ...any) (bool, error) { return true, nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer

	originalSvc := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	scenarioController.RegisterScenarioPermissions(mockEnforcer)
	scenarioRegistration.RegisterScenario(ems.GlobalEntityRegistrationService)
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.DisableAllHooks(false)
	scenarioHooks.InitScenarioHooks(db)

	t.Cleanup(func() {
		hooks.GlobalHookRegistry.ClearAllHooks()
		ems.GlobalEntityRegistrationService = originalSvc
		casdoor.Enforcer = originalEnforcer
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	passthrough := func(c *gin.Context) { c.Next() }
	swagger.NewSwaggerRouteGenerator(db).RegisterDocumentedRoutes(api, passthrough, passthrough)
	return r
}

func postArchive(t *testing.T, router *gin.Engine, scenarioID uuid.UUID, action string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenarios/"+scenarioID.String()+"/"+action, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestArchiveScenario_StampsArchivedAtAndUnarchiveClearsIt(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "archive-roundtrip")

	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})

	w := postArchive(t, router, scenario.ID, "archive")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.NotEmpty(t, out["archived_at"], "the response must report the archive stamp")

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	require.True(t, reloaded.IsArchived())

	w = postArchive(t, router, scenario.ID, "unarchive")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// A fresh struct on purpose: GORM's First leaves a pointer field holding
	// its previous value when the column comes back NULL.
	var afterUnarchive models.Scenario
	require.NoError(t, db.First(&afterUnarchive, "id = ?", scenario.ID).Error)
	assert.False(t, afterUnarchive.IsArchived(),
		"unarchive must clear archived_at, not leave the old stamp behind")
}

func TestArchiveScenario_RefusedForSomeoneWhoCannotManageIt(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "plain-member", orgModels.OrgRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "archive-authz")

	router := setupArchiveRouter(t, db, "plain-member", []string{"member"})

	w := postArchive(t, router, scenario.ID, "archive")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"archiving retires a scenario for a whole org — it follows the same "+
			"manage rule as PATCH and DELETE. Body: %s", w.Body.String())
}

// Archiving deliberately follows CanManageScenario, the rule that already
// guards PATCH, DELETE and step editing. Its group clause is wider than it
// looks: a manager of any group the scenario is assigned to can retire it for
// the whole organization, not just for their class. That reach is intended —
// decided 2026-08-20 — so it is pinned here rather than left to be discovered.
func TestArchiveScenario_AllowedForAManagerOfAnAssignedGroup(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	scenario := createTestScenarioForOrg(t, db, orgID, "archive-group-manager")

	// The caller manages one class the scenario reaches. They neither created
	// it nor belong to the organization that owns it.
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner")
	addGroupMember(t, db, groupID, "class-teacher", groupModels.GroupMemberRoleManager)
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")

	router := setupArchiveRouter(t, db, "class-teacher", []string{"member"})

	w := postArchive(t, router, scenario.ID, "archive")
	require.Equal(t, http.StatusOK, w.Code,
		"a manager of an assigned group archives on the same rule that already "+
			"lets them PATCH and DELETE the scenario. Body: %s", w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	assert.True(t, reloaded.IsArchived())
}

func TestArchivedScenario_LeavesTheAssignmentPicker(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	groupID := createTestGroupInOrg(t, db, orgID, "org-manager")
	addGroupMember(t, db, groupID, "org-manager", groupModels.GroupMemberRoleOwner)
	scenario := createTestScenarioForOrg(t, db, orgID, "picker-archive")

	router := setupOrgTestRouterWithUserAndRoles(t, db, "org-manager", []string{"member"})

	listNames := func() []string {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/groups/"+groupID.String()+"/scenarios", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var items []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item["name"].(string))
		}
		return names
	}

	require.Contains(t, listNames(), "picker-archive",
		"sanity: an active org scenario is offered to its groups")

	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).Update("archived_at", now).Error)

	assert.NotContains(t, listNames(), "picker-archive",
		"an archived scenario must no longer be offered for assignment")
}

func TestArchivedScenario_LeavesThePickerWhenItGotThereByAssignment(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	groupID := createTestGroupInOrg(t, db, orgID, "org-manager")
	addGroupMember(t, db, groupID, "org-manager", groupModels.GroupMemberRoleOwner)

	// No organization_id: this scenario reaches the picker through its group
	// assignment, which is the other of the two branches that build the list.
	scenario := createTestScenarioNoOrg(t, db, "picker-assigned-archive")
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")

	router := setupOrgTestRouterWithUserAndRoles(t, db, "org-manager", []string{"member"})

	listNames := func() []string {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/groups/"+groupID.String()+"/scenarios", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var items []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item["name"].(string))
		}
		return names
	}

	require.Contains(t, listNames(), "picker-assigned-archive",
		"sanity: an assigned scenario is listed for its group")

	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).Update("archived_at", now).Error)

	assert.NotContains(t, listNames(), "picker-assigned-archive",
		"the assignment survives for the results views, but the scenario must "+
			"no longer be offered for a new assignment")
}

func TestArchivedScenario_RefusesANewAssignment(t *testing.T) {
	db := freshTestDB(t)
	scenario := createTestScenarioNoOrg(t, db, "assign-archived")
	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).Update("archived_at", now).Error)

	groupID := uuid.New()
	hook := scenarioHooks.NewScenarioAssignmentArchivedHook(db)
	err := hook.Execute(&hooks.HookContext{
		HookType:  hooks.BeforeCreate,
		UserID:    "any-admin",
		NewEntity: &models.ScenarioAssignment{ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group"},
	})

	assert.ErrorIs(t, err, models.ErrScenarioArchived,
		"assigning an archived scenario would make it reachable again — the "+
			"refusal is a state rule, so it applies to administrators too")
}

func TestActiveScenario_StillAcceptsAnAssignment(t *testing.T) {
	db := freshTestDB(t)
	scenario := createTestScenarioNoOrg(t, db, "assign-active")

	groupID := uuid.New()
	hook := scenarioHooks.NewScenarioAssignmentArchivedHook(db)
	err := hook.Execute(&hooks.HookContext{
		HookType:  hooks.BeforeCreate,
		UserID:    "any-admin",
		NewEntity: &models.ScenarioAssignment{ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group"},
	})

	assert.NoError(t, err)
}

func TestArchivedScenario_RefusesBulkStartForAClass(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	groupID := createTestGroupInOrg(t, db, orgID, "org-manager")
	addGroupMember(t, db, groupID, "learner-1", groupModels.GroupMemberRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "bulk-archived")
	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).Update("archived_at", now).Error)

	svc := services.NewTeacherDashboardService(db, newCapturingTTService(),
		services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{}))
	_, err := svc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{}, 0, "org-manager")

	assert.ErrorIs(t, err, models.ErrScenarioArchived,
		"bulk-start is a launch path like any other")
}

func TestArchivedScenario_RefusesANewLaunch(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-archived-user"
	seedActivePlan(t, db, userID)
	seedUserTTKey(t, db, userID)

	scenario := createTestScenarioNoOrg(t, db, "launch-archived")
	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).Update("archived_at", now).Error)

	// Healthy host: the refusal must come from the archive gate, not capacity.
	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	router := setupLaunchRouterWithProdMiddleware(t, db, userID)
	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String()})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code,
		"an archived scenario must not be relaunchable, admin bypass included. "+
			"Got %d. Body: %s", w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "archived")
}

func TestArchivingKeepsPastResultsAndRunningSessionsIntact(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "history-archive")

	grade := 17.5
	completedAt := time.Now().Add(-time.Hour)
	finished := &models.ScenarioSession{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID:  scenario.ID,
		UserID:      "learner-past",
		Status:      "completed",
		StartedAt:   time.Now().Add(-2 * time.Hour),
		CompletedAt: &completedAt,
		Grade:       &grade,
	}
	require.NoError(t, db.Create(finished).Error)

	running := &models.ScenarioSession{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		ScenarioID: scenario.ID,
		UserID:     "learner-running",
		Status:     "active",
		StartedAt:  time.Now(),
	}
	require.NoError(t, db.Create(running).Error)

	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})
	require.Equal(t, http.StatusOK, postArchive(t, router, scenario.ID, "archive").Code)

	var reloadedFinished models.ScenarioSession
	require.NoError(t, db.Preload("Scenario").
		First(&reloadedFinished, "id = ?", finished.ID).Error)
	assert.Equal(t, scenario.Title, reloadedFinished.Scenario.Title,
		"a past result must keep naming the scenario it was earned on — this "+
			"is the whole reason archiving exists instead of deleting")
	require.NotNil(t, reloadedFinished.Grade)
	assert.Equal(t, grade, *reloadedFinished.Grade)

	var reloadedRunning models.ScenarioSession
	require.NoError(t, db.First(&reloadedRunning, "id = ?", running.ID).Error)
	assert.Equal(t, "active", reloadedRunning.Status,
		"archiving stops new runs only — a session already in flight finishes")
}

func TestArchiveScenario_ViaGeneratedAction_RefusedForSomeoneWhoCannotManageIt(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "plain-member", orgModels.OrgRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "archive-generated-authz")

	router := setupArchiveRouter(t, db, "plain-member", []string{"member"})

	w := postArchive(t, router, scenario.ID, "archive")
	require.Equal(t, http.StatusForbidden, w.Code,
		"the generated action is open to every member at Layer 2; the "+
			"BeforeArchive hook must refuse with the permission error so the "+
			"route answers 403, not the 409 kept for state refusals. Body: %s",
		w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	assert.False(t, reloaded.IsArchived(), "a refused archive must leave the row untouched")
}

func TestScenarioList_HidesArchivedByDefault_AndIncludeArchivedShowsThem(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner")
	addOrgMember(t, db, orgID, "org-manager", orgModels.OrgRoleManager)
	createTestScenarioForOrg(t, db, orgID, "list-active")
	archived := createTestScenarioForOrg(t, db, orgID, "list-archived")
	now := time.Now()
	require.NoError(t, db.Model(&models.Scenario{}).
		Where("id = ?", archived.ID).Update("archived_at", now).Error)

	router := setupArchiveRouter(t, db, "org-manager", []string{"member"})

	listNames := func(query string) []string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var body struct {
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		names := make([]string, 0, len(body.Data))
		for _, item := range body.Data {
			names = append(names, item["name"].(string))
		}
		return names
	}

	assert.ElementsMatch(t, []string{"list-active"}, listNames(""),
		"the generic list hides archived scenarios unless asked")
	assert.ElementsMatch(t, []string{"list-active", "list-archived"}, listNames("?include_archived=true"),
		"the admin scenarios page relies on include_archived to offer Restore")
}

func TestPermissionSetup_ScenarioArchiveRoute_DeclaredOnce(t *testing.T) {
	db := freshTestDB(t)
	setupArchiveRouter(t, db, "anyone", []string{"member"})

	declarations := map[string][]string{}
	for _, category := range access.RouteRegistry.GetReference().Categories {
		for _, route := range category.Routes {
			key := route.Method + " " + route.Path
			declarations[key] = append(declarations[key], category.Name)
		}
	}
	for _, action := range []string{"archive", "unarchive"} {
		key := http.MethodPost + " /api/v1/scenarios/:id/" + action
		assert.Equal(t, []string{"Scenario"}, declarations[key],
			"%s must be declared exactly once, by the generated action — a "+
				"second declaration in scenarios/routes/permissions.go would "+
				"be a parallel rule waiting to drift", key)
	}
}
