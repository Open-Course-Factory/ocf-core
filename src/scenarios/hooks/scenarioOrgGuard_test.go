package scenarioHooks

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// The generic PATCH maps organization_id to a uuid.UUID and is_public to a
// bool, but the guards must not rely on it: a value of any other type is
// refused, never read as "not patched".

func assertRefusedWith(t *testing.T, err error, status int) {
	t.Helper()
	require.Error(t, err, "an unexpected value type must be refused, not ignored")
	var entityErr *entityErrors.EntityError
	require.True(t, errors.As(err, &entityErr), "want a structured refusal, got %T: %v", err, err)
	assert.Equal(t, status, entityErr.HTTPStatus)
}

func orgScenarioContext(patch map[string]any) (*hooks.HookContext, uuid.UUID) {
	orgA := uuid.New()
	old := &models.Scenario{Name: "s", OrganizationID: &orgA}
	return &hooks.HookContext{
		EntityName: "Scenario", HookType: hooks.BeforeUpdate,
		OldEntity: old, NewEntity: patch,
		UserID: "org-a-manager", UserRoles: []string{"member"},
	}, orgA
}

func TestRefuseOrgChange_FailsClosedOnOddValues(t *testing.T) {
	orgB := uuid.New()
	cases := map[string]any{
		"string":      orgB.String(),
		"*uuid.UUID":  &orgB,
		"untyped nil": nil,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := orgScenarioContext(map[string]any{"organization_id": value})
			assertRefusedWith(t, refuseOrgChange(ctx), http.StatusForbidden)
		})
	}
}

func TestRefuseOrgChange_PlatformScenarioCannotJoinAnOrg(t *testing.T) {
	orgA := uuid.New()
	ctx := &hooks.HookContext{
		EntityName: "Scenario", HookType: hooks.BeforeUpdate,
		OldEntity: &models.Scenario{Name: "platform"}, NewEntity: map[string]any{"organization_id": orgA},
		UserID: "test-creator", UserRoles: []string{"member"},
	}
	assertRefusedWith(t, refuseOrgChange(ctx), http.StatusForbidden)
}

func TestRefusePublicOrgScenario_FailsClosedOnOddValues(t *testing.T) {
	yes := true
	cases := map[string]any{
		"string":  "true",
		"*bool":   &yes,
		"integer": 1,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := orgScenarioContext(map[string]any{"is_public": value})
			assertRefusedWith(t, refusePublicOrgScenario(ctx), http.StatusBadRequest)
		})
	}
	t.Run("organization_id of an odd type beside is_public", func(t *testing.T) {
		ctx, orgA := orgScenarioContext(nil)
		ctx.NewEntity = map[string]any{"is_public": true, "organization_id": orgA.String()}
		assertRefusedWith(t, refusePublicOrgScenario(ctx), http.StatusBadRequest)
	})
}
