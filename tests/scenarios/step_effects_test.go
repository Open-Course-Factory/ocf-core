package scenarios_test

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

const validCast = `{"version": 2, "width": 80, "height": 24}
[0.1, "o", "hello"]
[0.5, "o", " world"]
`

const invalidCast = "this is not an asciicast\n"

// effectsIndexJSON builds a one-step index.json declaring intro/outro effects.
const effectsIndexJSON = `{
	"title": "Effects Lab",
	"description": "Testing step effects",
	"difficulty": "beginner",
	"time": "5m",
	"details": {
		"steps": [
			{"title": "Step One", "text": "step1/text.md", "intro_effect": "step1/intro.cast", "outro_effect": "step1/outro.cast"}
		]
	},
	"backend": {"imageid": "debian"}
}`

func importEffectsScenario(t *testing.T, db *gorm.DB, introCast, outroCast string) *models.Scenario {
	t.Helper()
	importer := services.NewScenarioImporterService(db)
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755))
	writeTestFile(t, tmpDir, "index.json", effectsIndexJSON)
	writeTestFile(t, tmpDir, "step1/text.md", "Do the thing")
	if introCast != "" {
		writeTestFile(t, tmpDir, "step1/intro.cast", introCast)
	}
	if outroCast != "" {
		writeTestFile(t, tmpDir, "step1/outro.cast", outroCast)
	}
	scenario, err := importer.ImportFromDirectory(tmpDir, "user-fx", nil, "builtin")
	require.NoError(t, err)
	return scenario
}

func TestStepEffects_ImportCreatesCastProjectFiles(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, validCast)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)
	require.NotNil(t, step.IntroEffectFileID)
	require.NotNil(t, step.OutroEffectFileID)

	var intro models.ProjectFile
	require.NoError(t, db.First(&intro, "id = ?", *step.IntroEffectFileID).Error)
	assert.Equal(t, "cast", intro.ContentType)
	assert.Equal(t, validCast, intro.Content)
	assert.Equal(t, "step1/intro.cast", intro.RelPath)
}

func TestStepEffects_InvalidCastSkippedNotFatal(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, invalidCast, "")

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)
	assert.Nil(t, step.IntroEffectFileID)
	assert.Nil(t, step.OutroEffectFileID)

	var castCount int64
	db.Model(&models.ProjectFile{}).Where("content_type = ?", "cast").Count(&castCount)
	assert.Equal(t, int64(0), castCount)
}

func TestStepEffects_ReimportDoesNotLeakProjectFiles(t *testing.T) {
	db := setupTestDB(t)
	importEffectsScenario(t, db, validCast, validCast)
	importEffectsScenario(t, db, validCast, validCast)

	// Upsert path must delete the old cast files before creating new ones —
	// exactly 2 live cast files after re-import, or collectProjectFileIDs
	// missed the new columns.
	var castCount int64
	db.Model(&models.ProjectFile{}).Where("content_type = ?", "cast").Count(&castCount)
	assert.Equal(t, int64(2), castCount)
}

func TestStepEffects_ExportArchiveRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, validCast)

	exporter := services.NewScenarioExportService(db)
	zipBytes, _, err := exporter.ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	names := make(map[string]bool)
	var indexContent []byte
	for _, f := range reader.File {
		names[f.Name] = true
		if f.Name == "index.json" {
			rc, err := f.Open()
			require.NoError(t, err)
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			indexContent = buf.Bytes()
		}
	}
	assert.True(t, names["step1/intro.cast"], "archive should contain step1/intro.cast, got %v", names)
	assert.True(t, names["step1/outro.cast"], "archive should contain step1/outro.cast, got %v", names)
	assert.Contains(t, string(indexContent), `"intro_effect": "step1/intro.cast"`)
	assert.Contains(t, string(indexContent), `"outro_effect": "step1/outro.cast"`)
}

func TestStepEffects_ExportJSONAndSeedRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, "")

	exporter := services.NewScenarioExportService(db)
	output, err := exporter.ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.Len(t, output.Steps, 1)
	assert.Equal(t, validCast, output.Steps[0].IntroEffectCast)
	assert.Empty(t, output.Steps[0].OutroEffectCast)

	// Re-seed from the export shape under a different title
	seeder := services.NewScenarioSeedService(db)
	seeded, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title: "Seeded Effects Lab",
		Steps: []dto.SeedStepInput{{
			Title:           output.Steps[0].Title,
			TextContent:     output.Steps[0].TextContent,
			IntroEffectCast: output.Steps[0].IntroEffectCast,
		}},
	}, "user-fx", nil)
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", seeded.ID).Error)
	require.NotNil(t, step.IntroEffectFileID)
	var file models.ProjectFile
	require.NoError(t, db.First(&file, "id = ?", *step.IntroEffectFileID).Error)
	assert.Equal(t, validCast, file.Content)
	assert.Equal(t, "cast", file.ContentType)
}

func TestStepEffects_SeedInvalidCastSkipped(t *testing.T) {
	db := setupTestDB(t)
	seeder := services.NewScenarioSeedService(db)
	seeded, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title: "Bad Cast Lab",
		Steps: []dto.SeedStepInput{{
			Title:           "S1",
			TextContent:     "text",
			IntroEffectCast: invalidCast,
		}},
	}, "user-fx", nil)
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", seeded.ID).Error)
	assert.Nil(t, step.IntroEffectFileID)
}

func TestStepEffects_DuplicateRemapsFileIDs(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, validCast)

	dupService := services.NewScenarioDuplicateService(db)
	dup, err := dupService.DuplicateScenario(scenario.ID, "user-dup", nil)
	require.NoError(t, err)

	var srcStep, dupStep models.ScenarioStep
	require.NoError(t, db.First(&srcStep, "scenario_id = ?", scenario.ID).Error)
	require.NoError(t, db.First(&dupStep, "scenario_id = ?", dup.ID).Error)

	require.NotNil(t, dupStep.IntroEffectFileID)
	require.NotNil(t, dupStep.OutroEffectFileID)
	assert.NotEqual(t, *srcStep.IntroEffectFileID, *dupStep.IntroEffectFileID)

	var dupFile models.ProjectFile
	require.NoError(t, db.First(&dupFile, "id = ?", *dupStep.IntroEffectFileID).Error)
	assert.Equal(t, validCast, dupFile.Content)
}

func TestStepEffects_CurrentStepCarriesEffectURLs(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, validCast)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "user-fx",
		CurrentStep: step.Order,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID,
		StepOrder: step.Order,
		Status:    "active",
	}).Error)

	svc := services.NewScenarioSessionService(db, nil, nil)
	resp, err := svc.GetCurrentStep(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "/project-files/"+step.IntroEffectFileID.String()+"/content", resp.IntroEffectURL)
	assert.Equal(t, "/project-files/"+step.OutroEffectFileID.String()+"/content", resp.OutroEffectURL)
}

func TestStepEffects_ContentServedAsCastToMember(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, "")

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "learner")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	ctrl := scenarioController.NewProjectFileController(db)
	api.GET("/project-files/:id/content", ctrl.GetContent)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-files/"+step.IntroEffectFileID.String()+"/content", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-asciicast", w.Header().Get("Content-Type"))
	assert.Equal(t, validCast, w.Body.String())
}

// newEffectsRouter wires the effect endpoints behind a role-injecting middleware.
func newEffectsRouter(db *gorm.DB, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "editor")
		c.Set("userRoles", roles)
		c.Next()
	})
	ctrl := scenarioController.NewScenarioEffectsController(db)
	api.POST("/scenario-steps/:id/effect/:kind", ctrl.UploadEffect)
	api.DELETE("/scenario-steps/:id/effect/:kind", ctrl.DeleteEffect)
	return r
}

func multipartCastBody(t *testing.T, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "intro.cast")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func TestStepEffects_UploadReplaceDelete(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, validCast, "")

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)
	originalFileID := *step.IntroEffectFileID

	r := newEffectsRouter(db, []string{"admin"})

	// Upload replaces the imported intro effect
	body, contentType := multipartCastBody(t, validCast)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/intro", body)
	req.Header.Set("Content-Type", contentType)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.NoError(t, db.First(&step, "id = ?", step.ID).Error)
	require.NotNil(t, step.IntroEffectFileID)
	assert.NotEqual(t, originalFileID, *step.IntroEffectFileID)

	// Replaced file is gone
	var count int64
	db.Model(&models.ProjectFile{}).Where("id = ?", originalFileID).Count(&count)
	assert.Equal(t, int64(0), count)

	// Delete clears the FK and removes the file
	currentFileID := *step.IntroEffectFileID
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/intro", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Reload into a fresh struct: gorm's First skips NULL columns when
	// scanning into a reused struct, which would leave the stale pointer.
	var cleared models.ScenarioStep
	require.NoError(t, db.First(&cleared, "id = ?", step.ID).Error)
	assert.Nil(t, cleared.IntroEffectFileID)
	db.Model(&models.ProjectFile{}).Where("id = ?", currentFileID).Count(&count)
	assert.Equal(t, int64(0), count)

	// Deleting again → 404
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/intro", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestStepEffects_UploadRejections(t *testing.T) {
	db := setupTestDB(t)
	scenario := importEffectsScenario(t, db, "", "")

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)

	// Non-admin → 403
	memberRouter := newEffectsRouter(db, []string{"member"})
	body, contentType := multipartCastBody(t, validCast)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/intro", body)
	req.Header.Set("Content-Type", contentType)
	memberRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	adminRouter := newEffectsRouter(db, []string{"admin"})

	// Invalid cast → 400
	body, contentType = multipartCastBody(t, invalidCast)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/intro", body)
	req.Header.Set("Content-Type", contentType)
	adminRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Unknown kind → 400
	body, contentType = multipartCastBody(t, validCast)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/scenario-steps/"+step.ID.String()+"/effect/sideways", body)
	req.Header.Set("Content-Type", contentType)
	adminRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Unknown step → 404
	body, contentType = multipartCastBody(t, validCast)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/scenario-steps/"+uuid.NewString()+"/effect/intro", body)
	req.Header.Set("Content-Type", contentType)
	adminRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestValidateAsciicastV2(t *testing.T) {
	assert.NoError(t, services.ValidateAsciicastV2(validCast))
	assert.Error(t, services.ValidateAsciicastV2(""))
	assert.Error(t, services.ValidateAsciicastV2(invalidCast))
	assert.Error(t, services.ValidateAsciicastV2(`{"version": 1}`+"\n"))
}
