package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

// A step's catch-up script brings a fresh container to the state the step
// leaves behind — what a learner who solved it by hand would have done. It
// exists so a run whose container is gone can be rebuilt at step N by replaying
// the catch-up scripts of steps 1..N-1.
//
// That makes it the step's solution. Everything below follows from that:
// learners never see it, authors never lose it (every path a scenario travels
// carries it, like the foreground script), and normal play never runs it.

const leakCatchupScript = "SECRET-CATCHUP-script-the-solution: touch /home/student/done"

// catchupStepScenario creates a scenario with two steps, the second carrying a
// catch-up script both inline and as a project file.
func catchupStepScenario(t *testing.T, db *gorm.DB, name string) (*models.Scenario, *models.ProjectFile) {
	t.Helper()

	file := models.ProjectFile{
		Name: "catchup.sh", RelPath: "step2/catchup.sh", ContentType: "script",
		Content: "#!/bin/bash\nmkdir -p /opt/lab", StorageType: "database", SizeBytes: 30,
	}
	require.NoError(t, db.Create(&file).Error)

	scenario := models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Look around"},
			{
				Order:           1,
				Title:           "Step 2",
				TextContent:     "Create /opt/lab",
				VerifyScript:    "#!/bin/bash\ntest -d /opt/lab",
				CatchupScript:   "#!/bin/bash\nmkdir -p /opt/lab",
				CatchupScriptID: &file.ID,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)
	return &scenario, &file
}

// -----------------------------------------------------------------------------
// Redaction
// -----------------------------------------------------------------------------

// The catch-up script is the answer to the step. A learner who can read it can
// skip the exercise, so it is redacted exactly like the verify script.
func TestScenarioStep_CatchupScript_RedactedForLearners(t *testing.T) {
	db := freshTestDB(t)
	_, step, _ := buildLeakyScenarioWithSetupScript(t, db, "catchup-redact", "creator-catchup-redact", nil)

	catchupFile := models.ProjectFile{Name: "catchup.sh", ContentType: "script", Content: leakCatchupScript, StorageType: "database"}
	require.NoError(t, db.Create(&catchupFile).Error)
	require.NoError(t, db.Model(&models.ScenarioStep{}).Where("id = ?", step.ID).Updates(map[string]any{
		"catchup_script":    leakCatchupScript,
		"catchup_script_id": catchupFile.ID,
	}).Error)

	get := func(userID string, roles []string) (int, string) {
		router := setupExtendedReadAuthzTest(t, db, userID, roles)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/scenario-steps/"+step.ID.String(), nil)
		router.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	// Positive control: the author's tooling must see it, or the editor
	// cannot show it and every save would blank it.
	code, body := get("platform-admin-catchup", []string{"administrator"})
	require.Equal(t, http.StatusOK, code, "body=%s", body)
	var admin map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &admin))
	assert.Equal(t, leakCatchupScript, admin["catchup_script"], "a manager must see the catch-up script body")
	assert.Equal(t, catchupFile.ID.String(), admin["catchup_script_id"], "a manager must see the catch-up script id")

	code, body = get("outsider-catchup-redact", []string{"member"})
	require.Equal(t, http.StatusOK, code, "body=%s", body)
	assert.NotContains(t, body, leakCatchupScript, "the catch-up script is the step's solution and must never reach a learner")
	assert.NotContains(t, body, catchupFile.ID.String(), "the catch-up script id must not reach a learner either")
}

// -----------------------------------------------------------------------------
// Authoring: create and patch
// -----------------------------------------------------------------------------

// The editor saves the catch-up script through the generic step routes. Both
// the create and the partial-update DTOs have to carry it, under the same
// json names the rest of the step's scripts use.
func TestScenarioStepPatch_CatchupScript_Updates(t *testing.T) {
	db := freshTestDB(t)
	svc := setupScenarioRegistrationService(t)
	ops, ok := svc.GetEntityOps("ScenarioStep")
	require.True(t, ok)

	scenario := models.Scenario{Name: "catchup-patch", Title: "Catch-up patch", InstanceType: "ubuntu:22.04", CreatedByID: "u1"}
	require.NoError(t, db.Create(&scenario).Error)
	file := models.ProjectFile{Name: "catchup.sh", ContentType: "script", Content: "echo from-file", StorageType: "database"}
	require.NoError(t, db.Create(&file).Error)

	// Create.
	var create dto.CreateScenarioStepInput
	require.NoError(t, json.Unmarshal([]byte(`{
		"scenario_id": "`+scenario.ID.String()+`",
		"order": 1,
		"title": "Step",
		"catchup_script": "echo created"
	}`), &create))
	rawModel, err := ops.ConvertDtoToModel(create)
	require.NoError(t, err)
	step := rawModel.(*models.ScenarioStep)
	assert.Equal(t, "echo created", step.CatchupScript, "create must carry catchup_script")
	require.NoError(t, db.Create(step).Error)

	// Patch the body and point it at a project file.
	var edit dto.EditScenarioStepInput
	require.NoError(t, json.Unmarshal([]byte(`{
		"catchup_script": "echo patched",
		"catchup_script_id": "`+file.ID.String()+`"
	}`), &edit))
	updates, err := ops.ConvertEditDtoToMap(edit)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ScenarioStep{}).Where("id = ?", step.ID).Updates(updates).Error)

	var reloaded models.ScenarioStep
	require.NoError(t, db.First(&reloaded, "id = ?", step.ID).Error)
	assert.Equal(t, "echo patched", reloaded.CatchupScript)
	require.NotNil(t, reloaded.CatchupScriptID)
	assert.Equal(t, file.ID, *reloaded.CatchupScriptID)

	// A patch that does not mention it must leave it alone.
	title := "Renamed"
	updates, err = ops.ConvertEditDtoToMap(dto.EditScenarioStepInput{Title: &title})
	require.NoError(t, err)
	_, touched := updates["catchup_script"]
	assert.False(t, touched, "an unrelated patch must not blank the catch-up script")

	// And the author reads it back through the step output.
	rawOut, err := ops.ConvertModelToDto(&reloaded)
	require.NoError(t, err)
	out, err := json.Marshal(rawOut)
	require.NoError(t, err)
	var outMap map[string]any
	require.NoError(t, json.Unmarshal(out, &outMap))
	assert.Equal(t, "echo patched", outMap["catchup_script"])
	assert.Equal(t, file.ID.String(), outMap["catchup_script_id"])
}

// -----------------------------------------------------------------------------
// Seed and JSON export
// -----------------------------------------------------------------------------

func TestSeedScenario_CatchupScript_Persisted(t *testing.T) {
	db := freshTestDB(t)
	seedSvc := services.NewScenarioSeedService(db)

	input := dto.SeedScenarioInput{
		Title:        "Seeded catch-up",
		InstanceType: "ubuntu:22.04",
		Steps: []dto.SeedStepInput{
			{Title: "No catch-up"},
			{Title: "With catch-up", CatchupScript: "#!/bin/bash\nmkdir -p /opt/lab"},
		},
	}

	scenario, _, err := seedSvc.SeedScenario(input, "creator-1", nil)
	require.NoError(t, err)

	readSteps := func() []models.ScenarioStep {
		var steps []models.ScenarioStep
		require.NoError(t, db.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)
		require.Len(t, steps, 2)
		return steps
	}

	steps := readSteps()
	assert.Empty(t, services.ResolveScriptContent(db, steps[0].CatchupScriptID, steps[0].CatchupScript))
	assert.Equal(t, "#!/bin/bash\nmkdir -p /opt/lab",
		services.ResolveScriptContent(db, steps[1].CatchupScriptID, steps[1].CatchupScript))

	// Re-seeding the same scenario (the upsert path) must keep it too.
	input.Steps[1].CatchupScript = "#!/bin/bash\nmkdir -p /opt/lab2"
	_, isUpdate, err := seedSvc.SeedScenario(input, "creator-1", nil)
	require.NoError(t, err)
	require.True(t, isUpdate)
	steps = readSteps()
	assert.Equal(t, "#!/bin/bash\nmkdir -p /opt/lab2",
		services.ResolveScriptContent(db, steps[1].CatchupScriptID, steps[1].CatchupScript))
}

// The JSON export doubles as the seed input, so what it writes as
// catchup_script has to bind straight back onto a SeedStepInput.
func TestScenarioExportImport_CatchupScript_RoundTrips(t *testing.T) {
	db := freshTestDB(t)
	scenario, file := catchupStepScenario(t, db, "catchup-json-roundtrip")

	// The project file is the source of truth, as for every other script.
	require.NoError(t, db.Model(file).Update("content", "#!/bin/bash\necho from-project-file").Error)

	export, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.Len(t, export.Steps, 2)
	assert.Empty(t, export.Steps[0].CatchupScript)
	assert.Equal(t, "#!/bin/bash\necho from-project-file", export.Steps[1].CatchupScript,
		"export must resolve the catch-up script from its project file")

	raw, err := json.Marshal(export)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"catchup_script":`, "the export's json name mirrors foreground_script")

	var reseed dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &reseed))
	reseed.Title = export.Title + " (reimported)"
	require.Len(t, reseed.Steps, 2)
	assert.Equal(t, "#!/bin/bash\necho from-project-file", reseed.Steps[1].CatchupScript)

	reimported, _, err := services.NewScenarioSeedService(db).SeedScenario(reseed, "creator-2", nil)
	require.NoError(t, err)
	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", reimported.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 2)
	assert.Equal(t, "#!/bin/bash\necho from-project-file",
		services.ResolveScriptContent(db, steps[1].CatchupScriptID, steps[1].CatchupScript))
}

// -----------------------------------------------------------------------------
// KillerCoda archive
// -----------------------------------------------------------------------------

// KillerCoda has no catch-up script, so it travels as an OCF extension: the
// file sits next to the step's other scripts as stepN/catchup.sh and index.json
// points at it with a "catchup" field. KillerCoda ignores the unknown field.
func TestKillerCodaExportImport_CatchupScript_RoundTrips(t *testing.T) {
	db := freshTestDB(t)
	scenario, _ := catchupStepScenario(t, db, "catchup-kc-roundtrip")

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(scenario.ID)
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

	assert.Equal(t, "#!/bin/bash\nmkdir -p /opt/lab", string(files["step2/catchup.sh"]),
		"the archive must carry the catch-up script as stepN/catchup.sh")
	_, step1HasCatchup := files["step1/catchup.sh"]
	assert.False(t, step1HasCatchup, "a step without a catch-up script writes no catchup.sh")

	var rawIndex struct {
		Details struct {
			Steps []map[string]any `json:"steps"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(files["index.json"], &rawIndex))
	require.Len(t, rawIndex.Details.Steps, 2)
	assert.Equal(t, "step2/catchup.sh", rawIndex.Details.Steps[1]["catchup"],
		"index.json must point at the catch-up script with the OCF-extension 'catchup' field")

	// Import the archive back into an empty database.
	dir := t.TempDir()
	for name, data := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	db = freshTestDB(t)
	importer := services.NewScenarioImporterService(db)

	imported, err := importer.ImportFromDirectory(dir, "creator-2", nil, "upload")
	require.NoError(t, err)

	readSteps := func(scenarioID any) []models.ScenarioStep {
		var steps []models.ScenarioStep
		require.NoError(t, db.Where("scenario_id = ?", scenarioID).Order("\"order\" ASC").Find(&steps).Error)
		require.Len(t, steps, 2)
		return steps
	}
	steps := readSteps(imported.ID)
	assert.Empty(t, services.ResolveScriptContent(db, steps[0].CatchupScriptID, steps[0].CatchupScript))
	assert.Equal(t, "#!/bin/bash\nmkdir -p /opt/lab",
		services.ResolveScriptContent(db, steps[1].CatchupScriptID, steps[1].CatchupScript))
	require.NotNil(t, steps[1].CatchupScriptID, "import must store the catch-up script as a project file, like the other scripts")
	var file models.ProjectFile
	require.NoError(t, db.First(&file, "id = ?", *steps[1].CatchupScriptID).Error)
	assert.Equal(t, "step2/catchup.sh", file.RelPath, "the project file keeps the archive path for round-trip fidelity")

	// Re-importing the same archive replaces the old catch-up file rather than
	// orphaning it.
	reimported, err := importer.ImportFromDirectory(dir, "creator-2", nil, "upload")
	require.NoError(t, err)
	assert.Equal(t, imported.ID, reimported.ID)
	var catchupFiles int64
	require.NoError(t, db.Model(&models.ProjectFile{}).Where("name = ?", "catchup.sh").Count(&catchupFiles).Error)
	assert.Equal(t, int64(1), catchupFiles, "re-import must clean up the previous catch-up project file")
}

// An archive may name the catch-up file anything; the importer follows the
// path index.json declares.
func TestKillerCodaImport_CatchupScript_FollowsDeclaredPath(t *testing.T) {
	db := freshTestDB(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1/text.md"), []byte("Do it"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1/solve.sh"), []byte("echo solved"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "Declared catch-up path",
		"details": {
			"intro": {"text": ""},
			"steps": [{"title": "One", "text": "step1/text.md", "catchup": "step1/solve.sh"}],
			"finish": {"text": ""}
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`), 0o644))

	imported, err := services.NewScenarioImporterService(db).ImportFromDirectory(dir, "creator-1", nil, "upload")
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", imported.ID).Find(&steps).Error)
	require.Len(t, steps, 1)
	assert.Equal(t, "echo solved", services.ResolveScriptContent(db, steps[0].CatchupScriptID, steps[0].CatchupScript))
}

// -----------------------------------------------------------------------------
// Duplicate
// -----------------------------------------------------------------------------

func TestScenarioDuplicate_CopiesCatchupScriptAndRemapsFileID(t *testing.T) {
	db := freshTestDB(t)
	source, sourceFile := catchupStepScenario(t, db, "catchup-duplicate")

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "creator-2", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", copied.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 2)

	assert.Equal(t, "#!/bin/bash\nmkdir -p /opt/lab", steps[1].CatchupScript, "the inline body is copied")
	require.NotNil(t, steps[1].CatchupScriptID, "the file reference is copied")
	assert.NotEqual(t, sourceFile.ID, *steps[1].CatchupScriptID,
		"the copy must own its own project file, or editing it would edit the source")

	var copiedFile models.ProjectFile
	require.NoError(t, db.First(&copiedFile, "id = ?", *steps[1].CatchupScriptID).Error)
	assert.Equal(t, sourceFile.Content, copiedFile.Content)
	assert.Nil(t, steps[0].CatchupScriptID)
}

// -----------------------------------------------------------------------------
// Project-file usage
// -----------------------------------------------------------------------------

// The file browser lists what each project file is for, and refuses to let an
// author delete a file still in use. A catch-up file it does not know about
// shows as unused and can be deleted from under its step.
func TestProjectFileUsage_ReportsCatchupScriptRefs(t *testing.T) {
	db := freshTestDB(t)
	scenario, file := catchupStepScenario(t, db, "catchup-usage")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/:id/usage", adminMiddleware(), ctrl.GetUsage)
	r.GET("/api/v1/project-files/by-scenario/:scenarioId", adminMiddleware(), ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+file.ID.String()+"/usage", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var usage []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &usage))
	require.Len(t, usage, 1, "the catch-up file is used exactly once")
	assert.Equal(t, "catchup_script", usage[0]["field"])
	assert.Equal(t, "Step 2", usage[0]["step_title"])

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+scenario.ID.String(), nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var byScenario []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &byScenario))
	usedAs := make(map[string]bool)
	for _, item := range byScenario {
		if s, ok := item["used_as"].(string); ok {
			usedAs[s] = true
		}
	}
	assert.True(t, usedAs["Step 2 — catchup_script"], "by-scenario must list the catch-up file; got %v", usedAs)
}

// -----------------------------------------------------------------------------
// Never run during play
// -----------------------------------------------------------------------------

// The catch-up script replays the step's solution. Running it while the
// learner plays would solve the step for them — or, run after they solved it,
// do the work twice. Only a rebuild runs it, so starting a run and advancing
// through every step must never send it to the container in any form.
func TestAdvance_NeverRunsCatchupScript(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "catchup-never-runs",
		Title:        "Catch-up never runs",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", BackgroundScript: "echo bg-1", CatchupScript: "echo CATCHUP-MARKER-1"},
			{Order: 1, Title: "Step 2", BackgroundScript: "echo bg-2", ForegroundScript: "echo fg-2", CatchupScript: "echo CATCHUP-MARKER-2"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-catchup", scenario.ID, "terminal-catchup", "")
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	for i := 0; i < 2; i++ {
		result, err := sessionSvc.VerifyCurrentStep(session.ID)
		require.NoError(t, err)
		require.True(t, result.Passed)
	}

	// Prove both steps were actually provisioned, so the negatives below cannot
	// pass just because nothing ran.
	var execs []string
	for _, c := range verifySvc.execCalls {
		execs = append(execs, strings.Join(c.command, " "))
	}
	allExecs := strings.Join(execs, "\n")
	require.Contains(t, allExecs, "echo bg-1", "step 1's background script must have run")
	require.Contains(t, allExecs, "echo bg-2", "step 2's background script must have run")
	require.Len(t, verifySvc.consoleWrites, 1, "step 2's foreground script must have run")
	require.Equal(t, "echo fg-2", verifySvc.consoleWrites[0].text)

	for _, c := range verifySvc.execCalls {
		assert.NotContains(t, strings.Join(c.command, " "), "CATCHUP-MARKER", "exec must never carry the catch-up script")
	}
	for _, p := range verifySvc.pushFileCalls {
		assert.NotContains(t, p.content, "CATCHUP-MARKER", "no pushed file may carry the catch-up script")
	}
	for _, w := range verifySvc.consoleWrites {
		assert.NotContains(t, w.text, "CATCHUP-MARKER", "the catch-up script must never be typed into the learner's shell")
	}
}
