package terminalTrainer_tests

// TestBulkCreateTerminals_InsufficientRAM_Refused verifies that BulkCreateTerminalsForGroup
// refuses the request up-front when the Terminal Trainer server has insufficient RAM to
// provision terminals for all active group members.
//
// Issue #170 (security): the bulk-create route has no RAM availability check, allowing a
// group owner with many members to OOM the server.
//
// Planned fix: add a pre-flight RAM check inside BulkCreateTerminalsForGroup, before the
// provision loop, that calls GetServerMetrics and aborts with an error if
//   len(activeMembers) * perTerminalRAM > RAMAvailableGB * 0.95
//
// This test FAILS today because the check does not exist — the service proceeds into the
// provision loop regardless of available RAM and returns nil error.
//
// Seam note: the test uses t.Setenv("TERMINAL_TRAINER_URL", …) to point the real
// TerminalTrainerService at a fake httptest.Server. This works because GetServerMetrics
// and StartComposedSession both read tts.baseURL at call time. If the backend-dev needs
// to extract a ServerMetricsProvider interface instead, this test can be adapted.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// startLowRAMTTServer starts a fake tt-backend that:
//   - GET /1.0/metrics  → low-RAM response (1 GB available, 95% used)
//   - everything else   → 503 (should never be reached if the fix lands)
//
// Returns the test server; caller must call Close().
func startLowRAMTTServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/metrics") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dto.ServerMetricsResponse{
				RAMPercent:     95.0,
				RAMAvailableGB: 1.0,
				Timestamp:      time.Now().Unix(),
			})
			return
		}
		// Any other call (distributions, compose, …) means the RAM check was bypassed.
		http.Error(w, "unexpected call — RAM check should have refused first", http.StatusServiceUnavailable)
	}))
}

// addActiveGroupMember inserts a group member with is_active=true.
// We cannot reuse addGroupMember because it hardcodes is_active=false.
func addActiveGroupMember(t *testing.T, db interface {
	Exec(sql string, values ...interface{}) *gorm.DB
}, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) {
	t.Helper()
	id := uuid.New()
	err := db.Exec(
		`INSERT INTO group_members (id, created_at, updated_at, group_id, user_id, role, is_active, joined_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), time.Now(), time.Now(), groupID.String(), userID, string(role), true, time.Now(),
	).Error
	require.NoError(t, err)
}

// TestBulkCreateTerminals_InsufficientRAM_Refused is the red TDD test for issue #170.
//
// Setup: 5 active members, plan AllowedMachineSizes=["S"] (0.5 GB each),
//        server metrics: 1.0 GB available at 95% usage.
// Expected RAM needed: 5 × 0.5 = 2.5 GB > 1.0 GB available → refused.
//
// Current behaviour (FAILING): the service returns nil error and a Success=false response
// caused by downstream errors, not a pre-flight RAM refusal.
// Target behaviour: BulkCreateTerminalsForGroup returns a non-nil error whose message
// contains "RAM" or "capacity" before calling StartComposedSession for any member.
func TestBulkCreateTerminals_InsufficientRAM_Refused(t *testing.T) {
	// --- Fake tt-backend -------------------------------------------------------
	ttServer := startLowRAMTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	// Initialize Casdoor SDK with a dummy config so GetUserByUserId doesn't panic
	// with a nil client. The HTTP call will fail gracefully and the code falls back
	// to using member.UserID as the email — which is fine, we never reach that point
	// once the RAM check is implemented.
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")

	// --- Database & group setup ------------------------------------------------
	db := freshTestDB(t)

	ownerID := "bulk-ram-owner-" + uuid.New().String()
	group := createTestGroup(t, db, ownerID)

	// Add 5 active members — each will need 0.5 GB (size "S").
	// Total estimated RAM: 5 × 0.5 = 2.5 GB; available: 1.0 GB → should be refused.
	for i := 0; i < 5; i++ {
		memberID := "bulk-ram-member-" + uuid.New().String()
		addActiveGroupMember(t, db, group.ID, memberID, groupModels.GroupMemberRoleMember)
	}

	// --- Subscription plan -----------------------------------------------------
	planID := uuid.New()
	err := db.Exec(
		`INSERT INTO subscription_plans (id, created_at, updated_at, name, is_active, max_concurrent_terminals, max_session_duration_minutes, allowed_machine_sizes) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		planID.String(), time.Now(), time.Now(), "BulkRAMTestPlan", true, 100, 60, `["S"]`,
	).Error
	require.NoError(t, err)

	subID := uuid.New()
	err = db.Exec(
		`INSERT INTO user_subscriptions (id, created_at, updated_at, user_id, subscription_plan_id, status) VALUES (?, ?, ?, ?, ?, ?)`,
		subID.String(), time.Now(), time.Now(), ownerID, planID.String(), "active",
	).Error
	require.NoError(t, err)

	plan := &paymentModels.SubscriptionPlan{
		Name:                      "BulkRAMTestPlan",
		IsActive:                  true,
		AllowedMachineSizes:       []string{"S"},
		MaxConcurrentTerminals:    100,
		MaxSessionDurationMinutes: 60,
	}
	plan.ID = planID

	// --- Call the service directly ---------------------------------------------
	svc := services.NewTerminalTrainerService(db)

	request := dto.BulkCreateTerminalsRequest{
		Terms:        "accepted",
		InstanceType: "ubuntu-24.04",
	}

	response, svcErr := svc.BulkCreateTerminalsForGroup(
		group.ID.String(),
		ownerID,
		[]string{"member"},
		request,
		plan,
	)

	// --- Assertions (currently FAILING — no RAM check exists) ------------------
	//
	// After the fix, BulkCreateTerminalsForGroup must return a non-nil error
	// before reaching the provision loop (no StartComposedSession calls).
	//
	// Today it returns: err=nil, response.Success=false (downstream failures),
	// errors mentioning Casdoor/distributions — not RAM.
	require.Error(t, svcErr,
		"BulkCreateTerminalsForGroup must return an error when RAM is insufficient — "+
			"currently passes through without checking RAM (issue #170)")

	errMsg := strings.ToLower(svcErr.Error())
	assert.True(t,
		strings.Contains(errMsg, "ram") ||
			strings.Contains(errMsg, "capacity") ||
			strings.Contains(errMsg, "memory") ||
			strings.Contains(errMsg, "insufficient"),
		"error message should mention RAM/capacity/memory/insufficient, got: %s", svcErr.Error())

	// The response must be nil when an error is returned (no partial creation).
	assert.Nil(t, response,
		"response should be nil when a pre-flight RAM check fails")
}
