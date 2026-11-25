package terminalTrainer_tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A local session's machine runs in the learner's browser: the platform keeps
// the row (budget, expiry, state) exactly as for a container, and never asks
// tt-backend anything about it.
func localSessionFixture(t *testing.T) (services.TerminalTrainerService, *recorder, string, *paymentModels.SubscriptionPlan) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[],"count":0}`))
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "local-learner-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	plan := &paymentModels.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, Name: "local",
		MaxCPU: 4000, MaxMemoryMB: 4096, MaxSessionDurationMinutes: 90, CommandHistoryRetentionDays: 7,
	}
	return services.NewTerminalTrainerService(db), rec, userID, plan
}

// sessionCalls is what tt-backend was asked, less the enum catalog the
// service reads when it is built, whatever the session.
func sessionCalls(rec *recorder) []string {
	var calls []string
	for _, c := range rec.all() {
		if !strings.HasSuffix(c, "/enums") {
			calls = append(calls, c)
		}
	}
	return calls
}

func TestStartLocalSession_KeepsTheRowAndAsksTTBackendNothing(t *testing.T) {
	svc, rec, userID, plan := localSessionFixture(t)

	started, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S", Name: "lab"}, plan)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(started.SessionID, models.LocalSessionIDPrefix))
	assert.True(t, models.IsLocalSessionID(started.SessionID))
	assert.WithinDuration(t, time.Now().Add(90*time.Minute), started.ExpiresAt, time.Minute, "the plan's duration, as for a container")

	row, err := svc.GetSessionInfo(started.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateRunning, row.State)
	assert.Equal(t, models.LocalBackend, row.Backend)
	assert.Equal(t, services.PersistenceModeEphemeral, row.PersistenceMode, "no files kept after the session")
	assert.Equal(t, &plan.ID, row.SubscriptionPlanID)
	assert.Empty(t, sessionCalls(rec), "tt-backend is never asked to build a local machine")
}

func TestLocalSession_StopDeleteAndBuildCompleteAskTTBackendNothing(t *testing.T) {
	svc, rec, userID, plan := localSessionFixture(t)

	stopped, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)
	require.NoError(t, svc.BuildComplete(stopped.SessionID))
	require.NoError(t, svc.StopSession(stopped.SessionID))
	row, err := svc.GetSessionInfo(stopped.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateDeleted, row.State, "a local machine has no pause: stopping ends it")

	deleted, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteSession(deleted.SessionID))
	row, err = svc.GetSessionInfo(deleted.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateDeleted, row.State)

	assert.Empty(t, sessionCalls(rec))
}

func TestLocalSession_CannotBeStartedAgain(t *testing.T) {
	svc, rec, userID, plan := localSessionFixture(t)
	started, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)
	require.NoError(t, svc.StopSession(started.SessionID))

	err = svc.StartSession(started.SessionID, plan)
	require.Error(t, err)
	assert.True(t, errors.Is(err, services.ErrContainerGone))
	assert.Empty(t, sessionCalls(rec))
}

// tt-backend lists only its containers: a local row missing from that list is
// not a container that vanished, and sync must leave it alone.
func TestSyncUserSessions_LeavesLocalRowsAlone(t *testing.T) {
	svc, _, userID, plan := localSessionFixture(t)
	started, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)

	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	row, err := svc.GetSessionInfo(started.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateRunning, row.State)
}

// Opening, stopping or deleting one's own local session goes through the same
// access gate as a container's. tt-backend has no host for it and no record of
// it, so the gate must not ask: a "backend offline" answer turned the
// learner's Stop into a 503, and an API check would mark the live row deleted.
func TestValidateSessionAccess_LocalSessionAsksTTBackendNothing(t *testing.T) {
	svc, rec, userID, plan := localSessionFixture(t)
	started, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)

	for _, checkAPI := range []bool{false, true} {
		ok, state, err := svc.ValidateSessionAccess(started.SessionID, checkAPI)
		require.NoError(t, err)
		assert.True(t, ok, "checkAPI=%v: state %q", checkAPI, state)
	}
	row, err := svc.GetSessionInfo(started.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateRunning, row.State)
	assert.Empty(t, sessionCalls(rec))
}
