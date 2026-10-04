package scenarios_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func seedProblems(t *testing.T, input dto.SeedScenarioInput) []string {
	t.Helper()
	db := freshTestDB(t)
	_, _, err := services.NewScenarioSeedService(db).SeedScenario(input, "author", nil)
	var contentErr *services.ScenarioContentError
	require.True(t, errors.As(err, &contentErr), "expected a content error, got %v", err)

	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Zero(t, count, "a refused import must write nothing")
	return contentErr.Problems
}

func TestSeedScenario_InvalidContent_ListsEveryProblem(t *testing.T) {
	input := dto.SeedScenarioInput{
		Title:    "  ",
		Hostname: "Bad_Host",
		Steps: []dto.SeedStepInput{
			{Title: "Typo", StepType: "quizz"},
			{Title: "Empty quiz", StepType: "quiz"},
			{Title: "Click through", StepType: "terminal"},
			{Title: "Bad answers", StepType: "quiz", Questions: []dto.SeedQuestionInput{
				{QuestionText: "Out of range", QuestionType: "multiple_choice", Options: `["a","b"]`, CorrectAnswer: "4"},
				{QuestionText: "By text", QuestionType: "multiple_choice", Options: `["a","b"]`, CorrectAnswer: "a"},
				{QuestionText: "One option", QuestionType: "multiple_choice", Options: `["a"]`, CorrectAnswer: "0"},
				{QuestionText: "Not JSON", QuestionType: "multiple_choice", Options: `a, b`, CorrectAnswer: "0"},
				{QuestionText: "Yes?", QuestionType: "true_false", CorrectAnswer: "yes"},
				{QuestionText: "Essay", QuestionType: "essay", CorrectAnswer: "x"},
				{QuestionText: "Unsorted", QuestionType: "multi_answer", Options: `["a","b","c"]`, CorrectAnswer: "[2,0]"},
				{QuestionText: "Fine", QuestionType: "multi_answer", Options: `["a","b","c"]`, CorrectAnswer: "[0,2]"},
			}},
		},
		Translations: []dto.SeedScenarioTranslationInput{{Locale: "fr"}, {Locale: "fr"}},
		Lexicon:      []dto.LexiconEntryInput{{Key: "CELLAR", ParentKey: "NOWHERE"}},
	}

	assert.Equal(t, []string{
		"title is empty",
		`hostname "Bad_Host" is invalid: use 1 to 63 lowercase letters, digits or hyphens, not starting or ending with a hyphen`,
		`step 1 (Typo): step_type "quizz" is not one of terminal, flag, quiz, info`,
		"step 2 (Empty quiz): a quiz step needs at least one question",
		`step 4 (Bad answers), question 1: correct_answer "4" is not an option index (0 to 1 for 2 options)`,
		`step 4 (Bad answers), question 2: correct_answer "a" is not an option index (0 to 1 for 2 options)`,
		"step 4 (Bad answers), question 3: options needs at least 2 choices (got 1)",
		`step 4 (Bad answers), question 4: options must be a JSON array of strings encoded as a string, like "[\"yes\", \"no\"]" (got "a, b")`,
		`step 4 (Bad answers), question 5: correct_answer "yes" must be "true" or "false"`,
		`step 4 (Bad answers), question 6: question_type "essay" is not one of multiple_choice, multi_answer, free_text, true_false`,
		`step 4 (Bad answers), question 7: correct_answer "[2,0]" must list the right option indexes (0 to 2), sorted, without spaces, like "[0,2]"`,
		`translations: locale "fr" appears twice`,
		`lexicon: "CELLAR" sits inside "NOWHERE", which does not exist`,
	}, seedProblems(t, input))
}

func TestSeedScenario_NoSteps_IsRefused(t *testing.T) {
	assert.Equal(t, []string{"the scenario has no steps"},
		seedProblems(t, dto.SeedScenarioInput{Title: "Empty"}))
}

func TestImportFromDirectory_InvalidContent_IsAContentError(t *testing.T) {
	db := freshTestDB(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("do it"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "extensions.json"),
		[]byte(`{"step_type":"quiz","questions":[{"question_text":"?","question_type":"true_false","correct_answer":"maybe"}]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "Archive with a bad quiz",
		"details": {"steps": [{"title": "Quiz", "text": "step1/text.md"}]},
		"backend": {"imageid": "s"}
	}`), 0o644))

	_, err := services.NewScenarioImporterService(db).ImportFromDirectory(dir, "author", nil, "upload")

	var contentErr *services.ScenarioContentError
	require.True(t, errors.As(err, &contentErr), "expected a content error, got %v", err)
	assert.Equal(t, []string{`step 1 (Quiz), question 1: correct_answer "maybe" must be "true" or "false"`}, contentErr.Problems)
}

func TestOrgImportJSON_InvalidContent_Answers400WithDetails(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-invalid-import"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	body, _ := json.Marshal(map[string]any{
		"title": "Dead ends",
		"steps": []map[string]any{{"title": "Typo", "step_type": "quizz"}, {"title": "Empty quiz", "step_type": "quiz"}},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var response struct {
		ErrorMessage string   `json:"error_message"`
		Details      []string `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, response.ErrorMessage, "2 problem")
	assert.Equal(t, []string{
		`step 1 (Typo): step_type "quizz" is not one of terminal, flag, quiz, info`,
		"step 2 (Empty quiz): a quiz step needs at least one question",
	}, response.Details)
}

// A file says what a scenario is, not who may see it: an organisation's
// import never publishes, whatever the file claims.
func TestOrgImportJSON_IgnoresIsPublic(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-public-import"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	body, _ := json.Marshal(map[string]any{
		"title":     "Wants to be public",
		"is_public": true,
		"steps":     []map[string]any{{"title": "Read", "step_type": "info"}},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var stored models.Scenario
	require.NoError(t, db.Where("organization_id = ?", orgID).First(&stored).Error)
	assert.False(t, stored.IsPublic)
}

// Every real scenario in the challenges repository must still import. The
// validator is only worth having if it refuses broken content and nothing else.
func TestChallengesContent_PassesImportValidation(t *testing.T) {
	root := filepath.Join("..", "..", "..", "challenges")
	if _, err := os.Stat(root); err != nil {
		root = filepath.Join("..", "..", "..", "..", "..", "..", "challenges") // from a worktree
	}
	indexes, _ := filepath.Glob(filepath.Join(root, "*", "index.json"))
	if len(indexes) == 0 {
		t.Skip("challenges repository not checked out next to ocf-core")
	}

	importer := services.NewScenarioImporterService(nil)
	for _, indexPath := range indexes {
		dir := filepath.Dir(indexPath)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			data, err := os.ReadFile(indexPath)
			require.NoError(t, err)
			index, err := importer.ParseIndexJSON(data)
			require.NoError(t, err)
			_, err = importer.BuildScenarioFromIndex(index, dir, "author", nil, "builtin")
			assert.NoError(t, err)
		})
	}
}
