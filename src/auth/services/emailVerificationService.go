package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"soli/formations/src/auth/models"
	emailServices "soli/formations/src/email/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

var (
	ErrInvalidToken = errors.New("invalid verification token")
	ErrTokenExpired = errors.New("verification token expired")
	ErrTokenUsed    = errors.New("verification token already used")
)

type VerificationStatus struct {
	Verified   bool   `json:"verified"`
	VerifiedAt string `json:"verified_at,omitempty"`
	Email      string `json:"email"`
}

type EmailVerificationService interface {
	CreateVerificationToken(userID, email string) error
	VerifyEmail(token string) error
	ResendVerification(email string) error
	IsEmailVerified(userID string) (bool, error)
	GetVerificationStatus(userID string) (*VerificationStatus, error)
}

type emailVerificationService struct {
	db           *gorm.DB
	emailService emailServices.EmailService
}

func NewEmailVerificationService(db *gorm.DB) EmailVerificationService {
	return &emailVerificationService{
		db:           db,
		emailService: emailServices.NewEmailServiceWithDB(db),
	}
}

// generateVerificationToken generates a cryptographically secure random token
func generateVerificationToken() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// getExpiryDuration returns the token expiry duration from env or default 48 hours
func getExpiryDuration() time.Duration {
	hours := 48 // default
	if envHours := os.Getenv("EMAIL_VERIFICATION_EXPIRY_HOURS"); envHours != "" {
		if parsed, err := strconv.Atoi(envHours); err == nil && parsed > 0 {
			hours = parsed
		}
	}
	return time.Duration(hours) * time.Hour
}

// CreateVerificationToken creates a new verification token and sends email
func (s *emailVerificationService) CreateVerificationToken(userID, email string) error {
	// Get user from Casdoor to get display name
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Generate secure token
	token, err := generateVerificationToken()
	if err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Invalidate any existing unused tokens for this user
	s.db.Where("user_id = ? AND used_at IS NULL", userID).
		Delete(&models.EmailVerificationToken{})

	// Create new verification token
	verificationToken := models.EmailVerificationToken{
		UserID:      userID,
		Email:       email,
		Token:       token,
		ExpiresAt:   time.Now().Add(getExpiryDuration()),
		ResendCount: 0,
	}

	if err := s.db.Create(&verificationToken).Error; err != nil {
		return fmt.Errorf("failed to save verification token: %w", err)
	}

	// Send verification email
	if err := s.sendVerificationEmail(email, token, user.DisplayName); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	fmt.Printf("✅ Verification email sent to: %s\n", email)
	return nil
}

// sendVerificationEmail sends the verification email using the email service
func (s *emailVerificationService) sendVerificationEmail(email, token, userName string) error {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:4000"
	}

	expiryHours := getExpiryDuration().Hours()

	// Use existing SendTemplatedEmail method
	return s.emailService.SendTemplatedEmail(email, "email_verification", map[string]interface{}{
		"VerificationLink": fmt.Sprintf("%s/verify-email?token=%s", frontendURL, token),
		"Token":            token,
		"UserName":         userName,
		"PlatformName":     "OCF Platform",
		"ExpiryHours":      fmt.Sprintf("%.0f", expiryHours),
	})
}

// VerifyEmail validates the token and marks the email as verified
func (s *emailVerificationService) VerifyEmail(token string) error {
	// Find the verification token
	var verificationToken models.EmailVerificationToken
	if err := s.db.Where("token = ?", token).First(&verificationToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrInvalidToken
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Check if token is already used
	if verificationToken.IsUsed() {
		return ErrTokenUsed
	}

	// Check if token is expired
	if verificationToken.IsExpired() {
		return ErrTokenExpired
	}

	// Mark token as used
	now := time.Now()
	verificationToken.UsedAt = &now
	if err := s.db.Save(&verificationToken).Error; err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	// Update user's email verification status in Casdoor
	user, err := casdoorsdk.GetUserByUserId(verificationToken.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user.Properties == nil {
		user.Properties = make(map[string]string)
	}

	user.Properties["email_verified"] = "true"
	user.Properties["email_verified_at"] = time.Now().Format(time.RFC3339)

	if _, err := casdoorsdk.UpdateUser(user); err != nil {
		return fmt.Errorf("failed to update user verification status: %w", err)
	}

	fmt.Printf("✅ Email verified for user: %s\n", verificationToken.UserID)
	return nil
}

// ResendVerification resends the verification email with rate limiting
func (s *emailVerificationService) ResendVerification(email string) error {
	// Find user by email in Casdoor
	user, err := casdoorsdk.GetUserByEmail(email)
	if err != nil || user == nil {
		// Don't reveal if user exists or not (security best practice)
		fmt.Printf("Verification resend requested for non-existent email: %s\n", email)
		return nil // Return success to avoid user enumeration
	}

	// Check if user is already verified
	if user.Properties != nil && user.Properties["email_verified"] == "true" {
		// Already verified, but don't reveal this to avoid user enumeration
		fmt.Printf("Verification resend requested for already verified email: %s\n", email)
		return nil
	}

	// Find the most recent unused token for this user
	var token models.EmailVerificationToken
	if err := s.db.Where("user_id = ? AND used_at IS NULL", user.Id).
		Order("created_at DESC").
		First(&token).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// No existing token, create a new one
			return s.CreateVerificationToken(user.Id, email)
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Check rate limiting
	if !token.CanResend() {
		// Rate limited, but don't reveal this to avoid user enumeration
		fmt.Printf("Verification resend rate limited for email: %s\n", email)
		return nil // Return success to avoid user enumeration
	}

	// Update resend tracking
	now := time.Now()
	token.ResendCount++
	token.LastResent = &now

	if err := s.db.Save(&token).Error; err != nil {
		return fmt.Errorf("failed to update resend tracking: %w", err)
	}

	// Send verification email
	if err := s.sendVerificationEmail(email, token.Token, user.DisplayName); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	fmt.Printf("✅ Verification email resent to: %s (count: %d)\n", email, token.ResendCount)
	return nil
}

// IsEmailVerified checks if a user's email is verified
func (s *emailVerificationService) IsEmailVerified(userID string) (bool, error) {
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	if user.Properties != nil && user.Properties["email_verified"] == "true" {
		return true, nil
	}

	return false, nil
}

// GetVerificationStatus returns the verification status for a user
func (s *emailVerificationService) GetVerificationStatus(userID string) (*VerificationStatus, error) {
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	status := &VerificationStatus{
		Verified: false,
		Email:    user.Email,
	}

	if user.Properties != nil {
		if user.Properties["email_verified"] == "true" {
			status.Verified = true
			status.VerifiedAt = user.Properties["email_verified_at"]
		}
	}

	return status, nil
}
