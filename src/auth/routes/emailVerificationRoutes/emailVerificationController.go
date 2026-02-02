package emailVerificationRoutes

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EmailVerificationController struct {
	db      *gorm.DB
	service services.EmailVerificationService
}

func NewEmailVerificationController(db *gorm.DB) *EmailVerificationController {
	return &EmailVerificationController{
		db:      db,
		service: services.NewEmailVerificationService(db),
	}
}

// VerifyEmail godoc
// @Summary Verify email address
// @Description Verify user's email address using the verification token
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.VerifyEmailInput true "Verification token"
// @Success 200 {object} map[string]interface{} "Email verified successfully"
// @Failure 400 {object} map[string]interface{} "Invalid token format"
// @Failure 404 {object} map[string]interface{} "Token not found"
// @Failure 409 {object} map[string]interface{} "Token already used"
// @Failure 410 {object} map[string]interface{} "Token expired"
// @Router /auth/verify-email [post]
func (c *EmailVerificationController) VerifyEmail(ctx *gin.Context) {
	var input dto.VerifyEmailInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": "Invalid token format",
		})
		return
	}

	// Verify the email
	if err := c.service.VerifyEmail(input.Token); err != nil {
		// Determine appropriate status code and error message
		errMsg := err.Error()
		statusCode := http.StatusBadRequest

		switch {
		case errMsg == "invalid verification token":
			statusCode = http.StatusNotFound
		case errMsg == "verification token expired":
			statusCode = http.StatusGone
		case errMsg == "verification token already used":
			statusCode = http.StatusConflict
		}

		ctx.JSON(statusCode, gin.H{
			"error":   "VERIFICATION_FAILED",
			"message": errMsg,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Email verified successfully",
		"verified": true,
	})
}

// ResendVerification godoc
// @Summary Resend verification email
// @Description Resend the verification email to the user's email address
// @Tags auth
// @Accept json
// @Produce json
// @Param input body dto.ResendVerificationInput true "Email address"
// @Success 200 {object} map[string]interface{} "Verification email sent (always returns success)"
// @Failure 400 {object} map[string]interface{} "Invalid email format"
// @Router /auth/resend-verification [post]
func (c *EmailVerificationController) ResendVerification(ctx *gin.Context) {
	var input dto.ResendVerificationInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_INPUT",
			"message": "Invalid email format",
		})
		return
	}

	// Resend verification email (always returns success to prevent user enumeration)
	if err := c.service.ResendVerification(input.Email); err != nil {
		// Log error but return success to user
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "SERVER_ERROR",
			"message": "Failed to process request",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "If an account exists with this email, a verification link has been sent",
	})
}

// GetVerificationStatus godoc
// @Summary Get email verification status
// @Description Get the current user's email verification status
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.VerificationStatusOutput "Verification status"
// @Failure 401 {object} map[string]interface{} "User not authenticated"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Router /auth/verify-status [get]
func (c *EmailVerificationController) GetVerificationStatus(ctx *gin.Context) {
	// Get userId from context (set by auth middleware)
	userId := ctx.GetString("userId")
	if userId == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error":   "UNAUTHORIZED",
			"message": "User not authenticated",
		})
		return
	}

	// Get verification status
	status, err := c.service.GetVerificationStatus(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "SERVER_ERROR",
			"message": "Failed to get verification status",
		})
		return
	}

	ctx.JSON(http.StatusOK, status)
}
