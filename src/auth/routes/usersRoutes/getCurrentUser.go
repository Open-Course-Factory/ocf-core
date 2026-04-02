package userController

import (
	"net/http"
	"slices"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"soli/formations/src/utils"
)

// GetCurrentUser godoc
//
//	@Summary		Get current user
//	@Description	Retrieve basic information about the currently authenticated user including roles
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.CurrentUserOutput
//	@Failure		401	{object}	errors.APIError	"Unauthorized"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/auth/me [get]
func GetCurrentUser(ctx *gin.Context) {
	// Get authenticated user ID from JWT token
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	// Get user roles from context (already fetched by auth middleware)
	userRoles, _ := ctx.Get("userRoles")
	roles := []string{}
	if userRoles != nil {
		roles = userRoles.([]string)
	}

	// Get user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve user information: " + err.Error(),
		})
		return
	}

	if user == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "User not found",
		})
		return
	}

	// Check if user is admin based on roles
	isAdmin := slices.Contains(roles, "administrator")

	// Extract properties that remain in the custom map
	emailVerifiedAt := ""
	forcePasswordReset := false
	if user.Properties != nil {
		emailVerifiedAt = user.Properties["email_verified_at"]
		forcePasswordReset = user.Properties["force_password_reset"] == "true"
	}

	// Build response
	response := &dto.CurrentUserOutput{
		UserID:             user.Id,
		UserName:           user.Name,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		FirstName:          user.FirstName,
		LastName:           user.LastName,
		Avatar:             user.Avatar,
		Roles:              roles,
		IsAdmin:            isAdmin,
		EmailVerified:      user.EmailVerified,
		EmailVerifiedAt:    emailVerifiedAt,
		ForcePasswordReset: forcePasswordReset,
	}

	// Include user settings (reduces separate /users/me/settings call)
	if sqldb.DB != nil {
		settings, err := fetchUserSettings(sqldb.DB, userID)
		if err != nil {
			utils.Warn("Failed to fetch settings for user %s in /auth/me: %v", userID, err)
		} else {
			response.Settings = settings
		}
	}

	ctx.JSON(http.StatusOK, response)
}

// fetchUserSettings loads or creates default user settings and converts to output DTO.
// Exported as FetchUserSettings for testing.
func fetchUserSettings(db *gorm.DB, userID string) (*dto.UserSettingsOutput, error) {
	var settings models.UserSettings
	defaults := models.UserSettings{
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

	result := db.Where("user_id = ?", userID).Attrs(defaults).FirstOrCreate(&settings)
	if result.Error != nil {
		return nil, result.Error
	}

	return &dto.UserSettingsOutput{
		ID:                   settings.ID,
		UserID:               settings.UserID,
		DefaultLandingPage:   settings.DefaultLandingPage,
		PreferredLanguage:    settings.PreferredLanguage,
		Timezone:             settings.Timezone,
		Theme:                settings.Theme,
		CompactMode:          settings.CompactMode,
		EmailNotifications:   settings.EmailNotifications,
		DesktopNotifications: settings.DesktopNotifications,
		PasswordLastChanged:  settings.PasswordLastChanged,
		TwoFactorEnabled:     settings.TwoFactorEnabled,
		CreatedAt:            settings.CreatedAt,
		UpdatedAt:            settings.UpdatedAt,
	}, nil
}

// FetchUserSettings is the exported version for testing.
func FetchUserSettings(db *gorm.DB, userID string) (*dto.UserSettingsOutput, error) {
	return fetchUserSettings(db, userID)
}
