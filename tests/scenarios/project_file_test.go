package scenarios_test

import (
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
// ProjectFile column defaults
// ---------------------------------------------------------------------------

func TestProjectFile_Create_Success(t *testing.T) {
	db := freshTestDB(t)

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
	assert.Equal(t, int64(0), found.SizeBytes)     // default
	assert.Equal(t, "database", found.StorageType) // default
	assert.NotEqual(t, uuid.Nil, found.ID)
}

// ---------------------------------------------------------------------------
// Import stores scripts and texts inline only; images stay files
// ---------------------------------------------------------------------------

func writeInlineOnlyScenario(t *testing.T, dir, version string) {
	t.Helper()
	writeTestFile(t, dir, "intro.md", "# Intro "+version+"\n![diagram](diagram.png)")
	writeTestFile(t, dir, "diagram.png", "png "+version)
	os.MkdirAll(filepath.Join(dir, "step1"), 0755)
	writeTestFile(t, dir, "step1/text.md", "Text "+version)
	writeTestFile(t, dir, "step1/verify.sh", "#!/bin/bash\nverify "+version)
	writeTestFile(t, dir, "index.json", `{
		"title": "Inline Only Import",
		"details": {
			"intro": {"text": "intro.md", "background": "step1/verify.sh"},
			"steps": [{"title": "Step One", "text": "step1/text.md", "verify": "step1/verify.sh", "hint": "step1/text.md"}]
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`)
}

func assertInlineOnly(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, version string) {
	t.Helper()
	var stored models.Scenario
	require.NoError(t, db.Preload("Steps").First(&stored, "id = ?", scenarioID).Error)
	assert.Equal(t, "# Intro "+version+"\n![diagram](diagram.png)", stored.IntroText)
	assert.Nil(t, stored.IntroFileID)
	assert.Nil(t, stored.SetupScriptID)
	require.Len(t, stored.Steps, 1)
	assert.Equal(t, "Text "+version, stored.Steps[0].TextContent)
	assert.Equal(t, "#!/bin/bash\nverify "+version, stored.Steps[0].VerifyScript)
	assert.Nil(t, stored.Steps[0].TextFileID)
	assert.Nil(t, stored.Steps[0].VerifyScriptID)
	assert.Nil(t, stored.Steps[0].HintFileID)

	var files []models.ProjectFile
	require.NoError(t, db.Find(&files).Error)
	require.Len(t, files, 1, "the only file an import writes is the image")
	assert.Equal(t, "image", files[0].ContentType)
	assert.Equal(t, "diagram.png", files[0].RelPath)
	require.NotNil(t, files[0].ScenarioID)
	assert.Equal(t, scenarioID, *files[0].ScenarioID)
}

func TestImport_StoresContentInlineAndOnlyImagesAsFiles(t *testing.T) {
	db := freshTestDB(t)
	dir := t.TempDir()
	writeInlineOnlyScenario(t, dir, "v1")

	scenario, err := services.NewScenarioImporterService(db).ImportFromDirectory(dir, "user-inline", nil, "builtin")
	require.NoError(t, err)

	assertInlineOnly(t, db, scenario.ID, "v1")
}

func TestImport_Reimport_ReplacesContentAndImages(t *testing.T) {
	db := freshTestDB(t)
	importer := services.NewScenarioImporterService(db)
	dir := t.TempDir()
	writeInlineOnlyScenario(t, dir, "v1")
	first, err := importer.ImportFromDirectory(dir, "user-inline", nil, "builtin")
	require.NoError(t, err)

	writeInlineOnlyScenario(t, dir, "v2")
	second, err := importer.ImportFromDirectory(dir, "user-inline", nil, "builtin")
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	assertInlineOnly(t, db, second.ID, "v2")
}

// ---------------------------------------------------------------------------
// GET /project-files/by-scenario/:scenarioId endpoint
// ---------------------------------------------------------------------------

func TestProjectFileController_GetByScenario_ListsLinkedFiles(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{Name: "by-scenario-test", Title: "By Scenario Test", InstanceType: "ubuntu:22.04", CreatedByID: "user-1"}
	require.NoError(t, db.Create(&scenario).Error)
	image := models.ProjectFile{Name: "diagram.png", RelPath: "images/diagram.png", ContentType: "image", Content: "aW1n", StorageType: "database", SizeBytes: 3, ScenarioID: &scenario.ID}
	unlinked := models.ProjectFile{Name: "other.png", ContentType: "image", Content: "aW1n", StorageType: "database"}
	require.NoError(t, db.Create(&image).Error)
	require.NoError(t, db.Create(&unlinked).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/by-scenario/:scenarioId", adminMiddleware(), ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+scenario.ID.String(), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, image.ID.String(), result[0]["id"])
	assert.Equal(t, "image", result[0]["used_as"])
	_, hasContent := result[0]["content"]
	assert.False(t, hasContent, "by-scenario response should not include content")
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
	db := freshTestDB(t)

	// Scenario with no linked ProjectFile
	scenario := models.Scenario{
		Name:         "no-files-test",
		Title:        "No Files",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "user-1",
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

	var result []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Len(t, result, 0)
}

func TestProjectFileController_GetByScenario_NotFound(t *testing.T) {
	db := freshTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := scenarioController.NewProjectFileController(db)
	r.GET("/api/v1/project-files/by-scenario/:scenarioId", adminMiddleware(), ctrl.GetByScenario)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/project-files/by-scenario/"+uuid.New().String(), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
