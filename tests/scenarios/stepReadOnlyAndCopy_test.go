package scenarios_test

// The step library: a teacher reads another scenario's steps in full, read-only
// (GET /scenarios/:id/steps/read-only) — anyone allowed there may duplicate the
// scenario and read it all anyway — and copies the steps they want into a
// scenario of theirs (POST /scenarios/:id/steps/copy). Learners are refused:
// to them the steps are a walkthrough.

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

	f.public = &models.Scenario{Name: "lib-public", Title: "Public lab", InstanceType: "debian", CreatedByID: "platform-admin", IsPublic: true,
		SetupScript: "SECRET-SETUP", FlagsEnabled: true, FlagSecret: "FLAG-SECRET-NEVER-SENT"}
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
	api.GET("/scenarios/:id/steps/read-only", ctrl.GetReadOnlySteps)
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

// --- read-only steps ---------------------------------------------------------------

type readOnlyStepsResponse struct {
	SetupScript string `json:"setup_script"`
	Steps       []struct {
		ID                    uuid.UUID `json:"id"`
		Order                 int       `json:"order"`
		Title                 string    `json:"title"`
		StepType              string    `json:"step_type"`
		ShowImmediateFeedback bool      `json:"show_immediate_feedback"`
		TextContent           string    `json:"text_content"`
		HintContent           string    `json:"hint_content"`
		VerifyScript          string    `json:"verify_script"`
		BackgroundScript      string    `json:"background_script"`
		ForegroundScript      string    `json:"foreground_script"`
		IntroEffect           string    `json:"intro_effect"`
		HasFlag               bool      `json:"has_flag"`
		FlagPath              string    `json:"flag_path"`
		FlagLevel             int       `json:"flag_level"`
		Questions             []struct {
			CorrectAnswer string `json:"correct_answer"`
			Explanation   string `json:"explanation"`
		} `json:"questions"`
		Hints []struct {
			Level   int    `json:"level"`
			Content string `json:"content"`
		} `json:"hints"`
		Translations []struct {
			Locale      string `json:"locale"`
			Title       string `json:"title"`
			HintContent string `json:"hint_content"`
		} `json:"translations"`
	} `json:"steps"`
}

func TestReadOnlySteps_TeacherReadsTheFullSteps(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libTeacher), http.MethodGet, "/api/v1/scenarios/"+f.public.ID.String()+"/steps/read-only", nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "FLAG-SECRET-NEVER-SENT", "the flag secret never leaves the server")

	var got readOnlyStepsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "SECRET-SETUP", got.SetupScript)
	require.Len(t, got.Steps, 2)

	flag := got.Steps[0]
	assert.Equal(t, f.publicStep[0].ID, flag.ID)
	assert.Equal(t, "Find the file", flag.Title)
	assert.Equal(t, "SECRET-VERIFY", flag.VerifyScript)
	assert.Equal(t, "SECRET-BACKGROUND", flag.BackgroundScript)
	assert.Equal(t, "SECRET-FOREGROUND", flag.ForegroundScript)
	assert.Equal(t, "SECRET-HINT", flag.HintContent)
	assert.Equal(t, "matrix", flag.IntroEffect)
	assert.True(t, flag.HasFlag)
	assert.Equal(t, "/srv/SECRET-FLAG-PATH", flag.FlagPath)
	assert.Equal(t, 2, flag.FlagLevel)
	require.Len(t, flag.Hints, 2)
	assert.Equal(t, "SECRET-HINT-1", flag.Hints[0].Content)
	require.Len(t, flag.Translations, 1)
	assert.Equal(t, "Trouver le fichier", flag.Translations[0].Title)
	assert.Equal(t, "SECRET-HINT-FR", flag.Translations[0].HintContent)

	quiz := got.Steps[1]
	assert.Equal(t, "quiz", quiz.StepType)
	require.Len(t, quiz.Questions, 2)
	assert.Equal(t, "SECRET-ANSWER", quiz.Questions[0].CorrectAnswer)
	assert.Equal(t, "SECRET-EXPLANATION", quiz.Questions[0].Explanation)
}

// To a learner the steps are a walkthrough: they are for authors only.
func TestReadOnlySteps_LearnerRefused(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libLearner), http.MethodGet, "/api/v1/scenarios/"+f.public.ID.String()+"/steps/read-only", nil)
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRET")
}

func TestReadOnlySteps_UnseenScenarioIsNotFound(t *testing.T) {
	db := freshTestDB(t)
	f := buildStepLibraryFixture(t, db)

	w := stepLibraryRequest(stepLibraryRouter(t, db, libTeacher), http.MethodGet, "/api/v1/scenarios/"+f.foreign.ID.String()+"/steps/read-only", nil)
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
	var created readOnlyStepsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Len(t, created.Steps, 2, "the response lists the new steps")
	assert.Equal(t, []int{1, 2}, []int{created.Steps[0].Order, created.Steps[1].Order})

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
