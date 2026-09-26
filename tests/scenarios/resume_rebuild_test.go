// tests/scenarios/resume_rebuild_test.go
//
// Pins POST /scenario-sessions/:id/resume: how a learner gets back into an open
// run, whatever became of its terminal.
//
//   - live   → the terminal is running: 200 with it, nothing else happens;
//   - paused → the terminal is stopped and still holds its container: it is
//     started in place, 200 with it — and when tt-backend no longer knows the
//     container, the resume falls through to a rebuild;
//   - rebuild → the container is gone but the run is a normal one: a new
//     terminal is created in the run's own organisation, the run is reattached
//     to it, and the world is rebuilt — the scenario's setup, then every step's
//     background script through the current step, with the flags the DB
//     already holds. Progress, hints and score are untouched;
//   - none   → a crash-trap run whose container is gone is over: 409 run_over.
//
// And what a rebuild must never do: leave the run on a half-built machine when
// a script fails, keep building for a run abandoned under it, reattach twice
// when two resumes race, or let the stuck-provisioning reaper take a long
// replay for a hung one.
//
// Witness: the requests the controller sent to the fake tt-backend of
// preview_from_step_test.go — sessions created, started and deleted, scripts
// exec'd with their environment, files pushed — and the rows left in the DB.
package scenarios_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// resumeCurrentStep is the step the seeded runs are on: steps before it are
// completed, the one after it still locked.
const resumeCurrentStep = 3

// resumeResponse is the resume route's wire shape.
type resumeResponse struct {
	TerminalSessionID          string `json:"terminal_session_id"`
	ScenarioSessionID          string `json:"scenario_session_id"`
	Status                     string `json:"status"`
	ProvisioningPhase          string `json:"provisioning_phase"`
	ProvisioningTimeoutSeconds int    `json:"provisioning_timeout_seconds"`
}

// resumeFixture is a learner's open run on step resumeCurrentStep of a
// previewStepCount-step scenario, whose terminal is in the state the test
// asked for.
type resumeFixture struct {
	db          *gorm.DB
	learnerID   string
	scenario    *models.Scenario
	run         models.ScenarioSession
	oldTerminal string
	// orgID is the school owning the scenario and the old terminal, when the
	// seed asked for one.
	orgID *uuid.UUID
	// flags are the stored answers, by step order.
	flags map[int]string
}

// resumeSeed says what to seed; the zero value is a public, platform-wide
// scenario whose run's container is gone.
type resumeSeed struct {
	crashTraps bool
	// terminalState is the old terminal's state; deleted (container gone) when
	// empty.
	terminalState terminalModels.TerminalState
	// inOrg puts the scenario, its assignment and the old terminal in a
	// school the learner is a member of; otherwise the scenario is public and
	// the terminal personal.
	inOrg bool
	// foregroundOnCurrent gives the current step a foreground script.
	foregroundOnCurrent bool
}

// seedResumableRun seeds a learner — plan, terminal key — and their open run
// of a scenario of previewStepCount 1-based steps, each with a background
// script ("echo bg-step-N", which builtSteps recognises), a flag planted at a
// file path, and a stored answer. Steps before resumeCurrentStep are solved,
// with hints revealed and attempts spent.
func seedResumableRun(t *testing.T, name string, seed resumeSeed) resumeFixture {
	t.Helper()
	db := freshTestDB(t)
	learnerID := name + "-" + uuid.New().String()
	seedPersistencePlan(t, db, learnerID, true)
	seedPersistenceUserKey(t, db, learnerID)

	var orgID *uuid.UUID
	if seed.inOrg {
		school := createTestOrg(t, db, "school-owner")
		addOrgMember(t, db, school, learnerID, orgModels.OrgRoleMember)
		orgID = &school
	}

	scenario := &models.Scenario{
		Name:           name + "-" + uuid.New().String(),
		Title:          "Resume By Rebuild",
		InstanceType:   "M",
		OsType:         "deb",
		IsPublic:       orgID == nil,
		OrganizationID: orgID,
		CreatedByID:    "resume-author",
		SetupScript:    "echo scenario-setup",
		CrashTraps:     seed.crashTraps,
		FlagsEnabled:   true,
	}
	require.NoError(t, db.Create(scenario).Error)
	for order := 1; order <= previewStepCount; order++ {
		step := &models.ScenarioStep{
			ScenarioID:       scenario.ID,
			Order:            order,
			Title:            fmt.Sprintf("Step %d", order),
			StepType:         "flag",
			BackgroundScript: fmt.Sprintf("echo bg-step-%d", order),
			HasFlag:          true,
			FlagPath:         fmt.Sprintf("/tmp/flag-%d", order),
		}
		if seed.foregroundOnCurrent && order == resumeCurrentStep {
			step.ForegroundScript = "echo fg-current-step"
		}
		require.NoError(t, db.Create(step).Error)
	}
	require.NoError(t, db.Preload("Steps").First(scenario, "id = ?", scenario.ID).Error)
	if orgID != nil {
		require.NoError(t, db.Create(&models.ScenarioAssignment{
			ScenarioID:     scenario.ID,
			OrganizationID: orgID,
			Scope:          "org",
			CreatedByID:    "school-owner",
			IsActive:       true,
		}).Error)
	}

	state := seed.terminalState
	if state == "" {
		state = terminalModels.StateDeleted
	}
	oldTerminal := "resume-old-terminal-" + uuid.New().String()
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID:       oldTerminal,
		UserID:          learnerID,
		State:           state,
		PersistenceMode: terminalModels.PersistenceModePersistent,
		ExpiresAt:       time.Now().Add(30 * time.Minute),
		OrganizationID:  orgID,
	}).Error)

	terminalID := oldTerminal
	run := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            learnerID,
		TerminalSessionID: &terminalID,
		CurrentStep:       resumeCurrentStep,
		Status:            "active",
		StartedAt:         time.Now().Add(-2 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, db.Create(&run).Error)

	flags := map[int]string{}
	for order := 1; order <= previewStepCount; order++ {
		flags[order] = fmt.Sprintf("stored-flag-%d-%s", order, uuid.New().String()[:8])
		flag := models.ScenarioFlag{SessionID: run.ID, StepOrder: order, ExpectedFlag: flags[order]}
		progress := models.ScenarioStepProgress{SessionID: run.ID, StepOrder: order, StepType: "flag", Status: "locked"}
		switch {
		case order < resumeCurrentStep:
			submitted := flags[order]
			solvedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
			flag.SubmittedFlag = &submitted
			flag.SubmittedAt = &solvedAt
			flag.IsCorrect = true
			flag.FlagAttempts = order + 1
			progress.Status = "completed"
			progress.CompletedAt = &solvedAt
			progress.VerifyAttempts = order + 1
			progress.HintsRevealed = order
			progress.TimeSpentSeconds = 60 * order
		case order == resumeCurrentStep:
			flag.FlagAttempts = 2
			progress.Status = "active"
			progress.VerifyAttempts = 4
			progress.HintsRevealed = 1
		}
		require.NoError(t, db.Create(&flag).Error)
		require.NoError(t, db.Create(&progress).Error)
	}

	return resumeFixture{
		db:          db,
		learnerID:   learnerID,
		scenario:    scenario,
		run:         run,
		oldTerminal: oldTerminal,
		orgID:       orgID,
		flags:       flags,
	}
}

// resumeRouter wires POST /scenario-sessions/:id/resume as production registers
// it — no plan-chain middleware — for a plain Member. orgContext, when not
// empty, is the organisation the request itself claims (InjectOrgContext's
// key), which a resume must ignore.
func resumeRouter(t *testing.T, db *gorm.DB, userID string, orgContext string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		if orgContext != "" {
			c.Set("org_context_id", orgContext)
		}
		c.Next()
	})
	router.POST("/api/v1/scenario-sessions/:id/resume", scenarioController.NewScenarioLaunchController(db).ResumeScenario)
	return router
}

func postResume(router *gin.Engine, runID uuid.UUID) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+runID.String()+"/resume", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// resumeRun POSTs the resume as the fixture's learner.
func resumeRun(t *testing.T, f resumeFixture) *httptest.ResponseRecorder {
	t.Helper()
	return postResume(resumeRouter(t, f.db, f.learnerID, ""), f.run.ID)
}

func decodeResume(t *testing.T, w *httptest.ResponseRecorder) resumeResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "the resume must succeed; body=%s", w.Body.String())
	var resp resumeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// rebuiltRun decodes a rebuild response and waits for the replay to finish.
func rebuiltRun(t *testing.T, f resumeFixture, w *httptest.ResponseRecorder) (resumeResponse, models.ScenarioSession) {
	t.Helper()
	resp := decodeResume(t, w)
	return resp, settledRun(t, f.db, f.run.ID)
}

// settledRun waits for the run to leave provisioning and returns it with its
// progress and flags.
func settledRun(t *testing.T, db *gorm.DB, runID uuid.UUID) models.ScenarioSession {
	t.Helper()
	var run models.ScenarioSession
	require.Eventually(t, func() bool {
		run = models.ScenarioSession{}
		return db.Preload("StepProgress").Preload("Flags").First(&run, "id = ?", runID).Error == nil &&
			run.Status != "provisioning"
	}, 10*time.Second, 10*time.Millisecond, "the replay must finish")
	return run
}

func flagsByOrder(run models.ScenarioSession) map[int]models.ScenarioFlag {
	byOrder := make(map[int]models.ScenarioFlag, len(run.Flags))
	for _, f := range run.Flags {
		byOrder[f.StepOrder] = f
	}
	return byOrder
}

// envOf returns the environment the named script was exec'd with.
func (tt *previewTTBackend) envOf(marker string) (map[string]string, bool) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	for i, script := range tt.scripts {
		if strings.Contains(script, marker) {
			return tt.envs[i], true
		}
	}
	return nil, false
}

func (tt *previewTTBackend) pushedFiles() []pushedFile {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return append([]pushedFile(nil), tt.pushes...)
}

func (tt *previewTTBackend) startedSessions() []string {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return append([]string(nil), tt.started...)
}

// isBuildScript reports whether an exec'd command is one of the fixture's
// build scripts rather than a banner or a clean-up.
func isBuildScript(script string) bool {
	return strings.Contains(script, "echo scenario-setup") || strings.Contains(script, "echo bg-step-")
}

// -----------------------------------------------------------------------------
// rebuild
// -----------------------------------------------------------------------------

func TestResumeRebuild_CreatesTerminalAndReattachesRun(t *testing.T) {
	f := seedResumableRun(t, "resume-reattach", resumeSeed{})
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)
	resp, run := rebuiltRun(t, f, w)

	assert.Equal(t, 1, tt.createCalls(), "a gone container is rebuilt on one new terminal")
	assert.NotEqual(t, f.oldTerminal, resp.TerminalSessionID, "on a new terminal")
	assert.Equal(t, f.run.ID.String(), resp.ScenarioSessionID, "the run resumed is the learner's own, not a new one")
	assert.Equal(t, "provisioning", resp.Status, "the client polls while the world is rebuilt")
	assert.Equal(t, "replay", resp.ProvisioningPhase)
	assert.Positive(t, resp.ProvisioningTimeoutSeconds, "the client is told how long the replay may take")

	assert.Equal(t, "active", run.Status)
	require.NotNil(t, run.TerminalSessionID)
	assert.Equal(t, resp.TerminalSessionID, *run.TerminalSessionID, "the run is reattached to the new terminal")

	var open []models.ScenarioSession
	require.NoError(t, f.db.Where("user_id = ? AND scenario_id = ? AND status IN ?",
		f.learnerID, f.scenario.ID, models.OpenSessionStatuses).Find(&open).Error)
	require.Len(t, open, 1, "one run, reattached — not a second run beside the first")
	assert.Equal(t, f.run.ID, open[0].ID)

	var terminal terminalModels.Terminal
	require.NoError(t, f.db.First(&terminal, "session_id = ?", resp.TerminalSessionID).Error)
	assert.Equal(t, f.learnerID, terminal.UserID, "the new terminal is the learner's")
}

func TestResumeRebuild_ReplaysSetupThenBackgroundsThroughCurrentStep(t *testing.T) {
	f := seedResumableRun(t, "resume-replay", resumeSeed{})
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)

	assert.Equal(t, "active", run.Status)
	assert.Equal(t, []string{"setup", "bg1", "bg2", "bg3"}, tt.builtSteps(),
		"step 3 is rebuilt as a learner arriving there would find it: the setup, then "+
			"every step's background script through 3, in order — and nothing of step 4")
}

func TestResumeRebuild_ReplantsFlagsFromStoredValues(t *testing.T) {
	f := seedResumableRun(t, "resume-flags", resumeSeed{})
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)
	require.Equal(t, "active", run.Status)

	pushed := map[string]string{}
	for _, p := range tt.pushedFiles() {
		pushed[p.path] = p.content
	}
	for order := 1; order <= resumeCurrentStep; order++ {
		env, ok := tt.envOf(fmt.Sprintf("echo bg-step-%d", order))
		require.True(t, ok, "step %d's background script is replayed", order)
		assert.Equal(t, f.flags[order], env["OCF_FLAG_CURRENT"],
			"step %d's script is handed the answer the DB holds, not a new one", order)
		assert.Equal(t, f.flags[order]+"\n", pushed[fmt.Sprintf("/tmp/flag-%d", order)],
			"step %d's flag file is planted with the stored answer", order)
	}
	_, planted := pushed[fmt.Sprintf("/tmp/flag-%d", previewStepCount)]
	assert.False(t, planted, "a step after the current one plants nothing")
	for order, flag := range flagsByOrder(run) {
		assert.Equal(t, f.flags[order], flag.ExpectedFlag, "replanting changes no stored answer (step %d)", order)
	}
}

// A script that declares its answer draws a new one on every run. For a step
// the learner already solved that answer is history: it is never re-rolled.
// The current step's is adopted, by design — the world it belongs to is new.
func TestResumeRebuild_CompletedStepAnswersAreNotReRolled(t *testing.T) {
	f := seedResumableRun(t, "resume-answers", resumeSeed{})
	tt := newPreviewTTBackend(t)
	tt.onExec = func(script string) string {
		for order := 1; order <= previewStepCount; order++ {
			if strings.Contains(script, fmt.Sprintf("echo bg-step-%d", order)) {
				return fmt.Sprintf("OCF_ANSWER: redrawn-%d\n", order)
			}
		}
		return ""
	}

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)
	require.Equal(t, "active", run.Status)

	flags := flagsByOrder(run)
	for order := 1; order < resumeCurrentStep; order++ {
		assert.Equal(t, f.flags[order], flags[order].ExpectedFlag,
			"step %d is solved: its answer is the one the learner submitted", order)
	}
	assert.Equal(t, fmt.Sprintf("redrawn-%d", resumeCurrentStep), flags[resumeCurrentStep].ExpectedFlag,
		"the current step's answer follows the rebuilt world")
}

func TestResumeRebuild_KeepsProgressHintsAndScore(t *testing.T) {
	f := seedResumableRun(t, "resume-progress", resumeSeed{})
	var before models.ScenarioSession
	require.NoError(t, f.db.Preload("StepProgress").Preload("Flags").First(&before, "id = ?", f.run.ID).Error)
	newPreviewTTBackend(t)

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)

	assert.Equal(t, "active", run.Status)
	assert.Equal(t, resumeCurrentStep, run.CurrentStep, "the learner is back on the step they left")
	assert.True(t, before.StartedAt.Equal(run.StartedAt), "the run keeps its start time")
	assert.Nil(t, run.CompletedAt)

	was := progressByOrder(before)
	now := progressByOrder(run)
	require.Len(t, now, previewStepCount)
	for order := 1; order <= previewStepCount; order++ {
		assert.Equal(t, was[order].Status, now[order].Status, "step %d status", order)
		assert.Equal(t, was[order].HintsRevealed, now[order].HintsRevealed, "step %d hints", order)
		assert.Equal(t, was[order].VerifyAttempts, now[order].VerifyAttempts, "step %d attempts", order)
		assert.Equal(t, was[order].TimeSpentSeconds, now[order].TimeSpentSeconds, "step %d time spent", order)
		if was[order].CompletedAt != nil {
			require.NotNil(t, now[order].CompletedAt, "step %d stays completed", order)
			assert.True(t, was[order].CompletedAt.Equal(*now[order].CompletedAt), "step %d completion time", order)
		}
	}

	wasFlags := flagsByOrder(before)
	for order, flag := range flagsByOrder(run) {
		assert.Equal(t, wasFlags[order].IsCorrect, flag.IsCorrect, "step %d score", order)
		assert.Equal(t, wasFlags[order].SubmittedFlag, flag.SubmittedFlag, "step %d submission", order)
		assert.Equal(t, wasFlags[order].FlagAttempts, flag.FlagAttempts, "step %d flag attempts", order)
	}
}

// The current step's foreground script is typed when the learner's console
// first attaches to the rebuilt terminal (MR B !552, PendingForegroundOrder).
// This branch predates MR B: the test skips itself until the column exists and
// runs unchanged once the branch is rebased onto it.
func TestResumeRebuild_SetsPendingForegroundForCurrentStep(t *testing.T) {
	f := seedResumableRun(t, "resume-foreground", resumeSeed{foregroundOnCurrent: true})
	if !f.db.Migrator().HasColumn(&models.ScenarioSession{}, "pending_foreground_order") {
		t.Skip("needs MR B !552 (pending_foreground_order); runs once this branch is rebased onto it")
	}
	newPreviewTTBackend(t)

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)
	require.Equal(t, "active", run.Status)

	var pending *int
	require.NoError(t, f.db.Raw("SELECT pending_foreground_order FROM scenario_sessions WHERE id = ?", run.ID).
		Scan(&pending).Error)
	require.NotNil(t, pending, "the rebuild ends with the current step's foreground, typed on first attach")
	assert.Equal(t, resumeCurrentStep, *pending)
}

// A replay that fails puts the run back where it was — active on its old,
// gone terminal, so still rebuildable — and deletes the new, half-built one.
// Keeping the new id would turn a failed build into a "live" resume into a
// broken machine.
func TestResumeRebuild_ScriptFails_RestoresOldTerminalAndDeletesNewOne(t *testing.T) {
	f := seedResumableRun(t, "resume-fails", resumeSeed{})
	tt := newPreviewTTBackend(t)
	tt.failOn = "echo bg-step-2"

	w := resumeRun(t, f)
	resp, run := rebuiltRun(t, f, w)

	assert.Equal(t, "active", run.Status, "the run is not lost to a failed rebuild")
	assert.Empty(t, run.ProvisioningPhase)
	require.NotNil(t, run.TerminalSessionID)
	assert.Equal(t, f.oldTerminal, *run.TerminalSessionID, "the run is back on its old terminal id")
	assert.Equal(t, []string{"setup", "bg1", "bg2"}, tt.builtSteps(), "the replay stops at the first failure")
	assert.Eventually(t, func() bool {
		return assert.ObjectsAreEqual([]string{resp.TerminalSessionID}, tt.deletedSessions())
	}, 5*time.Second, 10*time.Millisecond,
		"the half-built terminal is deleted, and only it; deleted=%v", tt.deletedSessions())
}

// Abandon wins over a replay in flight: the build stops before its next
// script and the new terminal goes, since nobody is coming back to it.
func TestResumeRebuild_AbandonedMidReplay_StopsAndDeletesNewTerminal(t *testing.T) {
	f := seedResumableRun(t, "resume-abandoned", resumeSeed{})
	tt := newPreviewTTBackend(t)
	svc := services.NewScenarioSessionService(f.db, &mockFlagService{}, &mockVerificationService{})
	tt.onExec = func(script string) string {
		if strings.Contains(script, "echo bg-step-2") {
			assert.NoError(t, svc.AbandonSession(f.run.ID))
		}
		return ""
	}

	w := resumeRun(t, f)
	resp := decodeResume(t, w)

	assert.Eventually(t, func() bool {
		return assert.ObjectsAreEqual([]string{resp.TerminalSessionID}, tt.deletedSessions())
	}, 5*time.Second, 10*time.Millisecond,
		"the abandoned run's new terminal is deleted; deleted=%v", tt.deletedSessions())
	assert.Equal(t, []string{"setup", "bg1", "bg2"}, tt.builtSteps(),
		"nothing is built after the run was abandoned")
	assert.Equal(t, "abandoned", sessionStatus(t, f.db, f.run.ID), "the abandon stands")
}

// Two resumes of the same run — two tabs, a double click — race to rebuild
// it. One reattaches; the other must neither reattach a second terminal nor
// leave its own behind.
func TestResumeRebuild_ConcurrentResumes_OnlyOneReattaches(t *testing.T) {
	f := seedResumableRun(t, "resume-race", resumeSeed{})
	tt := newPreviewTTBackend(t)

	// Hold each terminal creation until both requests have got that far, so
	// both have decided to rebuild before either reattaches. An implementation
	// that serialises earlier never brings the second one here: the wait then
	// times out and the first goes on alone.
	var arrivals atomic.Int32
	bothArrived := make(chan struct{})
	tt.beforeCreate = func() {
		if arrivals.Add(1) == 2 {
			close(bothArrived)
		}
		select {
		case <-bothArrived:
		case <-time.After(2 * time.Second):
		}
	}

	router := resumeRouter(t, f.db, f.learnerID, "")
	responses := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = postResume(router, f.run.ID)
		}(i)
	}
	wg.Wait()

	run := settledRun(t, f.db, f.run.ID)
	require.Equal(t, "active", run.Status)
	require.NotNil(t, run.TerminalSessionID)
	final := *run.TerminalSessionID
	assert.NotEqual(t, f.oldTerminal, final)

	succeeded := 0
	for i, w := range responses {
		require.Contains(t, []int{http.StatusOK, http.StatusConflict}, w.Code,
			"response %d; body=%s", i, w.Body.String())
		if w.Code != http.StatusOK {
			continue
		}
		succeeded++
		var resp resumeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, final, resp.TerminalSessionID,
			"a successful resume sends the learner to the terminal the run is on")
	}
	assert.Positive(t, succeeded, "one of the two resumes goes through")

	setups := 0
	for _, step := range tt.builtSteps() {
		if step == "setup" {
			setups++
		}
	}
	assert.Equal(t, 1, setups, "the world is rebuilt once")

	assert.Eventually(t, func() bool {
		var leftovers int64
		f.db.Model(&terminalModels.Terminal{}).
			Where("user_id = ? AND session_id NOT IN ? AND state <> ?",
				f.learnerID, []string{f.oldTerminal, final}, terminalModels.StateDeleted).
			Count(&leftovers)
		return leftovers == 0
	}, 5*time.Second, 10*time.Millisecond, "the losing resume's terminal, if any, is deleted")
}

// The reaper sends a provisioning row untouched for 12 minutes to
// setup_failed. A replay can take longer than that in total, so every script
// it runs must refresh the row: here each exec ages the row by 20 minutes and
// the next one runs the reaper, which must find nothing to reap.
func TestResumeRebuild_HeartbeatKeepsLongReplayAwayFromReaper(t *testing.T) {
	f := seedResumableRun(t, "resume-heartbeat", resumeSeed{})
	tt := newPreviewTTBackend(t)
	tt.onExec = func(script string) string {
		if !isBuildScript(script) {
			return ""
		}
		reaped, err := services.CleanupStuckProvisioningSessions(f.db)
		assert.NoError(t, err)
		assert.Zero(t, reaped, "a replay between two scripts is not stuck (script: %q)", script)
		assert.NoError(t, f.db.Model(&models.ScenarioSession{}).Where("id = ?", f.run.ID).
			UpdateColumn("updated_at", time.Now().Add(-20*time.Minute)).Error)
		return ""
	}

	w := resumeRun(t, f)
	_, run := rebuiltRun(t, f, w)

	assert.Equal(t, "active", run.Status, "the long replay completes instead of being reaped")
	assert.Equal(t, []string{"setup", "bg1", "bg2", "bg3"}, tt.builtSteps())
}

// The rebuilt terminal belongs to the organisation the run's terminal belonged
// to, whatever organisation the request claims: that is the org whose trainers
// supervise the run. Following the request would let a learner rebuild into
// their personal space and drop out of supervision.
func TestResumeRebuild_UsesOldTerminalOrganisation(t *testing.T) {
	f := seedResumableRun(t, "resume-org", resumeSeed{inOrg: true})
	school := *f.orgID
	elsewhere := createTestOrg(t, f.db, f.learnerID)
	addOrgMember(t, f.db, elsewhere, f.learnerID, orgModels.OrgRoleOwner)
	tt := newPreviewTTBackend(t)

	w := postResume(resumeRouter(t, f.db, f.learnerID, elsewhere.String()), f.run.ID)
	resp, run := rebuiltRun(t, f, w)
	require.Equal(t, "active", run.Status)

	tt.persistenceRecorder.mu.Lock()
	sentOrg := tt.persistenceRecorder.gotBody["organization_id"]
	tt.persistenceRecorder.mu.Unlock()
	assert.Equal(t, school.String(), sentOrg, "tt-backend is asked for a terminal in the run's organisation")

	var terminal terminalModels.Terminal
	require.NoError(t, f.db.First(&terminal, "session_id = ?", resp.TerminalSessionID).Error)
	require.NotNil(t, terminal.OrganizationID, "the rebuilt terminal stays in an organisation")
	assert.Equal(t, school, *terminal.OrganizationID,
		"the rebuilt terminal is where the school's trainers supervise it, not the request's org")
}

// -----------------------------------------------------------------------------
// live, paused, none
// -----------------------------------------------------------------------------

func TestResume_Paused_StartsTerminalInPlace(t *testing.T) {
	f := seedResumableRun(t, "resume-paused", resumeSeed{terminalState: terminalModels.StateStopped})
	tt := newPreviewTTBackend(t)

	resp := decodeResume(t, resumeRun(t, f))

	assert.Equal(t, f.oldTerminal, resp.TerminalSessionID, "a paused run resumes on its own terminal")
	assert.Equal(t, []string{f.oldTerminal}, tt.startedSessions(), "which is started in place")
	assert.Zero(t, tt.createCalls(), "no terminal is created for a container that still exists")
	assert.Empty(t, tt.builtSteps(), "nothing is rebuilt: the container kept its world")

	var terminal terminalModels.Terminal
	require.NoError(t, f.db.First(&terminal, "session_id = ?", f.oldTerminal).Error)
	assert.Equal(t, terminalModels.StateRunning, terminal.State)
	var run models.ScenarioSession
	require.NoError(t, f.db.First(&run, "id = ?", f.run.ID).Error)
	assert.Equal(t, "active", run.Status)
	assert.Equal(t, f.oldTerminal, *run.TerminalSessionID)
}

// The row said stopped, but tt-backend has lost the container (reaped, host
// rebuilt): the resume does not fail on it, it rebuilds.
func TestResume_PausedButContainerGone_FallsThroughToRebuild(t *testing.T) {
	f := seedResumableRun(t, "resume-paused-gone", resumeSeed{terminalState: terminalModels.StateStopped})
	tt := newPreviewTTBackend(t)
	tt.startMissing = true

	resp, run := rebuiltRun(t, f, resumeRun(t, f))

	assert.Equal(t, []string{f.oldTerminal}, tt.startedSessions(), "the paused terminal is tried first")
	assert.NotEqual(t, f.oldTerminal, resp.TerminalSessionID, "then the run is rebuilt on a new one")
	assert.Equal(t, "replay", resp.ProvisioningPhase)
	assert.Equal(t, "active", run.Status)
	assert.Equal(t, resp.TerminalSessionID, *run.TerminalSessionID)

	var old terminalModels.Terminal
	require.NoError(t, f.db.First(&old, "session_id = ?", f.oldTerminal).Error)
	assert.Equal(t, terminalModels.StateDeleted, old.State, "the lost container's row says so")
}

func TestResume_Live_IsNoOp(t *testing.T) {
	f := seedResumableRun(t, "resume-live", resumeSeed{terminalState: terminalModels.StateRunning})
	tt := newPreviewTTBackend(t)

	resp := decodeResume(t, resumeRun(t, f))

	assert.Equal(t, f.oldTerminal, resp.TerminalSessionID, "a live run is resumed where it runs")
	assert.Equal(t, f.run.ID.String(), resp.ScenarioSessionID)
	assert.Zero(t, tt.createCalls())
	assert.Empty(t, tt.startedSessions())
	assert.Empty(t, tt.stoppedSessions())
	assert.Empty(t, tt.deletedSessions())
	assert.Empty(t, tt.builtSteps())
	assert.Equal(t, "active", sessionStatus(t, f.db, f.run.ID))
}

// A crash-trap run whose container is gone is over — permadeath is the point
// of the mode — so there is nothing to resume.
func TestResume_CrashTrapGone_409RunOver(t *testing.T) {
	f := seedResumableRun(t, "resume-crash-trap", resumeSeed{crashTraps: true})
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)

	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason":"run_over"`)
	assert.Zero(t, tt.createCalls(), "no terminal for a run that is over")
	var run models.ScenarioSession
	require.NoError(t, f.db.First(&run, "id = ?", f.run.ID).Error)
	assert.Equal(t, f.oldTerminal, *run.TerminalSessionID, "the run is left as it was")
}

// Layer 2: only the run's owner may resume it. Declared once, in
// permissions.go, as an EntityOwner rule on ScenarioSession.UserID — which
// also gives Members the Casbin POST policy.
func TestResume_NotOwner_403(t *testing.T) {
	f := seedResumableRun(t, "resume-not-owner", resumeSeed{})
	tt := newPreviewTTBackend(t)

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	enforcer := mocks.NewMockEnforcer()
	enforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	scenarioController.RegisterScenarioPermissions(enforcer)
	access.RegisterBuiltinEnforcers(access.NewGormEntityLoader(f.db), access.NewGormMembershipChecker(f.db))

	perm, declared := access.RouteRegistry.Lookup(http.MethodPost, "/api/v1/scenario-sessions/:id/resume")
	require.True(t, declared, "the resume route is declared in permissions.go")
	assert.Equal(t, access.RoleMember, perm.Role)
	assert.Equal(t, access.EntityOwner, perm.Access.Type)
	assert.Equal(t, "ScenarioSession", perm.Access.Entity)
	assert.Equal(t, "UserID", perm.Access.Field)

	strangerID := "resume-stranger-" + uuid.New().String()
	seedPersistencePlan(t, f.db, strangerID, true)
	seedPersistenceUserKey(t, f.db, strangerID)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", strangerID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	api.Use(access.Layer2Enforcement())
	api.POST("/scenario-sessions/:id/resume", scenarioController.NewScenarioLaunchController(f.db).ResumeScenario)

	w := postResume(router, f.run.ID)

	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Zero(t, tt.createCalls(), "a stranger never gets a terminal out of someone else's run")
	var run models.ScenarioSession
	require.NoError(t, f.db.First(&run, "id = ?", f.run.ID).Error)
	assert.Equal(t, f.oldTerminal, *run.TerminalSessionID, "the run is untouched")
}

// -----------------------------------------------------------------------------
// the budget the client polls against
// -----------------------------------------------------------------------------

// During a replay the client waits for the whole rebuild, not one step: the
// setup script's budget plus every replayed step's, through the current one.
func TestCurrentStepProvisioningTimeout_ReplayReturnsRemainingBudget(t *testing.T) {
	f := seedResumableRun(t, "resume-timeout", resumeSeed{})
	for order, seconds := range map[int]int{1: 100, 2: 0, 3: 200, 4: 400} {
		require.NoError(t, f.db.Model(&models.ScenarioStep{}).
			Where("scenario_id = ? AND \"order\" = ?", f.scenario.ID, order).
			Update("background_timeout_seconds", seconds).Error)
	}
	require.NoError(t, f.db.Model(&models.ScenarioSession{}).Where("id = ?", f.run.ID).
		Updates(map[string]any{"status": "provisioning", "provisioning_phase": "replay"}).Error)
	var replaying models.ScenarioSession
	require.NoError(t, f.db.First(&replaying, "id = ?", f.run.ID).Error)
	svc := services.NewScenarioSessionService(f.db, &mockFlagService{}, &mockVerificationService{})

	// setup 300 (initial budget) + step 1's 100 + step 2's default 30 +
	// step 3's 200; step 4 is not replayed.
	assert.Equal(t, 300+100+30+200, svc.CurrentStepProvisioningTimeout(&replaying))

	// A step's own provisioning still reports that step's budget alone.
	require.NoError(t, f.db.Model(&models.ScenarioSession{}).Where("id = ?", f.run.ID).
		Update("provisioning_phase", "step_setup").Error)
	var stepSetup models.ScenarioSession
	require.NoError(t, f.db.First(&stepSetup, "id = ?", f.run.ID).Error)
	assert.Equal(t, 200, svc.CurrentStepProvisioningTimeout(&stepSetup))
}
