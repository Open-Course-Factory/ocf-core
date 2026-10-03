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
// 1. ProjectFile CRUD
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
	assert.Equal(t, int64(0), found.SizeBytes)         // default
	assert.Equal(t, "database", found.StorageType)      // default
	assert.NotEqual(t, uuid.Nil, found.ID)
}

func TestProjectFile_Update_Success(t *testing.T) {
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
// 3. Import stores scripts and texts inline only; images stay files
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
// 4. GET /project-files/:id/content endpoint
// ---------------------------------------------------------------------------

func TestProjectFileController_GetContent_Script(t *testing.T) {
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
// 5. GET /project-files/by-scenario/:scenarioId endpoint
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
	var result []map[string]interface{}
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

// ---------------------------------------------------------------------------
// 6. Filter ProjectFiles by scenarioId (admin page filter)
// ---------------------------------------------------------------------------

func TestProjectFile_FilterByScenarioId(t *testing.T) {
	db := freshTestDB(t)

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
