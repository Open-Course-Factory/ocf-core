package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// These tests cover round-trip preservation of step_type (notably "quiz"),
// ShowImmediateFeedback, and ScenarioStepQuestion records through the
// Export/Import/Seed pipeline. They are designed to FAIL today (RED phase
// of TDD) because the export DTOs, the seed service, the archive writer
// and the importer all ignore these fields. They should PASS once
// backend-dev wires the missing fields through each stage.
//
// The tests use raw JSON marshal/unmarshal of export DTOs so the file
// COMPILES today even though the new DTO fields don't exist yet — the
// failures are on assertions, not on package build.

// localQuestionInput mirrors the shape backend-dev will add to
// dto.SeedStepInput.Questions. We marshal a SeedScenarioInput to JSON
// with these extra keys, then unmarshal back into dto.SeedScenarioInput
// — today's struct silently drops the unknown keys, so SeedScenario
// receives no questions and persists none, which is exactly what we
// want to assert against.
type localQuestionInput struct {
	Order         int    `json:"order"`
	QuestionText  string `json:"question_text"`
	QuestionType  string `json:"question_type"`
	Options       string `json:"options,omitempty"`
	CorrectAnswer string `json:"correct_answer,omitempty"`
	Explanation   string `json:"explanation,omitempty"`
	Points        int    `json:"points,omitempty"`
}

// localStepInputExt is dto.SeedStepInput with the (future) extension
// fields that backend-dev will add. We marshal this to JSON, then
// unmarshal into dto.SeedStepInput.
type localStepInputExt struct {
	Title                 string               `json:"title"`
	StepType              string               `json:"step_type,omitempty"`
	ShowImmediateFeedback bool                 `json:"show_immediate_feedback,omitempty"`
	TextContent           string               `json:"text_content,omitempty"`
	HintContent           string               `json:"hint_content,omitempty"`
	VerifyScript          string               `json:"verify_script,omitempty"`
	BackgroundScript      string               `json:"background_script,omitempty"`
	ForegroundScript      string               `json:"foreground_script,omitempty"`
	HasFlag               bool                 `json:"has_flag,omitempty"`
	FlagPath              string               `json:"flag_path,omitempty"`
	Questions             []localQuestionInput `json:"questions,omitempty"`
}

type localScenarioInputExt struct {
	Title         string              `json:"title"`
	Description   string              `json:"description,omitempty"`
	Difficulty    string              `json:"difficulty,omitempty"`
	EstimatedTime string              `json:"estimated_time,omitempty"`
	InstanceType  string              `json:"instance_type"`
	OsType        string              `json:"os_type,omitempty"`
	FlagsEnabled  bool                `json:"flags_enabled,omitempty"`
	GshEnabled    bool                `json:"gsh_enabled,omitempty"`
	CrashTraps    bool                `json:"crash_traps,omitempty"`
	IsPublic      bool                `json:"is_public,omitempty"`
	IntroText     string              `json:"intro_text,omitempty"`
	FinishText    string              `json:"finish_text,omitempty"`
	SetupScript   string              `json:"setup_script,omitempty"`
	Steps         []localStepInputExt `json:"steps"`
}

// buildSeedInputWithQuestions marshals a localScenarioInputExt to JSON
// and unmarshals it into dto.SeedScenarioInput. Today, the unknown
// step_type / show_immediate_feedback / questions keys are silently
// dropped. Once backend-dev adds the fields, they will be preserved.
func buildSeedInputWithQuestions(t *testing.T, ext localScenarioInputExt) dto.SeedScenarioInput {
	t.Helper()
	raw, err := json.Marshal(ext)
	require.NoError(t, err)
	var input dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &input))
	return input
}

// --- 1. Export JSON preserves step_type and quiz data ---

func TestExportService_ExportAsJSON_PreservesStepTypeAndQuiz(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "quiz-export",
		Title:        "Quiz Export",
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{
				Order:    0,
				Title:    "Terminal Step",
				StepType: "terminal",
			},
			{
				Order:                 1,
				Title:                 "Quiz Step",
				StepType:              "quiz",
				ShowImmediateFeedback: true,
				Questions: []models.ScenarioStepQuestion{
					{
						Order:         0,
						QuestionText:  "Pick A or B",
						QuestionType:  "multiple_choice",
						Options:       `["A","B"]`,
						CorrectAnswer: "A",
						Explanation:   "A is correct",
						Points:        2,
					},
					{
						Order:         1,
						QuestionText:  "Which shell?",
						QuestionType:  "free_text",
						CorrectAnswer: "bash",
						Points:        1,
					},
				},
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	export, err := exportService.ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.NotNil(t, export)
	require.Len(t, export.Steps, 2)

	// Marshal the export to JSON and inspect the raw shape so the test
	// compiles before backend-dev adds the new DTO fields.
	raw, err := json.Marshal(export)
	require.NoError(t, err)

	var parsed struct {
		Steps []map[string]any `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Len(t, parsed.Steps, 2)

	// Step 0: terminal
	assert.Equal(t, "terminal", parsed.Steps[0]["step_type"],
		"step 0 export must preserve step_type='terminal'")

	// Step 1: quiz
	assert.Equal(t, "quiz", parsed.Steps[1]["step_type"],
		"step 1 export must preserve step_type='quiz'")
	assert.Equal(t, true, parsed.Steps[1]["show_immediate_feedback"],
		"step 1 export must preserve show_immediate_feedback=true")

	questionsRaw, ok := parsed.Steps[1]["questions"].([]any)
	require.True(t, ok, "step 1 export must contain a questions array")
	require.Len(t, questionsRaw, 2)

	// Sort questions by order before assertions
	q0 := questionsRaw[0].(map[string]any)
	q1 := questionsRaw[1].(map[string]any)
	if int(q0["order"].(float64)) > int(q1["order"].(float64)) {
		q0, q1 = q1, q0
	}

	assert.Equal(t, "Pick A or B", q0["question_text"])
	assert.Equal(t, "multiple_choice", q0["question_type"])
	assert.Equal(t, `["A","B"]`, q0["options"])
	assert.Equal(t, "A", q0["correct_answer"])
	assert.Equal(t, "A is correct", q0["explanation"])
	assert.Equal(t, float64(2), q0["points"])

	assert.Equal(t, "Which shell?", q1["question_text"])
	assert.Equal(t, "free_text", q1["question_type"])
	assert.Equal(t, "bash", q1["correct_answer"])
	assert.Equal(t, float64(1), q1["points"])
}

// --- 2. Seed service creates quiz questions ---

func TestSeedService_SeedScenario_CreatesQuizQuestions(t *testing.T) {
	db := freshTestDB(t)
	seedService := services.NewScenarioSeedService(db)

	ext := localScenarioInputExt{
		Title:        "Quiz Seed",
		InstanceType: "ubuntu:22.04",
		Steps: []localStepInputExt{
			{
				Title:                 "Quiz Step",
				StepType:              "quiz",
				ShowImmediateFeedback: true,
				Questions: []localQuestionInput{
					{
						Order:         0,
						QuestionText:  "Q1?",
						QuestionType:  "multiple_choice",
						Options:       `["X","Y"]`,
						CorrectAnswer: "X",
						Explanation:   "Because",
						Points:        3,
					},
					{
						Order:         1,
						QuestionText:  "Q2?",
						QuestionType:  "free_text",
						CorrectAnswer: "answer",
						Points:        1,
					},
				},
			},
		},
	}

	input := buildSeedInputWithQuestions(t, ext)

	scenario, _, err := seedService.SeedScenario(input, "user-1", nil)
	require.NoError(t, err)

	// Reload with questions preloaded
	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).Preload("Steps.Questions", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error)

	require.Len(t, loaded.Steps, 1)
	step := loaded.Steps[0]

	assert.Equal(t, "quiz", step.StepType,
		"step.StepType must be 'quiz' after seeding (today: empty/'terminal')")
	assert.True(t, step.ShowImmediateFeedback,
		"step.ShowImmediateFeedback must be true after seeding")
	require.Len(t, step.Questions, 2,
		"two ScenarioStepQuestion rows must be persisted (today: 0)")

	q0 := step.Questions[0]
	assert.Equal(t, 0, q0.Order)
	assert.Equal(t, "Q1?", q0.QuestionText)
	assert.Equal(t, "multiple_choice", q0.QuestionType)
	assert.Equal(t, `["X","Y"]`, q0.Options)
	assert.Equal(t, "X", q0.CorrectAnswer)
	assert.Equal(t, "Because", q0.Explanation)
	assert.Equal(t, 3, q0.Points)

	q1 := step.Questions[1]
	assert.Equal(t, 1, q1.Order)
	assert.Equal(t, "Q2?", q1.QuestionText)
	assert.Equal(t, "free_text", q1.QuestionType)
	assert.Equal(t, "answer", q1.CorrectAnswer)
	assert.Equal(t, 1, q1.Points)
}

// --- 3. Full JSON round-trip: seed → export → re-seed ---

func TestExportImport_JSONRoundtrip_Quiz(t *testing.T) {
	db := freshTestDB(t)
	seedService := services.NewScenarioSeedService(db)
	exportService := services.NewScenarioExportService(db)

	ext := localScenarioInputExt{
		Title:        "Roundtrip Quiz",
		InstanceType: "ubuntu:22.04",
		Steps: []localStepInputExt{
			{
				Title:                 "Quiz",
				StepType:              "quiz",
				ShowImmediateFeedback: true,
				Questions: []localQuestionInput{
					{
						Order:         0,
						QuestionText:  "Q?",
						QuestionType:  "multiple_choice",
						Options:       `["A","B"]`,
						CorrectAnswer: "A",
						Explanation:   "Because A",
						Points:        2,
					},
				},
			},
		},
	}
	input := buildSeedInputWithQuestions(t, ext)

	original, _, err := seedService.SeedScenario(input, "user-1", nil)
	require.NoError(t, err)

	// Export the original as JSON
	export, err := exportService.ExportAsJSON(original.ID)
	require.NoError(t, err)

	// Re-encode the export, then re-decode it back into a SeedScenarioInput.
	// We change the title so the upsert creates a new scenario.
	raw, err := json.Marshal(export)
	require.NoError(t, err)

	var reimport dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &reimport))
	reimport.Title = "Roundtrip Quiz Reimported"

	reimported, isUpdate, err := seedService.SeedScenario(reimport, "user-2", nil)
	require.NoError(t, err)
	assert.False(t, isUpdate)
	assert.NotEqual(t, original.ID, reimported.ID)

	// Reload with questions
	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).Preload("Steps.Questions", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", reimported.ID).Error)

	require.Len(t, loaded.Steps, 1)
	step := loaded.Steps[0]
	assert.Equal(t, "quiz", step.StepType, "round-tripped step.StepType must be 'quiz'")
	assert.True(t, step.ShowImmediateFeedback, "round-tripped show_immediate_feedback must be true")
	require.Len(t, step.Questions, 1, "round-tripped quiz must have 1 question (today: 0)")

	q := step.Questions[0]
	assert.Equal(t, "Q?", q.QuestionText)
	assert.Equal(t, "multiple_choice", q.QuestionType)
	assert.Equal(t, `["A","B"]`, q.Options)
	assert.Equal(t, "A", q.CorrectAnswer)
	assert.Equal(t, "Because A", q.Explanation)
	assert.Equal(t, 2, q.Points)
}

// --- 4. Archive: ocf.json sidecar without polluting index.json ---

func TestExportArchive_AddsQuizFileWithoutChangingIndexJson(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "archive-quiz",
		Title:        "Archive Quiz",
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{
				Order:    0,
				Title:    "Intro",
				StepType: "terminal",
				TextContent: "intro text",
			},
			{
				Order:                 1,
				Title:                 "Quiz",
				StepType:              "quiz",
				ShowImmediateFeedback: true,
				TextContent:           "Answer the questions",
				Questions: []models.ScenarioStepQuestion{
					{
						Order:         0,
						QuestionText:  "Q1?",
						QuestionType:  "multiple_choice",
						Options:       `["A","B"]`,
						CorrectAnswer: "A",
						Explanation:   "A is right",
						Points:        2,
					},
					{
						Order:         1,
						QuestionText:  "Q2?",
						QuestionType:  "free_text",
						CorrectAnswer: "bash",
						Points:        1,
					},
				},
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	zipBytes, _, err := exportService.ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	files := make(map[string][]byte)
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		files[f.Name] = data
	}

	// index.json must still exist and parse as a valid KillerCodaIndex
	indexBytes, ok := files["index.json"]
	require.True(t, ok, "archive must contain index.json")

	var index services.KillerCodaIndex
	require.NoError(t, json.Unmarshal(indexBytes, &index))
	assert.Equal(t, "Archive Quiz", index.Title)
	assert.Equal(t, "ubuntu:22.04", index.Backend.ImageID)
	require.Len(t, index.Details.Steps, 2)

	// index.json's step entries must NOT carry step_type or questions
	// at the top level — those are an OCF extension and live in ocf.json.
	var rawIndex map[string]any
	require.NoError(t, json.Unmarshal(indexBytes, &rawIndex))
	details, ok := rawIndex["details"].(map[string]any)
	require.True(t, ok)
	stepsRaw, ok := details["steps"].([]any)
	require.True(t, ok)
	for i, s := range stepsRaw {
		stepMap := s.(map[string]any)
		_, hasStepType := stepMap["step_type"]
		assert.False(t, hasStepType, "index.json step %d must not contain step_type at the top level", i)
		_, hasQuestions := stepMap["questions"]
		assert.False(t, hasQuestions, "index.json step %d must not contain questions at the top level", i)
	}

	// step2/ocf.json must exist (the quiz step) with the expected payload.
	ocfBytes, ok := files["step2/ocf.json"]
	require.True(t, ok, "archive must contain step2/ocf.json for the quiz step (today: missing)")

	var ocf map[string]any
	require.NoError(t, json.Unmarshal(ocfBytes, &ocf))
	assert.Equal(t, "quiz", ocf["step_type"])
	assert.Equal(t, true, ocf["show_immediate_feedback"])

	questionsAny, ok := ocf["questions"].([]any)
	require.True(t, ok, "step2/ocf.json must include a questions array")
	require.Len(t, questionsAny, 2)

	q0 := questionsAny[0].(map[string]any)
	assert.Equal(t, "Q1?", q0["question_text"])
	assert.Equal(t, "multiple_choice", q0["question_type"])
	assert.Equal(t, `["A","B"]`, q0["options"])
	assert.Equal(t, "A", q0["correct_answer"])
	assert.Equal(t, "A is right", q0["explanation"])
	assert.Equal(t, float64(2), q0["points"])
}

// --- 5. Importer reads ocf.json sidecars for quiz steps ---

func TestImportFromDirectory_ReadsOcfJsonForQuizSteps(t *testing.T) {
	db := freshTestDB(t)

	dir := t.TempDir()

	indexJSON := []byte(`{
		"title": "Quiz Import",
		"description": "Has a quiz",
		"difficulty": "beginner",
		"time": "5",
		"intro": {"text": "", "background": "", "foreground": ""},
		"finish": {"text": "", "background": "", "foreground": ""},
		"details": {
			"intro": {"text": "", "background": "", "foreground": ""},
			"finish": {"text": "", "background": "", "foreground": ""},
			"steps": [
				{"title": "Quiz Only", "text": "step1/text.md", "verify": "", "background": "", "foreground": "", "hint": "", "has_flag": null, "flag_path": ""}
			]
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), indexJSON, 0o644))

	stepDir := filepath.Join(dir, "step1")
	require.NoError(t, os.MkdirAll(stepDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stepDir, "text.md"), []byte("Answer the questions"), 0o644))

	ocfJSON := []byte(`{
		"step_type": "quiz",
		"show_immediate_feedback": true,
		"questions": [
			{
				"order": 0,
				"question_text": "Q?",
				"question_type": "multiple_choice",
				"options": "[\"A\",\"B\"]",
				"correct_answer": "A",
				"explanation": "because",
				"points": 1
			}
		]
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(stepDir, "ocf.json"), ocfJSON, 0o644))

	importerService := services.NewScenarioImporterService(db)
	scenario, err := importerService.ImportFromDirectory(dir, "user-1", nil, "upload")
	require.NoError(t, err)

	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).Preload("Steps.Questions", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error)

	require.Len(t, loaded.Steps, 1)
	step := loaded.Steps[0]

	assert.Equal(t, "quiz", step.StepType,
		"importer must read step_type='quiz' from ocf.json (today: defaults to 'terminal')")
	assert.True(t, step.ShowImmediateFeedback,
		"importer must read show_immediate_feedback=true from ocf.json (today: false)")
	require.Len(t, step.Questions, 1,
		"importer must persist 1 quiz question from ocf.json (today: 0)")

	q := step.Questions[0]
	assert.Equal(t, 0, q.Order)
	assert.Equal(t, "Q?", q.QuestionText)
	assert.Equal(t, "multiple_choice", q.QuestionType)
	assert.Equal(t, `["A","B"]`, q.Options)
	assert.Equal(t, "A", q.CorrectAnswer)
	assert.Equal(t, "because", q.Explanation)
	assert.Equal(t, 1, q.Points)
}

// --- 6. Legacy archive (no ocf.json) still imports as terminal step ---

func TestImportFromDirectory_LegacyArchiveStillWorks(t *testing.T) {
	db := freshTestDB(t)

	dir := t.TempDir()

	indexJSON := []byte(`{
		"title": "Legacy KillerCoda",
		"description": "No ocf.json sidecars",
		"difficulty": "beginner",
		"time": "5",
		"intro": {"text": "", "background": "", "foreground": ""},
		"finish": {"text": "", "background": "", "foreground": ""},
		"details": {
			"intro": {"text": "", "background": "", "foreground": ""},
			"finish": {"text": "", "background": "", "foreground": ""},
			"steps": [
				{"title": "Plain Step", "text": "step1/text.md", "verify": "", "background": "", "foreground": "", "hint": "", "has_flag": null, "flag_path": ""}
			]
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), indexJSON, 0o644))

	stepDir := filepath.Join(dir, "step1")
	require.NoError(t, os.MkdirAll(stepDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stepDir, "text.md"), []byte("plain content"), 0o644))

	importerService := services.NewScenarioImporterService(db)
	scenario, err := importerService.ImportFromDirectory(dir, "user-1", nil, "upload")
	require.NoError(t, err)

	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).Preload("Steps.Questions", func(d *gorm.DB) *gorm.DB {
		return d.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error)

	require.Len(t, loaded.Steps, 1)
	step := loaded.Steps[0]

	// Default behaviour: legacy KillerCoda steps without ocf.json must
	// remain terminal steps with no questions. This guards against the
	// fix accidentally breaking legacy archives.
	stepType := step.StepType
	if stepType == "" {
		stepType = "terminal"
	}
	assert.Equal(t, "terminal", stepType, "legacy step must default to step_type='terminal'")
	assert.False(t, step.ShowImmediateFeedback, "legacy step must keep show_immediate_feedback=false")
	assert.Len(t, step.Questions, 0, "legacy step must have no quiz questions")
	assert.Equal(t, "plain content", step.TextContent, "legacy step text content must still load")
}
