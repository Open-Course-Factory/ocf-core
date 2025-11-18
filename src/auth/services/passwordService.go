package services

import (
	"errors"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// PasswordService handles all password-related operations
type PasswordService interface {
	// SetUserPassword updates a user's password in Casdoor
	// oldPassword can be empty string for password reset scenarios
	// oldPassword is required for user-initiated password changes
	SetUserPassword(userID, oldPassword, newPassword string) error

	// ValidatePasswordStrength checks if password meets minimum requirements
	ValidatePasswordStrength(password string) error
}

type passwordService struct{}

func NewPasswordService() PasswordService {
	return &passwordService{}
}

// SetUserPassword updates a user's password using Casdoor's SetPassword API
func (s *passwordService) SetUserPassword(userID, oldPassword, newPassword string) error {
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
func (s *passwordService) ValidatePasswordStrength(password string) error {
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
