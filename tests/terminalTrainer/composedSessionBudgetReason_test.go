// Coarse block_reason for the composed-session budget gate.
//
// Per the locked UX decision (feedback_quota_ux_size_count.md), the
// customer-facing API surface must not leak the CPU vs RAM axis. The
// scenario controller already collapses BudgetRejection.Reason into the
// single "budget_exhausted" code; this file pins the same behaviour on
// the terminal controller's StartComposedSession path.
//
// QuotaService keeps the granular reasons internally so server logs
// can still distinguish CPU- vs RAM-axis exhaustion. The HTTP body is
// what these tests pin.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"soli/formations/src/terminalTrainer/services"
)

// decodeBudgetResponse extracts the parts of the HTTP body that this
// test cares about. Using a named struct so a future change to extra
// fields doesn't accidentally break the assertion.
type budgetHTTPBody struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Source       string `json:"source"`
	Reason       string `json:"reason"`
	Remaining    struct {
		CPU      int `json:"cpu"`
		MemoryMB int `json:"memory_mb"`
	} `json:"remaining"`
}

// TestStartComposedSession_BudgetCPURejection_EmitsCoarseReason — when
// QuotaService rejects on the CPU axis (granular reason
// "budget_cpu_exceeded"), the HTTP body must collapse it to the coarse
// "budget_exhausted" so the frontend renders size-count UX rather than
// axis-specific copy.
func TestStartComposedSession_BudgetCPURejection_EmitsCoarseReason(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	// Granular internal reason from the service.
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, &services.BudgetRejection{
			Reason:            "budget_cpu_exceeded",
			RemainingCPU:      0,
			RemainingMemoryMB: 256,
		})

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var body budgetHTTPBody
	require := assert.New(t)
	require.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal("budget", body.Source)
	require.Equal("budget_exhausted", body.Reason,
		"CPU-axis exhaustion must surface as the coarse code — customers should not see CPU vs RAM")
	require.Equal(0, body.Remaining.CPU)
	require.Equal(256, body.Remaining.MemoryMB)

	svc.AssertExpectations(t)
}

// TestStartComposedSession_BudgetMemoryRejection_EmitsCoarseReason — same
// expectation as above but the RAM axis. Both granular reasons must
// flatten to "budget_exhausted".
func TestStartComposedSession_BudgetMemoryRejection_EmitsCoarseReason(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, &services.BudgetRejection{
			Reason:            "budget_memory_exceeded",
			RemainingCPU:      2,
			RemainingMemoryMB: 0,
		})

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var body budgetHTTPBody
	require := assert.New(t)
	require.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal("budget_exhausted", body.Reason,
		"RAM-axis exhaustion must surface as the coarse code")

	svc.AssertExpectations(t)
}

// TestStartComposedSession_PlanRestriction_PreservesReason — the
// "plan_restriction" reason is NOT a budget-axis reason; it must NOT be
// collapsed into "budget_exhausted". It surfaces a different message
// to the user (size not in plan vs budget exhausted).
func TestStartComposedSession_PlanRestriction_PreservesReason(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, &services.BudgetRejection{
			Reason:            "plan_restriction",
			RemainingCPU:      4,
			RemainingMemoryMB: 4096,
		})

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var body budgetHTTPBody
	require := assert.New(t)
	require.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal("plan_restriction", body.Reason,
		"plan_restriction is not a budget-axis reason and must NOT be flattened")

	svc.AssertExpectations(t)
}
