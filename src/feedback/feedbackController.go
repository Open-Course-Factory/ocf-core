package feedback

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	casdoorsdk "github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	errors "soli/formations/src/auth/errors"
	configRepo "soli/formations/src/configuration/repositories"
	emailServices "soli/formations/src/email/services"
	"soli/formations/src/feedback/dto"
	"soli/formations/src/feedback/services"

	"gorm.io/gorm"
)

type feedbackController struct {
	service services.FeedbackService
}

// NewFeedbackController creates a feedback controller wired with real dependencies
func NewFeedbackController(db *gorm.DB) *feedbackController {
	emailSvc := emailServices.NewEmailServiceWithDB(db)
	featureRepo := configRepo.NewFeatureRepository(db)
	svc := services.NewFeedbackService(emailSvc, featureRepo)

	return &feedbackController{
		service: svc,
	}
}

// SendFeedback handles POST /feedback/send
// @Summary Send user feedback
// @Description Submit feedback (bug report, suggestion, or question) from the platform
// @Tags Feedback
// @Accept json
// @Produce json
// @Security Bearer
// @Param input body dto.SendFeedbackInput true "Feedback details"
// @Success 200 {object} dto.SendFeedbackResponse
// @Failure 400 {object} errors.APIError
// @Failure 401 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /feedback/send [post]
func (fc *feedbackController) SendFeedback(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	// Get user email from Casdoor
	userEmail := ""
	casdoorUser, err := casdoorsdk.GetUserByUserId(userID)
	if err == nil && casdoorUser != nil {
		userEmail = casdoorUser.Email
	}

	var input dto.SendFeedbackInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Invalid input: %v", err),
		})
		return
	}

	if err := fc.service.SendFeedback(input, userID, userEmail); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to send feedback: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.SendFeedbackResponse{
		Success: true,
		Message: "Feedback sent successfully",
	})
}
