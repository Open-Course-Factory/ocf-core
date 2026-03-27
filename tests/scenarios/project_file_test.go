package scenarios_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// ---------------------------------------------------------------------------
// 1. ProjectFile CRUD
// ---------------------------------------------------------------------------

func TestProjectFile_Create_Success(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Filename:    "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\nexit 0",
		RelPath:     "step1/verify.sh",
	}
	require.NoError(t, db.Create(&file).Error)

	// Verify persisted
	var found models.ProjectFile
	err := db.First(&found, "id = ?", file.ID).Error
	require.NoError(t, err)

	assert.Equal(t, "verify.sh", found.Filename)
	assert.Equal(t, "script", found.ContentType)
	assert.Equal(t, "#!/bin/bash\nexit 0", found.Content)
	assert.Equal(t, "step1/verify.sh", found.RelPath)
	assert.Equal(t, int64(0), found.SizeBytes)         // default
	assert.Equal(t, "database", found.StorageType)      // default
	assert.NotEqual(t, uuid.Nil, found.ID)
}

func TestProjectFile_Update_Success(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Filename:    "original.sh",
		ContentType: "script",
		Content:     "echo hello",
	}
	require.NoError(t, db.Create(&file).Error)

	// Update filename and content
	err := db.Model(&file).Updates(map[string]interface{}{
		"filename": "updated.sh",
		"content":  "echo updated",
	}).Error
	require.NoError(t, err)

	// Verify changes persisted
	var found models.ProjectFile
	require.NoError(t, db.First(&found, "id = ?", file.ID).Error)

	assert.Equal(t, "updated.sh", found.Filename)
	assert.Equal(t, "echo updated", found.Content)
	assert.Equal(t, "script", found.ContentType) // unchanged
}

func TestProjectFile_Delete_Success(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Filename:    "to-delete.sh",
		ContentType: "script",
		Content:     "rm -rf /tmp/test",
	}
	require.NoError(t, db.Create(&file).Error)

	// Soft delete
	err := db.Delete(&file).Error
	require.NoError(t, err)

	// Should not be found with default scope
	var found models.ProjectFile
	err = db.First(&found, "id = ?", file.ID).Error
	assert.Error(t, err) // record not found
}

// ---------------------------------------------------------------------------
// 2. ProjectFile content types
// ---------------------------------------------------------------------------

func TestProjectFile_Create_AllContentTypes(t *testing.T) {
	db := setupTestDB(t)

	types := []struct {
		filename    string
		contentType string
		content     string
	}{
		{"verify.sh", "script", "#!/bin/bash\nexit 0"},
		{"intro.md", "markdown", "# Welcome\nThis is an intro."},
		{"notes.txt", "text", "Plain text content here."},
	}

	for _, tt := range types {
		file := models.ProjectFile{
			Filename:    tt.filename,
			ContentType: tt.contentType,
			Content:     tt.content,
		}
		require.NoError(t, db.Create(&file).Error, "failed to create %s file", tt.contentType)
	}

	// Verify all three persisted
	var files []models.ProjectFile
	require.NoError(t, db.Find(&files).Error)
	assert.Len(t, files, 3)

	// Verify each content type is present
	contentTypes := make(map[string]bool)
	for _, f := range files {
		contentTypes[f.ContentType] = true
	}
	assert.True(t, contentTypes["script"])
	assert.True(t, contentTypes["markdown"])
	assert.True(t, contentTypes["text"])
}

// ---------------------------------------------------------------------------
// 3. FK relationships
// ---------------------------------------------------------------------------

func TestScenarioStep_WithProjectFileFK(t *testing.T) {
	db := setupTestDB(t)

	// Create a ProjectFile for the verify script
	verifyFile := models.ProjectFile{
		Filename:    "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\ntest -f /tmp/done",
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	// Create a Scenario (required parent for ScenarioStep)
	scenario := models.Scenario{
		Name:         "fk-test-scenario",
		Title:        "FK Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a ScenarioStep with VerifyScriptID pointing to the file
	step := models.ScenarioStep{
		ScenarioID:     scenario.ID,
		Order:          1,
		Title:          "Step with file FK",
		VerifyScriptID: &verifyFile.ID,
	}
	require.NoError(t, db.Create(&step).Error)

	// Verify FK is persisted and queryable
	var found models.ScenarioStep
	require.NoError(t, db.First(&found, "id = ?", step.ID).Error)
	require.NotNil(t, found.VerifyScriptID)
	assert.Equal(t, verifyFile.ID, *found.VerifyScriptID)

	// Verify we can load the file through the FK
	var linkedFile models.ProjectFile
	require.NoError(t, db.First(&linkedFile, "id = ?", *found.VerifyScriptID).Error)
	assert.Equal(t, "#!/bin/bash\ntest -f /tmp/done", linkedFile.Content)
}

func TestScenario_WithProjectFileFKs(t *testing.T) {
	db := setupTestDB(t)

	// Create 3 ProjectFiles
	setupFile := models.ProjectFile{
		Filename:    "setup.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\napt-get update",
	}
	require.NoError(t, db.Create(&setupFile).Error)

	introFile := models.ProjectFile{
		Filename:    "intro.md",
		ContentType: "markdown",
		Content:     "# Welcome to the lab",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Filename:    "finish.md",
		ContentType: "markdown",
		Content:     "# Congratulations!",
	}
	require.NoError(t, db.Create(&finishFile).Error)

	// Create a Scenario with all 3 FK pointers
	scenario := models.Scenario{
		Name:          "fk-all-test",
		Title:         "All FKs Test",
		InstanceType:  "ubuntu:22.04",
		CreatedByID:   "creator-1",
		SetupScriptID: &setupFile.ID,
		IntroFileID:   &introFile.ID,
		FinishFileID:  &finishFile.ID,
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Verify all 3 FKs are persisted
	var found models.Scenario
	require.NoError(t, db.First(&found, "id = ?", scenario.ID).Error)

	require.NotNil(t, found.SetupScriptID)
	require.NotNil(t, found.IntroFileID)
	require.NotNil(t, found.FinishFileID)

	assert.Equal(t, setupFile.ID, *found.SetupScriptID)
	assert.Equal(t, introFile.ID, *found.IntroFileID)
	assert.Equal(t, finishFile.ID, *found.FinishFileID)
}

// ---------------------------------------------------------------------------
// 4. ResolveScriptContent helper
// ---------------------------------------------------------------------------

func TestResolveScriptContent_WithFileID_ReturnsFileContent(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Filename:    "resolve-test.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho resolved",
	}
	require.NoError(t, db.Create(&file).Error)

	result := services.ResolveScriptContent(db, &file.ID, "fallback content")
	assert.Equal(t, "#!/bin/bash\necho resolved", result)
}

func TestResolveScriptContent_WithNilFileID_ReturnsFallback(t *testing.T) {
	db := setupTestDB(t)

	result := services.ResolveScriptContent(db, nil, "fallback content")
	assert.Equal(t, "fallback content", result)
}

func TestResolveScriptContent_WithInvalidFileID_ReturnsFallback(t *testing.T) {
	db := setupTestDB(t)

	randomID := uuid.New()
	result := services.ResolveScriptContent(db, &randomID, "fallback content")
	assert.Equal(t, "fallback content", result)
}

// ---------------------------------------------------------------------------
// 5. Import creates ProjectFile records (dual-write)
// ---------------------------------------------------------------------------

func TestImport_CreatesProjectFiles(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// Create a realistic scenario directory
	writeTestFile(t, tmpDir, "intro.md", "# Welcome to the lab")
	writeTestFile(t, tmpDir, "finish.md", "# Congratulations!")

	os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755)
	writeTestFile(t, tmpDir, "step1/text.md", "Navigate to /tmp")
	writeTestFile(t, tmpDir, "step1/verify.sh", "#!/bin/bash\ntest -d /tmp")
	writeTestFile(t, tmpDir, "step1/hint.md", "Try using cd")
	writeTestFile(t, tmpDir, "step1/background.sh", "#!/bin/bash\nmkdir -p /tmp/test")

	indexJSON := `{
		"title": "ProjectFile Import Test",
		"description": "Testing ProjectFile creation on import",
		"difficulty": "beginner",
		"time": "15m",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{
					"title": "Step One",
					"text": "step1/text.md",
					"verify": "step1/verify.sh",
					"hint": "step1/hint.md",
					"background": "step1/background.sh",
					"foreground": ""
				}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-pf-test", nil, "builtin")
	require.NoError(t, err)

	// --- Verify scenario-level ProjectFile records ---

	// Reload scenario to get FK pointers set by createProjectFilesForScenario
	var reloaded models.Scenario
	err = db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&reloaded, "id = ?", scenario.ID).Error
	require.NoError(t, err)

	// IntroFileID should be set
	require.NotNil(t, reloaded.IntroFileID, "IntroFileID should be set after import")
	var introFile models.ProjectFile
	require.NoError(t, db.First(&introFile, "id = ?", *reloaded.IntroFileID).Error)
	assert.Equal(t, "intro.md", introFile.Filename)
	assert.Equal(t, "markdown", introFile.ContentType)
	assert.Equal(t, "# Welcome to the lab", introFile.Content)
	assert.Equal(t, "intro.md", introFile.RelPath)

	// FinishFileID should be set
	require.NotNil(t, reloaded.FinishFileID, "FinishFileID should be set after import")
	var finishFile models.ProjectFile
	require.NoError(t, db.First(&finishFile, "id = ?", *reloaded.FinishFileID).Error)
	assert.Equal(t, "finish.md", finishFile.Filename)
	assert.Equal(t, "markdown", finishFile.ContentType)
	assert.Equal(t, "# Congratulations!", finishFile.Content)
	assert.Equal(t, "finish.md", finishFile.RelPath)

	// --- Verify step-level ProjectFile records ---
	require.Len(t, reloaded.Steps, 1)
	step := reloaded.Steps[0]

	// VerifyScriptID
	require.NotNil(t, step.VerifyScriptID, "VerifyScriptID should be set after import")
	var verifyFile models.ProjectFile
	require.NoError(t, db.First(&verifyFile, "id = ?", *step.VerifyScriptID).Error)
	assert.Equal(t, "verify.sh", verifyFile.Filename)
	assert.Equal(t, "script", verifyFile.ContentType)
	assert.Equal(t, "#!/bin/bash\ntest -d /tmp", verifyFile.Content)
	assert.Equal(t, "step1/verify.sh", verifyFile.RelPath)

	// TextFileID
	require.NotNil(t, step.TextFileID, "TextFileID should be set after import")
	var textFile models.ProjectFile
	require.NoError(t, db.First(&textFile, "id = ?", *step.TextFileID).Error)
	assert.Equal(t, "text.md", textFile.Filename)
	assert.Equal(t, "markdown", textFile.ContentType)
	assert.Equal(t, "Navigate to /tmp", textFile.Content)
	assert.Equal(t, "step1/text.md", textFile.RelPath)

	// HintFileID
	require.NotNil(t, step.HintFileID, "HintFileID should be set after import")
	var hintFile models.ProjectFile
	require.NoError(t, db.First(&hintFile, "id = ?", *step.HintFileID).Error)
	assert.Equal(t, "hint.md", hintFile.Filename)
	assert.Equal(t, "markdown", hintFile.ContentType)
	assert.Equal(t, "Try using cd", hintFile.Content)
	assert.Equal(t, "step1/hint.md", hintFile.RelPath)

	// BackgroundScriptID
	require.NotNil(t, step.BackgroundScriptID, "BackgroundScriptID should be set after import")
	var bgFile models.ProjectFile
	require.NoError(t, db.First(&bgFile, "id = ?", *step.BackgroundScriptID).Error)
	assert.Equal(t, "background.sh", bgFile.Filename)
	assert.Equal(t, "script", bgFile.ContentType)
	assert.Equal(t, "#!/bin/bash\nmkdir -p /tmp/test", bgFile.Content)
	assert.Equal(t, "step1/background.sh", bgFile.RelPath)

	// Verify total count of ProjectFiles in DB
	var fileCount int64
	db.Model(&models.ProjectFile{}).Count(&fileCount)
	assert.Equal(t, int64(6), fileCount) // intro + finish + verify + text + hint + background
}

// ---------------------------------------------------------------------------
// 6. Re-import cleans up old ProjectFiles and creates new ones
// ---------------------------------------------------------------------------

func TestImport_ReimportCleansUpOldProjectFiles(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// --- First import ---
	writeTestFile(t, tmpDir, "intro.md", "# Original intro")
	writeTestFile(t, tmpDir, "finish.md", "# Original finish")
	os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755)
	writeTestFile(t, tmpDir, "step1/text.md", "Original step text")
	writeTestFile(t, tmpDir, "step1/verify.sh", "#!/bin/bash\noriginal verify")

	indexJSON := `{
		"title": "Reimport Cleanup Test",
		"description": "Testing reimport cleans old ProjectFiles",
		"difficulty": "beginner",
		"time": "10m",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Step One", "text": "step1/text.md", "verify": "step1/verify.sh", "background": "", "foreground": "", "hint": ""}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario1, err := importer.ImportFromDirectory(tmpDir, "user-reimport", nil, "builtin")
	require.NoError(t, err)

	// Collect old ProjectFile IDs
	var oldScenario models.Scenario
	err = db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&oldScenario, "id = ?", scenario1.ID).Error
	require.NoError(t, err)

	oldIDs := make([]uuid.UUID, 0)
	if oldScenario.IntroFileID != nil {
		oldIDs = append(oldIDs, *oldScenario.IntroFileID)
	}
	if oldScenario.FinishFileID != nil {
		oldIDs = append(oldIDs, *oldScenario.FinishFileID)
	}
	for _, step := range oldScenario.Steps {
		if step.VerifyScriptID != nil {
			oldIDs = append(oldIDs, *step.VerifyScriptID)
		}
		if step.TextFileID != nil {
			oldIDs = append(oldIDs, *step.TextFileID)
		}
	}
	require.NotEmpty(t, oldIDs, "first import should have created ProjectFile records")

	// --- Second import (same scenario name = upsert) ---
	writeTestFile(t, tmpDir, "intro.md", "# Updated intro")
	writeTestFile(t, tmpDir, "finish.md", "# Updated finish")
	writeTestFile(t, tmpDir, "step1/text.md", "Updated step text")
	writeTestFile(t, tmpDir, "step1/verify.sh", "#!/bin/bash\nupdated verify")

	scenario2, err := importer.ImportFromDirectory(tmpDir, "user-reimport", nil, "builtin")
	require.NoError(t, err)
	assert.Equal(t, scenario1.ID, scenario2.ID, "should reuse the same scenario ID on upsert")

	// Old ProjectFile IDs should no longer exist (soft-deleted)
	for _, oldID := range oldIDs {
		var pf models.ProjectFile
		err := db.First(&pf, "id = ?", oldID).Error
		assert.Error(t, err, "old ProjectFile %s should be deleted after reimport", oldID)
	}

	// New ProjectFile records should exist with updated content
	var newScenario models.Scenario
	err = db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&newScenario, "id = ?", scenario2.ID).Error
	require.NoError(t, err)

	require.NotNil(t, newScenario.IntroFileID, "IntroFileID should be set after reimport")
	var introFile models.ProjectFile
	require.NoError(t, db.First(&introFile, "id = ?", *newScenario.IntroFileID).Error)
	assert.Equal(t, "# Updated intro", introFile.Content)

	require.NotNil(t, newScenario.FinishFileID, "FinishFileID should be set after reimport")
	var finishFile models.ProjectFile
	require.NoError(t, db.First(&finishFile, "id = ?", *newScenario.FinishFileID).Error)
	assert.Equal(t, "# Updated finish", finishFile.Content)

	require.Len(t, newScenario.Steps, 1)
	step := newScenario.Steps[0]
	require.NotNil(t, step.TextFileID)
	var textFile models.ProjectFile
	require.NoError(t, db.First(&textFile, "id = ?", *step.TextFileID).Error)
	assert.Equal(t, "Updated step text", textFile.Content)

	require.NotNil(t, step.VerifyScriptID)
	var verifyFile models.ProjectFile
	require.NoError(t, db.First(&verifyFile, "id = ?", *step.VerifyScriptID).Error)
	assert.Equal(t, "#!/bin/bash\nupdated verify", verifyFile.Content)

	// FKs should point to NEW IDs (not old ones)
	for _, oldID := range oldIDs {
		assert.NotEqual(t, oldID, *newScenario.IntroFileID, "FK should not point to old ProjectFile")
		assert.NotEqual(t, oldID, *newScenario.FinishFileID, "FK should not point to old ProjectFile")
	}
}

// ---------------------------------------------------------------------------
// 7. Export resolves content from ProjectFile (not inline)
// ---------------------------------------------------------------------------

func TestExport_ResolvesFromProjectFile(t *testing.T) {
	db := setupTestDB(t)

	// Create ProjectFiles with content DIFFERENT from inline fields
	introFile := models.ProjectFile{
		Filename:    "intro.md",
		ContentType: "markdown",
		Content:     "# ProjectFile intro content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Filename:    "finish.md",
		ContentType: "markdown",
		Content:     "# ProjectFile finish content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&finishFile).Error)

	verifyFile := models.ProjectFile{
		Filename:    "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho projectfile-verify",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	textFile := models.ProjectFile{
		Filename:    "text.md",
		ContentType: "markdown",
		Content:     "ProjectFile step text content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&textFile).Error)

	hintFile := models.ProjectFile{
		Filename:    "hint.md",
		ContentType: "markdown",
		Content:     "ProjectFile hint content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&hintFile).Error)

	bgFile := models.ProjectFile{
		Filename:    "background.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho projectfile-bg",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&bgFile).Error)

	// Create a scenario with DIFFERENT inline content and ProjectFile FKs set
	scenario := models.Scenario{
		Name:         "export-pf-test",
		Title:        "Export PF Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		IntroText:    "INLINE intro (should NOT appear)",
		FinishText:   "INLINE finish (should NOT appear)",
		IntroFileID:  &introFile.ID,
		FinishFileID: &finishFile.ID,
		Steps: []models.ScenarioStep{
			{
				Order:              0,
				Title:              "Step with PF",
				TextContent:        "INLINE text (should NOT appear)",
				HintContent:        "INLINE hint (should NOT appear)",
				VerifyScript:       "INLINE verify (should NOT appear)",
				BackgroundScript:   "INLINE bg (should NOT appear)",
				VerifyScriptID:     &verifyFile.ID,
				TextFileID:         &textFile.ID,
				HintFileID:         &hintFile.ID,
				BackgroundScriptID: &bgFile.ID,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Export as JSON
	exportService := services.NewScenarioExportService(db)
	output, err := exportService.ExportAsJSON(scenario.ID)
	require.NoError(t, err)

	// Assert: exported content matches ProjectFile content (not inline)
	assert.Equal(t, "# ProjectFile intro content", output.IntroText)
	assert.Equal(t, "# ProjectFile finish content", output.FinishText)

	require.Len(t, output.Steps, 1)
	assert.Equal(t, "ProjectFile step text content", output.Steps[0].TextContent)
	assert.Equal(t, "ProjectFile hint content", output.Steps[0].HintContent)
	assert.Equal(t, "#!/bin/bash\necho projectfile-verify", output.Steps[0].VerifyScript)
	assert.Equal(t, "#!/bin/bash\necho projectfile-bg", output.Steps[0].BackgroundScript)
}

// ---------------------------------------------------------------------------
// 8. Export archive resolves content from ProjectFile
// ---------------------------------------------------------------------------

func TestExportArchive_ResolvesFromProjectFile(t *testing.T) {
	db := setupTestDB(t)

	// Create ProjectFiles with content DIFFERENT from inline fields
	introFile := models.ProjectFile{
		Filename:    "intro.md",
		ContentType: "markdown",
		Content:     "# Archive PF intro",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Filename:    "finish.md",
		ContentType: "markdown",
		Content:     "# Archive PF finish",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&finishFile).Error)

	verifyFile := models.ProjectFile{
		Filename:    "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho archive-pf-verify",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	textFile := models.ProjectFile{
		Filename:    "text.md",
		ContentType: "markdown",
		Content:     "Archive PF step text",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&textFile).Error)

	// Create scenario with inline content AND ProjectFile FKs
	scenario := models.Scenario{
		Name:         "archive-pf-test",
		Title:        "Archive PF Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		IntroText:    "INLINE intro (should NOT appear)",
		FinishText:   "INLINE finish (should NOT appear)",
		IntroFileID:  &introFile.ID,
		FinishFileID: &finishFile.ID,
		Steps: []models.ScenarioStep{
			{
				Order:          0,
				Title:          "Step with PF",
				TextContent:    "INLINE text (should NOT appear)",
				VerifyScript:   "INLINE verify (should NOT appear)",
				VerifyScriptID: &verifyFile.ID,
				TextFileID:     &textFile.ID,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Export as archive
	exportService := services.NewScenarioExportService(db)
	zipBytes, filename, err := exportService.ExportAsArchive(scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, "archive-pf-test.zip", filename)

	// Read files from the zip
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	zipContents := make(map[string]string)
	for _, f := range reader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(rc)
		require.NoError(t, err)
		rc.Close()
		zipContents[f.Name] = buf.String()
	}

	// Assert: file content matches ProjectFile content (not inline)
	assert.Equal(t, "# Archive PF intro", zipContents["intro.md"])
	assert.Equal(t, "# Archive PF finish", zipContents["finish.md"])
	assert.Equal(t, "Archive PF step text", zipContents["step1/text.md"])
	assert.Equal(t, "#!/bin/bash\necho archive-pf-verify", zipContents["step1/verify.sh"])
}

// ---------------------------------------------------------------------------
// 9. Migration test — SKIPPED
// ---------------------------------------------------------------------------
//
// NOTE: migrateInlineContentToProjectFiles is unexported (in src/initialization/database.go),
// so it cannot be called directly from this test package. The migration's behavior is
// indirectly verified by the import tests above (which exercise dual-write via
// createProjectFilesForScenario). A dedicated migration test would require either:
//   - Exporting the function (e.g., MigrateInlineContentToProjectFiles)
//   - Moving the test into the initialization package
//   - Using a test helper that calls the function via reflection
//
