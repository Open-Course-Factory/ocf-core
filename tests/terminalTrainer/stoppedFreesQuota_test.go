// tests/terminalTrainer/stoppedFreesQuota_test.go
//
// Pins the budget rule: a terminal consumes the plan's CPU/RAM budget only
// while it is running or starting. A stopped machine has released its CPU and
// RAM (an Incus stop keeps only the disk), so it is not charged — and so a
// resume has to pass the budget gate again, with the same refusal a creation
// gets.
//
// Witness: the budget sums the quota service reads, the my-usage response, the
// rows left in the DB after a resume, and the HTTP answer of the resume route.
package terminalTrainer_tests

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

func TestBudgetUsage_StoppedTerminal_ExcludedRunningAndStartingIncluded(t *testing.T) {
	freshTestDB(t)
	userID := "budget-states-user"

	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 1000, 1024)
	insertExistingTerminal(t, sharedTestDB, userID, nil, "starting", "ephemeral", 2000, 2048)
	insertExistingTerminal(t, sharedTestDB, userID, nil, "stopped", "persistent", 4000, 4096)

	qs := paymentServices.NewQuotaService(sharedTestDB, paymentServices.NewEffectivePlanService(sharedTestDB))
	usedCPU, usedMem, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)

	assert.Equal(t, 1000+2000, usedCPU, "running and starting consume CPU; a stopped machine does not")
	assert.Equal(t, 1024+2048, usedMem, "running and starting consume RAM; a stopped machine does not")
}

// CountsTowardQuota is OccupiesSlotScope asked of a row in memory; this runs
// both over the same rows and fails if they ever disagree.
func TestCountsTowardQuota_MatchesOccupiesSlotScope(t *testing.T) {
	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, "user-counts")
	require.NoError(t, err)

	cases := []struct {
		name    string
		state   models.TerminalState
		expires time.Duration
		want    bool
	}{
		{"running and unexpired", models.StateRunning, time.Hour, true},
		{"running but past expiry", models.StateRunning, -time.Hour, false},
		{"starting and unexpired", models.StateStarting, time.Hour, true},
		{"stopped and unexpired", models.StateStopped, time.Hour, false},
		{"deleted and unexpired", models.StateDeleted, time.Hour, false},
		{"revoked and unexpired", models.StateRevoked, time.Hour, false},
	}
	wantBySessionID := map[string]bool{}
	for _, c := range cases {
		terminal := &models.Terminal{
			SessionID:         "counts-" + c.name,
			UserID:            "user-counts",
			State:             c.state,
			ExpiresAt:         time.Now().Add(c.expires),
			InstanceType:      "ubuntu:22.04",
			UserTerminalKeyID: userKey.ID,
		}
		require.NoError(t, db.Create(terminal).Error)
		wantBySessionID[terminal.SessionID] = c.want
	}

	var counted []models.Terminal
	require.NoError(t, db.Table("terminals").Scopes(models.OccupiesSlotScope).
		Where("terminals.user_id = ?", "user-counts").Find(&counted).Error)
	countedBySessionID := map[string]bool{}
	for i := range counted {
		countedBySessionID[counted[i].SessionID] = true
	}

	var all []models.Terminal
	require.NoError(t, db.Where("user_id = ?", "user-counts").Find(&all).Error)
	require.Len(t, all, len(cases))
	for i := range all {
		row := &all[i]
		assert.Equal(t, wantBySessionID[row.SessionID], row.CountsTowardQuota(), "CountsTowardQuota for %s", row.SessionID)
		assert.Equal(t, countedBySessionID[row.SessionID], row.CountsTowardQuota(),
			"CountsTowardQuota and OccupiesSlotScope disagree about %s", row.SessionID)
	}
}

func TestMyTerminalUsage_StoppedSession_ListedButNotCounted(t *testing.T) {
	freshTestDB(t)
	userID := "usage-stopped-listed"
	seedPersonalPlanFor(t, userID, 8000, 8192, 60, "Pro")

	insertExistingTerminal(t, sharedTestDB, userID, nil, "running", "ephemeral", 2000, 1024)
	insertExistingTerminal(t, sharedTestDB, userID, nil, "stopped", "persistent", 4000, 2048)

	router := mountMyUsageRouter(t, sharedTestDB, userID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/terminals/my-usage", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2000), resp["used_cpu"], "only the running session is charged")
	assert.Equal(t, float64(1024), resp["used_memory_mb"])

	sessions, ok := resp["active_sessions"].([]any)
	require.True(t, ok)
	require.Len(t, sessions, 2, "a stopped session is still the user's: it stays listed")
	countsByState := map[string]any{}
	for _, s := range sessions {
		entry := s.(map[string]any)
		countsByState[entry["state"].(string)] = entry["counts_toward_quota"]
	}
	assert.Equal(t, true, countsByState["running"])
	assert.Equal(t, false, countsByState["stopped"])
}

// resumeBudgetFixture is a user on a 4000 mCPU / 4096 MiB plan with a stopped
// 2000 mCPU / 2048 MiB terminal to resume, and a running terminal using
// runningCPU / runningMem of the budget (session resumeOccupantID(userID)).
func resumeBudgetFixture(t *testing.T, userID string, runningCPU, runningMem int) (*paymentModels.SubscriptionPlan, string) {
	t.Helper()
	db := freshTestDB(t)
	plan := seedPersonalPlanFor(t, userID, 4000, 4096, 60, "Resume")
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	if runningCPU > 0 {
		require.NoError(t, db.Create(&models.Terminal{
			SessionID:         resumeOccupantID(userID),
			UserID:            userID,
			State:             models.StateRunning,
			PersistenceMode:   "ephemeral",
			ExpiresAt:         time.Now().Add(time.Hour),
			SizeCPU:           runningCPU,
			SizeMemoryMB:      runningMem,
			UserTerminalKeyID: userKey.ID,
		}).Error)
	}
	sessionID := "resume-budget-" + userID
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateStopped,
		PersistenceMode:   models.PersistenceModePersistent,
		ExpiresAt:         time.Now().Add(time.Hour),
		MachineSize:       "M",
		SizeCPU:           2000,
		SizeMemoryMB:      2048,
		UserTerminalKeyID: userKey.ID,
	}).Error)
	return plan, sessionID
}

func resumeOccupantID(userID string) string { return "resume-occupant-" + userID }

func TestStartSession_ResumeBudgetFull_RefusedAndStaysStopped(t *testing.T) {
	srv, rec := startSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)
	plan, sessionID := resumeBudgetFixture(t, "resume-full-user", 3000, 3072)

	err := services.NewTerminalTrainerService(sharedTestDB).StartSession(sessionID, plan)

	var rejection *services.BudgetRejection
	require.True(t, stderrors.As(err, &rejection), "a resume that does not fit is a budget rejection; got %v", err)
	assert.Equal(t, "budget_cpu_exceeded", rejection.Reason)
	assert.Equal(t, 1000, rejection.RemainingCPU)
	assert.Zero(t, rec.calls, "nothing is started on tt-backend")

	var row models.Terminal
	require.NoError(t, sharedTestDB.First(&row, "session_id = ?", sessionID).Error)
	assert.Equal(t, models.StateStopped, row.State, "the refused terminal stays paused")
}

func TestStartSession_ResumeBudgetFits_RunsAndCounts(t *testing.T) {
	srv, rec := startSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)
	userID := "resume-fits-user"
	plan, sessionID := resumeBudgetFixture(t, userID, 2000, 2048)

	require.NoError(t, services.NewTerminalTrainerService(sharedTestDB).StartSession(sessionID, plan))
	assert.Equal(t, 1, rec.calls)

	var row models.Terminal
	require.NoError(t, sharedTestDB.First(&row, "session_id = ?", sessionID).Error)
	assert.Equal(t, models.StateRunning, row.State)

	qs := paymentServices.NewQuotaService(sharedTestDB, paymentServices.NewEffectivePlanService(sharedTestDB))
	usedCPU, _, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	assert.Equal(t, 4000, usedCPU, "the resumed terminal is charged again")
}

// A resume that fails upstream for a reason other than a lost container gives
// the budget back: the terminal is paused again, not left charged.
func TestStartSession_ResumeUpstreamFailure_ReleasesBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)
	userID := "resume-upstream-fail"
	plan, sessionID := resumeBudgetFixture(t, userID, 0, 0)

	require.Error(t, services.NewTerminalTrainerService(sharedTestDB).StartSession(sessionID, plan))

	var row models.Terminal
	require.NoError(t, sharedTestDB.First(&row, "session_id = ?", sessionID).Error)
	assert.Equal(t, models.StateStopped, row.State)
	assert.WithinDuration(t, time.Now().Add(time.Hour), row.ExpiresAt, time.Minute, "its reap deadline is restored")

	qs := paymentServices.NewQuotaService(sharedTestDB, paymentServices.NewEffectivePlanService(sharedTestDB))
	usedCPU, _, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	assert.Zero(t, usedCPU)
}

func TestStartSessionRoute_ResumeBudgetFull_Returns403BudgetShape(t *testing.T) {
	userID := "resume-route-full"
	_, sessionID := resumeBudgetFixture(t, userID, 3000, 3072)
	// tt-backend knows both machines, so the access middleware's sync keeps
	// the running one charged.
	ttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{"ram_available_gb": 8.0, "ram_percent": 10.0})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			expires := time.Now().Add(time.Hour).Unix()
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []map[string]any{
				{"id": sessionID, "session_id": sessionID, "status": 1, "state": "stopped", "persistence_mode": "persistent", "expires_at": expires},
				{"id": resumeOccupantID(userID), "session_id": resumeOccupantID(userID), "status": 1, "state": "running", "persistence_mode": "ephemeral", "expires_at": expires},
			}})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(ttSrv.Close)
	configureTTServer(t, ttSrv.URL)

	router := setupResumeRouterWithProdMiddleware(t, userID, services.NewTerminalTrainerService(sharedTestDB))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/terminals/"+sessionID+"/start", nil))

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "budget", body["source"])
	assert.Equal(t, "budget_exhausted", body["reason"])
	remaining, ok := body["remaining"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1000), remaining["cpu"])

	var row models.Terminal
	require.NoError(t, sharedTestDB.First(&row, "session_id = ?", sessionID).Error)
	assert.Equal(t, models.StateStopped, row.State)
}

// roomyResumePlan is the plan a resume caller resolved, with budget to spare,
// for tests about what a resume does rather than whether it fits.
func roomyResumePlan() *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{Name: "roomy", MaxCPU: 64000, MaxMemoryMB: 65536, IsActive: true}
}
