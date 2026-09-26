// tests/scenarios/scenarioFileRefScope_test.go
//
// A scenario can only reference its own files (issue #516).
//
// Steps and scenarios point at ProjectFile rows by id (verify / background /
// foreground script, text, hint; setup script, intro, finish). The session
// engine runs those scripts and the export returns their content, so an id
// is a read of the file behind it. Before #516 a manager could set any of
// those ids to any ProjectFile — another organisation's included — and run
// or export it.
//
// The rule, for everyone but platform administrators: on create and update,
// every non-nil file id must be
//
//	(a) unchanged from the stored value (the editor round-trips its ids), or
//	(b) a file already referenced by the same scenario (the scenario row or
//	    any of its steps), or
//	(c) a file whose ScenarioID is this scenario.
//
// Anything else is refused with 403 and nothing is written. The importer,
// seed and duplicate services write ids directly, without the entity hooks,
// and are covered by their own tests (scenario_duplication_test.go,
// scenarioExportImport_test.go).
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	"soli/formations/src/entityManagement/swagger"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

// setupFileRefRouter mounts Scenario and ScenarioStep the way production
// does — generated CRUD, live scenario hooks, Layer 2 enforcement.
func setupFileRefRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
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
	scenarioRegistration.RegisterScenarioStep(ems.GlobalEntityRegistrationService)
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

func sendJSON(t *testing.T, router *gin.Engine, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func createFile(t *testing.T, db *gorm.DB, name string, scenarioID *uuid.UUID) uuid.UUID {
	t.Helper()
	f := models.ProjectFile{
		Name:        name,
		ContentType: "script",
		Content:     "#!/bin/sh\necho " + name,
		StorageType: "database",
		ScenarioID:  scenarioID,
	}
	require.NoError(t, db.Create(&f).Error)
	return f.ID
}

// fileRefWorld is two organisations, each with one scenario.
//
//   - scenario A (org A, managed by "a-manager"): step `target` references
//     nothing; step `sibling` references `shared` in every file field; the
//     scenario row references `ownSetup` / `ownIntro` / `ownFinish`. `ownImage`
//     is linked to A by ScenarioID and referenced by nobody.
//   - scenario B (org B): a step referencing `foreign` in every field and the
//     scenario row referencing it too. `foreignImage` is linked to B by
//     ScenarioID. `orphan` belongs to no scenario at all.
//
// All but the image files carry no ScenarioID, like the files the importer
// and seed service write.
type fileRefWorld struct {
	scenarioA, scenarioB                  *models.Scenario
	target, sibling                       *models.ScenarioStep
	shared, ownSetup, ownIntro, ownFinish uuid.UUID
	ownImage, foreign, foreignImage       uuid.UUID
	orphan                                uuid.UUID
}

func buildFileRefWorld(t *testing.T, db *gorm.DB) *fileRefWorld {
	t.Helper()
	w := &fileRefWorld{}

	orgA := createTestOrg(t, db, "a-owner")
	addOrgMember(t, db, orgA, "a-manager", orgModels.OrgRoleManager)
	orgB := createTestOrg(t, db, "b-owner")

	w.shared = createFile(t, db, "shared.sh", nil)
	w.ownSetup = createFile(t, db, "setup.sh", nil)
	w.ownIntro = createFile(t, db, "intro.md", nil)
	w.ownFinish = createFile(t, db, "finish.md", nil)
	w.foreign = createFile(t, db, "foreign-secret.sh", nil)
	w.orphan = createFile(t, db, "orphan.sh", nil)

	w.scenarioA = &models.Scenario{
		Name: "file-refs-a", Title: "A", InstanceType: "ubuntu:22.04", SourceType: "seed",
		CreatedByID: "a-owner", OrganizationID: &orgA,
		SetupScriptID: &w.ownSetup, IntroFileID: &w.ownIntro, FinishFileID: &w.ownFinish,
	}
	require.NoError(t, db.Create(w.scenarioA).Error)
	w.scenarioB = &models.Scenario{
		Name: "file-refs-b", Title: "B", InstanceType: "ubuntu:22.04", SourceType: "seed",
		CreatedByID: "b-owner", OrganizationID: &orgB,
		SetupScriptID: &w.foreign, IntroFileID: &w.foreign, FinishFileID: &w.foreign,
	}
	require.NoError(t, db.Create(w.scenarioB).Error)

	w.ownImage = createFile(t, db, "own.png", &w.scenarioA.ID)
	w.foreignImage = createFile(t, db, "foreign.png", &w.scenarioB.ID)

	w.target = &models.ScenarioStep{ScenarioID: w.scenarioA.ID, Order: 1, Title: "Target", StepType: "terminal"}
	require.NoError(t, db.Create(w.target).Error)
	w.sibling = &models.ScenarioStep{
		ScenarioID: w.scenarioA.ID, Order: 2, Title: "Sibling", StepType: "terminal",
		VerifyScriptID: &w.shared, BackgroundScriptID: &w.shared, ForegroundScriptID: &w.shared,
		TextFileID: &w.shared, HintFileID: &w.shared,
	}
	require.NoError(t, db.Create(w.sibling).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: w.scenarioB.ID, Order: 1, Title: "B step", StepType: "terminal",
		VerifyScriptID: &w.foreign, BackgroundScriptID: &w.foreign, ForegroundScriptID: &w.foreign,
		TextFileID: &w.foreign, HintFileID: &w.foreign,
	}).Error)
	return w
}

var stepFileFields = []struct {
	json string
	get  func(*models.ScenarioStep) *uuid.UUID
}{
	{"verify_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.VerifyScriptID }},
	{"background_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.BackgroundScriptID }},
	{"foreground_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.ForegroundScriptID }},
	{"text_file_id", func(s *models.ScenarioStep) *uuid.UUID { return s.TextFileID }},
	{"hint_file_id", func(s *models.ScenarioStep) *uuid.UUID { return s.HintFileID }},
}

var scenarioFileFields = []struct {
	json string
	get  func(*models.Scenario) *uuid.UUID
}{
	{"setup_script_id", func(s *models.Scenario) *uuid.UUID { return s.SetupScriptID }},
	{"intro_file_id", func(s *models.Scenario) *uuid.UUID { return s.IntroFileID }},
	{"finish_file_id", func(s *models.Scenario) *uuid.UUID { return s.FinishFileID }},
}

func reloadStep(t *testing.T, db *gorm.DB, id uuid.UUID) *models.ScenarioStep {
	t.Helper()
	var s models.ScenarioStep
	require.NoError(t, db.First(&s, "id = ?", id).Error)
	return &s
}

func reloadScenario(t *testing.T, db *gorm.DB, id uuid.UUID) *models.Scenario {
	t.Helper()
	var s models.Scenario
	require.NoError(t, db.First(&s, "id = ?", id).Error)
	return &s
}

// =============================================================================
// Refused: a file this scenario does not own
// =============================================================================

func TestStepFileRef_PatchWithAnotherScenariosFile_Forbidden(t *testing.T) {
	for _, field := range stepFileFields {
		t.Run(field.json, func(t *testing.T) {
			db := freshTestDB(t)
			w := buildFileRefWorld(t, db)
			router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

			for name, fileID := range map[string]uuid.UUID{
				"referenced by org B's scenario": w.foreign,
				"linked to org B's scenario":     w.foreignImage,
				"referenced by no scenario":      w.orphan,
			} {
				resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+w.target.ID.String(),
					map[string]any{field.json: fileID.String()})
				assert.Equal(t, http.StatusForbidden, resp.Code,
					"a file %s is not this scenario's to reference; body=%s", name, resp.Body.String())
				assert.Nil(t, field.get(reloadStep(t, db, w.target.ID)),
					"a refused PATCH must leave %s untouched (file %s)", field.json, name)
			}
		})
	}
}

func TestStepFileRef_CreateWithAnotherScenariosFile_Forbidden(t *testing.T) {
	for _, field := range stepFileFields {
		t.Run(field.json, func(t *testing.T) {
			db := freshTestDB(t)
			w := buildFileRefWorld(t, db)
			router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

			resp := sendJSON(t, router, http.MethodPost, "/scenario-steps", map[string]any{
				"scenario_id": w.scenarioA.ID.String(),
				"order":       3,
				"title":       "Smuggler",
				"step_type":   "terminal",
				field.json:    w.foreign.String(),
			})
			assert.Equal(t, http.StatusForbidden, resp.Code,
				"a new step cannot point at org B's file; body=%s", resp.Body.String())

			var count int64
			require.NoError(t, db.Model(&models.ScenarioStep{}).
				Where("scenario_id = ? AND title = ?", w.scenarioA.ID, "Smuggler").Count(&count).Error)
			assert.Zero(t, count, "a refused create must write no step")
		})
	}
}

func TestScenarioFileRef_PatchWithAnotherScenariosFile_Forbidden(t *testing.T) {
	for _, field := range scenarioFileFields {
		t.Run(field.json, func(t *testing.T) {
			db := freshTestDB(t)
			w := buildFileRefWorld(t, db)
			router := setupFileRefRouter(t, db, "a-manager", []string{"member"})
			before := *field.get(w.scenarioA)

			resp := sendJSON(t, router, http.MethodPatch, "/scenarios/"+w.scenarioA.ID.String(),
				map[string]any{field.json: w.foreign.String()})
			assert.Equal(t, http.StatusForbidden, resp.Code,
				"the scenario cannot point at org B's file; body=%s", resp.Body.String())

			after := field.get(reloadScenario(t, db, w.scenarioA.ID))
			require.NotNil(t, after)
			assert.Equal(t, before, *after, "a refused PATCH must leave %s untouched", field.json)
		})
	}
}

// =============================================================================
// Allowed: the scenario's own files
// =============================================================================

func TestStepFileRef_PatchResendingTheStoredId_Allowed(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

	// The editor sends back every id it loaded, changed or not.
	body := map[string]any{"title": "Sibling renamed"}
	for _, field := range stepFileFields {
		body[field.json] = w.shared.String()
	}
	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+w.sibling.ID.String(), body)
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	assert.Equal(t, "Sibling renamed", reloadStep(t, db, w.sibling.ID).Title)
}

func TestStepFileRef_PatchWithAFileOfTheSameScenario_Allowed(t *testing.T) {
	for _, field := range stepFileFields {
		t.Run(field.json, func(t *testing.T) {
			db := freshTestDB(t)
			w := buildFileRefWorld(t, db)
			router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

			for name, fileID := range map[string]uuid.UUID{
				"referenced by a sibling step":     w.shared,
				"referenced by the scenario row":   w.ownSetup,
				"linked to the scenario by its id": w.ownImage,
			} {
				resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+w.target.ID.String(),
					map[string]any{field.json: fileID.String()})
				require.Equal(t, http.StatusNoContent, resp.Code,
					"a file %s is the scenario's own; body=%s", name, resp.Body.String())
				got := field.get(reloadStep(t, db, w.target.ID))
				require.NotNil(t, got, "file %s", name)
				assert.Equal(t, fileID, *got, "file %s", name)
			}
		})
	}
}

// A JSON null never reaches the hook: the PATCH converter drops nil pointers,
// so null means "no change" — neither refused nor a way to clear the column.
func TestStepFileRef_PatchWithNull_IsIgnored(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

	body := map[string]any{}
	for _, field := range stepFileFields {
		body[field.json] = nil
	}
	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+w.sibling.ID.String(), body)
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	stored := reloadStep(t, db, w.sibling.ID)
	for _, field := range stepFileFields {
		got := field.get(stored)
		require.NotNil(t, got, "%s: null must leave the stored id alone", field.json)
		assert.Equal(t, w.shared, *got, "%s: null must leave the stored id alone", field.json)
	}
}

func TestStepFileRef_CreateWithAFileOfTheSameScenario_Allowed(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

	resp := sendJSON(t, router, http.MethodPost, "/scenario-steps", map[string]any{
		"scenario_id":      w.scenarioA.ID.String(),
		"order":            3,
		"title":            "Reuses the sibling's script",
		"step_type":        "terminal",
		"verify_script_id": w.shared.String(),
		"text_file_id":     w.ownImage.String(),
	})
	require.Equal(t, http.StatusCreated, resp.Code, "body=%s", resp.Body.String())
}

func TestScenarioFileRef_PatchWithItsOwnFiles_Allowed(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	router := setupFileRefRouter(t, db, "a-manager", []string{"member"})

	// Unchanged ids round-tripped by the editor.
	resp := sendJSON(t, router, http.MethodPatch, "/scenarios/"+w.scenarioA.ID.String(), map[string]any{
		"title":           "A renamed",
		"setup_script_id": w.ownSetup.String(),
		"intro_file_id":   w.ownIntro.String(),
		"finish_file_id":  w.ownFinish.String(),
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	// A file one of its steps references, and a file linked to it by id.
	resp = sendJSON(t, router, http.MethodPatch, "/scenarios/"+w.scenarioA.ID.String(), map[string]any{
		"setup_script_id": w.shared.String(),
		"intro_file_id":   w.ownImage.String(),
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	got := reloadScenario(t, db, w.scenarioA.ID)
	require.NotNil(t, got.SetupScriptID)
	assert.Equal(t, w.shared, *got.SetupScriptID)
}

// =============================================================================
// Platform administrators are not bound by the rule
// =============================================================================

func TestFileRef_AdministratorMayReferenceAnyFile(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	router := setupFileRefRouter(t, db, "platform-admin", []string{"administrator"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+w.target.ID.String(),
		map[string]any{"verify_script_id": w.foreign.String()})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	resp = sendJSON(t, router, http.MethodPatch, "/scenarios/"+w.scenarioA.ID.String(),
		map[string]any{"setup_script_id": w.orphan.String()})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
}

// =============================================================================
// A new scenario owns no file yet
// =============================================================================
//
// POST /organizations/:id/scenarios and POST /groups/:groupId/scenarios write
// the scenario directly, without the entity hooks. A scenario that does not
// exist yet references nothing, so any file id sent to them is someone
// else's file.

// createRoutes returns the two create paths for scenario A's organisation,
// with "a-manager" owning a class of it.
func createRoutes(t *testing.T, db *gorm.DB, w *fileRefWorld) map[string]string {
	t.Helper()
	orgA := *w.scenarioA.OrganizationID
	groupA := createTestGroupInOrg(t, db, orgA, "a-owner")
	addGroupMember(t, db, groupA, "a-manager", groupModels.GroupMemberRoleOwner)
	return map[string]string{
		"org create":   "/organizations/" + orgA.String() + "/scenarios",
		"group create": "/groups/" + groupA.String() + "/scenarios",
	}
}

func countScenariosNamed(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Scenario{}).Where("name = ?", name).Count(&n).Error)
	return n
}

func TestScenarioFileRef_CreateWithAnExistingFile_Forbidden(t *testing.T) {
	for _, field := range scenarioFileFields {
		t.Run(field.json, func(t *testing.T) {
			db := freshTestDB(t)
			w := buildFileRefWorld(t, db)
			routes := createRoutes(t, db, w)
			router := setupOrgTestRouterWithUserAndRoles(t, db, "a-manager", []string{"member"})

			for route, path := range routes {
				resp := sendJSON(t, router, http.MethodPost, path, map[string]any{
					"name":          "smuggler",
					"title":         "Smuggler",
					"instance_type": "ubuntu:22.04",
					field.json:      w.foreign.String(),
				})
				assert.Equal(t, http.StatusForbidden, resp.Code,
					"%s: a new scenario cannot point at org B's file; body=%s", route, resp.Body.String())
				assert.Zero(t, countScenariosNamed(t, db, "smuggler"),
					"%s: a refused create must write no scenario", route)
			}
		})
	}
}

func TestScenarioFileRef_CreateWithoutFiles_Allowed(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	routes := createRoutes(t, db, w)
	router := setupOrgTestRouterWithUserAndRoles(t, db, "a-manager", []string{"member"})

	for route, path := range routes {
		resp := sendJSON(t, router, http.MethodPost, path, map[string]any{
			"name":          "blank-" + route,
			"title":         "Blank",
			"instance_type": "ubuntu:22.04",
		})
		assert.Equal(t, http.StatusCreated, resp.Code, "%s: body=%s", route, resp.Body.String())
	}
}

func TestScenarioFileRef_CreateAsAdministratorWithAnyFile_Allowed(t *testing.T) {
	db := freshTestDB(t)
	w := buildFileRefWorld(t, db)
	routes := createRoutes(t, db, w)
	router := setupOrgTestRouterWithUserAndRoles(t, db, "platform-admin", []string{"administrator"})

	for route, path := range routes {
		resp := sendJSON(t, router, http.MethodPost, path, map[string]any{
			"name":            "admin-" + route,
			"title":           "Admin",
			"instance_type":   "ubuntu:22.04",
			"setup_script_id": w.foreign.String(),
		})
		assert.Equal(t, http.StatusCreated, resp.Code,
			"%s: administrators are not bound by the file rule; body=%s", route, resp.Body.String())
	}
}
