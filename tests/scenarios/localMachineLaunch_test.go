package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// localLaunchFixture is a learner on a plan, a scenario set to `machine`, the
// platform switch as given, and a fake tt-backend that records every session
// it is asked to build.
func localLaunchFixture(t *testing.T, switchOn bool, planSetting, machine string) (*gorm.DB, string, *models.Scenario, *persistenceRecorder) {
	t.Helper()
	db := freshTestDB(t)
	userID := "local-launch-" + uuid.NewString()
	seedPersistencePlan(t, db, userID, false)
	require.NoError(t, db.Model(&paymentModels.SubscriptionPlan{}).Where("name = ?", "PersistencePlan").
		Update("local_machines", planSetting).Error)
	seedPersistenceUserKey(t, db, userID)
	scenario := seedPersistenceScenario(t, db, userID, false)
	require.NoError(t, db.Model(scenario).Update("machine", machine).Error)
	if switchOn {
		require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	}
	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)
	return db, userID, scenario, rec
}

func decodeLaunch(t *testing.T, body []byte) dto.LaunchScenarioResponse {
	t.Helper()
	var resp dto.LaunchScenarioResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

func TestLaunchScenario_LocalMachineWhenSwitchPlanAndScenarioAgree(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := decodeLaunch(t, w.Body.Bytes())

	assert.True(t, strings.HasPrefix(resp.TerminalSessionID, terminalModels.LocalSessionIDPrefix), resp.TerminalSessionID)
	assert.Equal(t, models.ScenarioMachineLocal, resp.Machine)
	assert.Equal(t, 0, rec.calls, "no container is built for a local run")

	var run models.ScenarioSession
	require.NoError(t, db.First(&run, "id = ?", resp.ScenarioSessionID).Error)
	assert.Equal(t, models.ScenarioMachineLocal, run.Machine, "the run records its machine")
}

func TestLaunchScenario_SwitchOffStaysOnIncus(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, false, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := decodeLaunch(t, w.Body.Bytes())

	assert.Equal(t, 1, rec.calls)
	assert.False(t, terminalModels.IsLocalSessionID(resp.TerminalSessionID))
	assert.Equal(t, models.ScenarioMachineIncus, resp.Machine)
	var run models.ScenarioSession
	require.NoError(t, db.First(&run, "id = ?", resp.ScenarioSessionID).Error)
	assert.Equal(t, models.ScenarioMachineIncus, run.Machine)
}

func TestLaunchScenario_RefusesLocalForWhatTheLearnerCouldFake(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
	require.NoError(t, db.Model(scenario).Update("crash_traps", true).Error)

	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "container")
	assert.Equal(t, 0, rec.calls)

	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("user_id = ?", userID).Count(&runs).Error)
	assert.Zero(t, runs)
	var terminals int64
	require.NoError(t, db.Model(&terminalModels.Terminal{}).Where("user_id = ?", userID).Count(&terminals).Error)
	assert.Zero(t, terminals, "a refused launch leaves no terminal holding budget")
}

func TestPreviewScenario_LocalMachineLikeALaunch(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := previewScenario(t, db, userID, scenario.ID, map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := decodeLaunch(t, w.Body.Bytes())

	assert.True(t, terminalModels.IsLocalSessionID(resp.TerminalSessionID), resp.TerminalSessionID)
	assert.Equal(t, 0, rec.calls)
}

// A run keeps its machine (plan rule 6): rebuilding a local run whose machine
// is gone gives it a local machine again, whatever the switch says now.
func TestResumeRebuild_LocalRunStaysLocal(t *testing.T) {
	f := seedResumableRun(t, "resume-local", resumeSeed{})
	require.NoError(t, f.db.Model(&f.run).Update("machine", models.ScenarioMachineLocal).Error)
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := decodeLaunch(t, w.Body.Bytes())

	assert.True(t, terminalModels.IsLocalSessionID(resp.TerminalSessionID), resp.TerminalSessionID)
	assert.Equal(t, models.ScenarioMachineLocal, resp.Machine)
	assert.Equal(t, 0, tt.createCalls(), "no container is built to rebuild a local run")
}

// A class bulk start builds machines ahead of the learners. A learner whose run
// would be local has nothing to build ahead — their browser is not open — so
// they are left to launch it themselves, still on a local machine (rule 4), and
// the trainer is told why. A learner whose plan keeps them on Incus is started.
func TestBulkStartScenario_LeavesLocalRunsToTheLearner(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	scenario := models.Scenario{Name: "bulk-local", Title: "Bulk Local", InstanceType: "s", CreatedByID: "c1",
		Machine: models.ScenarioMachineLocal}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal"}).Error)

	groupID := createClassGroup(t, db, "bulk-local-class", "trainer", nil).ID
	ttMock := newMockTTService()
	for userID, setting := range map[string]string{
		"bulk-local-allowed": paymentModels.LocalMachinesAllowed,
		"bulk-local-off":     paymentModels.LocalMachinesOff,
	} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: userID, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
		seedPersistencePlan(t, db, userID, false)
		var sub paymentModels.UserSubscription
		require.NoError(t, db.First(&sub, "user_id = ?", userID).Error)
		require.NoError(t, db.Model(&paymentModels.SubscriptionPlan{}).Where("id = ?", sub.SubscriptionPlanID).
			Update("local_machines", setting).Error)
		ttMock.addKey(userID)
	}

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID,
		services.ScenarioProvisioning{Distribution: "debian", Size: "s"}, 0, "trainer")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Skipped)
	require.Len(t, result.SkippedUsers, 1)
	assert.Equal(t, services.SkippedLearner{UserID: "bulk-local-allowed", Reason: services.LocalMachineReason}, result.SkippedUsers[0])

	var localRuns int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("user_id = ?", "bulk-local-allowed").Count(&localRuns).Error)
	assert.Zero(t, localRuns)
}

func launchWithBody(t *testing.T, db *gorm.DB, userID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/launch", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	setupPersistenceRouter(t, db, userID).ServeHTTP(w, req)
	return w
}

// A Local scenario in a browser that cannot run its machine is refused with
// a code the launcher maps, and leaves nothing behind.
func TestLaunchScenario_LocalMachineUnavailableRefusesALocalScenario(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String(), "local_machine_unavailable": "browser"})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "local_machine_unavailable", body["reason"])
	assert.Equal(t, 0, rec.calls)
	var terminals int64
	require.NoError(t, db.Model(&terminalModels.Terminal{}).Where("user_id = ?", userID).Count(&terminals).Error)
	assert.Zero(t, terminals)
}

// The same browser launching a By account scenario gets a container.
func TestLaunchScenario_LocalMachineUnavailableSendsByAccountToIncus(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesByAccountDefault, models.ScenarioMachineByAccount)

	w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String(), "local_machine_unavailable": "device"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := decodeLaunch(t, w.Body.Bytes())
	assert.Equal(t, models.ScenarioMachineIncus, resp.Machine)
	assert.Equal(t, 1, rec.calls)
}

func TestLaunchScenario_LocalMachineUnavailableMustBeAKnownReason(t *testing.T) {
	db, userID, scenario, _ := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String(), "local_machine_unavailable": "bogus"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

func TestPreviewScenario_LocalMachineUnavailableLikeALaunch(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)

	w := previewScenario(t, db, userID, scenario.ID, map[string]any{"local_machine_unavailable": "memory"})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "local_machine_unavailable")
	assert.Equal(t, 0, rec.calls)

	w = previewScenario(t, db, userID, scenario.ID, map[string]any{"local_machine_unavailable": "nope"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

// fillBudget leaves userID's plan no room for any container.
func fillBudget(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Model(&paymentModels.SubscriptionPlan{}).Where("name = ?", "PersistencePlan").
		Updates(map[string]any{"max_cpu": 100, "max_memory_mb": 100}).Error)
}

// A run in the browser reserves none of the plan's CPU/RAM budget (plan rule
// 8, 2026-11-28): it starts on a budget no container fits in, and its card
// says it can be launched.
func TestLaunchScenario_LocalRunNeedsNoBudget(t *testing.T) {
	db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
	fillBudget(t, db)

	got := cards(t, db, userID)
	require.Len(t, got, 1)
	assert.Equal(t, true, got[0]["launchable"], "%v", got[0])

	w := launchScenarioForTest(t, setupPersistenceRouter(t, db, userID), scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, models.ScenarioMachineLocal, decodeLaunch(t, w.Body.Bytes()).Machine)
	assert.Equal(t, 0, rec.calls)
}

// One run in the browser at a time per learner: another local launch while
// one is live is refused with a code the launcher maps.
func TestLaunchScenario_SecondLocalRunIsRefused(t *testing.T) {
	db, userID, scenario, _ := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
	other := seedPersistenceScenario(t, db, userID, false)
	require.NoError(t, db.Model(other).Update("machine", models.ScenarioMachineLocal).Error)
	router := setupPersistenceRouter(t, db, userID)

	require.Equal(t, http.StatusOK, launchScenarioForTest(t, router, scenario.ID).Code)
	w := launchScenarioForTest(t, router, other.ID)
	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "local_machine_busy", body["reason"])
	var runs int64
	require.NoError(t, db.Model(&models.ScenarioSession{}).Where("scenario_id = ?", other.ID).Count(&runs).Error)
	assert.Zero(t, runs)
}

// Plan rule 3 (2026-11-28), on an Allowed plan: a By account lab goes to a
// container while one fits the budget, and to the browser when none does —
// the launch and the card say so with machine_reason.
func TestLaunchScenario_ByAccountOnAnAllowedPlanFollowsTheBudget(t *testing.T) {
	t.Run("fits", func(t *testing.T) {
		db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineByAccount)
		card := cards(t, db, userID)[0]
		assert.Equal(t, models.ScenarioMachineIncus, card["machine"])
		assert.Nil(t, card["machine_reason"])

		w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String()})
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, models.ScenarioMachineIncus, body["machine"])
		assert.Nil(t, body["machine_reason"])
		assert.Equal(t, 1, rec.calls)
	})

	t.Run("full", func(t *testing.T) {
		db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineByAccount)
		fillBudget(t, db)
		card := cards(t, db, userID)[0]
		assert.Equal(t, true, card["launchable"], "%v", card)
		assert.Equal(t, models.ScenarioMachineLocal, card["machine"])
		assert.Equal(t, "quota", card["machine_reason"])

		w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String()})
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, models.ScenarioMachineLocal, body["machine"])
		assert.Equal(t, "quota", body["machine_reason"])
		assert.Equal(t, 0, rec.calls)
	})

	t.Run("full, and the browser cannot run it", func(t *testing.T) {
		db, userID, scenario, rec := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineByAccount)
		fillBudget(t, db)

		w := launchWithBody(t, db, userID, map[string]any{"scenario_id": scenario.ID.String(), "local_machine_unavailable": "memory"})
		assert.Equal(t, http.StatusForbidden, w.Code, "the budget refusal: body=%s", w.Body.String())
		assert.Equal(t, 0, rec.calls)
	})
}
