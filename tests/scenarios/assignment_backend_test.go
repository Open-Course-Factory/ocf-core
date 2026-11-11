package scenarios_test

import (
	"context"
	"net/http"
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
