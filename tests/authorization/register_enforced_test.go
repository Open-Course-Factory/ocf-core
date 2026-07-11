package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/access"
)

// These tests pin the intended API of access.RegisterEnforced, the single
// helper MR N introduces to collapse the two-call "ReconcilePolicy (Layer 1) +
// RouteRegistry.Register (Layer 2)" idiom into one declaration per route.
//
// They are RED until the implementation lands: RegisterEnforced, the two new
// RoutePermission fields (CasbinPath, NoGateway), and the RoleMember /
// RoleAdministrator constants do not exist yet, so the package fails to COMPILE.
// That compile failure is the initial red state — the tests are written against
// the intended API so the implementer has an executable contract to satisfy.
//
// They reuse the stateful fake enforcer (newStatefulEnforcer / policyStore /
// containsTriple) defined in reconcile_policy_test.go — same package — so the
// Casbin rows RegisterEnforced actually produces are asserted, not just the
// mock call count.

// TestRegisterEnforced_DerivesCasbinAndRegistry_ForEachPerm is the core
// contract: for every RoutePermission passed, RegisterEnforced must (1) make it
// Lookup-able in the RouteRegistry under its Method:Path (Layer 2) AND (2) add a
// Casbin policy row for the exact (Role, Path, Method) triple (Layer 1). This
// must hold across roles — a member EntityOwner route and an administrator
// AdminOnly route registered in the same call.
func TestRegisterEnforced_DerivesCasbinAndRegistry_ForEachPerm(t *testing.T) {
	access.RouteRegistry.Reset()
	defer access.RouteRegistry.Reset()

	enforcer, store := newStatefulEnforcer()

	memberPerm := access.RoutePermission{
		Path: "/api/v1/widgets/:id", Method: "PATCH",
		Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Widget", Field: "UserID"},
		Description: "Update own widget",
	}
	adminPerm := access.RoutePermission{
		Path: "/api/v1/widgets/purge", Method: "POST",
		Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
		Description: "Purge widgets (admin only)",
	}

	access.RegisterEnforced(enforcer, "Widgets", memberPerm, adminPerm)

	// Layer 2: both routes must be Lookup-able under their Method:Path.
	gotMember, foundMember := access.RouteRegistry.Lookup("PATCH", "/api/v1/widgets/:id")
	require.True(t, foundMember, "member route must be registered in RouteRegistry")
	assert.Equal(t, "member", gotMember.Role)
	assert.Equal(t, access.EntityOwner, gotMember.Access.Type)
	assert.Equal(t, "Widgets", gotMember.Category, "category must be stamped on the registered perm")

	gotAdmin, foundAdmin := access.RouteRegistry.Lookup("POST", "/api/v1/widgets/purge")
	require.True(t, foundAdmin, "admin route must be registered in RouteRegistry")
	assert.Equal(t, "administrator", gotAdmin.Role)
	assert.Equal(t, access.AdminOnly, gotAdmin.Access.Type)

	// Layer 1: each route must have produced a Casbin row for its exact triple,
	// with the matching role (member vs administrator).
	snapshot := store.snapshot()
	assert.True(t,
		containsTriple(snapshot, "member", "/api/v1/widgets/:id", "PATCH"),
		"member perm must derive a Casbin policy for (member, /api/v1/widgets/:id, PATCH); snapshot=%v", snapshot)
	assert.True(t,
		containsTriple(snapshot, "administrator", "/api/v1/widgets/purge", "POST"),
		"admin perm must derive a Casbin policy for (administrator, /api/v1/widgets/purge, POST); snapshot=%v", snapshot)
}

// TestRegisterEnforced_CasbinPathOverride_UsesOverrideForPolicyButRegistryKeepsPath
// pins the incus-ui behavior. The Layer 2 registry needs the exact Gin route
// pattern (/api/v1/incus-ui/:backendId/*path) for its Method:Path lookup, but
// the Layer 1 Casbin keyMatch2 policy must use the bare wildcard
// (/api/v1/incus-ui/:backendId/*). CasbinPath overrides ONLY the Casbin path;
// the registry entry keeps Path.
func TestRegisterEnforced_CasbinPathOverride_UsesOverrideForPolicyButRegistryKeepsPath(t *testing.T) {
	access.RouteRegistry.Reset()
	defer access.RouteRegistry.Reset()

	enforcer, store := newStatefulEnforcer()

	registryPath := "/api/v1/incus-ui/:backendId/*path"
	casbinPath := "/api/v1/incus-ui/:backendId/*"

	perm := access.RoutePermission{
		Path: registryPath, CasbinPath: casbinPath, Method: "GET",
		Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Param: "backendId"},
		Description: "Proxy requests to Incus UI for a backend",
	}

	access.RegisterEnforced(enforcer, "Terminal Sessions", perm)

	// Layer 2: registry is keyed on the exact Gin pattern with /*path.
	_, foundReg := access.RouteRegistry.Lookup("GET", registryPath)
	assert.True(t, foundReg,
		"registry must be Lookup-able at the full Gin pattern %q", registryPath)
	_, foundByCasbinPath := access.RouteRegistry.Lookup("GET", casbinPath)
	assert.False(t, foundByCasbinPath,
		"registry must NOT be keyed on the Casbin override path %q", casbinPath)

	// Layer 1: the Casbin policy must use the override (/*), not the registry
	// path (/*path).
	snapshot := store.snapshot()
	assert.True(t,
		containsTriple(snapshot, "member", casbinPath, "GET"),
		"Casbin policy must use CasbinPath override %q; snapshot=%v", casbinPath, snapshot)
	assert.False(t,
		containsTriple(snapshot, "member", registryPath, "GET"),
		"Casbin policy must NOT use the registry path %q when CasbinPath is set; snapshot=%v", registryPath, snapshot)
}

// TestRegisterEnforced_NoGateway_RegistersRegistryButSkipsCasbin pins the
// securityAdmin /permissions/reference behavior: the route is mounted WITHOUT
// AuthManagement(), so it must be declared in the RouteRegistry (Layer 2 / the
// reference page) but must NOT get a Casbin policy (Layer 1). A stray policy on
// a gateway-less route is dead config at best and an over-grant at worst.
func TestRegisterEnforced_NoGateway_RegistersRegistryButSkipsCasbin(t *testing.T) {
	access.RouteRegistry.Reset()
	defer access.RouteRegistry.Reset()

	enforcer, store := newStatefulEnforcer()

	perm := access.RoutePermission{
		Path: "/api/v1/permissions/reference", Method: "GET", NoGateway: true,
		Role: "member", Access: access.AccessRule{Type: access.Public},
		Description: "View permission reference page",
	}

	access.RegisterEnforced(enforcer, "Security Administration", perm)

	// Layer 2: still declared in the registry.
	_, found := access.RouteRegistry.Lookup("GET", "/api/v1/permissions/reference")
	assert.True(t, found,
		"a NoGateway route must still be declared in the RouteRegistry")

	// Layer 1: no Casbin policy at all.
	assert.Empty(t, enforcer.AddPolicyCalls,
		"a NoGateway route must NOT produce any Casbin AddPolicy call")
	assert.Empty(t, store.snapshot(),
		"a NoGateway route must leave the Casbin store empty; snapshot=%v", store.snapshot())
}

// TestRegisterEnforced_UsesRoleConstants pins the two role-name constants MR N
// introduces so modules stop repeating the "member" / "administrator" string
// literals. Their string values must match the existing Casbin role names.
func TestRegisterEnforced_UsesRoleConstants(t *testing.T) {
	assert.Equal(t, "member", access.RoleMember,
		"RoleMember must equal the existing Casbin role name")
	assert.Equal(t, "administrator", access.RoleAdministrator,
		"RoleAdministrator must equal the existing Casbin role name")
}
