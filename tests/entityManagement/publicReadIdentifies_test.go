// tests/entityManagement/publicReadIdentifies_test.go
//
// #444: an administrator could not see hidden subscription plans.
//
// The cause was structural rather than a bug in the visibility predicate. An
// entity whose read routes are declared Security=false — as SubscriptionPlan's
// are, so the public pricing page can fetch the catalogue without a session —
// was mounted with NO auth middleware at all. userRoles was therefore never
// populated, even for an authenticated admin holding a valid token, so
// visibilityScope() could not recognise one and hid the rows from everybody.
//
// The fix mounts public reads behind IdentifyIfPresent: it attaches an identity
// when a token happens to be supplied and never rejects. These tests pin that
// the middleware is actually in the chain, because reverting to a bare mount
// would restore the bug silently — the endpoint would keep working, just lying
// to admins.
package entityManagement_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/swagger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mountProbeEntity registers an entity whose GetAll is PUBLIC (Security=false),
// mirroring SubscriptionPlan, and returns the engine plus flags recording which
// middleware actually ran.
func mountProbeEntity(t *testing.T) (*gin.Engine, *bool, *bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	stubGlobalEnforcer(t)

	const entityName = "PublicReadProbe"
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration(nil, &entityManagementInterfaces.EntitySwaggerConfig{
			Tag:        "public-read-probe",
			EntityName: entityName,
			// Public read: no session required, exactly like the plan catalogue.
			GetAll: &entityManagementInterfaces.SwaggerOperation{Security: false},
		}),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	// Both stubs short-circuit with 200: this test is about which middleware the
	// route is mounted behind, and the real handler needs a database.
	authRan, identifyRan := false, false
	auth := func(c *gin.Context) { authRan = true; c.AbortWithStatus(http.StatusOK) }
	identify := func(c *gin.Context) { identifyRan = true; c.AbortWithStatus(http.StatusOK) }

	engine := gin.New()
	apiGroup := engine.Group("/api/v1")
	swagger.NewSwaggerRouteGenerator(nil).RegisterDocumentedRoutes(apiGroup, auth, identify)

	return engine, &authRan, &identifyRan
}

// TestPublicRead_RunsIdentifyMiddleware is the regression guard: a public read
// must still pass through identification, so read scoping can widen for admins.
func TestPublicRead_RunsIdentifyMiddleware(t *testing.T) {
	engine, authRan, identifyRan := mountProbeEntity(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-read-probes", nil)
	engine.ServeHTTP(httptest.NewRecorder(), req)

	assert.True(t, *identifyRan,
		"a public read must run IdentifyIfPresent — mounted bare, userRoles is never set and an "+
			"administrator is indistinguishable from an anonymous caller (#444)")
	assert.False(t, *authRan,
		"it must NOT run the rejecting auth middleware: the route is public on purpose")
}

// TestPublicRead_IsStillMounted guards the trivial regression of dropping the
// public route altogether while rearranging the middleware: the pricing page
// fetches this path without a session and must keep resolving.
func TestPublicRead_IsStillMounted(t *testing.T) {
	engine, _, _ := mountProbeEntity(t)

	found := false
	for _, r := range engine.Routes() {
		if r.Method == http.MethodGet && r.Path == "/api/v1/public-read-probes" {
			found = true
			break
		}
	}
	require.True(t, found, "the public read route must remain registered")
}
