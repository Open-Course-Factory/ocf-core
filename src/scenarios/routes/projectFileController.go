package scenarioController

import (
	"log/slog"
	"net/http"

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

	contentType := "text/plain; charset=utf-8"
	switch file.ContentType {
	case "markdown":
		contentType = "text/markdown; charset=utf-8"
	case "script":
		contentType = "text/x-shellscript; charset=utf-8"
	}

	ctx.Data(http.StatusOK, contentType, []byte(file.Content))
}
