package scenarios_test

// RED tests for MR D — ScenarioSession ownership enforcement (H4).
//
// ScenarioSession is a Casbin "member GET" entity. Two holes were pinned here:
//
//  1. CREATE: the generic create route is no longer mounted; runs are created
//     by the launch routes only (see scenarioSessionGenericCreate_test.go).
//
//  2. READ scope. The registration carries no read OwnershipConfig, so a member
//     can LIST and GET other users' sessions (their flags, grade, progress).
//     The fix reuses the already-merged owner-read-scope mechanism
//     (OwnershipConfig{Operations:["read"]}) on the generic read path. These
//     tests mirror tests/terminalTrainer/terminalReadScope_test.go.
//
// All assertions are USER-OBSERVABLE (HTTP status, response body, DB row).
//
// RED today:
//   - ListAsNonOwner_ReturnsOnlyOwnSessions        (no read scope)
//   - GetByIdOtherUsersSession_Returns404          (no read scope)
// Green-guard today (regression protection against over-restriction):
//   - ListAsAdmin_ReturnsAllSessions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementController "soli/formations/src/entityManagement/routes"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// =============================================================================
// Seed helpers
// =============================================================================

// seedSessionScenario creates a distinct scenario so that every seeded session
// can be active without tripping the (user_id, scenario_id) partial unique
// index on active sessions.
func seedSessionScenario(t *testing.T, db *gorm.DB) models.Scenario {
	t.Helper()
	s := models.Scenario{
		Name:        "sc-" + time.Now().Format("150405.000000000"),
		Title:       "Session ownership scenario",
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&s).Error)
	return s
}

// seedActiveSession seeds one active ScenarioSession owned by userID (on its own
// fresh scenario) directly via GORM, bypassing the hook chain.
func seedActiveSession(t *testing.T, db *gorm.DB, userID string) models.ScenarioSession {
	t.Helper()
	scenario := seedSessionScenario(t, db)
	sess := models.ScenarioSession{
		ScenarioID: scenario.ID,
		UserID:     userID,
		Status:     "active",
		StartedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&sess).Error)
	return sess
}

// =============================================================================
// READ-scope harness — mirrors tests/terminalTrainer/terminalReadScope_test.go.
// Registers the REAL ScenarioSession entity and mounts the generic LIST + GET
// handlers behind a middleware that stamps the acting identity. Hooks are
// disabled so the direct db.Create seeding never triggers async ownership work.
// =============================================================================

func scenarioSessionReadRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()

	originalSvc := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenarioSession(ems.GlobalEntityRegistrationService)
	hooks.GlobalHookRegistry.DisableAllHooks(true)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = originalSvc
		hooks.GlobalHookRegistry.DisableAllHooks(false)
	})

	gen := entityManagementController.NewGenericController(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("userId", userID)
		}
		c.Set("userRoles", roles)
		c.Next()
	})
	api.GET("/scenario-sessions", gen.GetEntities)
	api.GET("/scenario-sessions/:id", gen.GetEntity)
	return r
}

type scenarioSessionListResponse struct {
	Data []struct {
		ID     string `json:"id"`
		UserID string `json:"user_id"`
	} `json:"data"`
	Total int64 `json:"total"`
}

// --- LIST as a non-owner returns only the caller's sessions -----------------

func TestScenarioSession_ListAsNonOwner_ReturnsOnlyOwnSessions(t *testing.T) {
	db := freshTestDB(t)

	a1 := seedActiveSession(t, db, "user-A")
	a2 := seedActiveSession(t, db, "user-A")
	b1 := seedActiveSession(t, db, "user-B")

	r := scenarioSessionReadRouter(t, db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/scenario-sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "list should succeed; body=%s", w.Body.String())

	var resp scenarioSessionListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[a1.ID.String()], "A must see own session a1")
	assert.True(t, ids[a2.ID.String()], "A must see own session a2")
	assert.False(t, ids[b1.ID.String()], "A must NOT see B's session (cross-user read / IDOR)")
	assert.Equal(t, int64(2), resp.Total, "Total must be owner-scoped to A's 2 sessions, not the global 3")
}

// --- GET another user's session → 404, no owner-only fields leak ------------

func TestScenarioSession_GetByIdOtherUsersSession_Returns404(t *testing.T) {
	db := freshTestDB(t)

	b1 := seedActiveSession(t, db, "user-B")

	r := scenarioSessionReadRouter(t, db, "user-A", []string{"member"})
	req := httptest.NewRequest("GET", "/api/v1/scenario-sessions/"+b1.ID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"A fetching B's session by id must get 404, got %d; body=%s", w.Code, w.Body.String())
	// The 404 body must not disclose B's owner-only session identity/progress.
	assert.NotContains(t, w.Body.String(), b1.ID.String(),
		"404 body must not leak the non-owner session id")
	assert.NotContains(t, w.Body.String(), "user-B",
		"404 body must not leak the non-owner session's UserID")
}

// --- Admin LIST returns every user's sessions (guard: AdminBypass) ----------

func TestScenarioSession_ListAsAdmin_ReturnsAllSessions(t *testing.T) {
	db := freshTestDB(t)

	a1 := seedActiveSession(t, db, "user-A")
	b1 := seedActiveSession(t, db, "user-B")

	r := scenarioSessionReadRouter(t, db, "admin-1", []string{"administrator"})
	req := httptest.NewRequest("GET", "/api/v1/scenario-sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "admin list should succeed; body=%s", w.Body.String())

	var resp scenarioSessionListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := map[string]bool{}
	for _, d := range resp.Data {
		ids[d.ID] = true
	}
	assert.True(t, ids[a1.ID.String()], "admin must see A's session")
	assert.True(t, ids[b1.ID.String()], "admin must see B's session (AdminBypass)")
	assert.Equal(t, int64(2), resp.Total, "admin Total must count all sessions")
}
