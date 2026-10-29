// tests/scenarios/resume_budget_test.go
//
// A paused run's terminal is stopped, and a stopped machine is not charged to
// the plan's CPU/RAM budget. Resuming it charges it again, so POST
// /scenario-sessions/:id/resume must refuse — with the creation path's budget
// 403 — when the learner's other machines leave no room for it.
//
// Witness: the HTTP answer, the requests the fake tt-backend received, and the
// rows left in the DB.
package scenarios_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalModels "soli/formations/src/terminalTrainer/models"
)

// seedPausedRunWithBudgetUse is a paused run whose stopped 2000 mCPU / 2048 MiB
// terminal resumes onto a plan (8000 / 8192) the learner already uses
// otherCPU / otherMem of.
func seedPausedRunWithBudgetUse(t *testing.T, name string, otherCPU, otherMem int) resumeFixture {
	t.Helper()
	f := seedResumableRun(t, name, resumeSeed{terminalState: terminalModels.StateStopped})
	require.NoError(t, f.db.Model(&terminalModels.Terminal{}).
		Where("session_id = ?", f.oldTerminal).
		Updates(map[string]any{"size_cpu": 2000, "size_memory_mb": 2048}).Error)
	require.NoError(t, f.db.Create(&terminalModels.Terminal{
		SessionID:    "other-machine-" + uuid.New().String(),
		UserID:       f.learnerID,
		State:        terminalModels.StateRunning,
		ExpiresAt:    time.Now().Add(time.Hour),
		SizeCPU:      otherCPU,
		SizeMemoryMB: otherMem,
	}).Error)
	return f
}

func TestResume_PausedBudgetFull_403BudgetAndStaysPaused(t *testing.T) {
	f := seedPausedRunWithBudgetUse(t, "resume-paused-budget-full", 7000, 4096)
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)

	require.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "budget", body["source"])
	assert.Equal(t, "budget_exhausted", body["reason"])
	remaining, ok := body["remaining"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1000), remaining["cpu"])

	assert.Empty(t, tt.startedSessions(), "nothing is started on tt-backend")
	assert.Zero(t, tt.createCalls(), "and nothing is rebuilt")
	var terminal terminalModels.Terminal
	require.NoError(t, f.db.First(&terminal, "session_id = ?", f.oldTerminal).Error)
	assert.Equal(t, terminalModels.StateStopped, terminal.State, "the terminal stays paused")
	assert.Equal(t, "active", sessionStatus(t, f.db, f.run.ID), "the run stays open")
}

func TestResume_PausedBudgetFits_StartsTerminalInPlace(t *testing.T) {
	f := seedPausedRunWithBudgetUse(t, "resume-paused-budget-fits", 6000, 4096)
	tt := newPreviewTTBackend(t)

	resp := decodeResume(t, resumeRun(t, f))

	assert.Equal(t, f.oldTerminal, resp.TerminalSessionID)
	assert.Equal(t, []string{f.oldTerminal}, tt.startedSessions())
	var terminal terminalModels.Terminal
	require.NoError(t, f.db.First(&terminal, "session_id = ?", f.oldTerminal).Error)
	assert.Equal(t, terminalModels.StateRunning, terminal.State)
}
