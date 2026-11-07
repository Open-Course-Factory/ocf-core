package terminalTrainer_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// staleKeyTTBackend refuses the first `refusals` session requests the way
// tt-backend refuses a key whose budget is too small. The rest of its surface,
// the catalogue and the key admin, works.
type staleKeyTTBackend struct {
	refusals   int32
	sessions   atomic.Int32
	keyUpdates atomic.Int32
	refusal    string
}

func (b *staleKeyTTBackend) start(t *testing.T) {
	t.Helper()
	notFailing := false
	catalogue := failingComposeTTBackend(t, &notFailing)
	t.Cleanup(catalogue.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sessions"):
			if b.sessions.Add(1) <= b.refusals {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(b.refusal))
				return
			}
			catalogue.Config.Handler.ServeHTTP(w, r)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/admin/api-keys/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 42, "name": "learner", "is_active": true}})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/admin/api-keys/"):
			b.keyUpdates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			catalogue.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("TERMINAL_TRAINER_URL", server.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
}

const cpuBudgetRefusal = `{"error":"budget_exhausted","axis":"cpu","limit":1,"current":0,"requested":4}`

func startLargeSessionWithKey(t *testing.T, ttKeyID int64) (*dto.TerminalSessionResponse, string, error) {
	t.Helper()
	freshTestDB(t)
	userID := "stale-key-" + uuid.NewString()[:8]
	planID := seedBudgetPlanForUser(t, userID, lSizeCPU, lSizeMemMB)
	key, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)
	require.NoError(t, sharedTestDB.Model(key).Update("terminal_trainer_key_id", ttKeyID).Error)
	plan := &paymentModels.SubscriptionPlan{}
	require.NoError(t, sharedTestDB.First(plan, "id = ?", planID).Error)

	response, err := services.NewTerminalTrainerService(sharedTestDB).StartComposedSession(userID,
		dto.CreateComposedSessionInput{Distribution: "ubuntu-24.04", Size: "L", Terms: "accepted"}, plan)
	return response, userID, err
}

func occupiedSlots(t *testing.T, userID string) int64 {
	t.Helper()
	var occupied int64
	require.NoError(t, sharedTestDB.Model(&terminalModels.Terminal{}).Scopes(terminalModels.OccupiesSlotScope).
		Where("terminals.user_id = ?", userID).Count(&occupied).Error)
	return occupied
}

func TestComposedSession_KeyBudgetBehindThePlan_IsRefreshedAndTheLaunchSucceeds(t *testing.T) {
	backend := &staleKeyTTBackend{refusals: 1, refusal: cpuBudgetRefusal}
	backend.start(t)

	response, _, err := startLargeSessionWithKey(t, 42)

	require.NoError(t, err)
	assert.NotEmpty(t, response.SessionID)
	assert.EqualValues(t, 1, backend.keyUpdates.Load(), "the key gets the plan's budget")
	assert.EqualValues(t, 2, backend.sessions.Load(), "the launch is tried once more")
}

func TestComposedSession_StillRefusedAfterTheRefresh_IsABudgetRefusal(t *testing.T) {
	backend := &staleKeyTTBackend{refusals: 2, refusal: cpuBudgetRefusal}
	backend.start(t)

	_, userID, err := startLargeSessionWithKey(t, 42)

	var rejection *services.BudgetRejection
	require.True(t, errors.As(err, &rejection), "got %v", err)
	assert.Equal(t, "budget_cpu_exceeded", rejection.Reason)
	assert.EqualValues(t, 2, backend.sessions.Load(), "one retry, no more")
	assert.Zero(t, occupiedSlots(t, userID), "the reservation is released")
}

func TestComposedSession_KeyThatCannotBeRefreshed_IsABudgetRefusal(t *testing.T) {
	backend := &staleKeyTTBackend{refusals: 2, refusal: cpuBudgetRefusal}
	backend.start(t)

	_, _, err := startLargeSessionWithKey(t, 0) // a legacy key, with no tt-backend id

	var rejection *services.BudgetRejection
	require.True(t, errors.As(err, &rejection), "got %v", err)
	assert.EqualValues(t, 1, backend.sessions.Load(), "nothing changed, so no retry")
}

func TestComposedSession_RateLimited_IsNotABudgetRefusal(t *testing.T) {
	backend := &staleKeyTTBackend{refusals: 1, refusal: `{"error":"rate_limited"}`}
	backend.start(t)

	_, _, err := startLargeSessionWithKey(t, 42)

	require.Error(t, err)
	var rejection *services.BudgetRejection
	assert.False(t, errors.As(err, &rejection))
	assert.EqualValues(t, 1, backend.sessions.Load())
	assert.Zero(t, backend.keyUpdates.Load())
}

