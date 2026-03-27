package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
