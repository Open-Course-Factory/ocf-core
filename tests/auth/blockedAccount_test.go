// tests/auth/blockedAccount_test.go
//
// Blocking an account in Casdoor stops a new sign-in, but ocf-core validates
// Casdoor JWTs locally, so a token issued before the block kept working until
// it expired. CasdoorUserClient.SetForbidden now mirrors the flag into
// blocked_accounts and the auth middleware refuses a blocked account's token.
package auth_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	authController "soli/formations/src/auth"
	"soli/formations/src/auth/casdoor"
	apiErrors "soli/formations/src/auth/errors"
	"soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	authServices "soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	testTools "soli/formations/tests/testTools"
)

// fakeCasdoorAccount stands in for the identity provider's user row: reads
// return it, column writes land on it, and token revocations are counted.
type fakeCasdoorAccount struct {
	user        casdoorsdk.User
	revokedFor  []string
	restoreSwap func()
}

func installFakeCasdoorAccount(t *testing.T, userID string) *fakeCasdoorAccount {
	t.Helper()
	fake := &fakeCasdoorAccount{user: casdoorsdk.User{Id: userID, Owner: "org", Name: "blocked-learner"}}
	restoreUser := authServices.SwapCasdoorUserWriter(
		func(string) (*casdoorsdk.User, error) { u := fake.user; return &u, nil },
		func(u *casdoorsdk.User, _ []string) (bool, error) {
			fake.user.IsForbidden = u.IsForbidden
			return true, nil
		},
	)
	restoreRevoker := authServices.SwapCasdoorTokenRevoker(func(u *casdoorsdk.User) error {
		fake.revokedFor = append(fake.revokedFor, u.Name)
		return nil
	})
	t.Cleanup(func() { restoreRevoker(); restoreUser() })
	return fake
}

// blockedAccountRouter serves GET /probe behind the real AuthManagement, with
// the bearer token taken verbatim as the caller's user id.
func blockedAccountRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testTools.MemoryDB(&authModels.TokenBlacklist{}, &authModels.BlockedAccount{}, &auditModels.AuditLog{})

	prevDB, prevParser, prevEnforcer := sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer
	sqldb.DB = db
	casdoor.JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
		return &casdoorsdk.Claims{User: casdoorsdk.User{Id: token}}, nil
	}
	casdoor.Enforcer = mocks.NewMockEnforcer()
	t.Cleanup(func() { sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer = prevDB, prevParser, prevEnforcer })

	r := gin.New()
	r.GET("/probe", authController.NewAuthMiddleware(db).AuthManagement(), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, db
}

func probeAs(r *gin.Engine, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthManagement_BlockedAccount_TokenRejectedUntilUnblocked(t *testing.T) {
	r, _ := blockedAccountRouter(t)
	const userID = "learner-to-block"
	installFakeCasdoorAccount(t, userID)
	client := authServices.NewCasdoorUserClient()

	require.Equal(t, http.StatusOK, probeAs(r, userID).Code, "token works before the block")

	require.NoError(t, client.SetForbidden(userID, true))
	w := probeAs(r, userID)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "401 so the front logs the user out")
	var body apiErrors.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "user_blocked", body.ErrorMessage)

	assert.Equal(t, http.StatusOK, probeAs(r, "someone-else").Code, "the block is per account")

	require.NoError(t, client.SetForbidden(userID, false))
	assert.Equal(t, http.StatusOK, probeAs(r, userID).Code, "unblock restores the same token")
}

func TestCasdoorSetForbidden_Block_MirrorsAndRevokesCasdoorTokens(t *testing.T) {
	_, db := blockedAccountRouter(t)
	const userID = "learner-mirror"
	fake := installFakeCasdoorAccount(t, userID)
	client := authServices.NewCasdoorUserClient()

	require.NoError(t, client.SetForbidden(userID, true))
	assert.True(t, fake.user.IsForbidden, "Casdoor is written")
	blocked, err := authModels.IsAccountBlocked(db, userID)
	require.NoError(t, err)
	assert.True(t, blocked, "the mirror row exists")
	assert.Equal(t, []string{"blocked-learner"}, fake.revokedFor, "Casdoor-side tokens revoked on block")

	require.NoError(t, client.SetForbidden(userID, true), "blocking twice is idempotent")

	require.NoError(t, client.SetForbidden(userID, false))
	blocked, err = authModels.IsAccountBlocked(db, userID)
	require.NoError(t, err)
	assert.False(t, blocked)
	assert.Len(t, fake.revokedFor, 2, "unblock revokes nothing")
}

func TestCasdoorSetForbidden_CasdoorWriteFails_MirrorUntouched(t *testing.T) {
	_, db := blockedAccountRouter(t)
	const userID = "learner-casdoor-down"
	restore := authServices.SwapCasdoorUserWriter(
		func(string) (*casdoorsdk.User, error) { return &casdoorsdk.User{Id: userID}, nil },
		func(*casdoorsdk.User, []string) (bool, error) { return false, nil },
	)
	t.Cleanup(restore)

	require.Error(t, authServices.NewCasdoorUserClient().SetForbidden(userID, true))
	blocked, err := authModels.IsAccountBlocked(db, userID)
	require.NoError(t, err)
	assert.False(t, blocked, "the mirror follows Casdoor, never leads it")
}
