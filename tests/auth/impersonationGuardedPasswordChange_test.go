// tests/auth/impersonationGuardedPasswordChange_test.go
//
// #501: the impersonation middleware sets userId to the target and keeps the
// admin in impersonatorId, so every handler deriving its subject from userId
// acts AS the target. For self-scoped irreversible actions that is an account
// takeover. DeleteMyAccount already refuses under impersonation; the two
// password changes did not.
package auth_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	userController "soli/formations/src/auth/routes/usersRoutes"
)

func impersonatedPasswordChangeContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("userId", "victim-501")
	ctx.Set("impersonatorId", "admin-501")
	ctx.Request = httptest.NewRequest(http.MethodPost, path,
		strings.NewReader(`{"current_password":"old","new_password":"NewPass123!","confirm_password":"NewPass123!"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Authorization", "Bearer victim-token")
	return ctx, w
}

func TestChangePassword_DuringImpersonation_Forbidden(t *testing.T) {
	ctx, w := impersonatedPasswordChangeContext(t, "/users/me/change-password")

	// The guard must answer before the settings service runs: that service
	// talks to Casdoor, which no test can reach, so a 400/500 here would mean
	// the request went through.
	userController.NewUserController().ChangePassword(ctx)

	assert.Equal(t, http.StatusForbidden, w.Code, "an impersonating admin must not change the target's password. Body: %s", w.Body.String())
}

func TestForceChangePassword_DuringImpersonation_Forbidden(t *testing.T) {
	ctx, w := impersonatedPasswordChangeContext(t, "/users/me/force-change-password")

	userController.NewUserController().ForceChangePassword(ctx)

	assert.Equal(t, http.StatusForbidden, w.Code, "Body: %s", w.Body.String())
}
