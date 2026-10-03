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

	"soli/formations/src/auth/access"
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
		errors.Respond(ctx, http.StatusBadRequest, "Invalid project file ID")
		return
	}

	var file models.ProjectFile
	if err := c.db.First(&file, "id = ?", fileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			errors.Respond(ctx, http.StatusNotFound, "Project file not found")
			return
		}
		slog.Error("failed to load project file", "id", fileID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to retrieve file")
		return
	}

	// Block script content for non-admin users (prevents verify script answer leakage)
	if file.ContentType == "script" && !access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		errors.Respond(ctx, http.StatusForbidden, "Admin access required for script files")
		return
	}

	// Images are stored as base64 — decode and serve with proper MIME type
	if file.ContentType == "image" {
		data, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			slog.Error("failed to decode image content", "id", fileID, "err", err)
			errors.Respond(ctx, http.StatusInternalServerError, "Failed to decode image")
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
