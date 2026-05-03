package scenarios_test

// Tests for the scenario content leak (issue #293).
//
// Security bug: GET /api/v1/scenarios and GET /api/v1/scenarios/:id return the
// full step + question content (HintContent, FlagPath, FlagLevel,
// VerifyScriptID, BackgroundScriptID, ForegroundScriptID, TextFileID,
// HintFileID, Questions[].CorrectAnswer, Questions[].Explanation) to every
// authenticated Member. Students who can read those fields can cheat on quizzes
// and CTF flag steps before they ever launch the lab.
//
// The authorization predicate already exists:
//   scenarioHooks.CanManageScenario(db, groupSvc, scenario, userID)
// This is the single source of truth used by the step + question hooks. The
// read path must use the same predicate (plus admin bypass) and strip step /
// question content for callers who fail it.
//
// These tests pin the contract for the read endpoints:
//
//   1. Admin / creator / org-manager / group-manager  -> full content
//   2. Regular member with no manage relation         -> stripped
//      - Steps must not leak HintContent, FlagPath, FlagLevel,
//        VerifyScriptID, BackgroundScriptID, ForegroundScriptID,
//        TextFileID, HintFileID
//      - Questions must not be returned at all (no CorrectAnswer / Explanation
//        leak)
//   3. ?include=Steps.Questions must NOT bypass the strip — the redaction is
//      applied post-fetch.
//   4. The list endpoint enforces the same per-item strip.
//
// The tests are HTTP-level so they remain valid whether the fix lives in the
// converter (ModelToDto) or in a controller-side filter applied after the
// generic GetEntity / GetEntities runs.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementController "soli/formations/src/entityManagement/routes"
	ems "soli/formations/src/entityManagement/entityManagementService"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// =============================================================================
// Sensitive field names — what MUST be stripped for non-managers.
// =============================================================================

// stepSensitiveFields are step-level fields a non-manager must never see.
var stepSensitiveFields = []string{
	"hint_content",
	"flag_path",
	"flag_level",
	"verify_script_id",
	"background_script_id",
	"foreground_script_id",
	"text_file_id",
	"hint_file_id",
}

// =============================================================================
// Test setup — shared between all tests in this file.
// =============================================================================

// setupScenarioReadAuthzTest registers Scenario in the global entity
// registration service (the converter under test) and returns a Gin router
// wired to the generic GetEntity / GetEntities controllers.
//
// The router does NOT mount Layer 2 enforcement — entity CRUD authorization
// for Member is granted at Layer 1 (Roles map), and the leak we're proving
// happens at the converter level for any Member who reaches the route. We
// just need to inject userId + userRoles and let the response speak.
func setupScenarioReadAuthzTest(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()

	// Reset and re-register the global registration service for test isolation.
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

	// Match the production paths so GetEntityNameFromPath resolves to "Scenario".
	api.GET("/scenarios", gen.GetEntities)
	api.GET("/scenarios/:id", gen.GetEntity)

	return r
}

// buildLeakyScenario creates a scenario with one step (full of sensitive
// content: hint, flag, scripts, file IDs) and one quiz question with
// CorrectAnswer + Explanation. This is the worst case for the leak.
func buildLeakyScenario(t *testing.T, db *gorm.DB, name, creatorID string, orgID *uuid.UUID) *models.Scenario {
	t.Helper()

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
	}
	require.NoError(t, db.Create(scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:         scenario.ID,
		Order:              1,
		Title:              "CTF step",
		StepType:           "flag",
		TextContent:        "Find the flag",
		HintContent:        "SECRET-HINT-look-in-/etc/secret",
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
	return &loaded
}

// makeOrgWithMember creates an org owned by ownerID and adds the given member
// (e.g. as manager). Returns the org ID.
func makeOrgWithMember(t *testing.T, db *gorm.DB, ownerID, memberID string, role orgModels.OrganizationMemberRole) uuid.UUID {
	t.Helper()
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name:             "Read Authz Org " + orgID.String()[:8],
		DisplayName:      "Read Authz Org",
		OwnerUserID:      ownerID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers:       100,
		IsActive:         true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID, UserID: ownerID, Role: orgModels.OrgRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)
	if memberID != "" && memberID != ownerID {
		require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
			OrganizationID: orgID, UserID: memberID, Role: role,
			JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}
	return orgID
}

// makeGroupWithOwner creates a class group with the given owner and returns its ID.
func makeGroupWithOwner(t *testing.T, db *gorm.DB, ownerID string) uuid.UUID {
	t.Helper()
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name:        "Read Authz Group " + groupID.String()[:8],
		DisplayName: "Read Authz Group",
		OwnerUserID: ownerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: ownerID, Role: groupModels.GroupMemberRoleOwner,
		InvitedBy: ownerID, JoinedAt: time.Now(), IsActive: true,
	}).Error)
	return groupID
}

// =============================================================================
// Helpers to assert the response shape.
// =============================================================================

// assertStepHasFullContent verifies that the step entry leaked nothing (i.e.
// the test user IS a manager and is allowed to see everything).
func assertStepHasFullContent(t *testing.T, step map[string]any) {
	t.Helper()
	assert.Equal(t, "SECRET-HINT-look-in-/etc/secret", step["hint_content"],
		"manager must see hint_content (full content path)")
	assert.Equal(t, "/etc/secret/flag.txt", step["flag_path"],
		"manager must see flag_path (full content path)")
	// flag_level may decode as float64 in JSON.
	if v, ok := step["flag_level"]; ok {
		assert.EqualValues(t, 3, v, "manager must see flag_level")
	} else {
		t.Errorf("manager must receive flag_level on step")
	}
	for _, f := range []string{"verify_script_id", "background_script_id", "foreground_script_id", "text_file_id", "hint_file_id"} {
		assert.NotNil(t, step[f], "manager must see %s (script/file id)", f)
	}
	questions, ok := step["questions"].([]any)
	require.True(t, ok, "manager must see questions array")
	require.Len(t, questions, 1, "manager must see all questions")
	q := questions[0].(map[string]any)
	assert.Equal(t, "SECRET-ANSWER-42", q["correct_answer"],
		"manager must see correct_answer")
	assert.Equal(t, "SECRET-EXPLANATION-don't-leak-this", q["explanation"],
		"manager must see explanation")
}

// assertStepStripped verifies that all sensitive step + question fields are
// absent for non-managers. This is the critical contract for the fix.
func assertStepStripped(t *testing.T, step map[string]any) {
	t.Helper()
	for _, f := range stepSensitiveFields {
		v, present := step[f]
		// JSON omitempty for strings/IDs may either omit the key or emit a
		// zero-ish value. We accept either ABSENT or zero value, but never the
		// real sensitive value.
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
		// Questions array, if present, must be empty — never expose CorrectAnswer.
		qSlice, ok := questions.([]any)
		require.True(t, ok, "questions, when present, must be an array; got %T", questions)
		assert.Empty(t, qSlice, "questions must not be returned to non-manager (CorrectAnswer / Explanation leak)")
	}
}

// assertScenarioBodyStripped checks an entire scenario response body.
//
// A non-manager must either see Steps fully empty (preferred) OR see steps
// with all sensitive fields stripped. Either is acceptable as a fix.
func assertScenarioBodyStripped(t *testing.T, body map[string]any) {
	t.Helper()
	stepsRaw, present := body["steps"]
	if !present || stepsRaw == nil {
		// No steps in response — this is the safe outcome.
		return
	}
	steps, ok := stepsRaw.([]any)
	require.True(t, ok, "steps, when present, must be an array; got %T", stepsRaw)
	if len(steps) == 0 {
		// Empty array — also safe.
		return
	}
	// Steps are present and non-empty: each must be stripped.
	for i, s := range steps {
		stepMap, ok := s.(map[string]any)
		require.True(t, ok, "step[%d] must be a JSON object", i)
		assertStepStripped(t, stepMap)
	}
}

// =============================================================================
// GetScenario (single) — positive controls (full content for managers).
// =============================================================================

func TestGetScenario_AsAdmin_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	scenario := buildLeakyScenario(t, db, "leak-admin", "creator-admin-001", nil)

	router := setupScenarioReadAuthzTest(t, db, "platform-admin", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin must read scenario; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	steps, ok := body["steps"].([]any)
	require.True(t, ok, "admin response must include steps; body=%v", body)
	require.Len(t, steps, 1)
	assertStepHasFullContent(t, steps[0].(map[string]any))
}

func TestGetScenario_AsCreator_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-self-001"
	scenario := buildLeakyScenario(t, db, "leak-creator", creatorID, nil)

	router := setupScenarioReadAuthzTest(t, db, creatorID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "creator must read their own scenario; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	steps, ok := body["steps"].([]any)
	require.True(t, ok, "creator response must include steps; body=%v", body)
	require.Len(t, steps, 1)
	assertStepHasFullContent(t, steps[0].(map[string]any))
}

func TestGetScenario_AsOrgManager_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-read-001"
	managerID := "org-manager-read-001"
	orgID := makeOrgWithMember(t, db, ownerID, managerID, orgModels.OrgRoleManager)

	scenario := buildLeakyScenario(t, db, "leak-org-manager", ownerID, &orgID)

	router := setupScenarioReadAuthzTest(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "org manager must read org-scoped scenario; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	steps, ok := body["steps"].([]any)
	require.True(t, ok, "org manager response must include steps; body=%v", body)
	require.Len(t, steps, 1)
	assertStepHasFullContent(t, steps[0].(map[string]any))
}

func TestGetScenario_AsGroupManager_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-grp-001"
	groupOwnerID := "group-owner-read-001"
	groupID := makeGroupWithOwner(t, db, groupOwnerID)

	scenario := buildLeakyScenario(t, db, "leak-group-manager", creatorID, nil)

	// Assign the scenario to the group so the group owner counts as manager
	// per CanManageScenario's group-assignment branch.
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: creatorID,
		IsActive:    true,
	}).Error)

	router := setupScenarioReadAuthzTest(t, db, groupOwnerID, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "group manager must read assigned scenario; body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	steps, ok := body["steps"].([]any)
	require.True(t, ok, "group manager response must include steps; body=%v", body)
	require.Len(t, steps, 1)
	assertStepHasFullContent(t, steps[0].(map[string]any))
}

// =============================================================================
// GetScenario — the actual leak: regular members must NOT see step content.
// =============================================================================

func TestGetScenario_AsRegularMember_StripsStepsAndQuestions(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-leak-001"
	scenario := buildLeakyScenario(t, db, "leak-regular-member", creatorID, nil)

	// Outsider has no relation to this scenario.
	router := setupScenarioReadAuthzTest(t, db, "outsider-leak-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the scenario record (just no sensitive fields); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// The scenario header (name, title, etc.) is fine — only step / question
	// content must be stripped.
	assert.Equal(t, "Leaky: leak-regular-member", body["title"])

	assertScenarioBodyStripped(t, body)
}

func TestGetScenario_AsRegularMember_WithIncludeQuery_StillStripped(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-leak-include-001"
	scenario := buildLeakyScenario(t, db, "leak-include-bypass", creatorID, nil)

	// Same outsider, but explicitly asks for Steps.Questions in the URL —
	// the strip must still happen post-fetch.
	router := setupScenarioReadAuthzTest(t, db, "outsider-leak-include-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		"/api/v1/scenarios/"+scenario.ID.String()+"?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"regular member should still get the scenario record (just no sensitive fields); body=%s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assertScenarioBodyStripped(t, body)
}

// =============================================================================
// GetScenarios (list) — same contract per item.
// =============================================================================

func TestListScenarios_AsRegularMember_StripsStepsAndQuestions(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-list-001"
	_ = buildLeakyScenario(t, db, "leak-list-1", creatorID, nil)
	_ = buildLeakyScenario(t, db, "leak-list-2", creatorID, nil)

	router := setupScenarioReadAuthzTest(t, db, "outsider-list-001", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios?include=Steps.Questions", nil)
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
		assertScenarioBodyStripped(t, body)
	}
}

func TestListScenarios_AsAdmin_ReturnsFullContent(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-list-admin-001"
	_ = buildLeakyScenario(t, db, "leak-list-admin-1", creatorID, nil)

	router := setupScenarioReadAuthzTest(t, db, "platform-admin-list", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios?include=Steps.Questions", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "admin list should respond; body=%s", w.Body.String())

	var page map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))

	dataRaw, ok := page["data"].([]any)
	require.True(t, ok, "list response must wrap items in 'data' (PaginationResponse); body=%v", page)
	require.Len(t, dataRaw, 1)

	first, ok := dataRaw[0].(map[string]any)
	require.True(t, ok)
	steps, ok := first["steps"].([]any)
	require.True(t, ok, "admin must see steps in list response; body=%v", first)
	require.Len(t, steps, 1)
	assertStepHasFullContent(t, steps[0].(map[string]any))
}
