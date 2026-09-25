package cron

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/terminalTrainer/models"

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
	scenario := scenarioModels.Scenario{Name: "boot-sweep", Title: "boot-sweep", CreatedByID: "creator"}
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
