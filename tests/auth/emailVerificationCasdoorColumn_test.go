package auth_tests

import (
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/models"
	"soli/formations/src/auth/services"
)

// ============================================================
// Casdoor wrote the property but not the column
//
// casdoorsdk.UpdateUser sends no column list, so Casdoor applies its own
// default whitelist. That whitelist carries `properties` but not
// `email_verified`, so the write half-landed: every user who verified came
// back with an email_verified_at stamp and a false flag. 36 production
// accounts were sitting like that, each one 403ing on every payment route.
//
// Casdoor reports that as affected=false with a nil error, and the old code
// read only the error — so it burned the token and logged success over a
// write that never happened.
// ============================================================

func TestVerifyEmail_RequestsTheEmailVerifiedColumnExplicitly(t *testing.T) {
	db := setupVerificationTestDB(t)
	token := createTestToken(db, "user-columns", "columns@example.com", time.Now().Add(48*time.Hour))

	var gotColumns []string
	var gotVerified bool
	restore := services.SwapCasdoorUserWriter(
		func(userID string) (*casdoorsdk.User, error) {
			return &casdoorsdk.User{Id: userID, Email: "stub@example.com"}, nil
		},
		func(user *casdoorsdk.User, columns []string) (bool, error) {
			gotColumns = columns
			gotVerified = user.EmailVerified
			return true, nil
		},
	)
	defer restore()

	require.NoError(t, services.NewEmailVerificationService(db).VerifyEmail(token.Token))

	assert.True(t, gotVerified, "the user handed to Casdoor must carry EmailVerified=true")
	assert.Contains(t, gotColumns, "email_verified",
		"email_verified must be named explicitly — Casdoor's default column set silently omits it")
	assert.Contains(t, gotColumns, "properties",
		"properties carries email_verified_at and must still be written")
}

func TestVerifyEmail_TokenSurvives_WhenCasdoorReportsNoRowsAffected(t *testing.T) {
	db := setupVerificationTestDB(t)
	token := createTestToken(db, "user-noaffect", "noaffect@example.com", time.Now().Add(48*time.Hour))

	restore := services.SwapCasdoorUserWriter(
		func(userID string) (*casdoorsdk.User, error) {
			return &casdoorsdk.User{Id: userID, Email: "stub@example.com"}, nil
		},
		// Exactly what production returned: no error, nothing written.
		func(user *casdoorsdk.User, columns []string) (bool, error) {
			return false, nil
		},
	)
	defer restore()

	err := services.NewEmailVerificationService(db).VerifyEmail(token.Token)
	require.Error(t, err, "a write that changed nothing is not a successful verification")

	var reloaded models.EmailVerificationToken
	require.NoError(t, db.Where("token = ?", token.Token).First(&reloaded).Error)
	assert.Nil(t, reloaded.UsedAt,
		"the token must survive so the user can retry — burning it is what made this unrecoverable")
}

// ============================================================
// One reader, one rule
//
// Four code paths answered "is this address verified?". /auth/me and
// /auth/verify-status consulted Casdoor and then fell back to a used token in
// PostgreSQL; the RequireVerifiedEmail middleware and IsEmailVerified read
// Casdoor alone. So the interface could show a verified account while the
// API refused it — which is exactly how the bug presented.
// ============================================================

func TestIsEmailVerified_FallsBackToAUsedTokenWhenCasdoorSaysNo(t *testing.T) {
	db := setupVerificationTestDB(t)

	used := time.Now().Add(-time.Hour)
	token := createTestToken(db, "user-fallback", "fallback@example.com", time.Now().Add(48*time.Hour))
	token.UsedAt = &used
	require.NoError(t, db.Save(token).Error)

	restore := services.SwapCasdoorUserReader(func(userID string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: userID, Email: "fallback@example.com", EmailVerified: false}, nil
	})
	defer restore()

	verified, err := services.NewEmailVerificationService(db).IsEmailVerified("user-fallback")
	require.NoError(t, err)
	assert.True(t, verified,
		"a spent token proves the address was confirmed, whatever Casdoor's column says")
}

func TestIsEmailVerified_StaysFalseWithoutAnyProof(t *testing.T) {
	db := setupVerificationTestDB(t)

	restore := services.SwapCasdoorUserReader(func(userID string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: userID, Email: "nobody@example.com", EmailVerified: false}, nil
	})
	defer restore()

	verified, err := services.NewEmailVerificationService(db).IsEmailVerified("user-unverified")
	require.NoError(t, err)
	assert.False(t, verified, "no Casdoor flag and no spent token means genuinely unverified")
}
