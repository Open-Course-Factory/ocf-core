// tests/scenarios/preview_from_step_test.go
//
// Pins POST /scenarios/:id/preview with { "from_step_order": N }: an author
// tests step N on the machine a learner arriving at N would get — the
// scenario's setup, then every step's background script from the first
// through N, and nothing after. Steps before N read as completed, N is the
// current step.
//
// It also pins what a preview must never do on the way:
//   - create a terminal for a caller who may not preview, or for a step that
//     does not exist (the terminal would be an orphan holding budget);
//   - fail on the author's previous preview: it is replaced, its terminal
//     deleted, since every "Test from this step" click is a new preview;
//   - replace a real (non-preview) run of the author's: that is a 409.
//
// Witness: the requests the controller sent to a fake tt-backend — the
// scripts it exec'd, in order, and the sessions it created, stopped and
// deleted — and the rows it left in the DB.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// previewStepCount is the number of steps of the scenario these tests preview.
// Orders are 1-based, as the editor makes them.
const previewStepCount = 4

// previewTTBackend fronts the launch fake (newPersistenceTTBackend) with what
// a build talks to: exec and file-push, stop and build-complete. It records
// every script exec'd, in order, and every session stopped. Anything else is
// forwarded to the launch fake, which records the sessions created and deleted.
type previewTTBackend struct {
	*persistenceRecorder

	mu      sync.Mutex
	scripts []string
	stopped []string
	// failOn makes the exec of any script containing it exit 1.
	failOn string
}

func newPreviewTTBackend(t *testing.T) *previewTTBackend {
	t.Helper()
	launchSrv, rec := newPersistenceTTBackend(t)
	target, err := url.Parse(launchSrv.URL)
	require.NoError(t, err)
	forward := httputil.NewSingleHostReverseProxy(target)

	tt := &previewTTBackend{persistenceRecorder: rec}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/exec":
			var body struct {
				Command []string `json:"command"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			script := ""
			if len(body.Command) > 0 {
				script = body.Command[len(body.Command)-1]
			}
			tt.mu.Lock()
			tt.scripts = append(tt.scripts, script)
			fail := tt.failOn != "" && strings.Contains(script, tt.failOn)
			tt.mu.Unlock()
			exitCode := 0
			if fail {
				exitCode = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": exitCode, "stdout": "", "stderr": ""})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/file-push":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/build-complete"):
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/1.0/sessions/"), "/stop")
			tt.mu.Lock()
			tt.stopped = append(tt.stopped, id)
			tt.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			forward.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	configureTTServerForPersistence(t, srv.URL)
	return tt
}

// builtSteps names the scripts this scenario ran, in order: "setup" for the
// scenario's setup script, "bgN" for step N's background script. Anything
// else the build exec'd is not the subject here and is left out.
func (tt *previewTTBackend) builtSteps() []string {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	var built []string
	for _, script := range tt.scripts {
		if strings.Contains(script, "echo scenario-setup") {
			built = append(built, "setup")
			continue
		}
		for order := 1; order <= previewStepCount; order++ {
			if strings.Contains(script, fmt.Sprintf("echo bg-step-%d", order)) {
				built = append(built, fmt.Sprintf("bg%d", order))
			}
		}
	}
	return built
}

func (tt *previewTTBackend) stoppedSessions() []string {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return append([]string(nil), tt.stopped...)
}

func (tt *previewTTBackend) createCalls() int {
	tt.persistenceRecorder.mu.Lock()
	defer tt.persistenceRecorder.mu.Unlock()
	return tt.calls
}

func (tt *previewTTBackend) deletedSessions() []string {
	tt.persistenceRecorder.mu.Lock()
	defer tt.persistenceRecorder.mu.Unlock()
	return append([]string(nil), tt.deleted...)
}

// seedPreviewableScenario creates a scenario of previewStepCount 1-based
// steps, each with a background script, plus a setup script, created by
// authorID — who has a plan and a terminal key, so a preview can go all the
// way to a terminal.
func seedPreviewableScenario(t *testing.T, name string) (*gorm.DB, string, *models.Scenario) {
	t.Helper()
	db := freshTestDB(t)
	authorID := name + "-" + uuid.New().String()
	seedPersistencePlan(t, db, authorID, true)
	seedPersistenceUserKey(t, db, authorID)

	scenario := &models.Scenario{
		Name:         name + "-" + uuid.New().String(),
		Title:        "Preview From Step",
		InstanceType: "M",
		OsType:       "deb",
		CreatedByID:  authorID,
		SetupScript:  "echo scenario-setup",
	}
	require.NoError(t, db.Create(scenario).Error)
	for order := 1; order <= previewStepCount; order++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID:       scenario.ID,
			Order:            order,
			Title:            fmt.Sprintf("Step %d", order),
			StepType:         "terminal",
			BackgroundScript: fmt.Sprintf("echo bg-step-%d", order),
		}).Error)
	}
	return db, authorID, scenario
}

// previewRouter wires POST /scenarios/:id/preview as production registers it
// (no plan-chain middleware), for a plain Member: no admin bypass, so the
// preview authorization is the real one.
func previewRouter(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.POST("/api/v1/scenarios/:id/preview", scenarioController.NewScenarioLaunchController(db).PreviewScenario)
	return router
}

// previewScenario POSTs body to the preview route as userID.
func previewScenario(t *testing.T, db *gorm.DB, userID string, scenarioID uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return previewScenarioRaw(t, db, userID, scenarioID, bytes.NewReader(raw))
}

// previewScenarioRaw POSTs body as is — nil for a request with no body at all.
func previewScenarioRaw(t *testing.T, db *gorm.DB, userID string, scenarioID uuid.UUID, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/scenarios/"+scenarioID.String()+"/preview", body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	previewRouter(t, db, userID).ServeHTTP(w, req)
	return w
}

// previewedRun decodes a successful preview and waits for its build to finish.
func previewedRun(t *testing.T, db *gorm.DB, w *httptest.ResponseRecorder) (dto.LaunchScenarioResponse, models.ScenarioSession) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "the preview must start; body=%s", w.Body.String())
	var resp dto.LaunchScenarioResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	var run models.ScenarioSession
	require.Eventually(t, func() bool {
		run = models.ScenarioSession{}
		return db.Preload("StepProgress").First(&run, "id = ?", resp.ScenarioSessionID).Error == nil &&
			run.Status != "provisioning"
	}, 10*time.Second, 10*time.Millisecond, "the preview's build must finish")
	return resp, run
}

// seedOpenRun gives userID an open run of the scenario on a live terminal:
// a preview run when preview is true, a learner's run otherwise.
func seedOpenRun(t *testing.T, db *gorm.DB, userID string, scenarioID uuid.UUID, preview bool) (models.ScenarioSession, string) {
	t.Helper()
	terminalID := "open-run-terminal-" + uuid.New().String()
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID:       terminalID,
		UserID:          userID,
		State:           terminalModels.StateRunning,
		PersistenceMode: "persistent",
		ExpiresAt:       time.Now().Add(30 * time.Minute),
	}).Error)

	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	var run *models.ScenarioSession
	var err error
	if preview {
		run, err = svc.PreviewScenario(userID, scenarioID, terminalID)
	} else {
		run, err = svc.StartScenario(userID, scenarioID, terminalID, "")
	}
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, run.ID))
	require.NoError(t, db.First(run, "id = ?", run.ID).Error)
	return *run, terminalID
}

func progressByOrder(run models.ScenarioSession) map[int]models.ScenarioStepProgress {
	byOrder := make(map[int]models.ScenarioStepProgress, len(run.StepProgress))
	for _, p := range run.StepProgress {
		byOrder[p.StepOrder] = p
	}
	return byOrder
}

func TestPreviewFromStep_ReplaysSetupAndBackgroundsThroughN(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-replay")
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 3})
	resp, run := previewedRun(t, db, w)

	assert.Equal(t, "active", run.Status)
	assert.Equal(t, []string{"setup", "bg1", "bg2", "bg3"}, tt.builtSteps(),
		"step 3 is built as a learner arriving there would find it: the setup, then "+
			"every step's background script through 3, in order — and nothing of step 4")
	assert.Equal(t, resp.TerminalSessionID, *run.TerminalSessionID)
}

func TestPreviewFromStep_EarlierStepsCompleted_CurrentStepIsN(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-progress")
	newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 3})
	_, run := previewedRun(t, db, w)

	assert.True(t, run.IsPreview, "a preview from a step is still a preview")
	assert.Equal(t, 3, run.CurrentStep, "the preview starts on the chosen step")

	progress := progressByOrder(run)
	require.Len(t, progress, previewStepCount, "every step has its progress row")
	for _, order := range []int{1, 2} {
		assert.Equal(t, "completed", progress[order].Status, "step %d comes before the chosen one", order)
		assert.NotNil(t, progress[order].CompletedAt, "a completed step %d carries its completion time", order)
	}
	assert.Equal(t, "active", progress[3].Status, "the chosen step is the one being played")
	assert.Nil(t, progress[3].CompletedAt)
	assert.Equal(t, "locked", progress[4].Status, "a step after the chosen one is still ahead")
	assert.Nil(t, progress[4].CompletedAt)
}

func TestPreviewFromStep_UnknownStep_400(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-unknown-step")
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 7})

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"step 7 is no step of this scenario; body=%s", w.Body.String())
	assert.Equal(t, 0, tt.createCalls(),
		"a preview refused for its step must be refused before a terminal exists")
	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("scenario_id = ?", scenario.ID).Count(&runs).Error)
	assert.Zero(t, runs, "a refused preview leaves no run")
}

// A preview from a step fails the way a launch does: the run is setup_failed
// and its terminal stopped, not left running a half-built world.
func TestPreviewFromStep_ScriptFails_SetupFailedAndTerminalStopped(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-script-fails")
	tt := newPreviewTTBackend(t)
	tt.failOn = "echo bg-step-2"

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 3})
	resp, run := previewedRun(t, db, w)

	assert.Equal(t, "setup_failed", run.Status, "a script of the replay failed")
	assert.Equal(t, []string{"setup", "bg1", "bg2"}, tt.builtSteps(),
		"the build stops at the first script that fails")
	assert.Equal(t, []string{resp.TerminalSessionID}, tt.stoppedSessions(),
		"the failed preview's terminal is stopped, and only it")
}

// Every "Test from this step" click is a new preview. The author's previous
// one is replaced — its run abandoned and its terminal deleted — rather than
// answered 500 with the new terminal leaked, as it was.
func TestPreview_ReplacesAuthorsPreviousOpenPreviewRun(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-replace")
	previous, previousTerminal := seedOpenRun(t, db, authorID, scenario.ID, true)
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 2})
	resp, run := previewedRun(t, db, w)

	assert.NotEqual(t, previous.ID, run.ID, "a new preview run is started")
	assert.True(t, run.IsPreview)
	assert.Equal(t, 2, run.CurrentStep)
	assert.NotEqual(t, previousTerminal, resp.TerminalSessionID, "on a new terminal")

	var old models.ScenarioSession
	require.NoError(t, db.First(&old, "id = ?", previous.ID).Error)
	assert.Equal(t, "abandoned", old.Status, "the previous preview run is over")
	assert.Equal(t, []string{previousTerminal}, tt.deletedSessions(),
		"the previous preview's terminal is deleted — and nothing else — or every "+
			"click would leak one terminal's budget until its TTL")
}

// Authorization comes before the terminal: a caller who may not preview gets
// a 403 and costs nothing. It used to be checked after the terminal was
// created, leaving that terminal behind.
func TestPreview_UnauthorizedUser_CreatesNoTerminal(t *testing.T) {
	db, _, scenario := seedPreviewableScenario(t, "preview-unauthorized")
	strangerID := "preview-stranger-" + uuid.New().String()
	seedPersistencePlan(t, db, strangerID, true)
	seedPersistenceUserKey(t, db, strangerID)
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, strangerID, scenario.ID, map[string]any{})

	assert.Equal(t, http.StatusForbidden, w.Code,
		"only the author, an org manager or an admin may preview; body=%s", w.Body.String())
	assert.Equal(t, 0, tt.createCalls(),
		"tt-backend must never be asked for a terminal the caller may not use")
	var terminals int64
	require.NoError(t, db.Model(&terminalModels.Terminal{}).Where("user_id = ?", strangerID).Count(&terminals).Error)
	assert.Zero(t, terminals, "no terminal row for a refused preview")
}

// Only a preview replaces a preview. The author's own learner run of the
// scenario is real progress: previewing over it is the session_exists
// conflict, detected before any terminal exists.
func TestPreview_OpenLearnerRunOfSameScenario_Returns409(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-learner-run")
	learnerRun, learnerTerminal := seedOpenRun(t, db, authorID, scenario.ID, false)
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"from_step_order": 2})

	require.Equal(t, http.StatusConflict, w.Code,
		"a learner run is never replaced by a preview; body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason":"session_exists"`)
	assert.Equal(t, 0, tt.createCalls(), "the conflict is detected before a terminal is created")
	assert.Empty(t, tt.deletedSessions(), "the learner run's terminal is left alone")

	var kept models.ScenarioSession
	require.NoError(t, db.First(&kept, "id = ?", learnerRun.ID).Error)
	assert.Equal(t, "active", kept.Status, "the learner run is untouched")
	assert.Equal(t, learnerTerminal, *kept.TerminalSessionID)
}

// Guard: without from_step_order a preview is what it has always been — the
// first step, built as a launch builds it.
func TestPreview_WithoutFromStep_StartsAtTheFirstStep(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-first-step")
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{})
	_, run := previewedRun(t, db, w)

	assert.Equal(t, "active", run.Status)
	assert.True(t, run.IsPreview)
	assert.Equal(t, 1, run.CurrentStep)
	assert.Equal(t, []string{"setup", "bg1"}, tt.builtSteps())

	progress := progressByOrder(run)
	assert.Equal(t, "active", progress[1].Status)
	for order := 2; order <= previewStepCount; order++ {
		assert.Equal(t, "locked", progress[order].Status, "step %d", order)
	}
}

// seedOrgScenarioManager creates a team organization, a scenario of it
// created by someone else, and managerID — a manager of that organization who
// may preview it — with a terminal key but no plan of their own. It returns
// the organization.
func seedOrgScenarioManager(t *testing.T, db *gorm.DB, scenario *models.Scenario, managerID string) uuid.UUID {
	t.Helper()
	org := orgModels.Organization{
		Name:             "preview-org-" + uuid.New().String(),
		DisplayName:      "Preview Org",
		OwnerUserID:      scenario.CreatedByID,
		OrganizationType: orgModels.OrgTypeTeam,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         managerID,
		Role:           orgModels.OrgRoleManager,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
	require.NoError(t, db.Model(scenario).Update("organization_id", org.ID).Error)
	seedPersistenceUserKey(t, db, managerID)
	return org.ID
}

// The editor previews an org scenario in the org it is working in, and says
// so in the body (scenarioSessionService.ts previewScenario sends
// organization_id there, never in the query). The plan is that org's: a
// manager whose only plan comes from the org must be able to preview, on a
// terminal of that org.
func TestPreview_OrgScenario_UsesTheBodyOrganisationPlan(t *testing.T) {
	db, _, scenario := seedPreviewableScenario(t, "preview-org-plan")
	managerID := "preview-org-manager-" + uuid.New().String()
	orgID := seedOrgScenarioManager(t, db, scenario, managerID)

	// The org grants its managers a plan through a role mapping — a plan the
	// manager holds only in this org's context, not globally.
	plan := paymentModels.SubscriptionPlan{
		Name:                      "OrgManagerPlan",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxCPU:                    8000,
		MaxMemoryMB:               8192,
		DataPersistenceEnabled:    true,
		IsActive:                  true,
		BillingInterval:           "month",
		Currency:                  "eur",
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&paymentModels.OrganizationRolePlan{
		OrganizationID:     orgID,
		Role:               string(orgModels.OrgRoleManager),
		SubscriptionPlanID: plan.ID,
	}).Error)
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, managerID, scenario.ID, map[string]any{"organization_id": orgID.String()})
	resp, _ := previewedRun(t, db, w)

	assert.Equal(t, 1, tt.createCalls())
	var terminal terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", resp.TerminalSessionID).First(&terminal).Error)
	require.NotNil(t, terminal.OrganizationID, "the preview's terminal belongs to an organization")
	assert.Equal(t, orgID, *terminal.OrganizationID, "the preview's terminal is the org's")
}

// Admin/Scenarios.vue posts the preview with no body at all: that is the
// plain preview from the first step, not a malformed request.
func TestPreview_NoBody_BehavesAsToday(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-no-body")
	tt := newPreviewTTBackend(t)

	w := previewScenarioRaw(t, db, authorID, scenario.ID, nil)
	_, run := previewedRun(t, db, w)

	assert.True(t, run.IsPreview)
	assert.Equal(t, 1, run.CurrentStep)
	assert.Equal(t, []string{"setup", "bg1"}, tt.builtSteps())
}

// A body that is not JSON is refused before anything is created: guessing
// what it meant could preview the wrong step.
func TestPreview_MalformedJSON_400(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-malformed")
	tt := newPreviewTTBackend(t)

	w := previewScenarioRaw(t, db, authorID, scenario.ID, strings.NewReader(`{"from_step_order": `))

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, 0, tt.createCalls(), "a refused preview creates no terminal")
}

// A preview run is a preview from the moment its row exists. Written as a
// learner run and flagged afterwards, it spends a window — the whole
// synchronous part of its start, build launch included — looking like real
// progress to everything that tells the two apart: a second preview gets 409
// instead of replacing it, the zombie rules treat it as a learner's.
//
// Observable: the is_preview column read back, inside the inserting
// transaction, right after the INSERT of the scenario_sessions row.
func TestPreview_RunIsMarkedPreviewFromTheStart(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-marked-at-insert")
	tt := newPreviewTTBackend(t)

	var mu sync.Mutex
	var insertedAsPreview []bool
	const callback = "test:read-back-inserted-run"
	require.NoError(t, db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		run, ok := tx.Statement.Dest.(*models.ScenarioSession)
		if tx.Statement.Table != "scenario_sessions" || !ok || tx.Error != nil {
			return
		}
		var isPreview bool
		assert.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Table("scenario_sessions").
			Select("is_preview").Where("id = ?", run.ID).Scan(&isPreview).Error)
		mu.Lock()
		insertedAsPreview = append(insertedAsPreview, isPreview)
		mu.Unlock()
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{})
	previewedRun(t, db, w)

	assert.Equal(t, 1, tt.createCalls())
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []bool{true}, insertedAsPreview,
		"the preview run's row is inserted with is_preview already true")
}

// The preview is authorized twice: by the controller before the terminal
// exists, and by the service when the run is created. When the second one
// refuses — here the caller lost their manager role while the terminal was
// being created — that is still a 403, and the terminal it no longer may use
// is deleted.
func TestPreview_AuthorisationLostAfterTerminalCreated_403AndTerminalDeleted(t *testing.T) {
	db, _, scenario := seedPreviewableScenario(t, "preview-late-refusal")
	managerID := "preview-late-manager-" + uuid.New().String()
	orgID := seedOrgScenarioManager(t, db, scenario, managerID)
	seedPersistencePlan(t, db, managerID, true)
	tt := newPreviewTTBackend(t)
	tt.onCreate = func() string {
		assert.NoError(t, db.Model(&orgModels.OrganizationMember{}).
			Where("organization_id = ? AND user_id = ?", orgID, managerID).
			Update("role", orgModels.OrgRoleMember).Error)
		return ""
	}

	w := previewScenario(t, db, managerID, scenario.ID, map[string]any{})

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a refused preview is a 403 whichever check refused it; body=%s", w.Body.String())
	require.Equal(t, 1, tt.createCalls(), "the refusal came after the terminal was created")
	tt.assertNewTerminalDeleted(t, db)
	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("scenario_id = ?", scenario.ID).Count(&runs).Error)
	assert.Zero(t, runs, "a refused preview leaves no run")
}

// A platform administrator previews any scenario, an org's included, without
// being a member of that org — as Admin/Scenarios.vue does, with no body. The
// org's plan is not theirs to spend: the preview runs on the admin's own plan,
// on a terminal outside the org.
func TestPreview_AdminNotMemberOfTheScenarioOrg_UsesTheirOwnPlan(t *testing.T) {
	db, _, scenario := seedPreviewableScenario(t, "preview-admin-other-org")
	seedOrgScenarioManager(t, db, scenario, "preview-admin-org-manager-"+uuid.New().String())
	adminID := "preview-admin-" + uuid.New().String()
	seedPersistencePlan(t, db, adminID, true)
	seedPersistenceUserKey(t, db, adminID)
	tt := newPreviewTTBackend(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenarios/"+scenario.ID.String()+"/preview", nil)
	w := httptest.NewRecorder()
	setupPreviewRouterWithAdminStub(t, db, adminID).ServeHTTP(w, req)
	resp, _ := previewedRun(t, db, w)

	assert.Equal(t, 1, tt.createCalls())
	var terminal terminalModels.Terminal
	require.NoError(t, db.Where("session_id = ?", resp.TerminalSessionID).First(&terminal).Error)
	assert.Nil(t, terminal.OrganizationID,
		"the admin's preview runs on their own plan, on a terminal outside the org")
}

// An organization_id that is not an id is refused before anything is created.
func TestPreview_InvalidOrganisationID_400(t *testing.T) {
	db, authorID, scenario := seedPreviewableScenario(t, "preview-invalid-org")
	tt := newPreviewTTBackend(t)

	w := previewScenario(t, db, authorID, scenario.ID, map[string]any{"organization_id": "not-a-uuid"})

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, 0, tt.createCalls(), "a refused preview creates no terminal")
}
