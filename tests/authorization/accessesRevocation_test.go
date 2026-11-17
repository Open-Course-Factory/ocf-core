package authorization_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	accessController "soli/formations/src/auth/routes/accessesRoutes"
	"soli/formations/src/utils"
)

// useStoredPolicyEnforcer installs a real Casbin enforcer backed by SQLite as
// casdoor.Enforcer: only storage and memory together show a stale rule.
func useStoredPolicyEnforcer(t *testing.T) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	previous := casdoor.Enforcer
	t.Cleanup(func() { casdoor.Enforcer = previous })
	casdoor.InitCasdoorEnforcer(db, filepath.Dir(thisFile)+"/../../")
	require.NoError(t, casdoor.Enforcer.LoadPolicy())
}

// callAccessesHandler runs an /accesses handler with the given JSON body and returns the status.
func callAccessesHandler(t *testing.T, handler gin.HandlerFunc, method string, input any) int {
	t.Helper()
	body, err := json.Marshal(input)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(method, "/api/v1/accesses", bytes.NewBuffer(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return w.Code
}

func TestAccessesEndpoint_RevokeAccess_DeniedWithoutPolicyReload(t *testing.T) {
	useStoredPolicyEnforcer(t)
	subject, route := "revoke-subject-001", "/api/v1/class-groups/revoke-group-001"
	err := utils.AddPolicy(casdoor.Enforcer, subject, route, "(GET)", utils.DefaultPermissionOptions())
	require.NoError(t, err)

	status := callAccessesHandler(t, accessController.NewAccessController().DeleteEntityAccesses, http.MethodDelete,
		dto.DeleteEntityAccessInput{GroupName: subject, Route: route})
	require.Equal(t, http.StatusNoContent, status)

	allowed, err := casdoor.Enforcer.Enforce(subject, route, "GET")
	require.NoError(t, err)
	assert.False(t, allowed, "a revoked access must be denied straight after the 204")
}

// Guards the outcome, not one mechanism: the broader grant is gone straight
// after the 201, whether through the handler's reload or the filtered removal.
func TestAccessesEndpoint_GrantNarrowerAccess_ReplacesBroaderWithoutPolicyReload(t *testing.T) {
	useStoredPolicyEnforcer(t)
	subject, route := "narrow-subject-001", "/api/v1/class-groups/narrow-group-001"
	err := utils.AddPolicy(casdoor.Enforcer, subject, route, "(GET|POST|DELETE)", utils.DefaultPermissionOptions())
	require.NoError(t, err)

	status := callAccessesHandler(t, accessController.NewAccessController().AddEntityAccesses, http.MethodPost,
		dto.CreateEntityAccessInput{GroupName: subject, Route: route, AuthorizedMethods: "(GET)"})
	require.Equal(t, http.StatusCreated, status)

	canDelete, err := casdoor.Enforcer.Enforce(subject, route, "DELETE")
	require.NoError(t, err)
	assert.False(t, canDelete, "the broader grant must be gone straight after the 201")
	canGet, err := casdoor.Enforcer.Enforce(subject, route, "GET")
	require.NoError(t, err)
	assert.True(t, canGet, "the narrower grant must be in force")
}
