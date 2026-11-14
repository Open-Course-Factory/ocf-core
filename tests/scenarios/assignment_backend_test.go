package scenarios_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/hooks"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// --- Launch reads the backend the teacher chose ---

// assignWithBackend writes the row directly, as a stale or hand-edited one
// would be: the launch must hold to it without trusting it.
func assignWithBackend(t *testing.T, db *gorm.DB, a *models.ScenarioAssignment, backend string) {
	t.Helper()
	require.NoError(t, db.Model(a).Update("backend", backend).Error)
}

func TestLaunchScenario_HoldsToTheAssignedBackend(t *testing.T) {
	db := freshTestDB(t)
	scenario := newLaunchableScenario(t, db, "assigned-backend")
	group := newGroupWithMember(t, db, "assigned-backend-class", "learner-ab")
	assignWithBackend(t, db, createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group"), "elsewhere")

	w := postLaunch(setupTestRouterWithRoles(db, "learner-ab", []string{"member"}), scenario.ID.String())

	assert.Equal(t, http.StatusForbidden, w.Code,
		"the assigned backend is not one this learner may use: refused, not swapped; body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "not allowed")
}

func TestLaunchScenario_ClassAssignmentBackendWinsOverOrgOne(t *testing.T) {
	db := freshTestDB(t)
	userID := "learner-precedence"
	org := orgModels.Organization{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "precedence-org", DisplayName: "precedence-org", OwnerUserID: "owner",
		IsActive: true, OrganizationType: orgModels.OrgTypeTeam, MaxGroups: 5, MaxMembers: 50,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, OrganizationID: org.ID, UserID: userID, Role: "member", IsActive: true,
	}).Error)

	scenario := newLaunchableScenario(t, db, "precedence")
	group := newGroupWithMember(t, db, "precedence-class", userID)
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group") // automatic
	assignWithBackend(t, db, createScenarioAssignment(t, db, scenario.ID, nil, &org.ID, "org"), "elsewhere")

	w := postLaunch(setupTestRouterWithRoles(db, userID, []string{"member"}), scenario.ID.String())

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"the class assignment (automatic) wins over the org one; body=%s", w.Body.String())
}

// --- The backend is checked when the assignment is saved ---

// policyOf serves a fixed policy and refuses to be asked with a plan: a class's
// server is checked against its organization's servers, not the teacher's plan.
func policyOf(t *testing.T, allowed ...string) func(*uuid.UUID, *paymentModels.SubscriptionPlan) (ttServices.BackendPolicy, error) {
	return func(_ *uuid.UUID, plan *paymentModels.SubscriptionPlan) (ttServices.BackendPolicy, error) {
		assert.Nil(t, plan, "a class-wide choice must not read the teacher's own plan")
		return ttServices.BackendPolicy{Allowed: allowed, Default: allowed[0]}, nil
	}
}

func runBackendHook(t *testing.T, db *gorm.DB, hookType hooks.HookType, oldEntity, newEntity any) error {
	t.Helper()
	hook := scenarioHooks.NewScenarioAssignmentBackendHook(db, policyOf(t, "a", "b"))
	return hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioAssignment",
		HookType:   hookType,
		OldEntity:  oldEntity,
		NewEntity:  newEntity,
		UserID:     "teacher-1",
		Context:    context.Background(),
	})
}

func TestScenarioAssignmentBackendHook_Create(t *testing.T) {
	db := freshTestDB(t)
	orgID := uuid.New()
	assignment := func(backend string) *models.ScenarioAssignment {
		return &models.ScenarioAssignment{ScenarioID: uuid.New(), OrganizationID: &orgID, Scope: "org", Backend: backend}
	}

	assert.NoError(t, runBackendHook(t, db, hooks.BeforeCreate, nil, assignment("")), "empty = automatic")
	assert.NoError(t, runBackendHook(t, db, hooks.BeforeCreate, nil, assignment("b")))
	assert.ErrorIs(t, runBackendHook(t, db, hooks.BeforeCreate, nil, assignment("elsewhere")), ttServices.ErrBackendNotAllowed)
}

func TestScenarioAssignmentBackendHook_Update(t *testing.T) {
	db := freshTestDB(t)
	orgID := uuid.New()
	existing := &models.ScenarioAssignment{ScenarioID: uuid.New(), OrganizationID: &orgID, Scope: "org"}

	assert.NoError(t, runBackendHook(t, db, hooks.BeforeUpdate, existing, map[string]any{"deadline": "x"}),
		"an update that leaves the backend alone is not checked")
	assert.NoError(t, runBackendHook(t, db, hooks.BeforeUpdate, existing, map[string]any{"backend": "a"}))
	assert.NoError(t, runBackendHook(t, db, hooks.BeforeUpdate, existing, map[string]any{"backend": ""}), "back to automatic")
	assert.ErrorIs(t, runBackendHook(t, db, hooks.BeforeUpdate, existing, map[string]any{"backend": "elsewhere"}), ttServices.ErrBackendNotAllowed)
}

// --- Launch honours the backend the learner requested (#582) ---

// launchRequestingBackend launches a scenario for a learner whose plan allows
// host-a and host-b, whose class assignment names assigned ("" = automatic),
// asking for requested ("" = none). The witness is the host the fake
// tt-backend was asked to build on.
func launchRequestingBackend(t *testing.T, assigned, requested string) (*httptest.ResponseRecorder, *persistenceRecorder) {
	t.Helper()
	db, userID, scenario := seedLaunchableScenario(t, "requested-backend")
	var plan paymentModels.SubscriptionPlan
	require.NoError(t, db.Where("name = ?", "PersistencePlan").First(&plan).Error)
	plan.AllowedBackends = []string{"host-a", "host-b"}
	require.NoError(t, db.Save(&plan).Error)
	if assigned != "" {
		group := newGroupWithMember(t, db, "requested-backend-class", userID)
		assignWithBackend(t, db, createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group"), assigned)
	}

	ttSrv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, ttSrv.URL)

	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String(), "backend": requested})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	setupPersistenceRouter(t, db, userID).ServeHTTP(w, req)
	return w, rec
}

func assertLaunchedOn(t *testing.T, w *httptest.ResponseRecorder, rec *persistenceRecorder, backend string) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Equal(t, 1, rec.calls)
	assert.Equal(t, backend, rec.backend, "the terminal must be built on %s", backend)
}

func TestLaunchScenario_RequestedBackend_NoAssignedOne_LaunchesThere(t *testing.T) {
	w, rec := launchRequestingBackend(t, "", "host-b")
	assertLaunchedOn(t, w, rec, "host-b")
}

func TestLaunchScenario_RequestedBackendNotAllowed_Returns403(t *testing.T) {
	w, rec := launchRequestingBackend(t, "", "elsewhere")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"a server outside the policy is refused, never swapped; body=%s", w.Body.String())
	assert.Equal(t, 0, rec.calls, "no terminal may be created")
}

func TestLaunchScenario_RequestedBackendDiffersFromAssignedOne_Returns409(t *testing.T) {
	w, rec := launchRequestingBackend(t, "host-a", "host-b")
	assert.Equal(t, http.StatusConflict, w.Code,
		"the assignment pins the server: asking for another one is a conflict; body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "host-a", "the answer must name the assigned server")
	assert.Equal(t, 0, rec.calls, "no terminal may be created")
}

func TestLaunchScenario_RequestedBackendIsTheAssignedOne_LaunchesThere(t *testing.T) {
	w, rec := launchRequestingBackend(t, "host-a", "host-a")
	assertLaunchedOn(t, w, rec, "host-a")
}

func TestLaunchScenario_NoRequestedBackend_LaunchesOnTheAssignedOne(t *testing.T) {
	w, rec := launchRequestingBackend(t, "host-a", "")
	assertLaunchedOn(t, w, rec, "host-a")
}
