package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

// ---------------------------------------------------------------------------
// 1. ProjectFile CRUD
// ---------------------------------------------------------------------------

func TestProjectFile_Create_Success(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Name:        "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\nexit 0",
		RelPath:     "step1/verify.sh",
	}
	require.NoError(t, db.Create(&file).Error)

	// Verify persisted
	var found models.ProjectFile
	err := db.First(&found, "id = ?", file.ID).Error
	require.NoError(t, err)

	assert.Equal(t, "verify.sh", found.Name)
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
		Name:        "original.sh",
		ContentType: "script",
		Content:     "echo hello",
	}
	require.NoError(t, db.Create(&file).Error)

	// Update name and content
	err := db.Model(&file).Updates(map[string]interface{}{
		"name": "updated.sh",
		"content":  "echo updated",
	}).Error
	require.NoError(t, err)

	// Verify changes persisted
	var found models.ProjectFile
	require.NoError(t, db.First(&found, "id = ?", file.ID).Error)

	assert.Equal(t, "updated.sh", found.Name)
	assert.Equal(t, "echo updated", found.Content)
	assert.Equal(t, "script", found.ContentType) // unchanged
}

func TestProjectFile_Delete_Success(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Name:        "to-delete.sh",
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
		name        string
		contentType string
		content     string
	}{
		{"verify.sh", "script", "#!/bin/bash\nexit 0"},
		{"intro.md", "markdown", "# Welcome\nThis is an intro."},
		{"notes.txt", "text", "Plain text content here."},
	}

	for _, tt := range types {
		file := models.ProjectFile{
			Name:        tt.name,
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
		Name:        "verify.sh",
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
		Name:        "setup.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\napt-get update",
	}
	require.NoError(t, db.Create(&setupFile).Error)

	introFile := models.ProjectFile{
		Name:        "intro.md",
		ContentType: "markdown",
		Content:     "# Welcome to the lab",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Name:        "finish.md",
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
		Name:        "resolve-test.sh",
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
	assert.Equal(t, "intro.md", introFile.Name)
	assert.Equal(t, "markdown", introFile.ContentType)
	assert.Equal(t, "# Welcome to the lab", introFile.Content)
	assert.Equal(t, "intro.md", introFile.RelPath)

	// FinishFileID should be set
	require.NotNil(t, reloaded.FinishFileID, "FinishFileID should be set after import")
	var finishFile models.ProjectFile
	require.NoError(t, db.First(&finishFile, "id = ?", *reloaded.FinishFileID).Error)
	assert.Equal(t, "finish.md", finishFile.Name)
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
	assert.Equal(t, "verify.sh", verifyFile.Name)
	assert.Equal(t, "script", verifyFile.ContentType)
	assert.Equal(t, "#!/bin/bash\ntest -d /tmp", verifyFile.Content)
	assert.Equal(t, "step1/verify.sh", verifyFile.RelPath)

	// TextFileID
	require.NotNil(t, step.TextFileID, "TextFileID should be set after import")
	var textFile models.ProjectFile
	require.NoError(t, db.First(&textFile, "id = ?", *step.TextFileID).Error)
	assert.Equal(t, "text.md", textFile.Name)
	assert.Equal(t, "markdown", textFile.ContentType)
	assert.Equal(t, "Navigate to /tmp", textFile.Content)
	assert.Equal(t, "step1/text.md", textFile.RelPath)

	// HintFileID
	require.NotNil(t, step.HintFileID, "HintFileID should be set after import")
	var hintFile models.ProjectFile
	require.NoError(t, db.First(&hintFile, "id = ?", *step.HintFileID).Error)
	assert.Equal(t, "hint.md", hintFile.Name)
	assert.Equal(t, "markdown", hintFile.ContentType)
	assert.Equal(t, "Try using cd", hintFile.Content)
	assert.Equal(t, "step1/hint.md", hintFile.RelPath)

	// BackgroundScriptID
	require.NotNil(t, step.BackgroundScriptID, "BackgroundScriptID should be set after import")
	var bgFile models.ProjectFile
	require.NoError(t, db.First(&bgFile, "id = ?", *step.BackgroundScriptID).Error)
	assert.Equal(t, "background.sh", bgFile.Name)
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
		Name:        "intro.md",
		ContentType: "markdown",
		Content:     "# ProjectFile intro content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Name:        "finish.md",
		ContentType: "markdown",
		Content:     "# ProjectFile finish content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&finishFile).Error)

	verifyFile := models.ProjectFile{
		Name:        "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho projectfile-verify",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	textFile := models.ProjectFile{
		Name:        "text.md",
		ContentType: "markdown",
		Content:     "ProjectFile step text content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&textFile).Error)

	hintFile := models.ProjectFile{
		Name:        "hint.md",
		ContentType: "markdown",
		Content:     "ProjectFile hint content",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&hintFile).Error)

	bgFile := models.ProjectFile{
		Name:        "background.sh",
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
		Name:        "intro.md",
		ContentType: "markdown",
		Content:     "# Archive PF intro",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&introFile).Error)

	finishFile := models.ProjectFile{
		Name:        "finish.md",
		ContentType: "markdown",
		Content:     "# Archive PF finish",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&finishFile).Error)

	verifyFile := models.ProjectFile{
		Name:        "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho archive-pf-verify",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&verifyFile).Error)

	textFile := models.ProjectFile{
		Name:        "text.md",
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
// 9. GET /project-files/:id/content endpoint
// ---------------------------------------------------------------------------

func TestProjectFileController_GetContent_Script(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Name:        "verify.sh",
		ContentType: "script",
		Content:     "#!/bin/bash\necho hello",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&file).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "test-user")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})
	ctrl := scenarioController.NewProjectFileController(db)
	api.GET("/project-files/:id/content", ctrl.GetContent)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+file.ID.String()+"/content", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "#!/bin/bash\necho hello", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "text/x-shellscript")
}

func TestProjectFileController_GetContent_Markdown(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{
		Name:        "intro.md",
		ContentType: "markdown",
		Content:     "# Welcome\n\nThis is the intro.",
		StorageType: "database",
	}
	require.NoError(t, db.Create(&file).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	ctrl := scenarioController.NewProjectFileController(db)
	api.GET("/project-files/:id/content", ctrl.GetContent)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+file.ID.String()+"/content", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "# Welcome\n\nThis is the intro.", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "text/markdown")
}

func TestProjectFileController_GetContent_NotFound(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/:id/content", ctrl.GetContent)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+uuid.New().String()+"/content", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProjectFileController_GetContent_InvalidID(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/:id/content", ctrl.GetContent)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/not-a-uuid/content", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// 10. GET /project-files/by-scenario/:scenarioId endpoint
// ---------------------------------------------------------------------------

func TestProjectFileController_GetByScenario_Success(t *testing.T) {
	db := setupTestDB(t)

	// Create ProjectFiles
	introFile := models.ProjectFile{Name: "intro.md", ContentType: "markdown", Content: "# Intro", StorageType: "database", SizeBytes: 7}
	verifyFile := models.ProjectFile{Name: "verify.sh", ContentType: "script", Content: "#!/bin/bash\ntrue", StorageType: "database", SizeBytes: 16}
	textFile := models.ProjectFile{Name: "text.md", ContentType: "markdown", Content: "Step text", StorageType: "database", SizeBytes: 9}
	require.NoError(t, db.Create(&introFile).Error)
	require.NoError(t, db.Create(&verifyFile).Error)
	require.NoError(t, db.Create(&textFile).Error)

	// Create scenario referencing these files
	scenario := models.Scenario{
		Name:        "by-scenario-test",
		Title:       "By Scenario Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID: "user-1",
		IntroFileID: &introFile.ID,
		Steps: []models.ScenarioStep{
			{
				Order:          0,
				Title:          "Step 1",
				VerifyScriptID: &verifyFile.ID,
				TextFileID:     &textFile.ID,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "test-user")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})
	ctrl := scenarioController.NewProjectFileController(db)
	api.GET("/project-files/by-scenario/:scenarioId", ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+scenario.ID.String(), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 3) // intro + verify + text

	// Verify each item has used_as and no content
	usedAsValues := make(map[string]bool)
	for _, item := range result {
		usedAs, ok := item["used_as"].(string)
		require.True(t, ok)
		usedAsValues[usedAs] = true
		_, hasContent := item["content"]
		assert.False(t, hasContent, "by-scenario response should not include content")
	}
	assert.True(t, usedAsValues["intro"])
	assert.True(t, usedAsValues["Step 1 — verify_script"])
	assert.True(t, usedAsValues["Step 1 — text"])
}

// adminMiddleware injects admin role for tests that require admin access.
func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userId", "test-user")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	}
}

func TestProjectFileController_GetByScenario_Empty(t *testing.T) {
	db := setupTestDB(t)

	// Scenario with no ProjectFile FKs
	scenario := models.Scenario{
		Name:        "no-files-test",
		Title:       "No Files",
		InstanceType: "ubuntu:22.04",
		CreatedByID: "user-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/by-scenario/:scenarioId", adminMiddleware(), ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+scenario.ID.String(), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 0)
}

func TestProjectFileController_GetByScenario_NotFound(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/by-scenario/:scenarioId", adminMiddleware(), ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+uuid.New().String(), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// 11. GET /project-files/:id/usage endpoint
// ---------------------------------------------------------------------------

func TestProjectFileController_GetUsage_Success(t *testing.T) {
	db := setupTestDB(t)

	// Create a ProjectFile
	file := models.ProjectFile{Name: "shared.sh", ContentType: "script", Content: "#!/bin/bash\ntrue", StorageType: "database"}
	require.NoError(t, db.Create(&file).Error)

	// Reference it from a scenario (intro) and a step (verify)
	scenario := models.Scenario{
		Name:        "usage-test",
		Title:       "Usage Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID: "user-1",
		IntroFileID: &file.ID,
		Steps: []models.ScenarioStep{
			{
				Order:          0,
				Title:          "Verify Step",
				VerifyScriptID: &file.ID,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	ctrl := scenarioController.NewProjectFileController(db)
	api.GET("/project-files/:id/usage", adminMiddleware(), ctrl.GetUsage)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+file.ID.String()+"/usage", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 2) // scenario intro + step verify

	fields := make(map[string]bool)
	for _, ref := range result {
		fields[ref["field"].(string)] = true
		assert.Equal(t, "usage-test", ref["scenario_name"])
	}
	assert.True(t, fields["intro"])
	assert.True(t, fields["verify_script"])
}

func TestProjectFileController_GetUsage_Unused(t *testing.T) {
	db := setupTestDB(t)

	file := models.ProjectFile{Name: "unused.sh", ContentType: "script", Content: "echo unused", StorageType: "database"}
	require.NoError(t, db.Create(&file).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/:id/usage", adminMiddleware(), ctrl.GetUsage)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+file.ID.String()+"/usage", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 0)
}

func TestProjectFileController_GetUsage_NotFound(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/:id/usage", adminMiddleware(), ctrl.GetUsage)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/"+uuid.New().String()+"/usage", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// 12. Filter ProjectFiles by scenarioId (admin page filter)
// ---------------------------------------------------------------------------

func TestProjectFile_FilterByScenarioId(t *testing.T) {
	db := setupTestDB(t)

	// Create two scenarios
	scenario1 := models.Scenario{
		Name:         "filter-scenario-1",
		Title:        "Filter Scenario 1",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "user-1",
	}
	scenario2 := models.Scenario{
		Name:         "filter-scenario-2",
		Title:        "Filter Scenario 2",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "user-1",
	}
	require.NoError(t, db.Create(&scenario1).Error)
	require.NoError(t, db.Create(&scenario2).Error)

	// Create ProjectFiles: 2 for scenario1, 1 for scenario2, 1 with no scenario
	s1Files := []models.ProjectFile{
		{Name: "s1-intro.md", ContentType: "markdown", Content: "Intro 1", ScenarioID: &scenario1.ID},
		{Name: "s1-verify.sh", ContentType: "script", Content: "#!/bin/bash\ntrue", ScenarioID: &scenario1.ID},
	}
	s2File := models.ProjectFile{Name: "s2-intro.md", ContentType: "markdown", Content: "Intro 2", ScenarioID: &scenario2.ID}
	orphanFile := models.ProjectFile{Name: "orphan.txt", ContentType: "text", Content: "No scenario"}

	for i := range s1Files {
		require.NoError(t, db.Create(&s1Files[i]).Error)
	}
	require.NoError(t, db.Create(&s2File).Error)
	require.NoError(t, db.Create(&orphanFile).Error)

	// Filter by scenario1 — should return exactly 2 files
	var filtered1 []models.ProjectFile
	err := db.Where("scenario_id = ?", scenario1.ID).Find(&filtered1).Error
	require.NoError(t, err)
	assert.Len(t, filtered1, 2, "should find 2 files for scenario1")
	for _, f := range filtered1 {
		assert.Equal(t, scenario1.ID, *f.ScenarioID)
	}

	// Filter by scenario2 — should return exactly 1 file
	var filtered2 []models.ProjectFile
	err = db.Where("scenario_id = ?", scenario2.ID).Find(&filtered2).Error
	require.NoError(t, err)
	assert.Len(t, filtered2, 1, "should find 1 file for scenario2")
	assert.Equal(t, "s2-intro.md", filtered2[0].Name)

	// No filter — should return all 4
	var all []models.ProjectFile
	err = db.Find(&all).Error
	require.NoError(t, err)
	assert.Len(t, all, 4, "should find all 4 files without filter")

	// Filter by nonexistent scenario — should return 0
	var none []models.ProjectFile
	err = db.Where("scenario_id = ?", uuid.New()).Find(&none).Error
	require.NoError(t, err)
	assert.Len(t, none, 0, "should find 0 files for nonexistent scenario")
}
