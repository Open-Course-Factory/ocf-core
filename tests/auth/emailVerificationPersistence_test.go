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

func init() {
	// Initialize Casdoor SDK with a dummy config so calls fail with
	// connection errors instead of nil pointer panics. This lets us
	// test service behavior when Casdoor is unavailable.
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
}

// ============================================================
// Bug 2: Token burned before Casdoor update
//
// The current code (emailVerificationService.go:152-176) marks
// the token as used in PostgreSQL BEFORE calling Casdoor to
// update the user's EmailVerified field. If the Casdoor update
// fails, the token is permanently consumed but the user remains
// unverified — they cannot retry.
//
// These tests verify the correct behavior: if the Casdoor update
// fails, the token must NOT be marked as used.
// ============================================================

func TestVerifyEmail_TokenNotConsumed_WhenCasdoorUpdateFails(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Create a valid, unused token
	token := createTestToken(db, "user-123", "test@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	// The service's VerifyEmail method currently:
	//   1. Finds token in DB ✓
	//   2. Checks not used/expired ✓
	//   3. Marks token as used in DB  ← BUG: happens BEFORE Casdoor
	//   4. Calls casdoorsdk.GetUserByUserId ← fails, token already burned
	//
	// With Casdoor SDK initialized to localhost:0, the GetUserByUserId call
	// at line 160 will fail with a connection error (not a nil panic).
	// The service returns an error, but the token is already consumed in the DB.

	svc := services.NewEmailVerificationService(db)

	// Attempt to verify — will fail because Casdoor is unreachable
	err := svc.VerifyEmail(tokenValue)

	// The Casdoor call should fail (no real server at localhost:0)
	require.Error(t, err, "VerifyEmail should fail when Casdoor is unavailable")

	// BUG CHECK: After the Casdoor failure, the token should NOT be marked as used.
	// The user should be able to retry verification with the same token.
	var reloadedToken models.EmailVerificationToken
	dbErr := db.Where("token = ?", tokenValue).First(&reloadedToken).Error
	require.NoError(t, dbErr, "Token should still exist in DB")

	// THIS IS THE ASSERTION THAT SHOULD FAIL WITH THE CURRENT CODE:
	// The current code marks the token as used (UsedAt != nil) BEFORE
	// the Casdoor update, so even when Casdoor fails, the token is consumed.
	assert.Nil(t, reloadedToken.UsedAt,
		"Token must NOT be marked as used when Casdoor update fails — "+
			"current code burns the token before Casdoor confirmation, "+
			"making retry impossible")
	assert.True(t, reloadedToken.IsValid(),
		"Token should remain valid (unused and not expired) after Casdoor failure")
}

func TestVerifyEmail_TokenConsumed_OnlyAfterCasdoorSuccess(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Create a valid token
	token := createTestToken(db, "user-456", "success@example.com", time.Now().Add(48*time.Hour))
	tokenValue := token.Token

	svc := services.NewEmailVerificationService(db)

	// Attempt verification — will fail because Casdoor is at localhost:0
	err := svc.VerifyEmail(tokenValue)
	require.Error(t, err, "Expected Casdoor failure")

	// Verify token is still usable after first failed attempt
	var tokenAfterFirstAttempt models.EmailVerificationToken
	db.Where("token = ?", tokenValue).First(&tokenAfterFirstAttempt)

	// THIS ASSERTION SHOULD FAIL: The token should still be valid for retry
	assert.False(t, tokenAfterFirstAttempt.IsUsed(),
		"Token must not be consumed after failed Casdoor update — "+
			"user must be able to retry verification")

	// Try again — the token should still be valid
	err = svc.VerifyEmail(tokenValue)
	// With the current buggy code, the second attempt will fail with ErrTokenUsed
	// because the token was marked as used during the first (failed) attempt.
	// The correct behavior: the second attempt should fail with the same
	// Casdoor error, NOT with ErrTokenUsed.
	if err != nil {
		assert.NotEqual(t, services.ErrTokenUsed, err,
			"Second verification attempt should NOT get 'token already used' error — "+
				"the first attempt failed at Casdoor, so the token was never truly verified. "+
				"Current code bug: token is burned before Casdoor confirmation")
	}
}

// ============================================================
// Bug 3: LoginOutput missing email_verified fields
//
// The LoginOutput DTO (loginDto.go) has no Email, EmailVerified,
// or EmailVerifiedAt fields. After login, the frontend has no way
// to know if the user's email is verified without making an
// additional API call.
//
// These tests verify that LoginOutput includes these fields and
// they serialize correctly to JSON.
// ============================================================

func TestLoginOutput_HasEmailVerifiedFields(t *testing.T) {
	loginOutputType := reflect.TypeOf(dto.LoginOutput{})

	t.Run("has Email field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("Email")
		assert.True(t, found, "LoginOutput must have an Email field")
		if found {
			assert.Equal(t, "string", field.Type.Name(),
				"Email field must be a string")
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email",
				"Email field must have json tag containing 'email'")
		}
	})

	t.Run("has EmailVerified field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("EmailVerified")
		assert.True(t, found, "LoginOutput must have an EmailVerified field")
		if found {
			assert.Equal(t, "bool", field.Type.Name(),
				"EmailVerified field must be a bool")
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email_verified",
				"EmailVerified field must have json tag containing 'email_verified'")
		}
	})

	t.Run("has EmailVerifiedAt field", func(t *testing.T) {
		field, found := loginOutputType.FieldByName("EmailVerifiedAt")
		assert.True(t, found, "LoginOutput must have an EmailVerifiedAt field")
		if found {
			assert.Equal(t, "string", field.Type.Name(),
				"EmailVerifiedAt field must be a string")
			jsonTag := field.Tag.Get("json")
			assert.Contains(t, jsonTag, "email_verified_at",
				"EmailVerifiedAt field must have json tag containing 'email_verified_at'")
		}
	})
}

func TestLoginOutput_JSONSerialization_IncludesEmailFields(t *testing.T) {
	// Marshal the current LoginOutput to JSON and check what fields are present.
	// With the current struct, email verification fields are missing entirely.
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

	// These assertions should FAIL with the current LoginOutput
	// because it lacks Email, EmailVerified, EmailVerifiedAt fields:
	assert.Contains(t, jsonMap, "email",
		"LoginOutput JSON must include 'email' field — "+
			"frontend needs to display user email after login")
	assert.Contains(t, jsonMap, "email_verified",
		"LoginOutput JSON must include 'email_verified' field — "+
			"frontend needs to know verification status after login")
	assert.Contains(t, jsonMap, "email_verified_at",
		"LoginOutput JSON must include 'email_verified_at' field — "+
			"frontend needs to know when email was verified")
}

func TestLoginOutput_JSONSerialization_EmailVerifiedValues(t *testing.T) {
	// Test that email verification data survives a JSON round-trip.
	// With the current struct, these fields are silently dropped
	// during Unmarshal because the struct has no matching fields.
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

	// Re-marshal and check the fields survived
	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	require.NoError(t, err)

	// With the current struct, email/email_verified/email_verified_at will be
	// silently dropped during Unmarshal and won't appear in re-marshaled output
	assert.Equal(t, "test@example.com", jsonMap["email"],
		"Email field must survive JSON round-trip")
	assert.Equal(t, true, jsonMap["email_verified"],
		"EmailVerified=true must survive JSON round-trip")
	assert.Equal(t, "2025-01-15T10:00:00Z", jsonMap["email_verified_at"],
		"EmailVerifiedAt timestamp must survive JSON round-trip")
}

// ============================================================
// Bug 4: No PostgreSQL fallback for verification status
//
// GetVerificationStatus only checks Casdoor. If the Casdoor
// update succeeded but Casdoor data is out of sync (or if the
// Casdoor update failed but the DB has a valid used token),
// the status should fall back to PostgreSQL records.
//
// Expected behavior: if Casdoor says EmailVerified=false but
// a used (UsedAt != nil) verification token exists in the
// email_verification_tokens table for this user, the status
// should return verified: true as a fallback.
// ============================================================

func TestGetVerificationStatus_FallsBackToPostgreSQL(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Simulate: A token was used (verified in DB) but Casdoor is out of sync.
	// Create a used verification token in PostgreSQL.
	usedAt := time.Now().Add(-1 * time.Hour)
	usedToken := &models.EmailVerificationToken{
		UserID:    "user-789",
		Email:     "verified@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(47 * time.Hour),
		UsedAt:    &usedAt,
	}
	err := db.Create(usedToken).Error
	require.NoError(t, err)

	// Verify the used token exists in PostgreSQL
	var found models.EmailVerificationToken
	err = db.Where("user_id = ? AND used_at IS NOT NULL", "user-789").First(&found).Error
	require.NoError(t, err, "Used token must exist in DB for this test")
	assert.True(t, found.IsUsed(), "Token should be marked as used")

	// The current GetVerificationStatus ONLY calls casdoorsdk.GetUserByUserId.
	// With our dummy Casdoor config (localhost:0), it will fail with a
	// connection error — and the service returns that error with no fallback.
	//
	// The CORRECT behavior: when Casdoor is unavailable, fall back to checking
	// if a used token exists in the email_verification_tokens table.
	svc := services.NewEmailVerificationService(db)

	status, err := svc.GetVerificationStatus("user-789")

	if err != nil {
		// Case 1: Service failed because Casdoor is down and there's no fallback.
		// THIS IS THE BUG: the service should fall back to PostgreSQL.
		assert.Fail(t,
			"GetVerificationStatus should not fail when PostgreSQL has verification proof — "+
				"it must fall back to checking used tokens in email_verification_tokens table "+
				"when Casdoor is unavailable. Error was: "+err.Error())
	} else {
		// Case 2: Casdoor returned something — check if DB fallback was applied.
		// Even if we somehow reach Casdoor and it says unverified, the DB has proof.
		assert.True(t, status.Verified,
			"Verification status should be true when a used token exists in PostgreSQL, "+
				"even if Casdoor says EmailVerified=false — the DB is the source of truth "+
				"for verification actions that happened in our system")
		assert.Equal(t, "verified@example.com", status.Email,
			"Email should come from the verification token when falling back to PostgreSQL")
	}
}

func TestGetVerificationStatus_NoFallbackWithoutUsedToken(t *testing.T) {
	db := setupVerificationTestDB(t)

	// Create an UNUSED token (verification was not completed)
	unusedToken := &models.EmailVerificationToken{
		UserID:    "user-999",
		Email:     "unverified@example.com",
		Token:     generateTestToken(),
		ExpiresAt: time.Now().Add(48 * time.Hour),
		UsedAt:    nil, // Not used — verification not completed
	}
	err := db.Create(unusedToken).Error
	require.NoError(t, err)

	// Confirm no used tokens exist for this user
	var usedTokens []models.EmailVerificationToken
	db.Where("user_id = ? AND used_at IS NOT NULL", "user-999").Find(&usedTokens)
	assert.Empty(t, usedTokens,
		"No used tokens should exist — user hasn't verified their email")

	// The service should NOT claim verified when there's no used token.
	// Even with the PostgreSQL fallback, the absence of a used token means
	// the user genuinely hasn't verified their email.
	svc := services.NewEmailVerificationService(db)

	status, svcErr := svc.GetVerificationStatus("user-999")

	if svcErr != nil {
		// With the fallback in place, even when Casdoor is down,
		// the service could return unverified status from PostgreSQL
		// instead of an error. But this is a secondary concern — the
		// primary bug is in the positive fallback case above.
		t.Logf("GetVerificationStatus returned error (acceptable for negative case): %v", svcErr)
		return
	}

	// If we got a status, verified must be false
	assert.False(t, status.Verified,
		"Verification status must be false when no used token exists in PostgreSQL "+
			"and Casdoor says EmailVerified=false")
}
