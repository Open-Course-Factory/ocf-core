package scenarioController

import (
	"encoding/base64"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/models"
)

type projectFileController struct {
	db *gorm.DB
}

func NewProjectFileController(db *gorm.DB) *projectFileController {
	return &projectFileController{db: db}
}

// GetContent returns the raw content of a ProjectFile with an appropriate Content-Type header.
// GET /api/v1/project-files/:id/content
func (c *projectFileController) GetContent(ctx *gin.Context) {
	fileID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid project file ID",
		})
		return
	}

	var file models.ProjectFile
	if err := c.db.First(&file, "id = ?", fileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Project file not found",
			})
			return
		}
		slog.Error("failed to load project file", "id", fileID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve file",
		})
		return
	}

	// Images are stored as base64 — decode and serve with proper MIME type
	if file.ContentType == "image" {
		data, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			slog.Error("failed to decode image content", "id", fileID, "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to decode image",
			})
			return
		}
		mimeType := file.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		ctx.Data(http.StatusOK, mimeType, data)
		return
	}

	contentType := "text/plain; charset=utf-8"
	switch file.ContentType {
	case "markdown":
		contentType = "text/markdown; charset=utf-8"
	case "script":
		contentType = "text/x-shellscript; charset=utf-8"
	}

	ctx.Data(http.StatusOK, contentType, []byte(file.Content))
}

// projectFileListItem is a DTO for the by-scenario list (metadata only, no content).
type projectFileListItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	RelPath     string    `json:"rel_path,omitempty"`
	ContentType string    `json:"content_type"`
	StorageType string    `json:"storage_type"`
	SizeBytes   int64     `json:"size_bytes"`
	UsedAs      string    `json:"used_as"`
}

// GetByScenario returns all ProjectFile records referenced by a given scenario and its steps.
// GET /api/v1/project-files/by-scenario/:scenarioId
func (c *projectFileController) GetByScenario(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Load scenario with steps
	var scenario models.Scenario
	if err := c.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&scenario, "id = ?", scenarioID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		slog.Error("failed to load scenario", "id", scenarioID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve scenario",
		})
		return
	}

	// Collect all referenced file IDs with their role
	type fileRef struct {
		id     uuid.UUID
		usedAs string
	}
	var refs []fileRef

	if scenario.SetupScriptID != nil {
		refs = append(refs, fileRef{*scenario.SetupScriptID, "setup_script"})
	}
	if scenario.IntroFileID != nil {
		refs = append(refs, fileRef{*scenario.IntroFileID, "intro"})
	}
	if scenario.FinishFileID != nil {
		refs = append(refs, fileRef{*scenario.FinishFileID, "finish"})
	}
	for _, step := range scenario.Steps {
		prefix := step.Title
		if step.VerifyScriptID != nil {
			refs = append(refs, fileRef{*step.VerifyScriptID, prefix + " — verify_script"})
		}
		if step.BackgroundScriptID != nil {
			refs = append(refs, fileRef{*step.BackgroundScriptID, prefix + " — background_script"})
		}
		if step.ForegroundScriptID != nil {
			refs = append(refs, fileRef{*step.ForegroundScriptID, prefix + " — foreground_script"})
		}
		if step.TextFileID != nil {
			refs = append(refs, fileRef{*step.TextFileID, prefix + " — text"})
		}
		if step.HintFileID != nil {
			refs = append(refs, fileRef{*step.HintFileID, prefix + " — hint"})
		}
	}

	if len(refs) == 0 {
		ctx.JSON(http.StatusOK, []projectFileListItem{})
		return
	}

	// Batch-load all ProjectFiles
	ids := make([]uuid.UUID, len(refs))
	for i, r := range refs {
		ids[i] = r.id
	}
	var files []models.ProjectFile
	if err := c.db.Where("id IN ?", ids).Find(&files).Error; err != nil {
		slog.Error("failed to load project files", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve project files",
		})
		return
	}

	// Index by ID for fast lookup
	fileMap := make(map[uuid.UUID]models.ProjectFile, len(files))
	for _, f := range files {
		fileMap[f.ID] = f
	}

	// Build response with role annotation
	result := make([]projectFileListItem, 0, len(refs))
	for _, ref := range refs {
		f, ok := fileMap[ref.id]
		if !ok {
			continue
		}
		result = append(result, projectFileListItem{
			ID:          f.ID,
			Name:        f.Name,
			RelPath:     f.RelPath,
			ContentType: f.ContentType,
			StorageType: f.StorageType,
			SizeBytes:   f.SizeBytes,
			UsedAs:      ref.usedAs,
		})
	}

	// Also include image files linked via ScenarioID
	var imageFiles []models.ProjectFile
	if err := c.db.Where("scenario_id = ? AND content_type = ?", scenarioID, "image").Find(&imageFiles).Error; err == nil {
		for _, f := range imageFiles {
			result = append(result, projectFileListItem{
				ID:          f.ID,
				Name:        f.Name,
				RelPath:     f.RelPath,
				ContentType: f.ContentType,
				StorageType: f.StorageType,
				SizeBytes:   f.SizeBytes,
				UsedAs:      "image",
			})
		}
	}

	ctx.JSON(http.StatusOK, result)
}

// usageRef describes one place where a ProjectFile is referenced.
type usageRef struct {
	ScenarioID   uuid.UUID `json:"scenario_id"`
	ScenarioName string    `json:"scenario_name"`
	StepID       *uuid.UUID `json:"step_id,omitempty"`
	StepTitle    string    `json:"step_title,omitempty"`
	Field        string    `json:"field"`
}

// GetUsage returns which scenarios and steps reference a given ProjectFile.
// GET /api/v1/project-files/:id/usage
func (c *projectFileController) GetUsage(ctx *gin.Context) {
	fileID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid project file ID",
		})
		return
	}

	// Check file exists
	var file models.ProjectFile
	if err := c.db.Select("id").First(&file, "id = ?", fileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Project file not found",
			})
			return
		}
		slog.Error("failed to load project file", "id", fileID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve file",
		})
		return
	}

	var refs []usageRef

	// Scenario-level references
	var scenarios []models.Scenario
	c.db.Select("id, name, setup_script_id, intro_file_id, finish_file_id").
		Where("setup_script_id = ? OR intro_file_id = ? OR finish_file_id = ?", fileID, fileID, fileID).
		Find(&scenarios)

	for _, s := range scenarios {
		if s.SetupScriptID != nil && *s.SetupScriptID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ID, ScenarioName: s.Name, Field: "setup_script"})
		}
		if s.IntroFileID != nil && *s.IntroFileID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ID, ScenarioName: s.Name, Field: "intro"})
		}
		if s.FinishFileID != nil && *s.FinishFileID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ID, ScenarioName: s.Name, Field: "finish"})
		}
	}

	// Step-level references
	type stepWithScenario struct {
		models.ScenarioStep
		ScenarioName string
	}
	var steps []stepWithScenario
	c.db.Table("scenario_steps").
		Select("scenario_steps.*, scenarios.name as scenario_name").
		Joins("JOIN scenarios ON scenarios.id = scenario_steps.scenario_id").
		Where("scenario_steps.verify_script_id = ? OR scenario_steps.background_script_id = ? OR scenario_steps.foreground_script_id = ? OR scenario_steps.text_file_id = ? OR scenario_steps.hint_file_id = ?",
			fileID, fileID, fileID, fileID, fileID).
		Find(&steps)

	for _, s := range steps {
		stepID := s.ID
		if s.VerifyScriptID != nil && *s.VerifyScriptID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ScenarioID, ScenarioName: s.ScenarioName, StepID: &stepID, StepTitle: s.Title, Field: "verify_script"})
		}
		if s.BackgroundScriptID != nil && *s.BackgroundScriptID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ScenarioID, ScenarioName: s.ScenarioName, StepID: &stepID, StepTitle: s.Title, Field: "background_script"})
		}
		if s.ForegroundScriptID != nil && *s.ForegroundScriptID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ScenarioID, ScenarioName: s.ScenarioName, StepID: &stepID, StepTitle: s.Title, Field: "foreground_script"})
		}
		if s.TextFileID != nil && *s.TextFileID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ScenarioID, ScenarioName: s.ScenarioName, StepID: &stepID, StepTitle: s.Title, Field: "text"})
		}
		if s.HintFileID != nil && *s.HintFileID == fileID {
			refs = append(refs, usageRef{ScenarioID: s.ScenarioID, ScenarioName: s.ScenarioName, StepID: &stepID, StepTitle: s.Title, Field: "hint"})
		}
	}

	ctx.JSON(http.StatusOK, refs)
}

// GetImage serves a scenario image by its relative path within the scenario directory.
// GET /api/v1/project-files/image/:scenarioId/*relPath
func (c *projectFileController) GetImage(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	relPath := ctx.Param("relPath")
	// Strip leading slash from wildcard param
	if len(relPath) > 0 && relPath[0] == '/' {
		relPath = relPath[1:]
	}
	if relPath == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Missing image path",
		})
		return
	}

	var file models.ProjectFile
	if err := c.db.Where("scenario_id = ? AND rel_path = ? AND content_type = ?", scenarioID, relPath, "image").
		First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Image not found",
			})
			return
		}
		slog.Error("failed to load image", "scenarioId", scenarioID, "relPath", relPath, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve image",
		})
		return
	}

	data, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil {
		slog.Error("failed to decode image content", "id", file.ID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to decode image",
		})
		return
	}

	mimeType := file.MimeType
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(file.Name))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}

	ctx.Header("Cache-Control", "private, max-age=86400")
	// Prevent script execution in SVGs opened directly
	ctx.Header("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'")
	ctx.Data(http.StatusOK, mimeType, data)
}
