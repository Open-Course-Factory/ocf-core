package cron

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The scenario sweep now syncs terminals over HTTP before it sweeps, and
// startJob runs the first pass synchronously — at boot, before the server
// listens. A slow or hung tt-backend then held the whole API down. The job
// must be started without waiting on its first sweep.
func TestStartScenarioSessionCleanupJob_DoesNotBlockOnASlowSync(t *testing.T) {
	// A file, not :memory: — the sweep runs on its own goroutine and a second
	// pooled connection to :memory: would open an empty database.
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cron.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.UserTerminalKey{}, &models.Terminal{},
		&scenarioModels.Scenario{}, &scenarioModels.ScenarioSession{}))

	// A learner whose terminal auto-stopped at its TTL: the sweep syncs them.
	userID := "boot-sweep-owner"
	require.NoError(t, db.Create(&models.UserTerminalKey{
		UserID: userID, APIKey: "key-" + userID, KeyName: userID, IsActive: true,
	}).Error)
	terminalID := "boot-sweep-terminal"
	require.NoError(t, db.Create(&models.Terminal{
		SessionID: terminalID, UserID: userID, State: models.StateRunning,
		PersistenceMode: models.PersistenceModePersistent, ExpiresAt: time.Now().Add(-30 * time.Minute),
	}).Error)
	// Crash traps: the sweep only syncs owners of runs it could abandon; a
	// normal run whose container is gone is rebuilt, not abandoned.
	scenario := scenarioModels.Scenario{Name: "boot-sweep", Title: "boot-sweep", CreatedByID: "creator", CrashTraps: true}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&scenarioModels.ScenarioSession{
		ScenarioID: scenario.ID, UserID: userID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}).Error)

	// tt-backend hangs on the listing until the test ends.
	listed := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case listed <- struct{}{}:
		default:
		}
		<-release
		http.Error(w, "too late", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first: unblocks the handler so srv.Close can return
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	started := make(chan struct{})
	go func() {
		StartScenarioSessionCleanupJob(db)
		close(started)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("StartScenarioSessionCleanupJob waited on its first sweep's tt-backend sync; startup must not")
	}

	// The first sweep still runs — in the background.
	select {
	case <-listed:
	case <-time.After(5 * time.Second):
		t.Fatal("the first sweep never reached tt-backend; the job must still run it at startup")
	}
}

// A rebuild replays in a goroutine, so a process restart mid-replay leaves the
// run in provisioning/replay on its new, half-built terminal. Left to the
// stuck-provisioning reaper it would become setup_failed, and the next launch
// would abandon it — losing the progress the rebuild existed to keep. One
// sweep must instead hand the run back to its old terminal id (so it reads as
// rebuildable again) and delete the half-built terminal, while a stalled
// ordinary launch, which has no progress to keep, is still reaped.
func TestSweepScenarioSessions_ReleasesStalledReplaysAndDeletesTheirTerminals(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cron.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.UserTerminalKey{}, &models.Terminal{},
		&scenarioModels.Scenario{}, &scenarioModels.ScenarioSession{}))

	var mu sync.Mutex
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			deleted = append(deleted, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	scenario := scenarioModels.Scenario{Name: "stalled-replay", Title: "stalled-replay", CreatedByID: "creator"}
	require.NoError(t, db.Create(&scenario).Error)
	stale := time.Now().Add(-15 * time.Minute)

	// The replaying run: its old terminal is gone (that is why it was being
	// rebuilt) and it points at the new one the replay was building.
	learner := "replay-owner"
	key := models.UserTerminalKey{UserID: learner, APIKey: "key-" + learner, KeyName: learner, IsActive: true}
	require.NoError(t, db.Create(&key).Error)
	oldTerminal, newTerminal := "replay-old-terminal", "replay-new-terminal"
	require.NoError(t, db.Create(&models.Terminal{
		SessionID: oldTerminal, UserID: learner, State: models.StateDeleted,
		ExpiresAt: time.Now().Add(-time.Hour), UserTerminalKeyID: key.ID,
	}).Error)
	require.NoError(t, db.Create(&models.Terminal{
		SessionID: newTerminal, UserID: learner, State: models.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour), UserTerminalKeyID: key.ID,
	}).Error)
	replay := scenarioModels.ScenarioSession{
		ScenarioID: scenario.ID, UserID: learner, Status: "provisioning", ProvisioningPhase: "replay",
		CurrentStep: 3, StartedAt: stale.Add(-time.Hour),
		TerminalSessionID: &newTerminal, RebuildFromTerminalID: &oldTerminal,
	}
	require.NoError(t, db.Create(&replay).Error)

	// A stalled ordinary launch: nothing to keep, the reaper's to write off.
	launchTerminal := "launch-terminal"
	launch := scenarioModels.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "launch-owner", Status: "provisioning", ProvisioningPhase: "step_setup",
		StartedAt: stale, TerminalSessionID: &launchTerminal,
	}
	require.NoError(t, db.Create(&launch).Error)

	for _, id := range []any{replay.ID, launch.ID} {
		require.NoError(t, db.Model(&scenarioModels.ScenarioSession{}).Where("id = ?", id).
			Update("updated_at", stale).Error)
	}

	sweepScenarioSessions(db, terminalServices.NewTerminalTrainerService(db))

	var released scenarioModels.ScenarioSession
	require.NoError(t, db.First(&released, "id = ?", replay.ID).Error)
	require.Equal(t, "active", released.Status, "a stalled replay goes back to an open run, not setup_failed")
	require.Equal(t, "", released.ProvisioningPhase)
	require.NotNil(t, released.TerminalSessionID)
	require.Equal(t, oldTerminal, *released.TerminalSessionID,
		"the run must point at its old (gone) terminal again, so it reads as rebuildable — never as a live resume into the half-built one")
	require.Nil(t, released.RebuildFromTerminalID, "the replay is over; nothing is being rebuilt from anymore")
	require.Equal(t, 3, released.CurrentStep, "releasing a replay keeps the learner's progress")

	mu.Lock()
	require.Equal(t, []string{newTerminal}, deleted, "tt-backend must be asked to delete the half-built terminal, and only it")
	mu.Unlock()
	var halfBuilt models.Terminal
	require.NoError(t, db.First(&halfBuilt, "session_id = ?", newTerminal).Error)
	require.Equal(t, models.StateDeleted, halfBuilt.State)

	var reaped scenarioModels.ScenarioSession
	require.NoError(t, db.First(&reaped, "id = ?", launch.ID).Error)
	require.Equal(t, "setup_failed", reaped.Status, "a stalled launch is still the reaper's")
}
