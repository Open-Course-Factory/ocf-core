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
	ChangePassword(userID string, input dto.ChangePasswordInput) error
}

type userSettingsService struct{}

func NewUserSettingsService() UserSettingsService {
	return &userSettingsService{}
}

// ChangePassword handles password change requests
func (s *userSettingsService) ChangePassword(userID string, input dto.ChangePasswordInput) error {
	// Validate that new password matches confirmation
	if input.NewPassword != input.ConfirmPassword {
		return errors.New("new password and confirmation do not match")
	}

	// Validate password strength (minimum 8 characters - additional rules can be added)
	if len(input.NewPassword) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	// Get user from Casdoor
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

	// Update password in Casdoor
	user.Password = input.NewPassword
	affected, err := casdoorsdk.UpdateUser(user)
	if err != nil || !affected {
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

	// TODO: Send email notification about password change
	// This could be implemented later with an email service

	return nil
}

// Helper function to validate password strength (can be extended)
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	// Add more validation rules as needed:
	// - Must contain uppercase letter
	// - Must contain lowercase letter
	// - Must contain number
	// - Must contain special character
	// etc.

	return nil
}
