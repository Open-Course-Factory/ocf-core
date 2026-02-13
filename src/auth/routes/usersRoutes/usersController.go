package userController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	services "soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
)

type UserController interface {
	AddUser(ctx *gin.Context)
	DeleteUser(ctx *gin.Context)
	GetUsers(ctx *gin.Context)
	GetUser(ctx *gin.Context)
	GetUsersBatch(ctx *gin.Context)
	SearchUsers(ctx *gin.Context)
	GetMySettings(ctx *gin.Context)
	UpdateMySettings(ctx *gin.Context)
	ChangePassword(ctx *gin.Context)
}

type userController struct {
	service         services.UserService
	settingsService services.UserSettingsService
}

func NewUserController() UserController {
	return &userController{
		service:         services.NewUserService(),
		settingsService: services.NewUserSettingsService(),
	}
}

// GetMySettings returns the current user's settings
// @Summary Get current user's settings
// @Description Returns the settings for the currently authenticated user
// @Tags user-settings
// @Accept json
// @Produce json
// @Success 200 {object} dto.UserSettingsOutput
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/me/settings [get]
// @Security Bearer
func (uc *userController) GetMySettings(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var settings models.UserSettings
	result := sqldb.DB.Where("user_id = ?", userID).First(&settings)

	// If settings don't exist, create them with defaults
	if result.Error != nil {
		utils.Info("Creating default settings for user %s", userID)
		settings = models.UserSettings{
			UserID:               userID,
			DefaultLandingPage:   "/dashboard",
			PreferredLanguage:    "en",
			Timezone:             "UTC",
			Theme:                "light",
			CompactMode:          false,
			EmailNotifications:   true,
			DesktopNotifications: false,
			TwoFactorEnabled:     false,
		}

		if err := sqldb.DB.Create(&settings).Error; err != nil {
			utils.Error("Failed to create default settings for user %s: %v", userID, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create settings"})
			return
		}
	}

	// Convert to output DTO
	ops, _ := ems.GlobalEntityRegistrationService.GetEntityOps("UserSettings")
	output, err := ops.ConvertModelToDto(&settings)
	if err != nil {
		utils.Error("Failed to convert settings to output: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve settings"})
		return
	}

	ctx.JSON(http.StatusOK, output)
}

// UpdateMySettings updates the current user's settings
// @Summary Update current user's settings
// @Description Updates preferences and settings for the currently authenticated user
// @Tags user-settings
// @Accept json
// @Produce json
// @Param settings body dto.EditUserSettingsInput true "Settings to update"
// @Success 200 {object} dto.UserSettingsOutput
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/me/settings [patch]
// @Security Bearer
func (uc *userController) UpdateMySettings(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var editInput dto.EditUserSettingsInput
	if err := ctx.ShouldBindJSON(&editInput); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find the user's settings
	var settings models.UserSettings
	result := sqldb.DB.Where("user_id = ?", userID).First(&settings)
	if result.Error != nil {
		utils.Warn("Settings not found for user %s: %v", userID, result.Error)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Settings not found"})
		return
	}

	// Convert DTO to update map
	ops, _ := ems.GlobalEntityRegistrationService.GetEntityOps("UserSettings")
	updateMap, _ := ops.ConvertEditDtoToMap(editInput)

	// Update the settings
	updateResult := sqldb.DB.Model(&settings).Updates(updateMap)
	if updateResult.Error != nil {
		utils.Error("Failed to update settings: %v", updateResult.Error)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	// Reload to get updated values
	sqldb.DB.First(&settings, settings.ID)

	// Convert to output DTO
	output, err := ops.ConvertModelToDto(&settings)
	if err != nil {
		utils.Error("Failed to convert settings to output: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	ctx.JSON(http.StatusOK, output)
}

// ChangePassword changes the current user's password
// @Summary Change user password
// @Description Changes the password for the currently authenticated user
// @Tags user-settings
// @Accept json
// @Produce json
// @Param password body dto.ChangePasswordInput true "Password change request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/me/change-password [post]
// @Security Bearer
func (uc *userController) ChangePassword(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var input dto.ChangePasswordInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Extract the raw token from Authorization header
	token := ctx.Request.Header.Get("Authorization")
	if token != "" {
		// Remove "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		} else if len(token) > 7 && token[:7] == "bearer " {
			token = token[7:]
		}
	}

	err := uc.settingsService.ChangePassword(userID, input, token)
	if err != nil {
		// Determine the appropriate status code based on the error
		if err.Error() == "current password is incorrect" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}
