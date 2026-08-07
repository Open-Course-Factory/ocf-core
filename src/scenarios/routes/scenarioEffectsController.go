package scenarioController

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// scenarioEffectsController manages per-step terminal effect assets
// (asciicast v2 recordings replayed by the frontend).
type scenarioEffectsController struct {
	db *gorm.DB
}

func NewScenarioEffectsController(db *gorm.DB) *scenarioEffectsController {
	return &scenarioEffectsController{db: db}
}

// effectFKColumn maps the :kind route param to the step column it drives.
// Unknown kinds return "".
func effectFKColumn(kind string) string {
	switch kind {
	case "intro":
		return "intro_effect_file_id"
	case "outro":
		return "outro_effect_file_id"
	}
	return ""
}

// stepEffectFileID returns a detached copy of the current FK value for the
// given kind. A copy, not the field's own pointer: gorm's Model(step).Update
// writes the new value back through the existing pointee, so an alias captured
// before the update would silently point at the NEW file.
func stepEffectFileID(step *models.ScenarioStep, kind string) *uuid.UUID {
	var current *uuid.UUID
	if kind == "intro" {
		current = step.IntroEffectFileID
	} else {
		current = step.OutroEffectFileID
	}
	if current == nil {
		return nil
	}
	idCopy := *current
	return &idCopy
}

// UploadEffect godoc
// @Summary Upload a step effect asset
// @Description Uploads an asciicast v2 (.cast) intro/outro effect for a scenario step (admin only). Replaces any existing asset of the same kind.
// @Tags scenario-steps
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Scenario step ID"
// @Param kind path string true "Effect kind (intro|outro)"
// @Param file formData file true "Asciicast v2 file"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-steps/{id}/effect/{kind} [post]
// @Security BearerAuth
func (c *scenarioEffectsController) UploadEffect(ctx *gin.Context) {
	step, kind, ok := c.resolveStepAndKind(ctx)
	if !ok {
		return
	}

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Missing multipart 'file' field: " + err.Error(),
		})
		return
	}
	if fileHeader.Size > services.MaxCastBytes {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Effect file exceeds %d bytes", services.MaxCastBytes),
		})
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, services.MaxCastBytes+1))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}
	if err := services.ValidateAsciicastV2(string(content)); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid asciicast file: " + err.Error(),
		})
		return
	}

	newFile := models.ProjectFile{
		Name:        kind + ".cast",
		RelPath:     fmt.Sprintf("step%d/%s.cast", step.Order+1, kind),
		ContentType: "cast",
		Content:     string(content),
		StorageType: "database",
		SizeBytes:   int64(len(content)),
		ScenarioID:  &step.ScenarioID,
	}

	oldFileID := stepEffectFileID(step, kind)
	txErr := c.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&newFile).Error; err != nil {
			return fmt.Errorf("failed to create project file: %w", err)
		}
		if err := tx.Model(step).Update(effectFKColumn(kind), newFile.ID).Error; err != nil {
			return fmt.Errorf("failed to update step: %w", err)
		}
		if oldFileID != nil {
			if err := tx.Delete(&models.ProjectFile{}, "id = ?", *oldFileID).Error; err != nil {
				return fmt.Errorf("failed to delete replaced file: %w", err)
			}
		}
		return nil
	})
	if txErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to store effect asset",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"file_id": newFile.ID.String(),
		"url":     fmt.Sprintf("/project-files/%s/content", newFile.ID.String()),
	})
}

// DeleteEffect godoc
// @Summary Delete a step effect asset
// @Description Removes the intro/outro effect asset from a scenario step (admin only).
// @Tags scenario-steps
// @Produce json
// @Param id path string true "Scenario step ID"
// @Param kind path string true "Effect kind (intro|outro)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-steps/{id}/effect/{kind} [delete]
// @Security BearerAuth
func (c *scenarioEffectsController) DeleteEffect(ctx *gin.Context) {
	step, kind, ok := c.resolveStepAndKind(ctx)
	if !ok {
		return
	}

	oldFileID := stepEffectFileID(step, kind)
	if oldFileID == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Step has no " + kind + " effect",
		})
		return
	}

	txErr := c.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(step).Update(effectFKColumn(kind), nil).Error; err != nil {
			return fmt.Errorf("failed to clear step FK: %w", err)
		}
		if err := tx.Delete(&models.ProjectFile{}, "id = ?", *oldFileID).Error; err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}
		return nil
	})
	if txErr != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to delete effect asset",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Effect deleted"})
}

// resolveStepAndKind performs the shared admin gate, param validation and step
// lookup for both effect endpoints. It writes the error response itself and
// returns ok=false when the request must not proceed.
func (c *scenarioEffectsController) resolveStepAndKind(ctx *gin.Context) (*models.ScenarioStep, string, bool) {
	if !isProjectFileAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return nil, "", false
	}

	kind := ctx.Param("kind")
	if effectFKColumn(kind) == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Effect kind must be 'intro' or 'outro'",
		})
		return nil, "", false
	}

	stepID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid step ID",
		})
		return nil, "", false
	}

	var step models.ScenarioStep
	if err := c.db.First(&step, "id = ?", stepID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario step not found",
		})
		return nil, "", false
	}

	return &step, kind, true
}
