package scenarios_test

// Extended tests for the scenario content leak (issue #293).
//
// The first round of tests + GREEN fix in this branch only redacted the
// scenario detail endpoint's nested Steps[].Questions[]. Three more vectors
// remain unprotected:
//
//   1. GET /api/v1/scenarios[/:id] still returns ScenarioOutput.SetupScript
//      (raw setup-script body executed in the lab container — often the lab
//      "answer" itself), plus SetupScriptID, IntroFileID, FinishFileID, to
//      any authenticated Member.
//
//   2. GET /api/v1/scenario-steps[/:id] returns the full step content
//      (HintContent, FlagPath, FlagLevel, VerifyScriptID, BackgroundScriptID,
//      ForegroundScriptID, TextFileID, HintFileID, plus the embedded
//      Questions[].CorrectAnswer / Explanation) directly to any Member.
//      This sibling route bypasses the redactor that lives on the Scenario
//      entity.
//
//   3. GET /api/v1/scenario-step-questions[/:id] returns CorrectAnswer +
//      Explanation directly to any Member.
//
// These tests pin the contract for these read paths:
//
//   - Admin / org-manager / group-manager (and creator for steps) -> full content
//   - Regular member with no manage relation                        -> stripped
//
// The strip predicate must be the same CanManageScenario rule used by the
// existing scenarioRedactor (and by the write-side hooks): creator,
// org-manager-of-scenario's-org, or group-manager-of-an-assigned-group.
//
// SetupScript is teacher-only — it is the lab's answer in many cases.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementController "soli/formations/src/entityManagement/routes"
	ems "soli/formations/src/entityManagement/entityManagementService"
	orgModels "soli/formations/src/organizations/models"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// =============================================================================
// Constants — sentinel values used to detect leaks vs. proper redaction.
// =============================================================================

const (
	leakSetupScript      = "SECRET-SETUP-script-do-not-leak: rm -rf /home/student/.solution"
	leakVerifyScript     = "SECRET-VERIFY-script: test -f /tmp/flag && grep -q 'OCF{leak}' /tmp/flag"
	leakBackgroundScript = "SECRET-BG-script-do-not-leak"
	leakForegroundScript = "SECRET-FG-script-do-not-leak"
)

// =============================================================================
// Test setup — extends the existing setupScenarioReadAuthzTest with the
// sibling routes (/scenario-steps, /scenario-step-questions). Reuses the same
// global registration swap pattern + same fake auth context middleware.
// =============================================================================

// setupExtendedReadAuthzTest registers all three entities (Scenario,
// ScenarioStep, ScenarioStepQuestion) on a fresh GlobalEntityRegistrationService
// and mounts the matching GET routes so GetEntityNameFromPath resolves
// correctly:
//
//   /api/v1/scenarios               -> "Scenario"
//   /api/v1/scenarios/:id           -> "Scenario"
//   /api/v1/scenario-steps          -> "ScenarioStep"
//   /api/v1/scenario-steps/:id      -> "ScenarioStep"
//   /api/v1/scenario-step-questions -> "ScenarioStepQuestion"
//   /api/v1/scenario-step-questions/:id -> "ScenarioStepQuestion"
func setupExtendedReadAuthzTest(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()

	originalSvc := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenario(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioStep(ems.GlobalEntityRegistrationService)
	scenarioRegistration.RegisterScenarioStepQuestion(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = originalSvc
	})

	gen := entityManagementController.NewGenericController(db, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()

	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})

	api.GET("/scenarios", gen.GetEntities)
	api.GET("/scenarios/:id", gen.GetEntity)
	api.GET("/scenario-steps", gen.GetEntities)
	api.GET("/scenario-steps/:id", gen.GetEntity)
	api.GET("/scenario-step-questions", gen.GetEntities)
	api.GET("/scenario-step-questions/:id", gen.GetEntity)

	return r
}

// buildLeakyScenarioWithSetupScript creates a scenario seeded with a
// SetupScript + script/file IDs (so we can test the top-level scenario
// fields) AND one step with sensitive content + one quiz question. Returns
// a fully-loaded scenario, the step, and the question for direct lookups.
func buildLeakyScenarioWithSetupScript(t *testing.T, db *gorm.DB, name, creatorID string, orgID *uuid.UUID) (*models.Scenario, *models.ScenarioStep, *models.ScenarioStepQuestion) {
	t.Helper()

	setupScriptID := uuid.New()
	introFileID := uuid.New()
	finishFileID := uuid.New()
	verifyScriptID := uuid.New()
	bgScriptID := uuid.New()
	fgScriptID := uuid.New()
	textFileID := uuid.New()
	hintFileID := uuid.New()

	scenario := &models.Scenario{
		Name:           name,
		Title:          "Leaky: " + name,
		InstanceType:   "ubuntu:22.04",
		SourceType:     "seed",
		CreatedByID:    creatorID,
		OrganizationID: orgID,
		FlagsEnabled:   true,
		SetupScript:    leakSetupScript,
		SetupScriptID:  &setupScriptID,
		IntroFileID:    &introFileID,
		FinishFileID:   &finishFileID,
	}
	require.NoError(t, db.Create(scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:         scenario.ID,
		Order:              1,
		Title:              "CTF step",
		StepType:           "flag",
		TextContent:        "Find the flag",
		HintContent:        "SECRET-HINT-look-in-/etc/secret",
		VerifyScript:       leakVerifyScript,
		BackgroundScript:   leakBackgroundScript,
		ForegroundScript:   leakForegroundScript,
		HasFlag:            true,
		FlagPath:           "/etc/secret/flag.txt",
		FlagLevel:          3,
		VerifyScriptID:     &verifyScriptID,
		BackgroundScriptID: &bgScriptID,
		ForegroundScriptID: &fgScriptID,
		TextFileID:         &textFileID,
		HintFileID:         &hintFileID,
	}
	require.NoError(t, db.Create(&step).Error)

	question := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "What is the answer?",
		QuestionType:  "free_text",
		CorrectAnswer: "SECRET-ANSWER-42",
		Explanation:   "SECRET-EXPLANATION-don't-leak-this",
		Points:        5,
	}
	require.NoError(t, db.Create(&question).Error)

	// Reload with preloads so the converter sees Steps + Questions.
	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps.Questions").First(&loaded, "id = ?", scenario.ID).Error)
	return &loaded, &step, &question
}

// =============================================================================
// Step assertion helpers — used for direct GET /scenario-steps/:id (where
// the response IS the step, not an embedded "steps" array).
// =============================================================================

// assertStepBodyHasFullContent verifies that the step JSON object leaks all
// sensitive fields — i.e. the user is a manager and is allowed to see them.
func assertStepBodyHasFullContent(t *testing.T, step map[string]any) {
	t.Helper()
	assert.Equal(t, "SECRET-HINT-look-in-/etc/secret", step["hint_content"],
		"manager must see hint_content")
	assert.Equal(t, leakVerifyScript, step["verify_script"],
		"manager must see verify_script body")
	assert.Equal(t, leakBackgroundScript, step["background_script"],
		"manager must see background_script body")
	assert.Equal(t, leakForegroundScript, step["foreground_script"],
		"manager must see foreground_script body")
	assert.Equal(t, "/etc/secret/flag.txt", step["flag_path"],
		"manager must see flag_path")
	if v, ok := step["flag_level"]; ok {
		assert.EqualValues(t, 3, v, "manager must see flag_level")
	} else {
		t.Errorf("manager must receive flag_level on step")
	}
	for _, f := range []string{
		"verify_script_id",
		"background_script_id",
		"foreground_script_id",
		"text_file_id",
		"hint_file_id",
	} {
		assert.NotNil(t, step[f], "manager must see %s", f)
	}
	questions, ok := step["questions"].([]any)
	require.True(t, ok, "manager must see questions array; body=%v", step)
	require.Len(t, questions, 1, "manager must see all questions")
	q := questions[0].(map[string]any)
	assert.Equal(t, "SECRET-ANSWER-42", q["correct_answer"],
		"manager must see correct_answer")
	assert.Equal(t, "SECRET-EXPLANATION-don't-leak-this", q["explanation"],
		"manager must see explanation")
}

// assertStepBodyStripped verifies that all sensitive step + question fields
// are absent (or zeroed) in the JSON object returned for a non-manager.
//
// This is the contract the future fix must enforce on /scenario-steps[/:id].
func assertStepBodyStripped(t *testing.T, step map[string]any) {
	t.Helper()
	for _, f := range stepSensitiveFields {
		v, present := step[f]
		if !present {
			continue
		}
		switch typed := v.(type) {
		case string:
			assert.Empty(t, typed, "step.%s must not leak content to non-manager", f)
		case float64:
			assert.Equal(t, float64(0), typed, "step.%s must not leak content to non-manager", f)
		case nil:
			// OK — explicit null is fine.
		default:
			t.Errorf("step.%s present with unexpected type %T (%v); should be absent or zero for non-manager", f, v, v)
		}
	}
	if questions, present := step["questions"]; present && questions != nil {
		qSlice, ok := questions.([]any)
		require.True(t, ok, "questions, when present, must be an array; got %T", questions)
		// Either the array is empty (preferred) or each entry has been
		// stripped of CorrectAnswer + Explanation.
		for i, raw := range qSlice {
			q, ok := raw.(map[string]any)
			require.True(t, ok, "questions[%d] must be a JSON object", i)
			assertQuestionBodyStripped(t, q)
		}
	}
}

// assertQuestionBodyHasFullContent verifies that a question JSON object leaks
// CorrectAnswer + Explanation — used for /scenario-step-questions/:id manager tests.
func assertQuestionBodyHasFullContent(t *testing.T, q map[string]any) {
	t.Helper()
	assert.Equal(t, "SECRET-ANSWER-42", q["correct_answer"],
		"manager must see correct_answer")
	assert.Equal(t, "SECRET-EXPLANATION-don't-leak-this", q["explanation"],
		"manager must see explanation")
}

// assertQuestionBodyStripped verifies that CorrectAnswer + Explanation are
// absent (or empty) for a non-manager.
func assertQuestionBodyStripped(t *testing.T, q map[string]any) {
	t.Helper()
	for _, f := range []string{"correct_answer", "explanation"} {
		v, present := q[f]
		if !present {
			continue
		}
		switch typed := v.(type) {
		case string:
			assert.Empty(t, typed, "question.%s must not leak content to non-manager", f)
		case nil:
			// OK
		default:
			t.Errorf("question.%s present with unexpected type %T (%v); should be absent or empty for non-manager", f, v, v)
		}
	}
}

// =============================================================================
// Scenario-level setup_script stripping (3 tests).
// =============================================================================

// assertScenarioSetupScriptStripped checks that the top-level SetupScript +
// script/file IDs are not leaked to a non-manager. Either absent (omitempty)
// or zero / null is acceptable.
func assertScenarioSetupScriptStripped(t *testing.T, body map[string]any) {
	t.Helper()
	for _, f := range []string{"setup_script", "setup_script_id", "intro_file_id", "finish_file_id"} {
		v, present := body[f]
		if !present {
			continue
		}
		switch typed := v.(type) {
		case string:
			assert.Empty(t, typed, "scenario.%s must not leak content to non-manager", f)
		case nil:
			// OK
		default:
			t.Errorf("scenario.%s present with unexpected type %T (%v); should be absent / empty / null for non-manager", f, v, v)
		}
	}
}

func TestGetScenario_AsAdmin_ReturnsSetupScript(t *testing.T) {
	db := freshTestDB(t)
	scenario, _, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-admin-setup", "creator-admin-setup-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "platform-admin-setup", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin must read scenario; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// Positive control: admin must see SetupScript + IDs.
	assert.Equal(t, leakSetupScript, body["setup_script"],
		"admin must see setup_script (positive control)")
	assert.NotNil(t, body["setup_script_id"], "admin must see setup_script_id")
	assert.NotNil(t, body["intro_file_id"], "admin must see intro_file_id")
	assert.NotNil(t, body["finish_file_id"], "admin must see finish_file_id")
}

func TestGetScenario_AsRegularMember_StripsSetupScript(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-leak-setup-001"
	scenario, _, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-regular-setup", creatorID, nil)

	// Outsider has no relation to the scenario.
	router := setupExtendedReadAuthzTest(t, db, "outsider-leak-setup-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the scenario record (just no sensitive fields); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// Header is fine, sensitive top-level fields must NOT leak.
	assert.Equal(t, "Leaky: leak-regular-setup", body["title"])
	assertScenarioSetupScriptStripped(t, body)
}

func TestListScenarios_AsRegularMember_StripsSetupScript(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-list-setup-001"
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-setup-1", creatorID, nil)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-setup-2", creatorID, nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-list-setup-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "list endpoint should respond; body=%s", w.Body.String())

	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))

	dataRaw, ok := page["data"].([]any)
	require.True(t, ok, "list response must wrap items in 'data' (PaginationResponse); body=%v", page)
	require.Len(t, dataRaw, 2, "should list both scenarios")

	for i, s := range dataRaw {
		body, ok := s.(map[string]any)
		require.True(t, ok, "scenario[%d] must be a JSON object", i)
		assertScenarioSetupScriptStripped(t, body)
	}
}

// =============================================================================
// GET /scenario-steps/:id — manager positive controls (full content).
// =============================================================================

func TestGetScenarioStep_AsAdmin_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-admin", "creator-step-admin-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "platform-admin-step", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin must read step; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertStepBodyHasFullContent(t, body)
}

func TestGetScenarioStep_AsCreator_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-step-self-001"
	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-creator", creatorID, nil)

	router := setupExtendedReadAuthzTest(t, db, creatorID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "creator must read their own step; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertStepBodyHasFullContent(t, body)
}

func TestGetScenarioStep_AsOrgManager_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-step-001"
	managerID := "org-manager-step-001"
	orgID := makeOrgWithMember(t, db, ownerID, managerID, orgModels.OrgRoleManager)

	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-org-manager", ownerID, &orgID)

	router := setupExtendedReadAuthzTest(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "org manager must read step; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertStepBodyHasFullContent(t, body)
}

func TestGetScenarioStep_AsGroupManager_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-step-grp-001"
	groupOwnerID := "group-owner-step-001"
	groupID := makeGroupWithOwner(t, db, groupOwnerID)

	scenario, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-group-manager", creatorID, nil)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: creatorID,
		IsActive:    true,
	}).Error)

	router := setupExtendedReadAuthzTest(t, db, groupOwnerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "group manager must read step; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertStepBodyHasFullContent(t, body)
}

// =============================================================================
// GET /scenario-steps/:id — the actual leak (regular member must be stripped).
// =============================================================================

func TestGetScenarioStep_AsRegularMember_StripsSensitiveFields(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-step-leak-001"
	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-regular", creatorID, nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-step-leak-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the step record (just no sensitive fields); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// Header (title, step_type) is fine.
	assert.Equal(t, "CTF step", body["title"])

	assertStepBodyStripped(t, body)
}

func TestGetScenarioStep_AsRegularMember_WithIncludeQuestions_StillStripped(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-step-include-001"
	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "leak-step-include", creatorID, nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-step-include-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/v1/scenario-steps/"+step.ID.String()+"?include=Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the step record (just no sensitive fields); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertStepBodyStripped(t, body)
}

func TestListScenarioSteps_AsAdmin_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-step-admin", "creator-list-step-admin-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "platform-admin-list-step", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin step list should respond; body=%s", w.Body.String())

	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))

	dataRaw, ok := page["data"].([]any)
	require.True(t, ok, "list response must wrap items in 'data' (PaginationResponse); body=%v", page)
	require.Len(t, dataRaw, 1)

	first, ok := dataRaw[0].(map[string]any)
	require.True(t, ok)
	assertStepBodyHasFullContent(t, first)
}

func TestListScenarioSteps_AsRegularMember_StripsSensitiveFields(t *testing.T) {
	db := freshTestDB(t)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-step-1", "creator-list-step-001", nil)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-step-2", "creator-list-step-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-list-step-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-steps", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "step list endpoint should respond; body=%s", w.Body.String())

	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))

	dataRaw, ok := page["data"].([]any)
	require.True(t, ok, "list response must wrap items in 'data' (PaginationResponse); body=%v", page)
	require.Len(t, dataRaw, 2, "should list both steps")

	for i, s := range dataRaw {
		body, ok := s.(map[string]any)
		require.True(t, ok, "step[%d] must be a JSON object", i)
		assertStepBodyStripped(t, body)
	}
}

// =============================================================================
// GET /scenario-step-questions/:id — manager positive controls.
// =============================================================================

func TestGetScenarioStepQuestion_AsAdmin_ReturnsCorrectAnswerAndExplanation(t *testing.T) {
	db := freshTestDB(t)
	_, _, question := buildLeakyScenarioWithSetupScript(t, db, "leak-q-admin", "creator-q-admin-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "platform-admin-q", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-step-questions/"+question.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin must read question; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertQuestionBodyHasFullContent(t, body)
}

func TestGetScenarioStepQuestion_AsOrgManager_ReturnsCorrectAnswerAndExplanation(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-q-001"
	managerID := "org-manager-q-001"
	orgID := makeOrgWithMember(t, db, ownerID, managerID, orgModels.OrgRoleManager)

	_, _, question := buildLeakyScenarioWithSetupScript(t, db, "leak-q-org-manager", ownerID, &orgID)

	router := setupExtendedReadAuthzTest(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-step-questions/"+question.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "org manager must read question; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertQuestionBodyHasFullContent(t, body)
}

func TestGetScenarioStepQuestion_AsGroupManager_ReturnsCorrectAnswerAndExplanation(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-q-grp-001"
	groupOwnerID := "group-owner-q-001"
	groupID := makeGroupWithOwner(t, db, groupOwnerID)

	scenario, _, question := buildLeakyScenarioWithSetupScript(t, db, "leak-q-group-manager", creatorID, nil)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: creatorID,
		IsActive:    true,
	}).Error)

	router := setupExtendedReadAuthzTest(t, db, groupOwnerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-step-questions/"+question.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "group manager must read question; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertQuestionBodyHasFullContent(t, body)
}

// =============================================================================
// GET /scenario-step-questions/:id — the actual leak.
// =============================================================================

func TestGetScenarioStepQuestion_AsRegularMember_StripsCorrectAnswerAndExplanation(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-q-leak-001"
	_, _, question := buildLeakyScenarioWithSetupScript(t, db, "leak-q-regular", creatorID, nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-q-leak-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-step-questions/"+question.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the question record (just no answer/explanation); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// QuestionText itself is fine — students need to see the question.
	assert.Equal(t, "What is the answer?", body["question_text"])

	assertQuestionBodyStripped(t, body)
}

func TestListScenarioStepQuestions_AsRegularMember_StripsCorrectAnswerAndExplanation(t *testing.T) {
	db := freshTestDB(t)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-q-1", "creator-list-q-001", nil)
	_, _, _ = buildLeakyScenarioWithSetupScript(t, db, "leak-list-q-2", "creator-list-q-001", nil)

	router := setupExtendedReadAuthzTest(t, db, "outsider-list-q-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-step-questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "question list endpoint should respond; body=%s", w.Body.String())

	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))

	dataRaw, ok := page["data"].([]any)
	require.True(t, ok, "list response must wrap items in 'data' (PaginationResponse); body=%v", page)
	require.Len(t, dataRaw, 2, "should list both questions")

	for i, q := range dataRaw {
		body, ok := q.(map[string]any)
		require.True(t, ok, "question[%d] must be a JSON object", i)
		assertQuestionBodyStripped(t, body)
	}
}
