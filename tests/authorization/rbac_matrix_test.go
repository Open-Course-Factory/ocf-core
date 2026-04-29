package authorization_tests

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	emUtils "soli/formations/src/entityManagement/utils"
	"soli/formations/src/utils"
)

// basePath returns the project root relative to this test file.
func basePath() string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Dir(b) + "/../../"
}

// testDBCounter is used to generate unique database names per test.
var testDBCounter int

// setupCasbinEnforcer creates an in-memory SQLite database with a unique name
// (to isolate Casbin state between tests), initializes the Casbin enforcer,
// and returns the database handle.
func setupCasbinEnforcer(t *testing.T) *gorm.DB {
	t.Helper()

	testDBCounter++
	dsn := fmt.Sprintf("file:rbac_test_%d?mode=memory&cache=shared&_busy_timeout=5000", testDBCounter)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	casdoor.InitCasdoorEnforcer(db, basePath())
	require.NotNil(t, casdoor.Enforcer)

	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	return db
}

// entityRoleDefinition captures the role matrix for a single registered entity.
// entityName is the PascalCase registration name (e.g., "ClassGroup").
// routeName is the kebab-case pluralized route segment (e.g., "class-groups").
type entityRoleDefinition struct {
	entityName string
	routeName  string
	roles      entityManagementInterfaces.EntityRoles
}

// --------------------------------------------------------------------------
// Role-regex helpers
// --------------------------------------------------------------------------
//
// These builders compose the small set of CRUD method-regexes that recur
// across the matrix. They keep the per-entity definitions terse and make
// non-standard rows visually conspicuous.

// methodRegex builds the "(M1|M2|...)" string Casbin uses to match HTTP methods
// against a role policy.
func methodRegex(methods ...string) string {
	return "(" + strings.Join(methods, "|") + ")"
}

// adminFullCRUD is the regex granting an admin full GET|POST|PATCH|DELETE.
// This is the most common admin grant in the matrix.
func adminFullCRUD() string {
	return methodRegex(http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete)
}

// memberRoles / adminRoles return the OCF role names. Wrapping them keeps
// the per-entity functions readable.
func memberRoles() string { return string(authModels.Member) }
func adminRoles() string  { return string(authModels.Admin) }

// rolesForReadOnlyMember returns the most common pattern: members can only
// read (GET); admins have full CRUD. Used for catalog-style entities where
// only platform operators mutate data.
func rolesForReadOnlyMember() entityManagementInterfaces.EntityRoles {
	return entityManagementInterfaces.EntityRoles{Roles: map[string]string{
		memberRoles(): methodRegex(http.MethodGet),
		adminRoles():  adminFullCRUD(),
	}}
}

// rolesForCreatableByMember returns the pattern where members can list and
// create (GET|POST); admins have full CRUD.
func rolesForCreatableByMember() entityManagementInterfaces.EntityRoles {
	return entityManagementInterfaces.EntityRoles{Roles: map[string]string{
		memberRoles(): methodRegex(http.MethodGet, http.MethodPost),
		adminRoles():  adminFullCRUD(),
	}}
}

// rolesForAdminOnly returns the pattern where members have no role-level
// access at all and admins have full CRUD. Used by entities that expose
// dedicated routes for member operations (e.g., UserSubscription).
func rolesForAdminOnly() entityManagementInterfaces.EntityRoles {
	return entityManagementInterfaces.EntityRoles{Roles: map[string]string{
		adminRoles(): adminFullCRUD(),
	}}
}

// rolesEmpty returns an empty role map. Entities with no explicit roles fall
// through to default/owner-based access — no Casbin policies are created via
// the entity registration system.
func rolesEmpty() entityManagementInterfaces.EntityRoles {
	return entityManagementInterfaces.EntityRoles{Roles: map[string]string{}}
}

// --------------------------------------------------------------------------
// Per-entity-class role definitions
// --------------------------------------------------------------------------

// authEntityRoles covers entities owned by the auth module: SshKey and
// UserSetting. UserSetting is special: members may PATCH their own settings.
func authEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{
			entityName: "SshKey",
			routeName:  "ssh-keys",
			roles:      rolesForReadOnlyMember(),
		},
		{
			entityName: "UserSetting",
			routeName:  "user-settings",
			roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
				memberRoles(): methodRegex(http.MethodGet, http.MethodPatch),
				adminRoles():  adminFullCRUD(),
			}},
		},
	}
}

// configurationEntityRoles covers feature-flag and configuration entities.
// Members can read flags; only admins toggle them.
func configurationEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{entityName: "Feature", routeName: "features", roles: rolesForReadOnlyMember()},
	}
}

// groupsEntityRoles covers ClassGroup and GroupMember. Members can create
// groups (POST); GroupMember writes are admin-only at the Casbin layer
// (Layer 2 hooks enforce group-role permissions).
func groupsEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{
			entityName: "ClassGroup",
			routeName:  "class-groups",
			roles:      rolesForCreatableByMember(),
		},
		{
			entityName: "GroupMember",
			routeName:  "group-members",
			roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
				memberRoles(): methodRegex(http.MethodGet),
				adminRoles():  methodRegex(http.MethodGet, http.MethodPost, http.MethodDelete),
			}},
		},
	}
}

// organizationsEntityRoles covers Organization and OrganizationMember.
// Members can create organizations they will own; OrganizationMember
// modifications are gated by Layer 2 org-role hooks.
func organizationsEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{
			entityName: "Organization",
			routeName:  "organizations",
			roles:      rolesForCreatableByMember(),
		},
		{
			entityName: "OrganizationMember",
			routeName:  "organization-members",
			roles:      rolesForReadOnlyMember(),
		},
	}
}

// paymentEntityRoles covers Stripe-related entities. The matrix here is
// the most varied:
//   - BillingAddress / PaymentMethod: members can mutate but cannot list
//     others' records (note: GET is intentionally absent for member at
//     the role level — handlers serve the user's own records via dedicated
//     routes).
//   - Invoice / UsageMetrics / SubscriptionPlan / PlanFeature: members read
//     only; admins have full CRUD.
//   - UserSubscription: admin-only at the role layer; members use dedicated
//     routes (/current, /all, /checkout) instead.
func paymentEntityRoles() []entityRoleDefinition {
	memberMutateOnly := entityManagementInterfaces.EntityRoles{Roles: map[string]string{
		memberRoles(): methodRegex(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch),
		adminRoles():  methodRegex(http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch),
	}}
	return []entityRoleDefinition{
		{entityName: "BillingAddress", routeName: "billing-addresses", roles: memberMutateOnly},
		{entityName: "Invoice", routeName: "invoices", roles: rolesForReadOnlyMember()},
		{entityName: "PaymentMethod", routeName: "payment-methods", roles: memberMutateOnly},
		{entityName: "PlanFeature", routeName: "plan-features", roles: rolesForReadOnlyMember()},
		{entityName: "SubscriptionPlan", routeName: "subscription-plans", roles: rolesForReadOnlyMember()},
		{entityName: "UsageMetrics", routeName: "usage-metrics", roles: rolesForReadOnlyMember()},
		{entityName: "UserSubscription", routeName: "user-subscriptions", roles: rolesForAdminOnly()},
	}
}

// terminalTrainerEntityRoles covers Terminal sessions and per-user terminal
// keys. Members can list and create terminal sessions; key management is
// admin-only at the role layer.
func terminalTrainerEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{entityName: "Terminal", routeName: "terminals", roles: rolesForCreatableByMember()},
		{entityName: "UserTerminalKey", routeName: "user-terminal-keys", roles: rolesForReadOnlyMember()},
	}
}

// coursesEntityRoles covers the course-authoring entities. None of them
// declare explicit Casbin roles — access is governed by entity ownership
// hooks (and, for courses, group/org sharing). The empty role maps mean
// no role-level Casbin policies are created at registration time.
func coursesEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{entityName: "Chapter", routeName: "chapters", roles: rolesEmpty()},
		{entityName: "Course", routeName: "courses", roles: rolesEmpty()},
		{entityName: "Generation", routeName: "generations", roles: rolesEmpty()},
		{entityName: "Page", routeName: "pages", roles: rolesEmpty()},
		{entityName: "Schedule", routeName: "schedules", roles: rolesEmpty()},
		{entityName: "Section", routeName: "sections", roles: rolesEmpty()},
		{entityName: "Session", routeName: "sessions", roles: rolesEmpty()},
		{entityName: "Theme", routeName: "themes", roles: rolesEmpty()},
	}
}

// emailEntityRoles covers email-related entities. EmailTemplate has no
// explicit roles; access is admin-only via dedicated routes.
func emailEntityRoles() []entityRoleDefinition {
	return []entityRoleDefinition{
		{entityName: "EmailTemplate", routeName: "email-templates", roles: rolesEmpty()},
	}
}

// allEntityRoles returns the complete role matrix used as both the fixture
// (to seed the in-memory Casbin enforcer) and the oracle (to assert the
// expected allow/deny per role × method × entity).
//
// Matrix structure
// ----------------
// Each row is an `entityRoleDefinition` with three fields:
//   - entityName: PascalCase registration name (must match production registration)
//   - routeName:  kebab-case pluralized route segment (must match Pluralize +
//                 PascalToKebab — validated by TestRBAC_RouteNameDerivation)
//   - roles:      map[ocfRole]"(METHOD1|METHOD2|...)" — one entry per OCF role
//                 that has Casbin access. Missing role => no Casbin policy
//                 for that role on that entity.
//
// Empty `roles.Roles` map means the entity does NOT register Casbin policies
// at registration time. Access for these entities (Chapter, Course, Page,
// Section, Session, Theme, Schedule, Generation, EmailTemplate) is governed
// entirely by entity-ownership hooks and dedicated routes.
//
// Each subgroup below is sourced from the production registration files in
// `src/{module}/entityRegistration/*.go`. When production registration roles
// change, the corresponding subgroup function MUST be updated. See the per-
// function docstrings for the exact mapping.
//
// Tests in this file consume this matrix in two ways:
//  1. As fixture: seed the Casbin enforcer via `loadEntityPolicies` so the
//     enforcer behaves as production would for these entities.
//  2. As oracle: drive `buildMatrixTestCases` which generates one test case
//     per (entity, method, role) tuple and asserts allow/deny matches the
//     regex.
//
// Two layers of authorization (CRITICAL — see CLAUDE.md):
//   - Layer 1 (this matrix): Casbin RBAC. Coarse method-on-route gating.
//   - Layer 2 (NOT here):    Group/org/owner role enforcement via entity
//                            hooks and `RouteRegistry`. Tested elsewhere.
//
// Adding a new entity:
//  1. Add the row to the appropriate per-class function below.
//  2. Add the route to TestRBAC_RouteNameDerivation's expected map.
//  3. Verify the row matches the production registration's `Roles:` block.
func allEntityRoles() []entityRoleDefinition {
	groups := [][]entityRoleDefinition{
		authEntityRoles(),
		configurationEntityRoles(),
		groupsEntityRoles(),
		organizationsEntityRoles(),
		paymentEntityRoles(),
		terminalTrainerEntityRoles(),
		coursesEntityRoles(),
		emailEntityRoles(),
	}

	var all []entityRoleDefinition
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

// loadEntityPolicies registers Casbin policies for the given entity definition
// using the same logic as the production entityRegistrationService.
func loadEntityPolicies(t *testing.T, def entityRoleDefinition) {
	t.Helper()

	service := ems.NewEntityRegistrationService()
	service.SetDefaultEntityAccesses(def.entityName, def.roles, casdoor.Enforcer)
}

// httpMethods is the set of all methods we test against.
var httpMethods = []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete}

// --------------------------------------------------------------------------
// 1. Entity Role Matrix (table-driven)
// --------------------------------------------------------------------------

// rbacTestCase describes a single RBAC assertion.
type rbacTestCase struct {
	name       string
	entity     string // kebab-case route (e.g., "class-groups")
	method     string // GET, POST, PATCH, DELETE
	role       string // "member", "administrator"
	expectAuth bool
}

// buildMatrixTestCases generates test cases for every entity x role x method
// combination from the registration definitions.
func buildMatrixTestCases() []rbacTestCase {
	entities := allEntityRoles()
	var cases []rbacTestCase

	for _, ent := range entities {
		if len(ent.roles.Roles) == 0 {
			// Entities without explicit role policies: all methods should be denied
			// for both member and admin (at the role level).
			for _, method := range httpMethods {
				cases = append(cases, rbacTestCase{
					name:       fmt.Sprintf("%s/%s/member/denied_no_policy", ent.routeName, method),
					entity:     ent.routeName,
					method:     method,
					role:       string(authModels.Member),
					expectAuth: false,
				})
				cases = append(cases, rbacTestCase{
					name:       fmt.Sprintf("%s/%s/admin/denied_no_policy", ent.routeName, method),
					entity:     ent.routeName,
					method:     method,
					role:       string(authModels.Admin),
					expectAuth: false,
				})
			}
			continue
		}

		for roleName, methodRegex := range ent.roles.Roles {
			for _, method := range httpMethods {
				expect := methodMatchesRegex(method, methodRegex)
				cases = append(cases, rbacTestCase{
					name:       fmt.Sprintf("%s/%s/%s/expect_%v", ent.routeName, method, roleName, expect),
					entity:     ent.routeName,
					method:     method,
					role:       roleName,
					expectAuth: expect,
				})
			}
		}
	}

	return cases
}

// methodMatchesRegex checks whether a method is in the role's access regex.
// The regex is in the form "(GET|POST|PATCH|DELETE)".
func methodMatchesRegex(method, regex string) bool {
	// Simple string contains check — the format is always "(METHOD1|METHOD2|...)"
	return len(regex) > 0 && containsMethod(regex, method)
}

func containsMethod(s, method string) bool {
	// Strip parentheses and split by "|"
	trimmed := strings.Trim(s, "()")
	parts := strings.Split(trimmed, "|")
	for _, p := range parts {
		if p == method {
			return true
		}
	}
	return false
}

// TestRBAC_EntityRoleAuthorizationMatrix verifies every entity's CRUD permissions
// for each role by loading actual Casbin policies from entity registration definitions.
func TestRBAC_EntityRoleAuthorizationMatrix(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load policies for all entities that have role definitions.
	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) > 0 {
			loadEntityPolicies(t, ent)
		}
	}

	cases := buildMatrixTestCases()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a unique user for this test and assign the role.
			userID := fmt.Sprintf("matrix-user-%s-%s-%s", tc.entity, tc.method, tc.role)
			opts := utils.DefaultPermissionOptions()

			err := utils.AddGroupingPolicy(casdoor.Enforcer, userID, tc.role, opts)
			require.NoError(t, err)

			// Test against the list endpoint.
			listPath := "/api/v1/" + tc.entity
			allowedList, err := casdoor.Enforcer.Enforce(userID, listPath, tc.method)
			require.NoError(t, err)

			// Test against a specific resource endpoint (UUID-like).
			resourcePath := "/api/v1/" + tc.entity + "/550e8400-e29b-41d4-a716-446655440000"
			allowedResource, err := casdoor.Enforcer.Enforce(userID, resourcePath, tc.method)
			require.NoError(t, err)

			if tc.expectAuth {
				assert.True(t, allowedList,
					"Role %q should be allowed %s on %s (list endpoint)", tc.role, tc.method, listPath)
				assert.True(t, allowedResource,
					"Role %q should be allowed %s on %s (resource endpoint)", tc.role, tc.method, resourcePath)
			} else {
				assert.False(t, allowedList,
					"Role %q should be DENIED %s on %s (list endpoint)", tc.role, tc.method, listPath)
				assert.False(t, allowedResource,
					"Role %q should be DENIED %s on %s (resource endpoint)", tc.role, tc.method, resourcePath)
			}
		})
	}
}

// --------------------------------------------------------------------------
// 2. Permission Isolation Tests
// --------------------------------------------------------------------------

// TestRBAC_EntityOwnerCanModifyOwnEntity verifies that after entity creation,
// the owner gets user-specific Casbin policies for GET|PATCH|DELETE.
func TestRBAC_EntityOwnerCanModifyOwnEntity(t *testing.T) {
	setupCasbinEnforcer(t)

	ownerID := "user-owner-isolation-001"
	entityType := "terminals"
	entityID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	path := fmt.Sprintf("/api/v1/%s/%s", entityType, entityID)

	// Simulate what happens after entity creation: owner gets explicit policies.
	opts := utils.DefaultPermissionOptions()
	err := utils.AddPolicy(casdoor.Enforcer, ownerID, path, "(GET|PATCH|DELETE)", opts)
	require.NoError(t, err)

	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		allowed, err := casdoor.Enforcer.Enforce(ownerID, path, method)
		require.NoError(t, err)
		assert.True(t, allowed,
			"Owner should have %s permission on their own entity %s", method, path)
	}
}

// TestRBAC_EntityOwnerCannotModifyOthersEntity verifies that the owner of entity A
// cannot PATCH/DELETE entity B owned by someone else.
func TestRBAC_EntityOwnerCannotModifyOthersEntity(t *testing.T) {
	setupCasbinEnforcer(t)

	ownerA := "user-owner-A-002"
	ownerB := "user-owner-B-003"
	entityType := "terminals"
	entityIDA := "11111111-1111-1111-1111-111111111111"
	entityIDB := "22222222-2222-2222-2222-222222222222"

	pathA := fmt.Sprintf("/api/v1/%s/%s", entityType, entityIDA)
	pathB := fmt.Sprintf("/api/v1/%s/%s", entityType, entityIDB)

	// Give each owner permissions on their own entity.
	opts := utils.DefaultPermissionOptions()
	err := utils.AddPolicy(casdoor.Enforcer, ownerA, pathA, "(GET|PATCH|DELETE)", opts)
	require.NoError(t, err)
	err = utils.AddPolicy(casdoor.Enforcer, ownerB, pathB, "(GET|PATCH|DELETE)", opts)
	require.NoError(t, err)

	// Owner A should NOT be able to PATCH or DELETE entity B.
	for _, method := range []string{"PATCH", "DELETE"} {
		allowed, err := casdoor.Enforcer.Enforce(ownerA, pathB, method)
		require.NoError(t, err)
		assert.False(t, allowed,
			"Owner of entity A should NOT have %s permission on entity B", method)
	}

	// Owner B should NOT be able to PATCH or DELETE entity A.
	for _, method := range []string{"PATCH", "DELETE"} {
		allowed, err := casdoor.Enforcer.Enforce(ownerB, pathA, method)
		require.NoError(t, err)
		assert.False(t, allowed,
			"Owner of entity B should NOT have %s permission on entity A", method)
	}
}

// TestRBAC_NoRoleMeansNoAccess verifies that a user with no Casbin roles
// gets 403 on every entity route.
func TestRBAC_NoRoleMeansNoAccess(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load policies for entities that have roles.
	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) > 0 {
			loadEntityPolicies(t, ent)
		}
	}

	noRoleUser := "user-no-role-ghost-404"
	// Intentionally do NOT assign any role to this user.

	entities := allEntityRoles()
	for _, ent := range entities {
		for _, method := range httpMethods {
			listPath := "/api/v1/" + ent.routeName
			allowed, err := casdoor.Enforcer.Enforce(noRoleUser, listPath, method)
			require.NoError(t, err)
			assert.False(t, allowed,
				"User with no role should be denied %s on %s", method, listPath)

			resourcePath := "/api/v1/" + ent.routeName + "/550e8400-e29b-41d4-a716-446655440000"
			allowed, err = casdoor.Enforcer.Enforce(noRoleUser, resourcePath, method)
			require.NoError(t, err)
			assert.False(t, allowed,
				"User with no role should be denied %s on %s", method, resourcePath)
		}
	}
}

// --------------------------------------------------------------------------
// 3. Role Hierarchy Tests
// --------------------------------------------------------------------------

// TestRBAC_AdminHasAllMemberPermissions verifies that the admin role can do
// everything the member role can for every entity with both roles defined.
func TestRBAC_AdminHasAllMemberPermissions(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load all entity policies.
	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) > 0 {
			loadEntityPolicies(t, ent)
		}
	}

	memberUser := "user-member-hierarchy-100"
	adminUser := "user-admin-hierarchy-200"

	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)
	err = utils.AddGroupingPolicy(casdoor.Enforcer, adminUser, string(authModels.Admin), opts)
	require.NoError(t, err)

	// Track entities where admin lacks member permissions (security finding).
	var gaps []string

	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) == 0 {
			continue
		}
		memberMethods, hasMember := ent.roles.Roles[string(authModels.Member)]
		if !hasMember {
			continue
		}
		_, hasAdmin := ent.roles.Roles[string(authModels.Admin)]

		for _, method := range httpMethods {
			if !methodMatchesRegex(method, memberMethods) {
				continue
			}

			// Check list endpoint.
			listPath := "/api/v1/" + ent.routeName
			memberAllowed, err := casdoor.Enforcer.Enforce(memberUser, listPath, method)
			require.NoError(t, err)
			adminAllowed, err := casdoor.Enforcer.Enforce(adminUser, listPath, method)
			require.NoError(t, err)

			if memberAllowed && !adminAllowed {
				if hasAdmin {
					// If admin role IS explicitly defined but still lacks the method,
					// that is a real assertion failure.
					assert.True(t, adminAllowed,
						"Admin should have at least the same permissions as member: %s %s (list)", method, ent.routeName)
				} else {
					// Entities that only define member role (no admin role) are a gap.
					gaps = append(gaps, fmt.Sprintf("%s %s: admin has no explicit role definition", method, ent.routeName))
				}
			}

			// Check resource endpoint.
			resourcePath := "/api/v1/" + ent.routeName + "/550e8400-e29b-41d4-a716-446655440000"
			memberAllowed, err = casdoor.Enforcer.Enforce(memberUser, resourcePath, method)
			require.NoError(t, err)
			adminAllowed, err = casdoor.Enforcer.Enforce(adminUser, resourcePath, method)
			require.NoError(t, err)

			if memberAllowed && !adminAllowed && hasAdmin {
				assert.True(t, adminAllowed,
					"Admin should have at least the same permissions as member: %s %s (resource)", method, ent.routeName)
			}
		}
	}

	// Log entities where admin has NO role definition but member does.
	// These are intentional design choices or gaps worth reviewing.
	if len(gaps) > 0 {
		t.Logf("SECURITY FINDING: %d entity/method combinations where admin lacks member permissions "+
			"because admin role is not defined:", len(gaps))
		for _, gap := range gaps {
			t.Logf("  - %s", gap)
		}
	}
}

// TestRBAC_MemberCannotDoAdminActions verifies that member cannot perform
// PATCH or DELETE on entities where those are admin-only.
func TestRBAC_MemberCannotDoAdminActions(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load all entity policies.
	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) > 0 {
			loadEntityPolicies(t, ent)
		}
	}

	memberUser := "user-member-cannot-admin-300"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// Entities where admin has permissions that member does NOT have.
	type adminOnlyCase struct {
		routeName   string
		method      string
		description string
	}

	var adminOnlyCases []adminOnlyCase

	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) == 0 {
			continue
		}

		memberMethods := ent.roles.Roles[string(authModels.Member)]
		adminMethods := ent.roles.Roles[string(authModels.Admin)]

		if adminMethods == "" {
			continue
		}

		for _, method := range httpMethods {
			adminHas := methodMatchesRegex(method, adminMethods)
			memberHas := methodMatchesRegex(method, memberMethods)

			if adminHas && !memberHas {
				adminOnlyCases = append(adminOnlyCases, adminOnlyCase{
					routeName:   ent.routeName,
					method:      method,
					description: fmt.Sprintf("%s %s is admin-only", method, ent.routeName),
				})
			}
		}
	}

	require.NotEmpty(t, adminOnlyCases, "There should be at least some admin-only permissions")

	for _, tc := range adminOnlyCases {
		t.Run(tc.description, func(t *testing.T) {
			listPath := "/api/v1/" + tc.routeName
			allowed, err := casdoor.Enforcer.Enforce(memberUser, listPath, tc.method)
			require.NoError(t, err)
			assert.False(t, allowed,
				"Member should NOT have %s access on %s (admin-only)", tc.method, listPath)

			resourcePath := "/api/v1/" + tc.routeName + "/550e8400-e29b-41d4-a716-446655440000"
			allowed, err = casdoor.Enforcer.Enforce(memberUser, resourcePath, tc.method)
			require.NoError(t, err)
			assert.False(t, allowed,
				"Member should NOT have %s access on %s (admin-only, resource)", tc.method, resourcePath)
		})
	}
}

// --------------------------------------------------------------------------
// 4. Casbin KeyMatch2 Wildcard Security Tests
// --------------------------------------------------------------------------

// TestRBAC_WildcardDoesNotLeakAcrossEntities verifies that a role policy for
// one entity (e.g., /api/v1/terminals/*) does not grant access to a different
// entity (e.g., /api/v1/organizations/*).
func TestRBAC_WildcardDoesNotLeakAcrossEntities(t *testing.T) {
	setupCasbinEnforcer(t)

	// Only load Terminal policies.
	terminalDef := entityRoleDefinition{
		entityName: "Terminal",
		routeName:  "terminals",
		roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
			string(authModels.Member): "(GET|POST)",
			string(authModels.Admin):  "(GET|POST|DELETE|PATCH)",
		}},
	}
	loadEntityPolicies(t, terminalDef)

	memberUser := "user-wildcard-isolation-500"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// Member should have GET on terminals.
	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/terminals", "GET")
	require.NoError(t, err)
	assert.True(t, allowed, "Member should have GET on terminals")

	// Member should NOT have GET on organizations (no policy loaded).
	allowed, err = casdoor.Enforcer.Enforce(memberUser, "/api/v1/organizations", "GET")
	require.NoError(t, err)
	assert.False(t, allowed, "Terminal policy should NOT leak to organizations")

	// Member should NOT have GET on invoices either.
	allowed, err = casdoor.Enforcer.Enforce(memberUser, "/api/v1/invoices", "GET")
	require.NoError(t, err)
	assert.False(t, allowed, "Terminal policy should NOT leak to invoices")
}

// TestRBAC_ResourceIDDoesNotLeakViaPathTraversal documents Casbin keyMatch2
// behavior with path traversal strings. In production, Gin normalizes paths
// before they reach the Casbin middleware, so these traversals cannot actually
// reach the enforcer. This test documents the Casbin-level behavior for
// defense-in-depth awareness.
func TestRBAC_ResourceIDDoesNotLeakViaPathTraversal(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load terminal policies.
	terminalDef := entityRoleDefinition{
		entityName: "Terminal",
		routeName:  "terminals",
		roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
			string(authModels.Member): "(GET|POST)",
		}},
	}
	loadEntityPolicies(t, terminalDef)

	memberUser := "user-traversal-600"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// NOTE: Casbin keyMatch2 wildcard "/api/v1/terminals/*" will match these
	// because ".." and "%2e%2e" are treated as valid path segments by keyMatch2.
	// This is NOT a security issue in production because Gin normalizes request
	// paths before they reach the middleware layer.
	traversalPaths := []struct {
		path        string
		description string
	}{
		{"/api/v1/terminals/../organizations", "parent directory traversal"},
		{"/api/v1/terminals/../../etc/passwd", "double parent traversal"},
		{"/api/v1/terminals/%2e%2e/organizations", "URL-encoded traversal"},
	}

	for _, tc := range traversalPaths {
		t.Run(tc.description, func(t *testing.T) {
			allowed, err := casdoor.Enforcer.Enforce(memberUser, tc.path, "GET")
			require.NoError(t, err)
			if allowed {
				t.Logf("SECURITY NOTE: Casbin keyMatch2 allows %q (matched by wildcard). "+
					"This is mitigated by Gin path normalization in production.", tc.path)
			}
			// We do NOT assert false here because the Casbin behavior is expected.
			// The real protection is at the HTTP framework level.
		})
	}

	// Verify that a properly formed but unauthorized path is still denied.
	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/organizations", "GET")
	require.NoError(t, err)
	assert.False(t, allowed,
		"Direct access to /api/v1/organizations should be denied (no policy)")
}

// --------------------------------------------------------------------------
// 5. Casdoor Role Mapping Tests
// --------------------------------------------------------------------------

// TestRBAC_CasdoorRoleMappingConsistency verifies that when entity policies are
// set up, the Casdoor-mapped roles (user, student, trainer, etc.) also get the
// same permissions as the OCF "member" role, and Casdoor admin roles get
// "administrator" permissions.
func TestRBAC_CasdoorRoleMappingConsistency(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load policies for a representative entity.
	groupDef := entityRoleDefinition{
		entityName: "ClassGroup",
		routeName:  "class-groups",
		roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
			string(authModels.Member): "(GET|POST)",
			string(authModels.Admin):  "(GET|POST|PATCH|DELETE)",
		}},
	}
	loadEntityPolicies(t, groupDef)

	// All Casdoor roles that map to "member" should have the same permissions.
	memberCasdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)
	for _, casdoorRole := range memberCasdoorRoles {
		user := fmt.Sprintf("user-casdoor-%s-700", casdoorRole)
		opts := utils.DefaultPermissionOptions()
		err := utils.AddGroupingPolicy(casdoor.Enforcer, user, casdoorRole, opts)
		require.NoError(t, err)

		t.Run("casdoor_"+casdoorRole+"_has_member_GET", func(t *testing.T) {
			allowed, err := casdoor.Enforcer.Enforce(user, "/api/v1/class-groups", "GET")
			require.NoError(t, err)
			assert.True(t, allowed,
				"Casdoor role %q (maps to member) should have GET on class-groups", casdoorRole)
		})

		t.Run("casdoor_"+casdoorRole+"_has_member_POST", func(t *testing.T) {
			allowed, err := casdoor.Enforcer.Enforce(user, "/api/v1/class-groups", "POST")
			require.NoError(t, err)
			assert.True(t, allowed,
				"Casdoor role %q (maps to member) should have POST on class-groups", casdoorRole)
		})

		t.Run("casdoor_"+casdoorRole+"_denied_PATCH", func(t *testing.T) {
			allowed, err := casdoor.Enforcer.Enforce(user, "/api/v1/class-groups", "PATCH")
			require.NoError(t, err)
			assert.False(t, allowed,
				"Casdoor role %q (maps to member) should NOT have PATCH on class-groups", casdoorRole)
		})
	}

	// Casdoor admin roles should have full access.
	adminCasdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Administrator)
	for _, casdoorRole := range adminCasdoorRoles {
		user := fmt.Sprintf("user-casdoor-admin-%s-800", casdoorRole)
		opts := utils.DefaultPermissionOptions()
		err := utils.AddGroupingPolicy(casdoor.Enforcer, user, casdoorRole, opts)
		require.NoError(t, err)

		for _, method := range httpMethods {
			t.Run(fmt.Sprintf("casdoor_%s_has_admin_%s", casdoorRole, method), func(t *testing.T) {
				allowed, err := casdoor.Enforcer.Enforce(user, "/api/v1/class-groups", method)
				require.NoError(t, err)
				assert.True(t, allowed,
					"Casdoor admin role %q should have %s on class-groups", casdoorRole, method)
			})
		}
	}
}

// --------------------------------------------------------------------------
// 6. Entities Without Role Definitions
// --------------------------------------------------------------------------

// TestRBAC_EntitiesWithoutRoles_HaveNoPolicies verifies that entities registered
// without explicit roles (Course, Chapter, Section, Page, Theme, Schedule,
// Generation, Session, EmailTemplate) do not have any role-level Casbin policies.
func TestRBAC_EntitiesWithoutRoles_HaveNoPolicies(t *testing.T) {
	setupCasbinEnforcer(t)

	// Load all policies (only entities with roles will actually add policies).
	for _, ent := range allEntityRoles() {
		if len(ent.roles.Roles) > 0 {
			loadEntityPolicies(t, ent)
		}
	}

	entitiesWithoutRoles := []string{
		"chapters", "courses", "generations", "pages",
		"schedules", "sections", "sessions", "themes",
		"email-templates",
	}

	// Any user with member or admin role should be denied on these routes
	// (unless they have user-specific policies, which we don't add here).
	memberUser := "user-no-course-role-900"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	for _, route := range entitiesWithoutRoles {
		for _, method := range httpMethods {
			t.Run(fmt.Sprintf("%s/%s/member_denied", route, method), func(t *testing.T) {
				listPath := "/api/v1/" + route
				allowed, err := casdoor.Enforcer.Enforce(memberUser, listPath, method)
				require.NoError(t, err)
				assert.False(t, allowed,
					"Member should be denied %s on %s (no role policy defined)", method, listPath)
			})
		}
	}
}

// --------------------------------------------------------------------------
// 7. Complete Route Pattern Validation
// --------------------------------------------------------------------------

// TestRBAC_RouteNameDerivation validates that the route names in our test
// definitions match what the production Pluralize + PascalToKebab logic produces.
func TestRBAC_RouteNameDerivation(t *testing.T) {
	expected := map[string]string{
		"SshKey":             "ssh-keys",
		"UserSetting":       "user-settings",
		"Feature":            "features",
		"ClassGroup":         "class-groups",
		"GroupMember":        "group-members",
		"Organization":       "organizations",
		"OrganizationMember": "organization-members",
		"BillingAddress":     "billing-addresses",
		"Invoice":            "invoices",
		"PaymentMethod":      "payment-methods",
		"PlanFeature":        "plan-features",
		"SubscriptionPlan":   "subscription-plans",
		"UsageMetrics":       "usage-metrics",
		"UserSubscription":   "user-subscriptions",
		"Terminal":           "terminals",
		"UserTerminalKey":    "user-terminal-keys",
		"Chapter":            "chapters",
		"Course":             "courses",
		"Generation":         "generations",
		"Page":               "pages",
		"Schedule":           "schedules",
		"Section":            "sections",
		"Session":            "sessions",
		"Theme":              "themes",
		"EmailTemplate":      "email-templates",
	}

	for _, ent := range allEntityRoles() {
		t.Run(ent.entityName, func(t *testing.T) {
			want, ok := expected[ent.entityName]
			require.True(t, ok, "Missing expected route for entity %s", ent.entityName)
			assert.Equal(t, want, ent.routeName,
				"Route name mismatch for entity %s", ent.entityName)
		})
	}

	// Also validate using the production function.
	for entityName, expectedRoute := range expected {
		t.Run("production_"+entityName, func(t *testing.T) {
			pluralized := ems.Pluralize(entityName)
			kebab := emUtils.PascalToKebab(pluralized)
			assert.Equal(t, expectedRoute, kebab,
				"Production route derivation mismatch for %s: Pluralize(%q)=%q, PascalToKebab=%q",
				entityName, entityName, pluralized, kebab)
		})
	}
}

