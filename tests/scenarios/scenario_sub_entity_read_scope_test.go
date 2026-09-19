package scenarios_test

// Follow-up to #294 for the scenario sub-entities a non-manager could still
// read through the generic routes:
//
//   - ScenarioTranslation / ScenarioStepTranslation had no redactor at all, so
//     translated hint_content (which the step redactor strips) leaked in every
//     locale but the original.
//   - ScenarioStep kept text_content for non-managers, and GET /scenario-steps
//     listed every step of every scenario.
//   - GET /scenario-assignments listed every class's assignments to everyone.
//
// Contract: translations are stripped to their ids for anyone who cannot
// manage the parent scenario; steps and assignments are listed only where the
// caller can manage the scenario (steps) or the group / org (assignments).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	entityManagementController "soli/formations/src/entityManagement/routes"
	ems "soli/formations/src/entityManagement/entityManagementService"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

func setupSubEntityReadTest(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	originalSvc := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenario(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioStep(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioTranslation(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioStepTranslation(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioAssignment(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService = originalSvc })

	gen := entityManagementController.NewGenericController(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	for _, route := range []string{"/scenario-steps", "/scenario-translations", "/scenario-step-translations", "/scenario-assignments"} {
		api.GET(route, gen.GetEntities)
		api.GET(route+"/:id", gen.GetEntity)
	}
	return r
}

func getJSON(t *testing.T, r *gin.Engine, path string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1"+path, nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "GET %s body=%s", path, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func listData(t *testing.T, r *gin.Engine, path string) []map[string]any {
	t.Helper()
	raw, _ := getJSON(t, r, path)["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]any))
	}
	return out
}

type subEntityFixture struct {
	scenario     *models.Scenario // private, org-less, created by "creator"
	step         models.ScenarioStep
	groupID      uuid.UUID
	groupManager string
}

func buildSubEntityFixture(t *testing.T, db *gorm.DB) subEntityFixture {
	t.Helper()
	f := subEntityFixture{groupManager: "group-manager"}
	f.scenario = buildLeakyScenario(t, db, "sub-entity-scenario", "creator", nil)
	f.step = f.scenario.Steps[0]
	f.groupID = makeGroupWithOwner(t, db, f.groupManager)

	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: f.scenario.ID, Locale: "fr", Title: "Titre secret", IntroText: "Intro FR",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: f.step.ID, Locale: "fr", Title: "Étape", TextContent: "Texte FR", HintContent: "INDICE-SECRET",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: f.scenario.ID, Scope: "group", GroupID: &f.groupID, CreatedByID: f.groupManager, IsActive: true,
	}).Error)
	return f
}

// --- translations -----------------------------------------------------------

func TestScenarioTranslations_Outsider_StrippedToIds(t *testing.T) {
	db := freshTestDB(t)
	f := buildSubEntityFixture(t, db)
	// Public so the parent scenario is listable at all; the content must still
	// be hidden, since public means launchable, not readable in the editor.
	require.NoError(t, db.Model(f.scenario).Update("is_public", true).Error)
	r := setupSubEntityReadTest(t, db, "outsider", []string{"member"})

	for _, path := range []string{"/scenario-translations", "/scenario-step-translations"} {
		rows := listData(t, r, path)
		require.Len(t, rows, 1, path)
		for _, field := range []string{"title", "description", "intro_text", "text_content", "hint_content"} {
			require.Empty(t, rows[0][field], "%s must not leak %s", path, field)
		}
		require.NotEmpty(t, rows[0]["locale"], "locale stays: it says a translation exists")
	}
}

func TestScenarioTranslations_Creator_FullContent(t *testing.T) {
	db := freshTestDB(t)
	f := buildSubEntityFixture(t, db)
	r := setupSubEntityReadTest(t, db, f.scenario.CreatedByID, []string{"member"})

	rows := listData(t, r, "/scenario-step-translations")
	require.Len(t, rows, 1)
	require.Equal(t, "INDICE-SECRET", rows[0]["hint_content"])

	translationID := listData(t, r, "/scenario-translations")[0]["id"].(string)
	require.Equal(t, "Titre secret", getJSON(t, r, "/scenario-translations/"+translationID)["title"])
}

// --- steps ------------------------------------------------------------------

func TestScenarioSteps_Outsider_NoTextContent_AndPrivateStepsUnlisted(t *testing.T) {
	db := freshTestDB(t)
	f := buildSubEntityFixture(t, db)
	r := setupSubEntityReadTest(t, db, "outsider", []string{"member"})

	require.Empty(t, listData(t, r, "/scenario-steps"), "steps of a private scenario are not listed")

	require.NoError(t, db.Model(f.scenario).Update("is_public", true).Error)
	rows := listData(t, r, "/scenario-steps")
	require.Len(t, rows, 1)
	require.Empty(t, rows[0]["text_content"], "step body is editor content, not catalogue metadata")
	require.Empty(t, rows[0]["hint_content"])
}

func TestScenarioSteps_Creator_ListsAndReadsFull(t *testing.T) {
	db := freshTestDB(t)
	f := buildSubEntityFixture(t, db)
	r := setupSubEntityReadTest(t, db, f.scenario.CreatedByID, []string{"member"})

	rows := listData(t, r, "/scenario-steps")
	require.Len(t, rows, 1)
	require.Equal(t, "Find the flag", rows[0]["text_content"])
}

// --- assignments ------------------------------------------------------------

func TestScenarioAssignments_ListScopedToManagedGroups(t *testing.T) {
	db := freshTestDB(t)
	f := buildSubEntityFixture(t, db)
	// A second scenario by the same creator, assigned to a class the fixture's
	// group manager has nothing to do with. (Assigning the SAME scenario would
	// make that manager a manager of the scenario, and rightly show both.)
	otherScenario := makeScenario(t, db, "other-scenario", f.scenario.CreatedByID, nil, false)
	otherGroup := makeGroupWithOwner(t, db, "other-manager")
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: otherScenario.ID, Scope: "group", GroupID: &otherGroup, CreatedByID: "other-manager", IsActive: true,
	}).Error)

	require.Len(t, listData(t, setupSubEntityReadTest(t, db, f.groupManager, []string{"member"}), "/scenario-assignments"), 1,
		"a group manager lists the assignments of their groups only")
	require.Len(t, listData(t, setupSubEntityReadTest(t, db, f.scenario.CreatedByID, []string{"member"}), "/scenario-assignments"), 2,
		"a scenario's manager lists every assignment of their scenarios")
	require.Empty(t, listData(t, setupSubEntityReadTest(t, db, "outsider", []string{"member"}), "/scenario-assignments"))
	require.Len(t, listData(t, setupSubEntityReadTest(t, db, "x", []string{"administrator"}), "/scenario-assignments"), 2)
}
