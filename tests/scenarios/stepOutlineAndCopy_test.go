package scenarios_test

// The step library: a teacher browsing another scenario sees what its steps
// are (GET /scenarios/:id/step-outline) without seeing how they are graded,
// and copies the steps they want into a scenario of theirs
// (POST /scenarios/:id/steps/copy). The copy is made server-side, so the
// scripts and answers it carries never pass through the client.

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
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

type stepLibraryFixture struct {
	org        uuid.UUID
	public     *models.Scenario // platform catalogue: a terminal step and a quiz step
	foreign    *models.Scenario // another organisation's private scenario
	target     *models.Scenario // the teacher's own scenario: two steps
	publicStep []models.ScenarioStep
}

const (
	libTeacher   = "lib-teacher"
	libColleague = "lib-colleague"
	libLearner   = "lib-learner"
)

func buildStepLibraryFixture(t *testing.T, db *gorm.DB) stepLibraryFixture {
	t.Helper()
	f := stepLibraryFixture{org: createTestOrg(t, db, "lib-owner")}
	addOrgMember(t, db, f.org, libTeacher, orgModels.OrganizationMemberRole(access.RoleTeacher))
	addOrgMember(t, db, f.org, libColleague, orgModels.OrganizationMemberRole(access.RoleTeacher))
	addOrgMember(t, db, f.org, libLearner, orgModels.OrgRoleMember)

	f.public = &models.Scenario{Name: "lib-public", Title: "Public lab", InstanceType: "debian", CreatedByID: "platform-admin", IsPublic: true}
	require.NoError(t, db.Create(f.public).Error)
	f.publicStep = []models.ScenarioStep{
		{ScenarioID: f.public.ID, Order: 0, Title: "Find the file", StepType: "flag", TextContent: "Look around /srv.",
			HintContent: "SECRET-HINT", VerifyScript: "SECRET-VERIFY", BackgroundScript: "SECRET-BACKGROUND",
			ForegroundScript: "SECRET-FOREGROUND", FlagPath: "/srv/SECRET-FLAG-PATH", FlagLevel: 2,
			IntroEffect: "matrix", IntroText: "Welcome"},
		{ScenarioID: f.public.ID, Order: 1, Title: "Check your knowledge", StepType: "quiz", TextContent: "Answer below."},
	}
	for i := range f.publicStep {
		require.NoError(t, db.Create(&f.publicStep[i]).Error)
	}
	require.NoError(t, db.Create(&[]models.ScenarioStepHint{
		{StepID: f.publicStep[0].ID, Level: 1, Content: "SECRET-HINT-1"},
		{StepID: f.publicStep[0].ID, Level: 2, Content: "SECRET-HINT-2"},
	}).Error)
	require.NoError(t, db.Create(&[]models.ScenarioStepQuestion{
		{StepID: f.publicStep[1].ID, Order: 1, QuestionText: "Which command lists files?", QuestionType: "free_text", CorrectAnswer: "SECRET-ANSWER", Explanation: "SECRET-EXPLANATION", Points: 1},
		{StepID: f.publicStep[1].ID, Order: 2, QuestionText: "Which command prints a file?", QuestionType: "free_text", CorrectAnswer: "SECRET-ANSWER-2", Points: 1},
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: f.publicStep[0].ID, Locale: "fr", Title: "Trouver le fichier", TextContent: "Cherchez dans /srv.", HintContent: "SECRET-HINT-FR",
	}).Error)

	otherOrg := createTestOrg(t, db, "lib-other-owner")
	f.foreign = createTestScenarioForOrg(t, db, otherOrg, "lib-foreign")

	f.target = &models.Scenario{Name: "lib-target", Title: "My lab", InstanceType: "debian", CreatedByID: libTeacher, OrganizationID: &f.org}
	require.NoError(t, db.Create(f.target).Error)
	for i, title := range []string{"Mine first", "Mine second"} {
		require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: f.target.ID, Order: i, Title: title, TextContent: title}).Error)
	}
	return f
}

func stepLibraryRouter(t *testing.T, db *gorm.DB, userID string, roles ...string) *gin.Engine {
	t.Helper()
	if len(roles) == 0 {
		roles = []string{"member"}
	}
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	scenarioController.RegisterScenarioPermissions(mocks.NewMockEnforcer())
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())
	ctrl := scenarioController.NewScenarioController(db)
	api.GET("/scenarios/:id/step-outline", ctrl.GetStepOutline)
	api.POST("/scenarios/:id/steps/copy", ctrl.CopySteps)
	return r
}

func stepLibraryRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- outline ----------------------------------------------------------------------

func TestStepOutline_TeacherSeesWhatTheStepsAreNotHowTheyAreGraded(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libTeacher), http.MethodGet, "/api/v1/scenarios/"+f.public.ID.String()+"/step-outline", nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRET", "no script, hint, flag path or answer may reach the outline")

	var outline []struct {
		ID            uuid.UUID `json:"id"`
		Order         int       `json:"order"`
		Title         string    `json:"title"`
		StepType      string    `json:"step_type"`
		TextContent   string    `json:"text_content"`
		HasFlag       bool      `json:"has_flag"`
		HintCount     int       `json:"hint_count"`
		QuestionCount int       `json:"question_count"`
		Translations  []struct {
			Locale      string `json:"locale"`
			Title       string `json:"title"`
			TextContent string `json:"text_content"`
		} `json:"translations"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &outline))
	require.Len(t, outline, 2)

	assert.Equal(t, f.publicStep[0].ID, outline[0].ID)
	assert.Equal(t, "Find the file", outline[0].Title)
	assert.Equal(t, "flag", outline[0].StepType)
	assert.Equal(t, "Look around /srv.", outline[0].TextContent)
	assert.True(t, outline[0].HasFlag)
	assert.Equal(t, 2, outline[0].HintCount)
	require.Len(t, outline[0].Translations, 1)
	assert.Equal(t, "fr", outline[0].Translations[0].Locale)
	assert.Equal(t, "Trouver le fichier", outline[0].Translations[0].Title)
	assert.Equal(t, "Cherchez dans /srv.", outline[0].Translations[0].TextContent)

	assert.Equal(t, 1, outline[1].Order)
	assert.Equal(t, "quiz", outline[1].StepType)
	assert.Equal(t, 2, outline[1].QuestionCount)
}

// A learner would read the outline as a walkthrough: it is for authors only.
func TestStepOutline_LearnerRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libLearner), http.MethodGet, "/api/v1/scenarios/"+f.public.ID.String()+"/step-outline", nil)
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}

func TestStepOutline_UnseenScenarioIsNotFound(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libTeacher), http.MethodGet, "/api/v1/scenarios/"+f.foreign.ID.String()+"/step-outline", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

// --- copy -------------------------------------------------------------------------

func copySteps(t *testing.T, db *gorm.DB, user string, target uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return stepLibraryRequest(stepLibraryRouter(t, db, user), http.MethodPost, "/api/v1/scenarios/"+target.String()+"/steps/copy", body)
}

func targetSteps(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) []models.ScenarioStep {
	t.Helper()
	return loadStepsWithRelations(t, db, scenarioID)
}

func TestCopySteps_FromPublicScenario_InsertsCompleteStepsAtPosition(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := copySteps(t, db, libTeacher, f.target.ID, map[string]any{
		"source_step_ids": []uuid.UUID{f.publicStep[0].ID, f.publicStep[1].ID},
		"position":        1,
	})
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRET", "the copy is made server-side; its scripts and answers are not echoed back")

	steps := targetSteps(t, db, f.target.ID)
	titles := make([]string, len(steps))
	for i, s := range steps {
		titles[i] = s.Title
		assert.Equal(t, i, s.Order, "orders are renumbered contiguously")
	}
	assert.Equal(t, []string{"Mine first", "Find the file", "Check your knowledge", "Mine second"}, titles)

	flagStep, quizStep := steps[1], steps[2]
	assert.NotEqual(t, f.publicStep[0].ID, flagStep.ID, "a copy is a new row")
	assert.Equal(t, "SECRET-VERIFY", flagStep.VerifyScript)
	assert.Equal(t, "SECRET-BACKGROUND", flagStep.BackgroundScript)
	assert.Equal(t, "SECRET-FOREGROUND", flagStep.ForegroundScript)
	assert.Equal(t, "SECRET-HINT", flagStep.HintContent)
	assert.Equal(t, "/srv/SECRET-FLAG-PATH", flagStep.FlagPath)
	assert.Equal(t, 2, flagStep.FlagLevel)
	assert.Equal(t, "flag", flagStep.StepType)
	assert.True(t, flagStep.HasFlag, "a flag step keeps its flag")
	assert.Equal(t, "matrix", flagStep.IntroEffect)
	require.Len(t, flagStep.Hints, 2)
	assert.Equal(t, "SECRET-HINT-2", flagStep.Hints[1].Content)

	require.Len(t, quizStep.Questions, 2)
	assert.Equal(t, "SECRET-ANSWER", quizStep.Questions[0].CorrectAnswer)
	assert.Equal(t, "SECRET-EXPLANATION", quizStep.Questions[0].Explanation)

	var translations []models.ScenarioStepTranslation
	require.NoError(t, db.Where("step_id = ?", flagStep.ID).Find(&translations).Error)
	require.Len(t, translations, 1)
	assert.Equal(t, "Trouver le fichier", translations[0].Title)
	assert.Equal(t, "SECRET-HINT-FR", translations[0].HintContent)

	untouched := targetSteps(t, db, f.public.ID)
	require.Len(t, untouched, 2, "the source keeps its steps")
}

func TestCopySteps_WithoutPosition_Appends(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := copySteps(t, db, libTeacher, f.target.ID, map[string]any{"source_step_ids": []uuid.UUID{f.publicStep[1].ID}})
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	steps := targetSteps(t, db, f.target.ID)
	require.Len(t, steps, 3)
	assert.Equal(t, "Check your knowledge", steps[2].Title)
	assert.Equal(t, 2, steps[2].Order)
}

func TestCopySteps_FromAnotherOrgsScenario_NotFound(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)
	var foreignStep models.ScenarioStep
	require.NoError(t, db.First(&foreignStep, "scenario_id = ?", f.foreign.ID).Error)

	w := copySteps(t, db, libTeacher, f.target.ID, map[string]any{"source_step_ids": []uuid.UUID{f.publicStep[0].ID, foreignStep.ID}})
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Len(t, targetSteps(t, db, f.target.ID), 2, "nothing is copied when one source is refused")
}

func TestCopySteps_UnknownStep_NotFound(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := copySteps(t, db, libTeacher, f.target.ID, map[string]any{"source_step_ids": []uuid.UUID{uuid.New()}})
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

func TestCopySteps_IntoAScenarioTheCallerDoesNotManage_Forbidden(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := copySteps(t, db, libColleague, f.target.ID, map[string]any{"source_step_ids": []uuid.UUID{f.publicStep[0].ID}})
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Len(t, targetSteps(t, db, f.target.ID), 2)
}

func TestCopySteps_BadRequests(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	for name, body := range map[string]map[string]any{
		"no steps":          {"source_step_ids": []uuid.UUID{}},
		"negative position": {"source_step_ids": []uuid.UUID{f.publicStep[0].ID}, "position": -1},
		"position past end": {"source_step_ids": []uuid.UUID{f.publicStep[0].ID}, "position": 3},
	} {
		t.Run(name, func(t *testing.T) {
			w := copySteps(t, db, libTeacher, f.target.ID, body)
			assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
		})
	}
	assert.Len(t, targetSteps(t, db, f.target.ID), 2)
}
