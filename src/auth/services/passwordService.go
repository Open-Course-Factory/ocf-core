package services

import (
	"errors"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// PasswordService handles all password-related operations
type PasswordService struct{}

func NewPasswordService() *PasswordService {
	return &PasswordService{}
}

// SetUserPassword updates a user's password using Casdoor's SetPassword API.
// oldPassword is empty for password resets and required for user-initiated changes.
func (s *PasswordService) SetUserPassword(userID, oldPassword, newPassword string) error {
	// Validate new password strength
	if err := s.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Get user from Casdoor to get owner and name
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Update password using Casdoor's SetPassword API
	// Note: oldPassword can be empty string for password reset scenarios
	success, err := casdoorsdk.SetPassword(user.Owner, user.Name, oldPassword, newPassword)
	if err != nil {
		return fmt.Errorf("failed to update password in Casdoor: %w", err)
	}

	if !success {
		return errors.New("password update was rejected by Casdoor")
	}

	return nil
}

// ValidatePasswordStrength checks if password meets minimum security requirements
func (s *PasswordService) ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	// Add more validation rules as needed:
	// - Must contain uppercase
	// - Must contain lowercase
	// - Must contain number
	// - Must contain special character
	// etc.

	return nil
}
