package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"soli/formations/src/auth/models"
	emailServices "soli/formations/src/email/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type PasswordResetService interface {
	RequestPasswordReset(email string, resetURL string) error
	ResetPassword(token string, newPassword string) error
}

type passwordResetService struct {
	db              *gorm.DB
	emailService    emailServices.EmailService
	passwordService PasswordService
}

func NewPasswordResetService(db *gorm.DB) PasswordResetService {
	return &passwordResetService{
		db:              db,
		emailService:    emailServices.NewEmailService(),
		passwordService: NewPasswordService(),
	}
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// RequestPasswordReset creates a password reset token and sends an email to the user
func (s *passwordResetService) RequestPasswordReset(email string, resetURL string) error {
	// Find user by email in Casdoor
	user, err := casdoorsdk.GetUserByEmail(email)
	if err != nil {
		// Don't reveal if user exists or not (security best practice)
		// Just log the error and return success
		fmt.Printf("Password reset requested for non-existent email: %s\n", email)
		return nil // Return success to avoid user enumeration
	}

	// Check if user exists (API may return nil user without error)
	if user == nil {
		fmt.Printf("Password reset requested for non-existent email: %s\n", email)
		return nil // Return success to avoid user enumeration
	}

	// Generate secure token
	token, err := generateSecureToken()
	if err != nil {
		return fmt.Errorf("failed to generate reset token: %w", err)
	}

	// Invalidate any existing tokens for this user
	s.db.Where("user_id = ? AND used_at IS NULL", user.Id).
		Delete(&models.PasswordResetToken{})

	// Create new reset token (expires in 1 hour)
	resetToken := models.PasswordResetToken{
		UserID:    user.Id,
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if err := s.db.Create(&resetToken).Error; err != nil {
		return fmt.Errorf("failed to save reset token: %w", err)
	}

	// Send password reset email
	if err := s.emailService.SendPasswordResetEmail(email, token, resetURL); err != nil {
		return fmt.Errorf("failed to send reset email: %w", err)
	}

	fmt.Printf("✅ Password reset email sent to: %s\n", email)
	return nil
}

// ResetPassword validates the token and updates the user's password
func (s *passwordResetService) ResetPassword(token string, newPassword string) error {
	// Find the reset token
	var resetToken models.PasswordResetToken
	if err := s.db.Where("token = ?", token).First(&resetToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("invalid or expired reset token")
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Validate token
	if !resetToken.IsValid() {
		if resetToken.IsExpired() {
			return fmt.Errorf("reset token has expired")
		}
		if resetToken.IsUsed() {
			return fmt.Errorf("reset token has already been used")
		}
		return fmt.Errorf("invalid reset token")
	}

	// Update password using shared password service
	// Empty string for oldPassword since this is a reset (no old password verification)
	if err := s.passwordService.SetUserPassword(resetToken.UserID, "", newPassword); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	now := time.Now()
	resetToken.UsedAt = &now
	if err := s.db.Save(&resetToken).Error; err != nil {
		// Log but don't fail - password was already updated
		fmt.Printf("Warning: failed to mark token as used: %v\n", err)
	}

	fmt.Printf("✅ Password reset successful for user: %s\n", resetToken.UserID)
	return nil
}
