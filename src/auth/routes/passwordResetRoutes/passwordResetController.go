package passwordResetRoutes

import (
	"fmt"
	"net/http"
	"os"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PasswordResetController struct {
	passwordResetService services.PasswordResetService
}

func NewPasswordResetController(db *gorm.DB) *PasswordResetController {
	return &PasswordResetController{
		passwordResetService: services.NewPasswordResetService(db),
	}
}

// RequestPasswordReset handles the password reset request
// @Summary Request a password reset
// @Description Send a password reset email to the user's email address
// @Tags Auth
// @Accept json
// @Produce json
// @Param input body dto.RequestPasswordResetInput true "Email address"
// @Success 200 {object} dto.PasswordResetResponse
// @Failure 400 {object} dto.PasswordResetResponse
// @Failure 500 {object} dto.PasswordResetResponse
// @Router /auth/password-reset/request [post]
func (c *PasswordResetController) RequestPasswordReset(ctx *gin.Context) {
	var input dto.RequestPasswordResetInput

	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.PasswordResetResponse{
			Success: false,
			Message: "Invalid email format",
		})
		return
	}

	// Build the reset URL based on frontend URL
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:4000" // Default for development
	}
	resetURL := fmt.Sprintf("%s/reset-password", frontendURL)

	// Request password reset
	err := c.passwordResetService.RequestPasswordReset(input.Email, resetURL)
	if err != nil {
		// Log the error but always return success to prevent user enumeration
		utils.Error("Password reset error: %v", err)
	}

	// Always return success to prevent user enumeration attacks
	ctx.JSON(http.StatusOK, dto.PasswordResetResponse{
		Success: true,
		Message: "If an account with that email exists, a password reset link has been sent.",
	})
}

// ResetPassword handles the actual password reset with the token
// @Summary Reset password with token
// @Description Reset user password using the reset token from email
// @Tags Auth
// @Accept json
// @Produce json
// @Param input body dto.ResetPasswordInput true "Reset token and new password"
// @Success 200 {object} dto.PasswordResetResponse
// @Failure 400 {object} dto.PasswordResetResponse
// @Failure 500 {object} dto.PasswordResetResponse
// @Router /auth/password-reset/confirm [post]
func (c *PasswordResetController) ResetPassword(ctx *gin.Context) {
	var input dto.ResetPasswordInput

	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.PasswordResetResponse{
			Success: false,
			Message: "Invalid input. Please check your token and password.",
		})
		return
	}

	// Validate password strength (basic validation)
	if len(input.NewPassword) < 8 {
		ctx.JSON(http.StatusBadRequest, dto.PasswordResetResponse{
			Success: false,
			Message: "Password must be at least 8 characters long",
		})
		return
	}

	// Reset the password
	err := c.passwordResetService.ResetPassword(input.Token, input.NewPassword)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.PasswordResetResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.PasswordResetResponse{
		Success: true,
		Message: "Password has been reset successfully. You can now login with your new password.",
	})
}
