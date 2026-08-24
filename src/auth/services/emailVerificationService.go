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
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

var (
	ErrInvalidToken = errors.New("invalid verification token")
	ErrTokenExpired = errors.New("verification token expired")
	ErrTokenUsed    = errors.New("verification token already used")
	// ErrVerificationNotPersisted means the identity provider accepted the call
	// and changed nothing. Distinct from a transport error: retrying the same
	// write will not help, but the token must survive so the user can.
	ErrVerificationNotPersisted = errors.New("identity provider did not persist the verification")
)

// Swappable seams for the Casdoor calls, so tests can drive the responses that
// actually caused incidents without standing up an identity provider.
var (
	readCasdoorUser = casdoorsdk.GetUserByUserId
	// UpdateUserForColumns, never UpdateUser: with no column list Casdoor
	// applies a default whitelist that carries `properties` but omits
	// `email_verified`, so the flag silently never lands.
	writeCasdoorUserColumns = casdoorsdk.UpdateUserForColumns
)

// SwapCasdoorUserWriter replaces the read and write seams for a test and
// returns a function restoring them.
func SwapCasdoorUserWriter(
	read func(userID string) (*casdoorsdk.User, error),
	write func(user *casdoorsdk.User, columns []string) (bool, error),
) func() {
	prevRead, prevWrite := readCasdoorUser, writeCasdoorUserColumns
	readCasdoorUser, writeCasdoorUserColumns = read, write
	return func() { readCasdoorUser, writeCasdoorUserColumns = prevRead, prevWrite }
}

// SwapCasdoorUserReader replaces only the read seam for a test.
func SwapCasdoorUserReader(read func(userID string) (*casdoorsdk.User, error)) func() {
	prev := readCasdoorUser
	readCasdoorUser = read
	return func() { readCasdoorUser = prev }
}

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

	utils.Info("Verification email sent to: %s", email)
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

// VerifyEmail validates the token and marks the email as verified.
//
// Concurrency safety: the token is claimed atomically using a conditional
// UPDATE (WHERE used_at IS NULL). If two requests race with the same token,
// only one UPDATE will match a row; the other receives rowsAffected == 0 and
// returns ErrTokenUsed. The Casdoor call intentionally happens outside the
// UPDATE to avoid holding a lock during an external HTTP request.
func (s *emailVerificationService) VerifyEmail(token string) error {
	// Find the verification token (read-only, no lock needed here — the
	// atomic claim below is what serializes concurrent requests).
	var verificationToken models.EmailVerificationToken
	if err := s.db.Where("token = ?", token).First(&verificationToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Fast-path: token was already used before this request arrived.
	if verificationToken.IsUsed() {
		return ErrTokenUsed
	}

	// Check if token is expired
	if verificationToken.IsExpired() {
		return ErrTokenExpired
	}

	// Update user's email verification status in Casdoor BEFORE marking token as used.
	// If Casdoor fails, the token remains valid so the user can retry.
	user, err := readCasdoorUser(verificationToken.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("failed to get user: user not found in identity provider")
	}

	// Use native Casdoor EmailVerified field
	user.EmailVerified = true

	// Store timestamp in Properties (no native Casdoor field for this)
	if user.Properties == nil {
		user.Properties = make(map[string]string)
	}
	user.Properties["email_verified_at"] = time.Now().Format(time.RFC3339)

	// Name both columns. Casdoor's default whitelist covers `properties` but
	// not `email_verified`, which is how production ended up full of accounts
	// carrying an email_verified_at stamp next to a false flag.
	affected, err := writeCasdoorUserColumns(user, []string{"email_verified", "properties"})
	if err != nil {
		return fmt.Errorf("failed to update user verification status: %w", err)
	}
	// A nil error only means the call was accepted. Without this the token
	// below gets burned over a write that never happened, and the user is left
	// unverifiable for good.
	if !affected {
		utils.Warn("Casdoor accepted the verification write for user %s but changed nothing", verificationToken.UserID)
		return ErrVerificationNotPersisted
	}

	// Atomically claim the token: UPDATE … SET used_at = now WHERE id = ? AND used_at IS NULL.
	// If another concurrent request already claimed it (rowsAffected == 0), we treat it as a
	// conflict. The Casdoor update above is idempotent (setting EmailVerified = true twice is safe).
	now := time.Now()
	result := s.db.Model(&verificationToken).
		Where("used_at IS NULL").
		Update("used_at", &now)
	if result.Error != nil {
		utils.Warn("Casdoor already updated for user %s but failed to mark token as used: %v", verificationToken.UserID, result.Error)
		return fmt.Errorf("failed to mark token as used: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Another concurrent request won the race and already claimed this token.
		return ErrTokenUsed
	}

	utils.Info("Email verified for user: %s", verificationToken.UserID)
	return nil
}

// ResendVerification resends the verification email with rate limiting
func (s *emailVerificationService) ResendVerification(email string) error {
	// Find user by email in Casdoor
	user, err := casdoorsdk.GetUserByEmail(email)
	if err != nil || user == nil {
		// Don't reveal if user exists or not (security best practice)
		utils.Debug("Verification resend requested for non-existent email: %s", email)
		return nil // Return success to avoid user enumeration
	}

	// Check if user is already verified using native Casdoor field
	if user.EmailVerified {
		// Already verified, but don't reveal this to avoid user enumeration
		utils.Debug("Verification resend requested for already verified email: %s", email)
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
		utils.Debug("Verification resend rate limited for email: %s", email)
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

	utils.Info("Verification email resent to: %s (count: %d)", email, token.ResendCount)
	return nil
}

// IsEmailVerified is the single answer to "is this address confirmed?".
//
// It used to read the Casdoor flag alone while /auth/me and /auth/verify-status
// also consulted a spent token in PostgreSQL. Those two rules drifted, and the
// gap was invisible from either side: the interface showed a verified account
// while RequireVerifiedEmail — built on this function — returned 403 on every
// payment route. Both callers now share GetVerificationStatus, so there is one
// rule left to be wrong.
func (s *emailVerificationService) IsEmailVerified(userID string) (bool, error) {
	status, err := s.GetVerificationStatus(userID)
	if err != nil {
		return false, err
	}
	return status.Verified, nil
}

// GetVerificationStatus returns the verification status for a user.
// It checks Casdoor first, then falls back to PostgreSQL if Casdoor
// is unavailable or reports the email as unverified.
func (s *emailVerificationService) GetVerificationStatus(userID string) (*VerificationStatus, error) {
	user, err := readCasdoorUser(userID)
	if err != nil {
		// Casdoor unavailable — fall back to PostgreSQL
		return s.getVerificationStatusFromDB(userID)
	}

	status := &VerificationStatus{
		Verified: user.EmailVerified,
		Email:    user.Email,
	}

	if user.EmailVerified && user.Properties != nil {
		status.VerifiedAt = user.Properties["email_verified_at"]
	}

	// If Casdoor says unverified, check PostgreSQL for used tokens as fallback
	if !status.Verified {
		dbStatus, dbErr := s.getVerificationStatusFromDB(userID)
		if dbErr == nil && dbStatus.Verified {
			status.Verified = dbStatus.Verified
			status.VerifiedAt = dbStatus.VerifiedAt
			if dbStatus.Email != "" {
				status.Email = dbStatus.Email
			}
		}
	}

	return status, nil
}

// getVerificationStatusFromDB checks PostgreSQL for a used verification token
func (s *emailVerificationService) getVerificationStatusFromDB(userID string) (*VerificationStatus, error) {
	var token models.EmailVerificationToken
	err := s.db.Where("user_id = ? AND used_at IS NOT NULL", userID).Order("used_at DESC").First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No used token found — user is genuinely unverified
			return &VerificationStatus{
				Verified: false,
			}, nil
		}
		return nil, fmt.Errorf("database error checking verification: %w", err)
	}

	status := &VerificationStatus{
		Verified: true,
		Email:    token.Email,
	}
	if token.UsedAt != nil {
		status.VerifiedAt = token.UsedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return status, nil
}
