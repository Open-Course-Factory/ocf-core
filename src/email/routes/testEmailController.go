package routes

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"soli/formations/src/email/dto"
	"soli/formations/src/email/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TemplateController struct {
	db              *gorm.DB
	templateService services.TemplateService
}

func NewTemplateController(db *gorm.DB) *TemplateController {
	return &TemplateController{
		db:              db,
		templateService: services.NewTemplateService(db),
	}
}

// SendTestEmail sends a test email using the specified template
// @Summary Send test email
// @Description Send a test email using a template with example data (admin only)
// @Tags Email Templates
// @Accept json
// @Produce json
// @Security Bearer
// @Param id path string true "Template ID (UUID)"
// @Param input body dto.TestEmailInput true "Test email address"
// @Success 200 {object} dto.TemplateResponse
// @Failure 400 {object} dto.TemplateResponse
// @Router /email-templates/{id}/test [post]
func (c *TemplateController) SendTestEmail(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.TemplateResponse{
			Success: false,
			Message: "Invalid template ID",
		})
		return
	}

	var input dto.TestEmailInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.TemplateResponse{
			Success: false,
			Message: fmt.Sprintf("Invalid input: %v", err),
		})
		return
	}

	if err := c.templateService.TestTemplate(id, input.Email); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.TemplateResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to send test email: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.TemplateResponse{
		Success: true,
		Message: fmt.Sprintf("Test email sent successfully to %s", input.Email),
	})
}
