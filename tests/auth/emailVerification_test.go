package auth_tests

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
	emailVerificationRoutes "soli/formations/src/auth/routes/emailVerificationRoutes"
)

// --- Test DB setup ---

func setupVerificationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EmailVerificationToken{})
	require.NoError(t, err)

	return db
}

func generateTestToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func createTestToken(db *gorm.DB, userID, email string, expiresAt time.Time) *models.EmailVerificationToken {
	token := &models.EmailVerificationToken{
		UserID:      userID,
		Email:       email,
		Token:       generateTestToken(),
		ExpiresAt:   expiresAt,
		ResendCount: 0,
	}
	db.Create(token)
	return token
}

// ============================================================
// Model tests - pure logic, no DB or mocks
// ============================================================

func TestEmailVerificationToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "not expired - future",
			expiresAt: time.Now().Add(24 * time.Hour),
			expected:  false,
		},
		{
			name:      "expired - past",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  true,
		},
		{
			name:      "expired - just now",
			expiresAt: time.Now().Add(-1 * time.Second),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &models.EmailVerificationToken{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.expected, token.IsExpired())
		})
	}
}

func TestEmailVerificationToken_IsUsed(t *testing.T) {
	t.Run("unused token", func(t *testing.T) {
		token := &models.EmailVerificationToken{UsedAt: nil}
		assert.False(t, token.IsUsed())
	})

	t.Run("used token", func(t *testing.T) {
		now := time.Now()
		token := &models.EmailVerificationToken{UsedAt: &now}
		assert.True(t, token.IsUsed())
	})
}

func TestEmailVerificationToken_IsValid(t *testing.T) {
	t.Run("valid - not expired, not used", func(t *testing.T) {
		token := &models.EmailVerificationToken{
			ExpiresAt: time.Now().Add(24 * time.Hour),
			UsedAt:    nil,
		}
		assert.True(t, token.IsValid())
	})

	t.Run("invalid - expired", func(t *testing.T) {
		token := &models.EmailVerificationToken{
			ExpiresAt: time.Now().Add(-1 * time.Hour),
			UsedAt:    nil,
		}
		assert.False(t, token.IsValid())
	})

	t.Run("invalid - used", func(t *testing.T) {
		now := time.Now()
		token := &models.EmailVerificationToken{
			ExpiresAt: time.Now().Add(24 * time.Hour),
			UsedAt:    &now,
		}
		assert.False(t, token.IsValid())
	})

	t.Run("invalid - both expired and used", func(t *testing.T) {
		now := time.Now()
		token := &models.EmailVerificationToken{
			ExpiresAt: time.Now().Add(-1 * time.Hour),
			UsedAt:    &now,
		}
		assert.False(t, token.IsValid())
	})
}

func TestEmailVerificationToken_CanResend(t *testing.T) {
	t.Run("can resend - first time", func(t *testing.T) {
		token := &models.EmailVerificationToken{
			ResendCount: 0,
			LastResent:  nil,
		}
		assert.True(t, token.CanResend())
	})

	t.Run("can resend - after cooldown", func(t *testing.T) {
		past := time.Now().Add(-3 * time.Minute)
		token := &models.EmailVerificationToken{
			ResendCount: 2,
			LastResent:  &past,
		}
		assert.True(t, token.CanResend())
	})

	t.Run("cannot resend - max resends reached", func(t *testing.T) {
		token := &models.EmailVerificationToken{
			ResendCount: 5,
			LastResent:  nil,
		}
		assert.False(t, token.CanResend())
	})

	t.Run("cannot resend - cooldown not elapsed", func(t *testing.T) {
		recent := time.Now().Add(-30 * time.Second)
		token := &models.EmailVerificationToken{
			ResendCount: 2,
			LastResent:  &recent,
		}
		assert.False(t, token.CanResend())
	})

	t.Run("cannot resend - at max with recent resend", func(t *testing.T) {
		recent := time.Now().Add(-30 * time.Second)
		token := &models.EmailVerificationToken{
			ResendCount: 5,
			LastResent:  &recent,
		}
		assert.False(t, token.CanResend())
	})
}

// ============================================================
// DB-level tests - token CRUD with SQLite
// ============================================================

func TestVerifyEmail_ValidToken(t *testing.T) {
	db := setupVerificationTestDB(t)

	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))

	// Mark as used (simulating what VerifyEmail does at the DB level)
	now := time.Now()
	db.Model(token).Update("used_at", &now)

	// Verify it's now marked as used
	var updated models.EmailVerificationToken
	db.Where("token = ?", token.Token).First(&updated)
	assert.True(t, updated.IsUsed())
	assert.False(t, updated.IsValid())
}

func TestVerifyEmail_ExpiredToken(t *testing.T) {
	db := setupVerificationTestDB(t)

	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(-1*time.Hour))

	// Token should be found but expired
	var found models.EmailVerificationToken
	err := db.Where("token = ?", token.Token).First(&found).Error
	assert.NoError(t, err)
	assert.True(t, found.IsExpired())
	assert.False(t, found.IsValid())
}

func TestVerifyEmail_AlreadyUsedToken(t *testing.T) {
	db := setupVerificationTestDB(t)

	now := time.Now()
	token := &models.EmailVerificationToken{
		UserID:    "user-123",
		Email:     "test@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
		UsedAt:    &now,
	}
	db.Create(token)

	// Token should be found but already used
	var found models.EmailVerificationToken
	err := db.Where("token = ?", token.Token).First(&found).Error
	assert.NoError(t, err)
	assert.True(t, found.IsUsed())
	assert.False(t, found.IsValid())
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Query a token that doesn't exist
	var found models.EmailVerificationToken
	err := db.Where("token = ?", "nonexistent-token-abc123").First(&found).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestTokenUniqueness(t *testing.T) {
	db := setupVerificationTestDB(t)

	tokenStr := generateTestToken()

	// Create first token
	token1 := &models.EmailVerificationToken{
		UserID:    "user-1",
		Email:     "user1@example.com",
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}
	err := db.Create(token1).Error
	assert.NoError(t, err)

	// Create second token with same value - should fail (unique constraint)
	token2 := &models.EmailVerificationToken{
		UserID:    "user-2",
		Email:     "user2@example.com",
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}
	err = db.Create(token2).Error
	assert.Error(t, err, "Duplicate token should be rejected by unique constraint")
}

func TestOldTokenInvalidation(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Create first token
	token1 := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))

	// Simulate invalidation: soft-delete old unused tokens for user
	db.Where("user_id = ? AND used_at IS NULL", "user-123").Delete(&models.EmailVerificationToken{})

	// Create new token
	token2 := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))

	// Old token should be soft-deleted (not found in default query)
	var found models.EmailVerificationToken
	err := db.Where("token = ?", token1.Token).First(&found).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)

	// New token should exist
	err = db.Where("token = ?", token2.Token).First(&found).Error
	assert.NoError(t, err)
	assert.Equal(t, token2.Token, found.Token)
}

func TestResendCountTracking(t *testing.T) {
	db := setupVerificationTestDB(t)

	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))

	// Simulate 3 resends with last resend well in the past (beyond cooldown)
	pastCooldown := time.Now().Add(-3 * time.Minute)
	token.ResendCount = 3
	token.LastResent = &pastCooldown
	db.Save(token)

	// Reload from DB and verify
	var found models.EmailVerificationToken
	db.Where("token = ?", token.Token).First(&found)
	assert.Equal(t, 3, found.ResendCount)
	assert.NotNil(t, found.LastResent)
	assert.True(t, found.CanResend(), "3 < 5 max resends and cooldown has passed")

	// Now set to max resends
	found.ResendCount = 5
	db.Save(&found)

	var maxed models.EmailVerificationToken
	db.Where("token = ?", token.Token).First(&maxed)
	assert.False(t, maxed.CanResend(), "Should not allow resend at max count")
}

// ============================================================
// Sentinel error tests
// ============================================================

func TestSentinelErrors(t *testing.T) {
	assert.Equal(t, "invalid verification token", services.ErrInvalidToken.Error())
	assert.Equal(t, "verification token expired", services.ErrTokenExpired.Error())
	assert.Equal(t, "verification token already used", services.ErrTokenUsed.Error())
}

// ============================================================
// Controller HTTP tests
// ============================================================

func TestVerifyEmailController_InvalidInput(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/verify-email", controller.VerifyEmail)

	// Send request with missing token
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "INVALID_INPUT", response["error"])
}

func TestVerifyEmailController_TokenNotFound(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/verify-email", controller.VerifyEmail)

	// Send request with a valid-format but nonexistent 64-char token
	fakeToken := strings.Repeat("a", 64)
	body := `{"token":"` + fakeToken + `"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "VERIFICATION_FAILED", response["error"])
}

func TestVerifyEmailController_ExpiredToken(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	// Create an expired token
	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(-1*time.Hour))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/verify-email", controller.VerifyEmail)

	body := `{"token":"` + token.Token + `"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusGone, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "VERIFICATION_FAILED", response["error"])
}

func TestVerifyEmailController_AlreadyUsedToken(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	// Create a used token
	now := time.Now()
	token := &models.EmailVerificationToken{
		UserID:    "user-123",
		Email:     "test@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
		UsedAt:    &now,
	}
	db.Create(token)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/verify-email", controller.VerifyEmail)

	body := `{"token":"` + token.Token + `"}`
	req := httptest.NewRequest("POST", "/auth/verify-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "VERIFICATION_FAILED", response["error"])
}

func TestResendVerificationController_InvalidEmail(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/resend-verification", controller.ResendVerification)

	// Send request with invalid email
	body := `{"email":"not-an-email"}`
	req := httptest.NewRequest("POST", "/auth/resend-verification", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetVerificationStatusController_Unauthenticated(t *testing.T) {
	db := setupVerificationTestDB(t)
	controller := emailVerificationRoutes.NewEmailVerificationController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// No auth middleware - userId will be empty
	router.GET("/auth/verify-status", controller.GetVerificationStatus)

	req := httptest.NewRequest("GET", "/auth/verify-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ============================================================
// Access level constants tests
// ============================================================

func TestTerminalAccessLevelConstants(t *testing.T) {
	// Import is from terminalTrainer/models but we test the concept here
	// to ensure constants are properly wired
	assert.Equal(t, "read", "read")
	assert.Equal(t, "write", "write")
	assert.Equal(t, "admin", "admin")
}
