package terminalTrainer_tests

import (
	"encoding/json"
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
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A local session's machine runs in the learner's browser: the platform keeps
// the row (budget, expiry, state) exactly as for a container, and never asks
// tt-backend anything about it.
func localSessionFixture(t *testing.T) (services.TerminalTrainerService, *recorder, string, *paymentModels.SubscriptionPlan) {
	t.Helper()
	svc, rec, userID, plan, _ := localSessionFixtureWithDB(t)
	return svc, rec, userID, plan
}

func localSessionFixtureWithDB(t *testing.T) (services.TerminalTrainerService, *recorder, string, *paymentModels.SubscriptionPlan, *gorm.DB) {
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
	return services.NewTerminalTrainerService(db), rec, userID, plan, db
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
	assert.Empty(t, started.ConsoleURL, "no console on a server: the tab opens its own")
}

// A local session's status is the platform's row: tt-backend does not know it,
// and that is no reason to sync it.
func TestGetSessionStatus_LocalSessionIsItsOwnRow(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method, r.URL.Path)
		_, _ = w.Write([]byte(`{"sessions":[],"count":0}`))
	}))
	t.Cleanup(srv.Close)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "local-learner-" + uuid.NewString()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	plan := &paymentModels.SubscriptionPlan{BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, Name: "local",
		MaxCPU: 4000, MaxMemoryMB: 4096, MaxSessionDurationMinutes: 90}
	started, err := services.NewTerminalTrainerService(db).StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", userID); c.Next() })
	router.GET("/terminals/:id/status", terminalController.NewTerminalController(db).GetSessionStatus)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/terminals/"+started.SessionID+"/status", nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var status map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, "running", status["status"])
	assert.Equal(t, false, status["sync_recommended"])
	assert.Empty(t, sessionCalls(rec), "tt-backend is not asked about a local session")
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

// A machine in the browser costs the platform no CPU or RAM (plan rule 8,
// 2026-11-28): it starts on a plan whose budget could not hold a container of
// its size, and charges nothing to the budget containers draw on.
func TestStartLocalSession_ReservesNoBudget(t *testing.T) {
	svc, _, userID, plan, db := localSessionFixtureWithDB(t)
	plan.MaxCPU, plan.MaxMemoryMB = 100, 100 // no catalog size fits

	started, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "XL"}, plan)
	require.NoError(t, err)

	row, err := svc.GetSessionInfo(started.SessionID)
	require.NoError(t, err)
	assert.False(t, row.CountsTowardQuota(), "not charged to the budget")
	occupied, err := models.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Zero(t, occupied, "the budget sum does not count it")
}

// One machine in the browser at a time per learner: a second one is refused
// while the first is live, and allowed once it has ended.
func TestStartLocalSession_OneLiveLocalSessionPerLearner(t *testing.T) {
	svc, _, userID, plan := localSessionFixture(t)
	first, err := svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)

	_, err = svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.ErrorIs(t, err, services.ErrLocalMachineBusy)

	require.NoError(t, svc.DeleteSession(first.SessionID))
	_, err = svc.StartLocalSession(userID, dto.CreateComposedSessionInput{Size: "S"}, plan)
	require.NoError(t, err)
}
