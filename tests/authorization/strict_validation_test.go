package authorization_tests

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	impersonationController "soli/formations/src/auth/routes/impersonationRoutes"
	securityAdminController "soli/formations/src/auth/routes/securityAdminRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	adminUsersController "soli/formations/src/admin/routes/adminUsersRoutes"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	observabilityController "soli/formations/src/observability/routes"
	organizationRoutes "soli/formations/src/organizations/routes"
	paymentController "soli/formations/src/payment/routes"
	scenarioController "soli/formations/src/scenarios/routes"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// newStrictTestRouter returns a bare Gin engine. ValidatePermissionSetupStrict
// takes a *gin.Engine for signature symmetry with ValidatePermissionSetup, but
// the strict guarantee (M2) is about the RouteRegistry, not the Gin routing
// table — so a minimal router is sufficient for these tests.
func newStrictTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// TestValidatePermissionSetupStrict_UnregisteredRuleType_ReturnsError is the
// core M2 guarantee: a route whose AccessRuleType has no registered enforcer
// must be caught by strict validation (the log-only ValidatePermissionSetup
// silently degraded such routes to pass-through, which is the vulnerability).
func TestValidatePermissionSetupStrict_UnregisteredRuleType_ReturnsError(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer access.RouteRegistry.Reset()
	defer access.ResetEnforcers()

	const bogusRuleType access.AccessRuleType = "nonexistent_rule"

	// Declare a route using a rule type nobody registered an enforcer for.
	access.RouteRegistry.Register("Test",
		access.RoutePermission{
			Path:        "/api/v1/test/bogus",
			Method:      "GET",
			Role:        "member",
			Access:      access.AccessRule{Type: bogusRuleType},
			Description: "Route with an unregistered access rule type",
		},
	)

	err := access.ValidatePermissionSetupStrict(newStrictTestRouter())

	require.Error(t, err, "strict validation must fail when a route's rule type has no enforcer")
	assert.Contains(t, err.Error(), string(bogusRuleType),
		"error must name the offending rule type so the operator can find it")
}

// TestValidatePermissionSetupStrict_AllRuleTypesRegistered_ReturnsNil verifies
// the happy path: when every declared rule type has an enforcer, strict
// validation passes.
func TestValidatePermissionSetupStrict_AllRuleTypesRegistered_ReturnsNil(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer access.RouteRegistry.Reset()
	defer access.ResetEnforcers()

	// Register the built-in enforcers (needs an EntityLoader + MembershipChecker;
	// reuse the package mocks from enforcement_middleware_test.go).
	access.RegisterBuiltinEnforcers(&mockEntityLoader{}, &mockMembershipChecker{})

	// Declare a route using a built-in rule type.
	access.RouteRegistry.Register("Test",
		access.RoutePermission{
			Path:        "/api/v1/test/owned/:id",
			Method:      "PATCH",
			Role:        "member",
			Access:      access.AccessRule{Type: access.EntityOwner, Entity: "TestWidget", Field: "UserID"},
			Description: "Route with a built-in access rule type",
		},
	)

	err := access.ValidatePermissionSetupStrict(newStrictTestRouter())

	assert.NoError(t, err, "strict validation must pass when all rule types have enforcers")
}

// registerAllProductionPermissions mirrors the permission-registration block in
// main.go (RegisterAuthPermissions ... RegisterOrganizationPermissions). Each of
// these Register* functions populates the global RouteRegistry with that
// module's RoutePermission declarations. The enforcer argument only receives the
// Casbin policies (which we discard via a mock), so we pass a throwaway mock.
func registerAllProductionPermissions() {
	enforcer := mocks.NewMockEnforcer()
	userController.RegisterAuthPermissions(enforcer)
	userController.RegisterUserPermissions(enforcer)
	userController.RegisterFeedbackPermissions(enforcer)
	terminalController.RegisterTerminalPermissions(enforcer)
	securityAdminController.RegisterSecurityAdminPermissions(enforcer)
	scenarioController.RegisterScenarioPermissions(enforcer)
	courseController.RegisterCoursePermissions(enforcer)
	paymentController.RegisterPaymentPermissions(enforcer)
	paymentController.RegisterAdminStripePermissions(enforcer)
	organizationRoutes.RegisterOrganizationPermissions(enforcer)
	impersonationController.RegisterImpersonationPermissions(enforcer)
	adminUsersController.RegisterPermissions(enforcer)
	observabilityController.RegisterPermissions(enforcer)
}

// TestPermissionSetup_ProductionRoutesHaveEnforcers_Strict is THE CI guard.
//
// It reconstructs the full production RouteRegistry (every module's permission
// declarations) plus the full production enforcer set, then asserts strict
// validation finds zero routes with a missing enforcer. If a future MR adds a
// route with a new AccessRuleType but forgets to register its enforcer, this
// test fails in CI instead of the route silently degrading to pass-through in
// production.
//
// KEY: the production enforcer set is RegisterBuiltinEnforcers PLUS the
// IncusBackendAccess enforcer, which main.go does NOT register in the builtin
// block — it is wired inside IncusUIRoutes() (terminalRoutes.go), because its
// closure needs the live IncusUIController. This test must register a stub for
// IncusBackendAccess to mirror production; otherwise it false-positives (see
// TestValidatePermissionSetupStrict_ProductionMissingIncusEnforcer_Fails, which
// documents exactly that gap).
func TestPermissionSetup_ProductionRoutesHaveEnforcers_Strict(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer access.RouteRegistry.Reset()
	defer access.ResetEnforcers()

	registerAllProductionPermissions()

	// Full production enforcer set.
	access.RegisterBuiltinEnforcers(&mockEntityLoader{}, &mockMembershipChecker{})
	// IncusBackendAccess is registered outside RegisterBuiltinEnforcers in
	// production (terminalRoutes.go:IncusUIRoutes). Mirror it here with a stub.
	access.RegisterAccessEnforcer(terminalController.IncusBackendAccess,
		func(ctx *gin.Context, rule access.AccessRule, userID string, roles []string) bool {
			return true
		})

	err := access.ValidatePermissionSetupStrict(newStrictTestRouter())

	assert.NoError(t, err,
		"every AccessRuleType used by a production route must have a registered enforcer; "+
			"a failure here means a route declares a rule type with no enforcer (it would "+
			"silently pass-through in prod). Register the missing enforcer, and if it is "+
			"wired outside RegisterBuiltinEnforcers, add it to this test's enforcer set too.")
}

// TestValidatePermissionSetupStrict_ProductionMissingIncusEnforcer_Fails locks in
// the load-bearing finding: IncusBackendAccess is the one production rule type
// whose enforcer is NOT registered by RegisterBuiltinEnforcers. If main.go ever
// calls ValidatePermissionSetupStrict BEFORE IncusUIRoutes() has wired that
// enforcer, startup must fail loudly. This test proves strict validation does
// catch that ordering bug.
func TestValidatePermissionSetupStrict_ProductionMissingIncusEnforcer_Fails(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer access.RouteRegistry.Reset()
	defer access.ResetEnforcers()

	registerAllProductionPermissions()

	// Deliberately register ONLY the builtin enforcers, omitting the
	// IncusBackendAccess enforcer that IncusUIRoutes() would wire.
	access.RegisterBuiltinEnforcers(&mockEntityLoader{}, &mockMembershipChecker{})

	err := access.ValidatePermissionSetupStrict(newStrictTestRouter())

	require.Error(t, err,
		"strict validation must fail when the IncusBackendAccess enforcer is not registered")
	assert.Contains(t, err.Error(), string(terminalController.IncusBackendAccess),
		"error must name incus_backend_access as the missing enforcer")
}
