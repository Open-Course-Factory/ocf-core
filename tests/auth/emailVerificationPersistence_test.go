package auth_tests

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
)

// setupUnreachableCasdoor configures the Casdoor SDK to point at an
// unreachable endpoint. Must only be called from tests that run in
// -short mode (unit tests) to avoid interfering with integration tests.
func setupUnreachableCasdoor() {
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
}

// ============================================================
// Bug 2: Token burned before Casdoor update
//
// These tests verify: if the Casdoor update fails, the token
// must NOT be marked as used so the user can retry.
// They require Casdoor to be unreachable, so they only run
// in -short (unit test) mode.
// ============================================================

func TestVerifyEmail_TokenNotConsumed_WhenCasdoorUpdateFails(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupVerificationTestDB(t)

	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	// Attempt to verify — will fail because Casdoor is unreachable
	err := svc.VerifyEmail(tokenValue)
	require.Error(t, err, "VerifyEmail should fail when Casdoor is unavailable")

	// After Casdoor failure, the token should NOT be marked as used
	var reloadedToken models.EmailVerificationToken
	dbErr := db.Where("token = ?", tokenValue).First(&reloadedToken).Error
	require.NoError(t, dbErr, "Token should still exist in DB")

	assert.Nil(t, reloadedToken.UsedAt,
		"Token must NOT be marked as used when Casdoor update fails")
	assert.True(t, reloadedToken.IsValid(),
		"Token should remain valid (unused and not expired) after Casdoor failure")
}

func TestVerifyEmail_TokenConsumed_OnlyAfterCasdoorSuccess(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupVerificationTestDB(t)

	token := createTestToken(db, "user-456", "success@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	// First attempt — fails because Casdoor is unreachable
	err := svc.VerifyEmail(tokenValue)
	require.Error(t, err, "Expected Casdoor failure")

	// Token should still be usable after failed attempt
	var tokenAfterFirstAttempt models.EmailVerificationToken
	db.Where("token = ?", tokenValue).First(&tokenAfterFirstAttempt)

	assert.False(t, tokenAfterFirstAttempt.IsUsed(),
		"Token must not be consumed after failed Casdoor update")

	// Retry — should fail with same Casdoor error, NOT ErrTokenUsed
	err = svc.VerifyEmail(tokenValue)
	if err != nil {
		assert.NotEqual(t, services.ErrTokenUsed, err,
			"Second attempt should NOT get 'token already used' — first attempt never reached Casdoor")
	}
}

// ============================================================
// Bug 3: LoginOutput missing email_verified fields
//
// These tests verify LoginOutput includes Email, EmailVerified,
// and EmailVerifiedAt fields with correct JSON serialization.
// No Casdoor dependency — always run.
// ============================================================

func TestLoginOutput_HasEmailVerifiedFields(t *testing.T) {
	loginOutputType := reflect.TypeOf(dto.LoginOutput{})

	t.Run("has Email field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("Email")
		assert.True(t, found, "LoginOutput must have an Email field")
		if found {
			assert.Equal(t, "string", field.Type.Name())
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email")
		}
	})

	t.Run("has EmailVerified field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("EmailVerified")
		assert.True(t, found, "LoginOutput must have an EmailVerified field")
		if found {
			assert.Equal(t, "bool", field.Type.Name())
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email_verified")
		}
	})

	t.Run("has EmailVerifiedAt field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("EmailVerifiedAt")
		assert.True(t, found, "LoginOutput must have an EmailVerifiedAt field")
		if found {
			assert.Equal(t, "string", field.Type.Name())
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email_verified_at")
		}
	})
}

func TestLoginOutput_JSONSerialization_IncludesEmailFields(t *testing.T) {
	output := dto.LoginOutput{
		UserName:         "testuser",
		DisplayName:      "Test User",
		UserId:           "user-123",
		AccessToken:      "token-abc",
		RenewAccessToken: "renew-xyz",
		UserRoles:        []string{"member"},
	}

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	require.NoError(t, err)

	assert.Contains(t, jsonMap, "email")
	assert.Contains(t, jsonMap, "email_verified")
	assert.Contains(t, jsonMap, "email_verified_at")
}

func TestLoginOutput_JSONSerialization_EmailVerifiedValues(t *testing.T) {
	jsonStr := `{
		"user_name": "testuser",
		"display_name": "Test User",
		"user_id": "user-123",
		"access_token": "token-abc",
		"renew_access_token": "renew-xyz",
		"user_roles": ["member"],
		"email": "test@example.com",
		"email_verified": true,
		"email_verified_at": "2025-01-15T10:00:00Z"
	}`

	var output dto.LoginOutput
	err := json.Unmarshal([]byte(jsonStr), &output)
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	require.NoError(t, err)

	assert.Equal(t, "test@example.com", jsonMap["email"])
	assert.Equal(t, true, jsonMap["email_verified"])
	assert.Equal(t, "2025-01-15T10:00:00Z", jsonMap["email_verified_at"])
}

// ============================================================
// Bug 4: No PostgreSQL fallback for verification status
//
// These tests verify: when Casdoor is unavailable or says
// unverified, the service falls back to checking used tokens
// in PostgreSQL.
// They require Casdoor to be unreachable, so they only run
// in -short (unit test) mode.
// ============================================================

func TestGetVerificationStatus_FallsBackToPostgreSQL(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupVerificationTestDB(t)

	// Create a used verification token (proof of past verification)
	usedAt := time.Now().Add(-1 * time.Hour)
	usedToken := &models.EmailVerificationToken{
		UserID:    "user-789",
		Email:     "verified@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(47 * time.Hour),
		UsedAt:    &usedAt,
	}
	require.NoError(t, db.Create(usedToken).Error)

	svc := services.NewEmailVerificationService(db)

	status, err := svc.GetVerificationStatus("user-789")
	require.NoError(t, err,
		"GetVerificationStatus should fall back to PostgreSQL when Casdoor is unavailable")
	assert.True(t, status.Verified,
		"Should be verified — used token exists in PostgreSQL")
	assert.Equal(t, "verified@example.com", status.Email,
		"Email should come from the verification token")
}

func TestGetVerificationStatus_NoFallbackWithoutUsedToken(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires unreachable Casdoor — skipping in integration mode")
	}
	setupUnreachableCasdoor()

	db := setupVerificationTestDB(t)

	// Create an UNUSED token
	unusedToken := &models.EmailVerificationToken{
		UserID:    "user-999",
		Email:     "unverified@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
		UsedAt:    nil,
	}
	require.NoError(t, db.Create(unusedToken).Error)

	svc := services.NewEmailVerificationService(db)

	status, err := svc.GetVerificationStatus("user-999")
	if err != nil {
		t.Logf("GetVerificationStatus returned error (acceptable): %v", err)
		return
	}

	assert.False(t, status.Verified,
		"Must be false when no used token exists")
}
