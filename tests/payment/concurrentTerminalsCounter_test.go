// tests/payment/concurrentTerminalsCounter_test.go
//
// Failing-first tests for the MaxConcurrentTerminals bypass via stop/start
// cycling.
//
// Bug: the `concurrent_terminals` usage counter at
// `src/payment/services/effectivePlanService.go:164` queries
//
//     WHERE user_id = ? AND status = 'active' AND deleted_at IS NULL
//
// but `StopSession` sets `terminal.Status = "stopped"`
// (see `src/terminalTrainer/services/terminalTrainerService.go:280`),
// so stopped sessions disappear from the count.
//
// The design contract is the opposite — the same file at line 257 documents:
//
//     // Le compteur concurrent_terminals N'est PAS décrémenté ici — une
//     // session arrêtée occupe toujours un slot. La décrémentation se fait
//     // dans DeleteSession.
//
// Effect: a user on a plan with MaxConcurrentTerminals=1 can launch terminal
// A, stop it, then launch terminal B (the counter is back to 0), bypassing
// the cap. POST /terminals/:id/start on A then resurrects the slot to 2.
//
// These tests assert the CORRECT behavior. They will fail until the counter
// is fixed to include stopped sessions (e.g. `state != 'terminated' AND
// state != 'deleted'`, or `status IN ('active','stopped')`).
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestConcurrentTerminalsCounter_StoppedSessionStillOccupiesSlot verifies the
// design contract: a stopped session keeps its quota slot until it is
// permanently deleted. This is the root cause of the bypass — the counter
// currently filters on status='active' and silently drops stopped rows.
func TestConcurrentTerminalsCounter_StoppedSessionStillOccupiesSlot(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-stopped-occupies-slot"

	// Plan with MaxConcurrentTerminals=1 — the smallest meaningful cap.
	plan := createPlan(t, db, "Solo", 5, 1)
	createUserSubscription(t, db, userID, plan)

	// One stopped session — matches the lifecycle StopSession produces:
	// terminal.Status = "stopped", terminal.State = "stopped".
	db.Exec("INSERT INTO terminals (id, user_id, status, state) VALUES (?, ?, ?, ?)",
		uuid.New().String(), userID, "stopped", "stopped")

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, int64(1), check.Limit)

	// The fix must make the counter include the stopped row.
	assert.Equal(t, int64(1), check.CurrentUsage,
		"stopped session must still occupy the concurrent_terminals slot "+
			"(see StopSession comment in terminalTrainerService.go:257-258)")

	// And the next launch attempt must be rejected — this is the
	// user-visible behavior that protects the cap.
	assert.False(t, check.Allowed,
		"launching another terminal while a stopped one occupies the slot "+
			"must be denied — otherwise the user bypasses MaxConcurrentTerminals "+
			"by cycling stop/launch")
	assert.Equal(t, int64(0), check.RemainingUsage)
	assert.Contains(t, check.Message, "Usage limit exceeded")
}

// TestConcurrentTerminalsCounter_StopStartCycleBypass simulates the full
// bypass sequence at the counter level:
//
//  1. user has 1 running terminal → at cap (1/1)
//  2. user stops it → counter MUST still report 1/1 (it currently reports 0/1)
//  3. launching a second terminal MUST be denied
//
// This is the integration-level proof that the bug allows MaxConcurrentTerminals
// to be circumvented entirely.
func TestConcurrentTerminalsCounter_StopStartCycleBypass(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-stop-start-bypass"

	plan := createPlan(t, db, "Solo", 5, 1)
	createUserSubscription(t, db, userID, plan)

	svc := services.NewEffectivePlanService(db)

	// Step 1 — one running terminal, user is at cap.
	runningID := uuid.New().String()
	db.Exec("INSERT INTO terminals (id, user_id, status, state) VALUES (?, ?, ?, ?)",
		runningID, userID, "active", "running")

	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)
	assert.NoError(t, err)
	assert.False(t, check.Allowed, "user with 1 running terminal on cap=1 must be denied")

	// Step 2 — stop the terminal (matches what StopSession does in production).
	db.Exec("UPDATE terminals SET status = ?, state = ? WHERE id = ?",
		"stopped", "stopped", runningID)

	check, err = svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)
	assert.NoError(t, err)

	// The bug: after stopping, the counter drops to 0/1 and the user is
	// allowed to spawn a second terminal. The fix must keep the counter
	// at 1/1 — stopped sessions still occupy the slot.
	assert.Equal(t, int64(1), check.CurrentUsage,
		"stopped session must continue to count after the stop transition")
	assert.False(t, check.Allowed,
		"stop/start cycle bypass: launching a second terminal after stopping "+
			"the first must remain denied — the slot is not freed until DELETE")
}
