package scenarios_test

// Layer-1 Casbin policy contract for the ScenarioStep and ScenarioStepQuestion
// entity routes.
//
// The current authorization model is two-layered:
//
//   - Layer 1 (entity Roles map): grants Member full CRUD on the auto-generated
//     /scenario-steps and /scenario-step-questions routes.
//   - Layer 2 (hooks in src/scenarios/hooks/scenarioStepHooks.go): gates writes
//     to scenarios the user can manage (creator / org-manager / group-manager
//     via assignment). The hook-level tests live in
//     tests/scenarios/scenarioStepAuthorization_test.go.
//
// These two layers are coupled. If someone reverts the Layer 1 change (e.g.
// removes Member from the Roles map back to admin-only), the hook tests would
// keep passing on their own — they call the hook function directly and never
// hit Casbin. The HTTP request would 403 at Layer 1 before reaching the hooks.
//
// This file pins the Layer 1 contract. If it fails, someone narrowed the
// permitted methods or roles for these entity routes — confirm the change was
// intended and update both layers together.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
)

// expectedCRUD lists every HTTP method that Member must keep on these routes.
// Adding a new method here should be a deliberate choice paired with an
// updated hook + request-path test.
var expectedCRUD = []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete}

func TestScenarioStep_Layer1_MemberHasFullCRUD(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenarioStep(svc)

	all := svc.GetAllEntityRoles()
	roles, ok := all["ScenarioStep"]
	require.True(t, ok, "ScenarioStep must be registered")

	memberMethods, ok := roles.Roles[string(authModels.Member)]
	require.True(t, ok, "Member must be in the Roles map for ScenarioStep — "+
		"removing Member would silently 403 every org-manager and group-manager "+
		"trying to add a step from the editor")

	for _, m := range expectedCRUD {
		assert.Contains(t, memberMethods, m,
			"ScenarioStep Layer 1: Member must keep %s — Layer 2 hook gates writes "+
				"to manageable scenarios; if you remove a method here, also update "+
				"hooks/scenarioStepHooks.go and the editor", m)
	}

	adminMethods, ok := roles.Roles[string(authModels.Admin)]
	require.True(t, ok, "Admin must remain in the Roles map (audit trail / break-glass)")
	for _, m := range expectedCRUD {
		assert.Contains(t, adminMethods, m, "Admin keeps full CRUD on ScenarioStep")
	}
}

func TestScenarioStepQuestion_Layer1_MemberHasFullCRUD(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenarioStepQuestion(svc)

	all := svc.GetAllEntityRoles()
	roles, ok := all["ScenarioStepQuestion"]
	require.True(t, ok, "ScenarioStepQuestion must be registered")

	memberMethods, ok := roles.Roles[string(authModels.Member)]
	require.True(t, ok, "Member must be in the Roles map for ScenarioStepQuestion — "+
		"the editor's question CRUD goes through this entity route")

	for _, m := range expectedCRUD {
		assert.Contains(t, memberMethods, m,
			"ScenarioStepQuestion Layer 1: Member must keep %s — Layer 2 hook "+
				"transitively checks the parent step → scenario manageability", m)
	}
}

// Sanity guard: the current Roles map opens routes to "Member" + "Admin"
// strings. If someone renames the Member role constant or introduces a new
// role tier, this test signals that the audit needs revisiting.
func TestScenarioStep_Layer1_NoUnexpectedRoles(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenarioStep(svc)
	scenarioRegistration.RegisterScenarioStepQuestion(svc)

	all := svc.GetAllEntityRoles()
	for _, entity := range []string{"ScenarioStep", "ScenarioStepQuestion"} {
		roles, ok := all[entity]
		require.True(t, ok)
		for role := range roles.Roles {
			assert.Contains(t,
				[]string{string(authModels.Member), string(authModels.Admin)},
				role,
				"%s entity should only grant access to Member and Admin — got %q. "+
					"If you add a new role, update both this audit and hooks/scenarioStepHooks.go.",
				entity, role)
		}
	}
}
