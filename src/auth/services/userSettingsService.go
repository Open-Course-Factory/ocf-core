package services

import (
	"errors"
	"fmt"
	"io"
	authController "soli/formations/src/auth"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	"soli/formations/src/utils"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type UserSettingsService interface {
	ChangePassword(userID string, input dto.ChangePasswordInput, token string) error
	ForceChangePassword(userID string, input dto.ForceChangePasswordInput, token string) error
}

type userSettingsService struct {
	passwordService PasswordService
}

func NewUserSettingsService() UserSettingsService {
	return &userSettingsService{
		passwordService: NewPasswordService(),
	}
}

// ChangePassword handles password change requests and invalidates the current session
func (s *userSettingsService) ChangePassword(userID string, input dto.ChangePasswordInput, token string) error {
	// Validate that new password matches confirmation
	if input.NewPassword != input.ConfirmPassword {
		return errors.New("new password and confirmation do not match")
	}

	// Get user from Casdoor (needed for current password verification)
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		utils.Error("Failed to get user from Casdoor: %v", err)
		return errors.New("failed to retrieve user information")
	}

	// Verify current password by attempting authentication
	resp, errLogin := authController.LoginToCasdoor(user, input.CurrentPassword)
	if errLogin != nil {
		utils.Warn("Invalid current password for user %s", userID)
		return errors.New("current password is incorrect")
	}
	defer resp.Body.Close()

	_, errReadBody := io.ReadAll(resp.Body)
	if errReadBody != nil {
		utils.Warn("Invalid current password for user %s", userID)
		return errors.New("current password is incorrect")
	}

	if resp.StatusCode >= 400 {
		utils.Warn("Invalid current password for user %s", userID)
		return errors.New("current password is incorrect")
	}

	// Update password using shared password service
	// Includes password strength validation and Casdoor update
	if err := s.passwordService.SetUserPassword(userID, input.CurrentPassword, input.NewPassword); err != nil {
		utils.Error("Failed to update password in Casdoor: %v", err)
		return errors.New("failed to update password")
	}

	// Update password last changed timestamp in user settings
	var userSettings models.UserSettings
	result := sqldb.DB.Where("user_id = ?", userID).First(&userSettings)
	if result.Error == nil {
		now := time.Now()
		userSettings.PasswordLastChanged = &now
		sqldb.DB.Save(&userSettings)
	}

	utils.Info("Password changed successfully for user %s", userID)

	// Invalidate the current session by deleting the token
	if token != "" {
		err := s.invalidateToken(token)
		if err != nil {
			utils.Warn("Failed to invalidate token after password change for user %s: %v", userID, err)
			// Don't fail the password change if token invalidation fails
			// The password has been changed successfully, the user just needs to re-login
		} else {
			utils.Info("Successfully invalidated session token for user %s", userID)
		}
	}

	// TODO: Send email notification about password change
	// This could be implemented later with an email service

	return nil
}

// ForceChangePassword handles forced password reset for imported users (no current password required)
func (s *userSettingsService) ForceChangePassword(userID string, input dto.ForceChangePasswordInput, token string) error {
	// Validate that new password matches confirmation
	if input.NewPassword != input.ConfirmPassword {
		return errors.New("new password and confirmation do not match")
	}

	// Get user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		utils.Error("Failed to get user from Casdoor: %v", err)
		return errors.New("failed to retrieve user information")
	}

	// Verify that force_password_reset flag is set
	if user.Properties == nil || user.Properties["force_password_reset"] != "true" {
		return errors.New("password reset not required")
	}

	// Update password using shared password service (empty oldPassword for reset scenario)
	if err := s.passwordService.SetUserPassword(userID, "", input.NewPassword); err != nil {
		utils.Error("Failed to update password in Casdoor: %v", err)
		return errors.New("failed to update password")
	}

	// Clear the force_password_reset flag
	user.Properties["force_password_reset"] = ""
	_, err = casdoorsdk.UpdateUser(user)
	if err != nil {
		utils.Warn("Failed to clear force_password_reset flag for user %s: %v", userID, err)
	}

	// Update password last changed timestamp in user settings
	var userSettings models.UserSettings
	result := sqldb.DB.Where("user_id = ?", userID).First(&userSettings)
	if result.Error == nil {
		now := time.Now()
		userSettings.PasswordLastChanged = &now
		sqldb.DB.Save(&userSettings)
	}

	utils.Info("Force password change completed for user %s", userID)

	// Invalidate the current session
	if token != "" {
		err := s.invalidateToken(token)
		if err != nil {
			utils.Warn("Failed to invalidate token after force password change for user %s: %v", userID, err)
		} else {
			utils.Info("Successfully invalidated session token for user %s", userID)
		}
	}

	return nil
}

// invalidateToken adds the JWT token to a blacklist
// Note: JWT tokens are stateless and cannot be deleted from Casdoor's side.
// We maintain a local blacklist of invalidated tokens that the auth middleware checks.
func (s *userSettingsService) invalidateToken(tokenString string) error {
	// Parse the JWT token to extract claims
	claims, err := casdoorsdk.ParseJwtToken(tokenString)
	if err != nil {
		return fmt.Errorf("failed to parse JWT token: %w", err)
	}

	// Extract the JWT ID (jti)
	jti := claims.ID
	if jti == "" {
		return fmt.Errorf("token does not have a JWT ID (jti) claim")
	}

	utils.Debug("🔍 Blacklisting token - JTI: %s, User: %s", jti, claims.Id)

	// Add token to blacklist
	blacklistedToken := &models.TokenBlacklist{
		TokenJTI:  jti,
		UserID:    claims.Id,
		ExpiresAt: time.Unix(claims.ExpiresAt.Unix(), 0),
		Reason:    "password_change",
	}

	result := sqldb.DB.Create(blacklistedToken)
	if result.Error != nil {
		return fmt.Errorf("failed to blacklist token: %v", result.Error)
	}

	return nil
}
