package authorization_tests

// Contract for the class archiving routes (#491). They are entity actions on
// the ClassGroup registration — archive/unarchive synthesized from
// `Archivable: true`, archive-preview declared in its ActionConfig — so their
// Layer 1 policy and Layer 2 RoutePermission come from the registration alone,
// not from a module permissions.go. Helpers collectPolicies / assertPolicy live
// in all_permissions_test.go.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	groupRegistration "soli/formations/src/groups/entityRegistration"
)

var classArchiveActionRoutes = []struct {
	path   string
	method string
}{
	{"/api/v1/class-groups/:id/archive", http.MethodPost},
	{"/api/v1/class-groups/:id/unarchive", http.MethodPost},
	{"/api/v1/class-groups/:id/archive-preview", http.MethodGet},
}

func bootClassGroupRegistration(t *testing.T) *mocks.MockEnforcer {
	t.Helper()
	mock := mocks.NewMockEnforcer()
	orig := casdoor.Enforcer
	casdoor.Enforcer = mock
	access.RouteRegistry.Reset()
	t.Cleanup(func() {
		casdoor.Enforcer = orig
		access.RouteRegistry.Reset()
	})
	groupRegistration.RegisterGroup(ems.NewEntityRegistrationService())
	return mock
}

func TestClassGroupRegistration_RegistersLayer1MemberPolicyForArchiveRoutes(t *testing.T) {
	ps := collectPolicies(bootClassGroupRegistration(t))
	for _, r := range classArchiveActionRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, access.RoleMember, r.path, r.method)
		})
	}
}

// Layer 2 carries no gate of its own on these routes, exactly like PATCH on a
// class: archive/unarchive inherit the entity's Update rule, the preview is
// declared SelfScoped. The authority is GroupWriteAuthorizationHook
// (archive/unarchive) and the preview handler's own CanUserManageGroup check.
func TestClassGroupRegistration_RegistersLayer2RoutePermissionsLikePatch(t *testing.T) {
	bootClassGroupRegistration(t)

	var updateRule access.AccessRule
	for _, entity := range access.RouteRegistry.GetReference().Entities {
		if entity.Entity == "ClassGroup" {
			updateRule = entity.Update
		}
	}
	require.NotEmpty(t, updateRule.Type, "ClassGroup CRUD permissions must be registered")

	expected := map[string]access.AccessRuleType{
		"/api/v1/class-groups/:id/archive":         updateRule.Type,
		"/api/v1/class-groups/:id/unarchive":       updateRule.Type,
		"/api/v1/class-groups/:id/archive-preview": access.SelfScoped,
	}
	for _, r := range classArchiveActionRoutes {
		perm, found := access.RouteRegistry.Lookup(r.method, r.path)
		require.True(t, found, "expected a RoutePermission for %s %s", r.method, r.path)
		assert.Equal(t, access.RoleMember, perm.Role, r.path)
		assert.Equal(t, expected[r.path], perm.Access.Type, r.path)
		assert.Equal(t, "ClassGroup", perm.Category, r.path)
	}
}
