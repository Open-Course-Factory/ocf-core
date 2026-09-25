package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// A learner who already has a live run of a scenario must be told to resume it,
// not handed a server error. These tests pin the two halves of that together:
// the launch path refuses with a typed error, and the listing reports the same
// run — so the card and the button cannot disagree about whether a launch is
// possible. They disagreed once, and the learner met a 500 on a card that
// still offered Launch.
func startScenarioWithLiveTerminal(t *testing.T, name string) (db *gorm.DB, svc *services.ScenarioSessionService, scenario models.Scenario, userID string) {
	t.Helper()
	db = freshTestDB(t)

	scenario = models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
	}).Error)

	// The rule turns on whether the terminal behind the session is still
	// alive, so the fixture has to carry a real terminal row — running AND
	// inside its TTL. An expires_at left at its zero value is a terminal that
	// died in year 1: it used to read as live here only because the rule
	// ignored expiry.
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID: "live-terminal",
		UserID:    "student-1",
		State:     terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	svc = services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})
	_, err := svc.StartScenario("student-1", scenario.ID, "live-terminal", "")
	require.NoError(t, err)
	return db, svc, scenario, "student-1"
}

func TestStartScenarioWithLiveRunReturnsTypedConflict(t *testing.T) {
	_, svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-conflict")

	_, err := svc.StartScenario(userID, scenario.ID, "", "")

	require.ErrorIs(t, err, services.ErrActiveSessionExists,
		"a second launch must be a conflict the caller can act on, not an opaque failure")
}

func TestResumableSessionsReportTheRunThatBlocksALaunch(t *testing.T) {
	_, svc, scenario, userID := startScenarioWithLiveTerminal(t, "dup-launch-listing")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})

	require.NoError(t, err)
	require.NotNil(t, resumable[scenario.ID],
		"the listing must see the same run the launch path refuses for")
}

// The other half of the same rule, and the one that actually reached learners:
// a terminal that reached its TTL keeps `state = "running"` because nothing
// tears it down, so a state-only check reported the run as live forever. The
// learner saw "a run is already in progress" for a container deleted hours
// earlier, and could neither resume it nor start another.
func startScenarioThenExpireItsTerminal(t *testing.T, name string) (*services.ScenarioSessionService, models.Scenario, string) {
	t.Helper()
	db, svc, scenario, userID := startScenarioWithLiveTerminal(t, name)

	require.NoError(t, db.Model(&terminalModels.Terminal{}).
		Where("session_id = ?", "live-terminal").
		Update("expires_at", time.Now().Add(-time.Hour)).Error)

	return svc, scenario, userID
}

func TestStartScenarioAfterTerminalExpiredIsAllowed(t *testing.T) {
	svc, scenario, userID := startScenarioThenExpireItsTerminal(t, "relaunch-after-expiry")

	_, err := svc.StartScenario(userID, scenario.ID, "", "")
	require.NoError(t, err, "a run whose terminal has expired must not block the next one")
}

func TestResumableSessionsOmitsRunOnExpiredTerminal(t *testing.T) {
	svc, scenario, userID := startScenarioThenExpireItsTerminal(t, "resumable-after-expiry")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})
	require.NoError(t, err)
	require.Empty(t, resumable, "an expired terminal leaves nothing to resume")
}

func TestMySessionsReportsExpiredRunAsNotResumable(t *testing.T) {
	svc, _, userID := startScenarioThenExpireItsTerminal(t, "my-sessions-after-expiry")

	sessions, err := svc.GetMySessions(userID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.False(t, sessions[0].Resumable,
		"the launcher offers Resume from this flag; a dead terminal must not set it")
}

// A paused run is a run. Pausing a persistent terminal stops the container but
// keeps it (and its disk) until the reap deadline, which the stop moved
// forward; the learner resumes it at the step they left. The launch path, the
// catalogue card and the learner's session list must all see it as the run to
// resume — and say which kind of resume it is, because "Resume" on a paused
// run first has to start the container.
func startScenarioThenPauseItsTerminal(t *testing.T, name string) (*gorm.DB, *services.ScenarioSessionService, models.Scenario, string) {
	t.Helper()
	db, svc, scenario, userID := startScenarioWithLiveTerminal(t, name)

	require.NoError(t, db.Model(&terminalModels.Terminal{}).
		Where("session_id = ?", "live-terminal").
		Updates(map[string]any{
			"state":            terminalModels.StateStopped,
			"persistence_mode": "persistent",
			"expires_at":       time.Now().Add(30 * time.Minute),
		}).Error)

	return db, svc, scenario, userID
}

func TestStartScenarioWithPausedRunReturnsTypedConflict(t *testing.T) {
	db, svc, scenario, userID := startScenarioThenPauseItsTerminal(t, "paused-launch-conflict")

	_, err := svc.StartScenario(userID, scenario.ID, "", "")

	require.ErrorIs(t, err, services.ErrActiveSessionExists,
		"a paused run is resumable in place; launching again must be the typed conflict, not a silent abandon of the paused run")

	var open int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("user_id = ? AND scenario_id = ? AND status = ?", userID, scenario.ID, "active").
		Count(&open).Error)
	require.Equal(t, int64(1), open, "the paused run must still be the learner's open run")
}

func TestResumableSessionsReportPausedRunWithMode(t *testing.T) {
	db, svc, scenario, userID := startScenarioThenPauseItsTerminal(t, "paused-listing")

	resumable, err := svc.GetResumableSessions(userID, []uuid.UUID{scenario.ID})
	require.NoError(t, err)
	paused := resumable[scenario.ID]
	require.NotNil(t, paused, "the listing must report the paused run the launch path refuses for")

	var terminal terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", "live-terminal").First(&terminal).Error)
	require.Equal(t, services.ResumeModePaused, services.RunResumeMode(paused, &terminal, false))

	// The catalogue card is where the learner meets it: it must name the run
	// and say it is paused, from the same evaluation.
	router := setupAvailableRouter(db, userID, []string{"admin"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var cards []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	var card map[string]any
	for _, c := range cards {
		if c["id"] == scenario.ID.String() {
			card = c
		}
	}
	require.NotNil(t, card, "the scenario must be listed")
	require.Equal(t, paused.ID.String(), card["active_session_id"])
	require.Equal(t, "paused", card["active_session_resume_mode"],
		"the card must say the run is paused so it can offer 'resume at step N' rather than a plain relaunch")
}

func TestMySessionsReportsPausedRunResumeMode(t *testing.T) {
	_, svc, _, userID := startScenarioThenPauseItsTerminal(t, "my-sessions-paused")

	sessions, err := svc.GetMySessions(userID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	raw, err := json.Marshal(sessions[0])
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))

	require.Equal(t, "paused", wire["resume_mode"],
		"the learner's session list must say the run is paused")
	require.Equal(t, true, wire["resumable"],
		"resumable stays true for any resume mode, for clients that read only the flag")
}

// A finished run is never resumable, whatever its terminal still looks like.
// Completing or abandoning a run does not stop its terminal, so a learner who
// finishes a scenario and walks away leaves a live — or, once paused, a
// stopped-but-held — terminal behind. The status is what says the run is
// over; reading only the terminal offered "Resume" on a run already graded.
func TestRunResumeMode_FinishedRunIsNeverResumable(t *testing.T) {
	terminalSessionID := "t-1"
	live := &terminalModels.Terminal{
		SessionID: terminalSessionID,
		State:     terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	paused := &terminalModels.Terminal{
		SessionID:       terminalSessionID,
		State:           terminalModels.StateStopped,
		PersistenceMode: terminalModels.PersistenceModePersistent,
		ExpiresAt:       time.Now().Add(30 * time.Minute),
	}
	terminals := map[string]*terminalModels.Terminal{"live": live, "paused": paused}

	for _, status := range []string{"completed", "abandoned", "setup_failed"} {
		for name, terminal := range terminals {
			t.Run(status+"/"+name, func(t *testing.T) {
				session := &models.ScenarioSession{Status: status, TerminalSessionID: &terminalSessionID}
				require.Equal(t, services.ResumeModeNone, services.RunResumeMode(session, terminal, false),
					"a run with status %s is over; its %s terminal must not make it resumable", status, name)
			})
		}
	}

	// Every other open status on a paused terminal is still the learner's run.
	// provisioning included: it is in OpenSessionStatuses (it occupies the
	// one-run slot), and a run mid-setup whose terminal holds its container is
	// one the learner gets back to, not one to relaunch over.
	for _, status := range []string{"active", "in_progress", "provisioning"} {
		t.Run(status+"/paused", func(t *testing.T) {
			session := &models.ScenarioSession{Status: status, TerminalSessionID: &terminalSessionID}
			require.Equal(t, services.ResumeModePaused, services.RunResumeMode(session, paused, false))
		})
	}
}

func TestMySessionsDoesNotOfferResumeForCompletedRun(t *testing.T) {
	db, _, scenario, userID := startScenarioThenPauseItsTerminal(t, "my-sessions-completed-paused")

	now := time.Now()
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("user_id = ? AND scenario_id = ?", userID, scenario.ID).
		Updates(map[string]any{"status": "completed", "completed_at": now}).Error)

	router := setupMySessionsRouter(db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/my", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var sessions []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sessions))
	require.Len(t, sessions, 1)
	require.Equal(t, "completed", sessions[0]["status"])
	require.NotContains(t, sessions[0], "resume_mode",
		"a completed run has nothing to resume, even while its terminal is paused")
	require.Equal(t, false, sessions[0]["resumable"],
		"the launcher offers Resume from this flag; a completed run must not set it")
}

// The launch path refuses a resumable run twice: once up front, before any
// terminal exists, and again inside StartScenario's transaction for the race
// where a concurrent launch commits in between. When the second refusal fires
// the terminal was already created for a run that will never exist; left
// behind, it would hold the learner's budget with nothing attached. The
// controller must delete it and answer the same typed 409 as the up-front
// check — and must leave the winning run, here a paused one, untouched.
func TestLaunchScenario_ConcurrentRunWinsRace_DeletesTheNewTerminal(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-race-" + uuid.New().String()
	seedPersistencePlan(t, db, userID, true)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedPersistenceScenario(t, db, userID, false)

	catalog, _ := newPersistenceTTBackend(t)
	catalogURL, err := url.Parse(catalog.URL)
	require.NoError(t, err)
	forward := httputil.NewSingleHostReverseProxy(catalogURL)

	var (
		mu         sync.Mutex
		created    string
		deleted    []string
		winnerID   uuid.UUID
		winnerTerm = "concurrent-winner-terminal"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/sessions":
			// The concurrent launch commits while this one waits on tt-backend:
			// by the time StartScenario runs, a paused run holds the slot.
			require.NoError(t, db.Create(&terminalModels.Terminal{
				SessionID:       winnerTerm,
				UserID:          userID,
				State:           terminalModels.StateStopped,
				PersistenceMode: terminalModels.PersistenceModePersistent,
				ExpiresAt:       time.Now().Add(30 * time.Minute),
			}).Error)
			winner := models.ScenarioSession{
				ScenarioID:        scenario.ID,
				UserID:            userID,
				Status:            "active",
				StartedAt:         time.Now(),
				TerminalSessionID: &winnerTerm,
			}
			require.NoError(t, db.Create(&winner).Error)

			mu.Lock()
			winnerID = winner.ID
			created = "race-loser-" + uuid.New().String()
			id := created
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			// tt-backend names the new session "id" on the wire.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         id,
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "local",
				"status":     0,
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/1.0/sessions/"):
			mu.Lock()
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/1.0/sessions/"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			forward.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	configureTTServerForPersistence(t, srv.URL)

	router := setupPersistenceRouter(t, db, userID)
	w := launchScenarioForTest(t, router, scenario.ID)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "session_exists", body["reason"],
		"the race must answer the same typed conflict as the up-front check")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, created, "the launch must have reached terminal creation for the race to exist")
	require.Equal(t, []string{created}, deleted,
		"the terminal created for the refused launch must be deleted in tt-backend, and only that one")

	var loser terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", created).First(&loser).Error)
	require.Equal(t, terminalModels.StateDeleted, loser.State,
		"the orphan terminal must leave the budget scope locally too")

	var winner models.ScenarioSession
	require.NoError(t, db.First(&winner, "id = ?", winnerID).Error)
	require.Equal(t, "active", winner.Status, "the winning paused run must stay the learner's open run")
	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("user_id = ? AND scenario_id = ?", userID, scenario.ID).Count(&runs).Error)
	require.Equal(t, int64(1), runs, "the refused launch must not leave a second run")
}

// The card's two resume modes are one field; a paused run must not be the only
// value it has ever been seen to carry. A live run reads "live", so the
// launcher reattaches instead of first starting a container.
func TestAvailableScenariosReportLiveRunWithMode(t *testing.T) {
	db, _, scenario, userID := startScenarioWithLiveTerminal(t, "live-listing")

	router := setupAvailableRouter(db, userID, []string{"admin"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var cards []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	var card map[string]any
	for _, c := range cards {
		if c["id"] == scenario.ID.String() {
			card = c
		}
	}
	require.NotNil(t, card, "the scenario must be listed")
	require.NotEmpty(t, card["active_session_id"])
	require.Equal(t, "live", card["active_session_resume_mode"])
	require.Equal(t, "live-terminal", card["active_terminal_session_id"])
}
