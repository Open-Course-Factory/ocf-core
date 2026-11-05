package scenarioController

import (
	"encoding/base64"
	stderrors "errors"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
)

type projectFileController struct {
	scenarioControllerBase
}

func NewProjectFileController(db *gorm.DB) *projectFileController {
	return &projectFileController{scenarioControllerBase: newScenarioControllerBase(db)}
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

// GetByScenario returns the ProjectFile records linked to a scenario — its
// imported images. Admin-only.
// GET /api/v1/project-files/by-scenario/:scenarioId
func (c *projectFileController) GetByScenario(ctx *gin.Context) {
	if !access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		errors.Respond(ctx, http.StatusForbidden, "Admin access required")
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid scenario ID")
		return
	}

	var scenario models.Scenario
	if err := c.db.Select("id").First(&scenario, "id = ?", scenarioID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			errors.Respond(ctx, http.StatusNotFound, "Scenario not found")
			return
		}
		slog.Error("failed to load scenario", "id", scenarioID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to retrieve scenario")
		return
	}

	var files []models.ProjectFile
	if err := c.db.Where("scenario_id = ?", scenarioID).Find(&files).Error; err != nil {
		slog.Error("failed to load project files", "scenarioId", scenarioID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to retrieve project files")
		return
	}

	result := make([]projectFileListItem, 0, len(files))
	for _, f := range files {
		result = append(result, projectFileListItem{
			ID:          f.ID,
			Name:        f.Name,
			RelPath:     f.RelPath,
			ContentType: f.ContentType,
			StorageType: f.StorageType,
			SizeBytes:   f.SizeBytes,
			UsedAs:      f.ContentType,
		})
	}

	ctx.JSON(http.StatusOK, result)
}

// canReadImagesOf answers whether the caller may see a scenario's images: they
// can see the scenario, it is assigned to them, or they ran it and may review
// that run after its assignment has ended. A scenario never leaves its
// organisation, and its images don't either.
func (c *projectFileController) canReadImagesOf(ctx *gin.Context, scenarioID uuid.UUID) (bool, error) {
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return true, nil
	}
	userID := ctx.GetString("userId")

	var scenario models.Scenario
	err := c.db.First(&scenario, "id = ?", scenarioID).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	canSee, err := scenarioHooks.CanSeeScenario(c.db, c.groupService, &scenario, userID)
	if canSee || err != nil {
		return canSee, err
	}

	var runs int64
	err = c.db.Model(&models.ScenarioSession{}).
		Where("scenario_id = ? AND user_id = ?", scenarioID, userID).
		Count(&runs).Error
	if runs > 0 || err != nil {
		return runs > 0, err
	}

	return c.checkScenarioAccess(userID, scenarioID)
}

// GetImage serves a scenario image by its relative path within the scenario directory.
// GET /api/v1/project-files/image/:scenarioId/*relPath
func (c *projectFileController) GetImage(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid scenario ID")
		return
	}

	relPath := ctx.Param("relPath")
	// Strip leading slash from wildcard param
	if len(relPath) > 0 && relPath[0] == '/' {
		relPath = relPath[1:]
	}
	if relPath == "" {
		errors.Respond(ctx, http.StatusBadRequest, "Missing image path")
		return
	}

	canRead, err := c.canReadImagesOf(ctx, scenarioID)
	if err != nil {
		slog.Error("failed to check access to scenario images", "scenarioId", scenarioID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to retrieve image")
		return
	}
	if !canRead {
		errors.Respond(ctx, http.StatusNotFound, "Image not found") // does not confirm what exists
		return
	}

	var file models.ProjectFile
	if err := c.db.Where("scenario_id = ? AND rel_path = ? AND content_type = ?", scenarioID, relPath, "image").
		First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			errors.Respond(ctx, http.StatusNotFound, "Image not found")
			return
		}
		slog.Error("failed to load image", "scenarioId", scenarioID, "relPath", relPath, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to retrieve image")
		return
	}

	data, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil {
		slog.Error("failed to decode image content", "id", file.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to decode image")
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
