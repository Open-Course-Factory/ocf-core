// tests/terminalTrainer/myTerminalUsage_test.go
//
// GET /terminals/my-usage — the live budget snapshot endpoint that
// powers the dashboard "Utilisation Actuelle" panel.
//
// Contract:
//   - Plan envelope: plan_name, plan_source ("personal"|"organization"),
//     plan_source_name (org name when source=organization), max_cpu,
//     max_memory_mb, max_session_duration_minutes.
//   - Live counters: used_cpu, used_memory_mb (sum of CPU/RAM held by
//     budget-occupying sessions — same predicate as the budget gate).
//   - active_sessions: per-session list keyed by the same scope as the
//     bars (OccupiesSlotScope, the SSOT post-D6') so the list cannot
//     disagree with the totals.
//
// Edge predicates exercised:
//   - Stopped (persistent OR ephemeral): counted + listed (D6': "a stop
//     is a stop"; the slot is reserved until sync confirms tt-backend
//     reaped the container).
//   - Past-expiry zombie: NOT counted, NOT listed.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	paymentServices "soli/formations/src/payment/services"
	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// mountMyUsageRouter wires the controller with a stub auth middleware that
// sets userId / userRoles, and exposes GET /terminals/my-usage.
func mountMyUsageRouter(t *testing.T, db interface{}, userID string) *gin.Engine {
	t.Helper()
	ctrl := terminalController.NewTerminalController(sharedTestDB)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/terminals/my-usage", ctrl.MyTerminalUsage)
	return router
}

// seedPersonalPlanFor creates a SubscriptionPlan + active UserSubscription
// linking the user to it. Returns the plan so callers can assert against it.
func seedPersonalPlanFor(t *testing.T, userID string, maxCPU, maxMem, maxDur int, name string) *paymentModels.SubscriptionPlan {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		Name:                      name,
		MaxCPU:                    maxCPU,
		MaxMemoryMB:               maxMem,
		MaxSessionDurationMinutes: maxDur,
		IsActive:                  true,
		IsCatalog:                 true,
	}
	require.NoError(t, sharedTestDB.Create(plan).Error)
	sub := &paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, sharedTestDB.Create(sub).Error)
	return plan
}

// TestMyTerminalUsage_EmptyState — user with a plan but no terminals.
// used_* = 0, active_sessions = [].
func TestMyTerminalUsage_EmptyState(t *testing.T) {
	freshTestDB(t)
	userID := "user-empty"
	seedPersonalPlanFor(t, userID, 8000, 4096, 60, "Pro")

	router := mountMyUsageRouter(t, sharedTestDB, userID)
	req := httptest.NewRequest("GET", "/terminals/my-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "Pro", resp["plan_name"])
	assert.Equal(t, "personal", resp["plan_source"])
	assert.Equal(t, "", resp["plan_source_name"])
	assert.Equal(t, float64(8000), resp["max_cpu"])
	assert.Equal(t, float64(4096), resp["max_memory_mb"])
	assert.Equal(t, float64(60), resp["max_session_duration_minutes"])
	assert.Equal(t, float64(0), resp["used_cpu"])
	assert.Equal(t, float64(0), resp["used_memory_mb"])

	sessions, ok := resp["active_sessions"].([]interface{})
	require.True(t, ok, "active_sessions must be present as an array")
	assert.Equal(t, 0, len(sessions))
}

// TestMyTerminalUsage_RunningEphemeral — one running ephemeral M
// (2000 mCPU / 1g). Counted + listed with state=running,
// persistence_mode=ephemeral.
func TestMyTerminalUsage_RunningEphemeral(t *testing.T) {
	freshTestDB(t)
	userID := "user-running-eph"
	seedPersonalPlanFor(t, userID, 8000, 4096, 60, "Pro")

	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 2000, 1024)

	router := mountMyUsageRouter(t, sharedTestDB, userID)
	req := httptest.NewRequest("GET", "/terminals/my-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, float64(2000), resp["used_cpu"])
	assert.Equal(t, float64(1024), resp["used_memory_mb"])

	sessions, ok := resp["active_sessions"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 1, len(sessions))
	entry := sessions[0].(map[string]interface{})
	assert.Equal(t, "running", entry["state"])
	assert.Equal(t, "ephemeral", entry["persistence_mode"])
	assert.Equal(t, float64(2000), entry["size_cpu"])
	assert.Equal(t, float64(1024), entry["size_memory_mb"])
}

// TestMyTerminalUsage_Stopped_CountedRegardlessOfPersistence — every
// stopped session counts and appears in the list, regardless of
// persistence_mode (D6', supersedes D6).
func TestMyTerminalUsage_Stopped_CountedRegardlessOfPersistence(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			freshTestDB(t)
			userID := "user-stopped-" + tc.mode
			seedPersonalPlanFor(t, userID, 8000, 4096, 60, "Pro")

			insertExistingTerminal(t, sharedTestDB, userID, nil, "stopped", tc.mode, 4000, 2048)

			router := mountMyUsageRouter(t, sharedTestDB, userID)
			req := httptest.NewRequest("GET", "/terminals/my-usage", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			assert.Equal(t, float64(4000), resp["used_cpu"],
				"stopped %s reserves CPU under OccupiesSlotScope (D6')", tc.mode)
			assert.Equal(t, float64(2048), resp["used_memory_mb"])

			sessions, ok := resp["active_sessions"].([]interface{})
			require.True(t, ok)
			require.Equal(t, 1, len(sessions),
				"stopped %s must appear in the session list (D6': paused = reserved)", tc.mode)
			entry := sessions[0].(map[string]interface{})
			assert.Equal(t, "stopped", entry["state"])
			assert.Equal(t, tc.mode, entry["persistence_mode"])
		})
	}
}

// TestMyTerminalUsage_ExpiredZombie_Excluded — past-expiry rows whose state
// column was never reset must not poison the snapshot.
func TestMyTerminalUsage_ExpiredZombie_Excluded(t *testing.T) {
	freshTestDB(t)
	userID := "user-zombie"
	seedPersonalPlanFor(t, userID, 8000, 4096, 60, "Pro")

	insertExistingTerminalWithExpiry(t, sharedTestDB, userID, nil,
		"running", "persistent", 4000, 2048, time.Now().Add(-1*time.Hour))

	router := mountMyUsageRouter(t, sharedTestDB, userID)
	req := httptest.NewRequest("GET", "/terminals/my-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, float64(0), resp["used_cpu"], "expired zombies must not count")
	assert.Equal(t, float64(0), resp["used_memory_mb"])
	sessions, _ := resp["active_sessions"].([]interface{})
	assert.Equal(t, 0, len(sessions), "expired zombies must not appear in the list")
}

// insertReservationPlaceholder writes a live (future-expiry) StateStarting
// reservation row with a `reserving:<uuid>` placeholder session_id — the exact
// shape the reserve-first composed path commits before it POSTs to tt-backend.
// Such a row MUST count toward the budget (StateStarting is in
// TerminalStatesOccupyingSlot) but MUST be hidden from the user-facing session
// list (no console URL; vanishes on finalize or TTL lapse). Raw SQL mirrors
// insertExistingTerminal but pins the placeholder prefix so the predicate the
// production list-filter keys on (IsReservationPlaceholderSessionID) is what's
// under test.
func insertReservationPlaceholder(t *testing.T, userID string, cpu, memMB int) string {
	t.Helper()
	id := uuid.New().String()
	sessionID := terminalModels.TerminalReservationSessionIDPrefix + uuid.New().String()
	err := sharedTestDB.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, user_id, organization_id, session_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		id, time.Now(), time.Now(), userID, nil,
		sessionID, string(terminalModels.StateStarting), "ephemeral",
		cpu, memMB, time.Now().Add(5*time.Minute), uuid.New().String()).Error
	require.NoError(t, err)
	return sessionID
}

// TestGetUserTerminalUsage_ReservationCountsBudgetButHiddenFromSessionList pins
// the contract added by the reserve-first fix: a live reservation placeholder
// (StateStarting, `reserving:<uuid>` session_id, future expiry) must
//
//   - COUNT toward the budget (so concurrent starts can't overshoot while a
//     reservation is in flight) — proven via QuotaService.GetBudgetUsage, the
//     exact surface the budget gate enforces, AND
//   - be ABSENT from GetUserTerminalUsage().ActiveSessions (no console URL; a
//     placeholder is not a real session) — while a genuinely-running session
//     seeded alongside it IS still listed.
//
// The two halves together prove the fix's split: the budget SUM and the
// per-session LIST read the same OccupiesSlotScope, but the list applies the
// placeholder filter (IsReservationPlaceholderSessionID) the sum does not.
func TestGetUserTerminalUsage_ReservationCountsBudgetButHiddenFromSessionList(t *testing.T) {
	freshTestDB(t)
	userID := "user-resv-hidden-" + uuid.New().String()[:8]
	// Budget large enough to hold both rows so neither is reaped for over-budget.
	seedPersonalPlanFor(t, userID, 16000, 8192, 60, "Pro")

	// A genuinely-running session (normal session_id) — the contrast case that
	// MUST stay visible in the list.
	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 2000, 1024)

	// A live reservation placeholder — counts for budget, hidden from the list.
	insertReservationPlaceholder(t, userID, 4000, 2048)

	eps := paymentServices.NewEffectivePlanService(sharedTestDB)
	qs := paymentServices.NewQuotaService(sharedTestDB, eps)

	// (1) The reservation COUNTS: the budget sum is running + reservation.
	usedCPU, usedMem, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	assert.Equal(t, 2000+4000, usedCPU,
		"the live reservation placeholder MUST count toward the CPU budget alongside the running session")
	assert.Equal(t, 1024+2048, usedMem,
		"the live reservation placeholder MUST count toward the RAM budget alongside the running session")

	// (2) The reservation is HIDDEN from the user-facing session list, but the
	// real running session is still present — asserted on the production
	// GetUserTerminalUsage struct, not on the budget sum.
	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	usage, err := svc.GetUserTerminalUsage(userID, nil)
	require.NoError(t, err)

	var sawReservation, sawRunning bool
	for _, s := range usage.ActiveSessions {
		if terminalModels.IsReservationPlaceholderSessionID(s.SessionID) {
			sawReservation = true
		}
		if s.State == terminalModels.StateRunning && !terminalModels.IsReservationPlaceholderSessionID(s.SessionID) {
			sawRunning = true
		}
	}
	assert.False(t, sawReservation,
		"a reservation placeholder (reserving:<uuid>) must NOT appear in the user-facing session list")
	assert.True(t, sawRunning,
		"a genuinely-running session must still appear in the session list — only placeholders are hidden")
	assert.Len(t, usage.ActiveSessions, 1,
		"exactly the one real running session is listed; the reservation is filtered out")
}

// TestMyTerminalUsage_OrgContext — when ?organization_id=<orgID> is provided,
// the response reflects the org's plan and the org-aggregated usage.
func TestMyTerminalUsage_OrgContext(t *testing.T) {
	freshTestDB(t)
	ownerID := "org-owner-1"
	memberID := "org-member-1"
	// Seed a personal plan for the calling user so the org-context path can
	// be exercised distinctly — used to assert that the org plan supersedes.
	seedPersonalPlanFor(t, ownerID, 2000, 1024, 30, "PersonalSolo")

	// Build org + members.
	org := createTestOrgForHistory(t, sharedTestDB, ownerID)
	createTestOrgMember(t, sharedTestDB, org.ID, ownerID, orgModels.OrgRoleOwner)
	createTestOrgMember(t, sharedTestDB, org.ID, memberID, orgModels.OrgRoleMember)

	// Org-level plan (richer than the personal one).
	orgPlan := &paymentModels.SubscriptionPlan{
		Name:                      "OrgPro",
		MaxCPU:                    16000, // 16 vCPU in mCPU
		MaxMemoryMB:                16384,
		MaxSessionDurationMinutes: 120,
		IsActive:                  true,
		IsCatalog:                 true,
	}
	require.NoError(t, sharedTestDB.Create(orgPlan).Error)
	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: orgPlan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, sharedTestDB.Create(orgSub).Error)

	// Two terminals — one per org member — both tied to the org via the
	// terminals.organization_id column. The org-context sum is built by
	// joining through organization_members (mirrors sumActiveResourcesForOrg).
	insertExistingTerminal(t, sharedTestDB, ownerID, &org.ID, "running", "ephemeral", 2000, 1024)
	insertExistingTerminal(t, sharedTestDB, memberID, &org.ID, "stopped", "persistent", 4000, 2048)

	ctrl := terminalController.NewTerminalController(sharedTestDB)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", ownerID)
		c.Set("userRoles", []string{"member"})
		// Mirror InjectOrgContext: stash the org under the well-known key.
		c.Set("org_context_id", org.ID.String())
		c.Next()
	})
	router.GET("/terminals/my-usage", ctrl.MyTerminalUsage)

	req := httptest.NewRequest("GET", "/terminals/my-usage?organization_id="+org.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "OrgPro", resp["plan_name"], "org plan must supersede personal")
	assert.Equal(t, "organization", resp["plan_source"])
	assert.Equal(t, org.Name, resp["plan_source_name"])
	assert.Equal(t, float64(16000), resp["max_cpu"])
	assert.Equal(t, float64(16384), resp["max_memory_mb"])
	assert.Equal(t, float64(120), resp["max_session_duration_minutes"])

	// 2000 + 4000 mCPU = 6000; 1024 + 2048 = 3072.
	assert.Equal(t, float64(6000), resp["used_cpu"],
		"org context must sum across all org members (owner + member) in mCPU")
	assert.Equal(t, float64(3072), resp["used_memory_mb"])
}
