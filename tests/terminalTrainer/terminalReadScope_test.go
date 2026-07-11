// tests/terminalTrainer/terminalReadScope_test.go
//
// RED tests for MR C at the Terminal layer (the actual H2 IDOR): the REAL
// Terminal entity registration must be owner-read-scoped on the generic read
// path. Today the Terminal registration carries no read OwnershipConfig, so a
// member can list and fetch OTHER users' terminal sessions.
//
// These drive the real generic router (controller.GetEntities / GetEntity)
// over the real terminalRegistration.RegisterTerminal, and assert
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

// registerTerminalForReadScope registers the REAL Terminal entity into a fresh
// global registration service and restores an empty service afterwards.
// No terminalTrainer test depends on a pre-existing global registration, so the
// reset is safe.
func registerTerminalForReadScope(t *testing.T) {
	t.Helper()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	terminalRegistration.RegisterTerminal(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	})
}

func terminalReadScopeRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
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
	r.GET("/api/v1/terminals/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/terminals/:id", func(c *gin.Context) { gc.GetEntity(c) })
	return r
}

type terminalReadScopeListResponse struct {
	Data []struct {
		ID        string `json:"id"`
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
	} `json:"data"`
	Total int64 `json:"total"`
}

// --- Terminal LIST as a non-owner returns only the caller's terminals -------

func TestTerminalReadScope_ListAsNonOwner_ReturnsOnlyOwnTerminals(t *testing.T) {
	registerTerminalForReadScope(t)
	db := freshTestDB(t)

	ta, err := createTestTerminal(db, "user-A", "running", nil)
	require.NoError(t, err)
	tb, err := createTestTerminal(db, "user-B", "running", nil)
	require.NoError(t, err)

	r := terminalReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/terminals/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp terminalReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[ta.ID.String()], "A must see own terminal")
	assert.False(t, ids[tb.ID.String()], "A must NOT see B's terminal (H2 cross-user read)")
	assert.Equal(t, int64(1), resp.Total, "Total must be owner-scoped to A's single terminal")
}

// --- Terminal GET another user's terminal → 404, no owner-only leak ---------

func TestTerminalReadScope_GetByIdOtherUsersTerminal_Returns404(t *testing.T) {
	registerTerminalForReadScope(t)
	db := freshTestDB(t)

	tb, err := createTestTerminal(db, "user-B", "running", nil)
	require.NoError(t, err)

	r := terminalReadScopeRouter(db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/terminals/"+tb.ID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A fetching B's terminal by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	// The 404 body must not disclose B's owner-only session identifier.
	assert.NotContains(t, w.Body.String(), tb.SessionID,
		"404 body must not leak the non-owner terminal's SessionID")
}

// --- Admin sees every user's terminals (regression guard: AdminBypass) ------

func TestTerminalReadScope_ListAsAdmin_ReturnsAllTerminals(t *testing.T) {
	registerTerminalForReadScope(t)
	db := freshTestDB(t)

	ta, err := createTestTerminal(db, "user-A", "running", nil)
	require.NoError(t, err)
	tb, err := createTestTerminal(db, "user-B", "running", nil)
	require.NoError(t, err)

	r := terminalReadScopeRouter(db, "admin-1", []string{"administrator"})
	req := httptest.NewRequest("GET", "/api/v1/terminals/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp terminalReadScopeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[ta.ID.String()], "admin must see A's terminal")
	assert.True(t, ids[tb.ID.String()], "admin must see B's terminal (AdminBypass)")
	assert.Equal(t, int64(2), resp.Total, "admin Total must count all terminals")
}
