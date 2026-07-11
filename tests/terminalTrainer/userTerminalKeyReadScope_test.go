// tests/terminalTrainer/userTerminalKeyReadScope_test.go
//
// RED tests for MR G at the UserTerminalKey layer: the REAL UserTerminalKey
// entity registration must be owner-read-scoped on the generic read path.
// Today RegisterUserTerminalKey carries no read OwnershipConfig, so a member
// can list and fetch OTHER users' terminal keys through the generic
// GET /user-terminal-keys and GET /user-terminal-keys/:id handlers.
//
// These drive the real generic router (controller.GetEntities / GetEntity)
// over the real terminalRegistration.RegisterUserTerminalKey, and assert
// USER-OBSERVABLE state (HTTP status + response body rows), never a mock call.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	controller "soli/formations/src/entityManagement/routes"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// registerUserTerminalKeyForReadScope registers the REAL UserTerminalKey entity
// into a fresh global registration service and restores the previous service
// afterwards, so sibling terminalTrainer tests are unaffected.
func registerUserTerminalKeyForReadScope(t *testing.T) {
	t.Helper()
	prev := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	terminalRegistration.RegisterUserTerminalKey(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = prev
	})
}

func userTerminalKeyReadScopeRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	gc := controller.NewGenericController(db, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
		}
		c.Set("userRoles", roles)
		c.Next()
	})
	r.GET("/api/v1/user-terminal-keys/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/user-terminal-keys/:id", func(c *gin.Context) { gc.GetEntity(c) })
	return r
}

type userTerminalKeyReadScopeListResponse struct {
	Data []struct {
		ID      string `json:"id"`
		UserID  string `json:"user_id"`
		KeyName string `json:"key_name"`
	} `json:"data"`
	Total int64 `json:"total"`
}

// --- UserTerminalKey LIST as a non-owner returns only the caller's keys ------

func TestUserTerminalKeyReadScope_ListAsNonOwner_ReturnsOnlyOwnRows(t *testing.T) {
	registerUserTerminalKeyForReadScope(t)
	db := freshTestDB(t)

	ka, err := createTestUserKey(db, "user-A")
	require.NoError(t, err)
	kb, err := createTestUserKey(db, "user-B")
	require.NoError(t, err)

	r := userTerminalKeyReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/user-terminal-keys/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp userTerminalKeyReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[ka.ID.String()], "A must see own terminal key")
	assert.False(t, ids[kb.ID.String()], "A must NOT see B's terminal key (cross-user read)")
	assert.Equal(t, int64(1), resp.Total, "Total must be owner-scoped to A's single key")
}

// --- UserTerminalKey GET another user's key → 404, no owner-only leak --------

func TestUserTerminalKeyReadScope_GetByIdOtherUsersRow_Returns404(t *testing.T) {
	registerUserTerminalKeyForReadScope(t)
	db := freshTestDB(t)

	kb, err := createTestUserKey(db, "user-B")
	require.NoError(t, err)

	r := userTerminalKeyReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/user-terminal-keys/"+kb.ID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A fetching B's terminal key by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	// The 404 body must not disclose B's owner-only key name.
	assert.NotContains(t, w.Body.String(), kb.KeyName,
		"404 body must not leak the non-owner key's KeyName")
}

// --- Admin sees every user's keys (regression guard: AdminBypass) ------------

func TestUserTerminalKeyReadScope_ListAsAdmin_ReturnsAllRows(t *testing.T) {
	registerUserTerminalKeyForReadScope(t)
	db := freshTestDB(t)

	ka, err := createTestUserKey(db, "user-A")
	require.NoError(t, err)
	kb, err := createTestUserKey(db, "user-B")
	require.NoError(t, err)

	r := userTerminalKeyReadScopeRouter(db, "admin-1", []string{"administrator"})
	req := httptest.NewRequest("GET", "/api/v1/user-terminal-keys/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp userTerminalKeyReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[ka.ID.String()], "admin must see A's key")
	assert.True(t, ids[kb.ID.String()], "admin must see B's key (AdminBypass)")
	assert.Equal(t, int64(2), resp.Total, "admin Total must count all keys")
}
