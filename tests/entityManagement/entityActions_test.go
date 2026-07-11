// tests/entityManagement/entityActions_test.go
//
// RED-phase contract for the declarative entity-actions capability (MR-B).
// Every production symbol referenced here (ActionConfig, ActionScope*,
// ActionHandlerFactory, TypedEntityRegistration.Actions, ems.ResourceBasePath,
// ems.ActionRelativePath, service.GetActions, service.SetEntityActionAccesses,
// and the route-generator changes) does NOT exist yet — these tests DEFINE it.
package entityManagement_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/swagger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Test fixtures — reuse the shared TestEntityWithBaseModel / *Dto types already
// defined in entityRegistrationService_test.go (same package). Only the entity
// NAME (a map key) is unique per test so global-registry tests don't collide.
// ---------------------------------------------------------------------------

const actionSentinelStatus = 299

// sentinelHandlerFactory builds a handler that writes a unique status + body so
// a mounted action route can be proven reachable through httptest.
func sentinelHandlerFactory(_ *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(actionSentinelStatus, "action-reached")
	}
}

// itemAction is a minimal item-scoped ActionConfig used across mount/spec tests.
func itemAction(name string) entityManagementInterfaces.ActionConfig {
	return entityManagementInterfaces.ActionConfig{
		Name:        name,
		Method:      http.MethodPost,
		Scope:       entityManagementInterfaces.ActionScopeItem,
		Handler:     sentinelHandlerFactory,
		Role:        access.RoleMember,
		Access:      access.AccessRule{Type: access.SelfScoped},
		Description: "Run the " + name + " action",
	}
}

func actionTestRegistration(actions []entityManagementInterfaces.ActionConfig, swaggerCfg *entityManagementInterfaces.EntitySwaggerConfig) entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto] {
	return entityManagementInterfaces.TypedEntityRegistration[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
		Converters: entityManagementInterfaces.TypedEntityConverters[TestEntityWithBaseModel, TestEntityInputDto, TestEntityInputDto, TestEntityOutputDto]{
			ModelToDto: func(e *TestEntityWithBaseModel) (TestEntityOutputDto, error) {
				return TestEntityOutputDto{ID: e.ID.String(), Name: e.Name}, nil
			},
			DtoToModel: func(dto TestEntityInputDto) *TestEntityWithBaseModel {
				return &TestEntityWithBaseModel{Name: dto.Name}
			},
		},
		Roles: entityManagementInterfaces.EntityRoles{
			Roles: map[string]string{access.RoleMember: "(" + http.MethodGet + ")"},
		},
		Actions:       actions,
		SwaggerConfig: swaggerCfg,
	}
}

// stubGlobalEnforcer swaps casdoor.Enforcer for a permissive mock so
// RegisterTypedEntity's setDefaultEntityAccesses does not touch a real DB.
func stubGlobalEnforcer(t *testing.T) {
	t.Helper()
	mockEnforcer := authMocks.NewMockEnforcer()
	orig := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	t.Cleanup(func() { casdoor.Enforcer = orig })
}

// ---------------------------------------------------------------------------
// 1. ResourceBasePath must reproduce EXACTLY what BOTH legacy derivation sites
//    (setDefaultEntityAccesses + routeGenerator basePath) produce today, minus
//    the /api/v1 prefix. Literals were derived empirically from the two legacy
//    code paths — they agree for all five names.
// ---------------------------------------------------------------------------

func TestResourceBasePath_MatchesLegacyDerivation(t *testing.T) {
	cases := []struct {
		entity string
		want   string
	}{
		{"EmailTemplate", "/email-templates"},
		{"Terminal", "/terminals"},
		{"UserSetting", "/user-settings"},
		{"ScenarioStepProgress", "/scenario-step-progresses"},
		{"OrganizationRolePlan", "/organization-role-plans"},
	}
	for _, tc := range cases {
		t.Run(tc.entity, func(t *testing.T) {
			assert.Equal(t, tc.want, ems.ResourceBasePath(tc.entity))
		})
	}
}

// ---------------------------------------------------------------------------
// 2. ActionRelativePath: item scope prefixes /:id, collection scope does not.
// ---------------------------------------------------------------------------

func TestActionRelativePath_ItemAndCollectionScopes(t *testing.T) {
	item := entityManagementInterfaces.ActionConfig{Name: "test", Scope: entityManagementInterfaces.ActionScopeItem}
	collection := entityManagementInterfaces.ActionConfig{Name: "test", Scope: entityManagementInterfaces.ActionScopeCollection}

	assert.Equal(t, "/:id/test", ems.ActionRelativePath(item))
	assert.Equal(t, "/test", ems.ActionRelativePath(collection))
}

// ---------------------------------------------------------------------------
// 3. RegisterTypedEntity stores the declared actions; Reset() clears them.
// ---------------------------------------------------------------------------

func TestRegisterTypedEntity_WithActions_StoresActions(t *testing.T) {
	stubGlobalEnforcer(t)
	service := ems.NewEntityRegistrationService()

	actions := []entityManagementInterfaces.ActionConfig{itemAction("verb"), itemAction("other")}
	ems.RegisterTypedEntity(service, "ActionWidget", actionTestRegistration(actions, nil))

	stored := service.GetActions("ActionWidget")
	require.Len(t, stored, 2)
	assert.Equal(t, "verb", stored[0].Name)
	assert.Equal(t, "other", stored[1].Name)

	// Unknown entity → empty.
	assert.Empty(t, service.GetActions("NoSuchEntity"))

	// Reset clears the action registry.
	service.Reset()
	assert.Empty(t, service.GetActions("ActionWidget"))
}

// ---------------------------------------------------------------------------
// 4. SetEntityActionAccesses registers the exact Layer 1 Casbin triple
//    (role, /api/v1<basePath><relPath>, method) for each action.
// ---------------------------------------------------------------------------

func TestSetEntityActionAccesses_RegistersCasbinPolicy(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	mockEnforcer := authMocks.NewMockEnforcer()

	action := entityManagementInterfaces.ActionConfig{
		Name:        "verb",
		Method:      http.MethodPost,
		Scope:       entityManagementInterfaces.ActionScopeItem,
		Handler:     sentinelHandlerFactory,
		Role:        access.RoleMember,
		Access:      access.AccessRule{Type: access.SelfScoped},
		Description: "Run verb",
	}

	service.SetEntityActionAccesses("ActionWidget", []entityManagementInterfaces.ActionConfig{action}, mockEnforcer)

	// Outcome: AddPolicy was invoked with the exact (role, path, method) triple.
	wantPath := "/api/v1/action-widgets/:id/verb"
	found := false
	for _, call := range mockEnforcer.AddPolicyCalls {
		if len(call) == 3 &&
			call[0] == access.RoleMember &&
			call[1] == wantPath &&
			call[2] == http.MethodPost {
			found = true
		}
	}
	assert.True(t, found, "expected Casbin policy (member, %s, POST) to be registered; got calls: %v", wantPath, mockEnforcer.AddPolicyCalls)
}

// ---------------------------------------------------------------------------
// 5. SetEntityActionAccesses declares the Layer 2 RoutePermission in the
//    RouteRegistry, carrying Role / Access / Description, keyed by METHOD:path.
// ---------------------------------------------------------------------------

func TestSetEntityActionAccesses_PopulatesRouteRegistry(t *testing.T) {
	access.RouteRegistry.Reset()
	service := ems.NewEntityRegistrationService()
	mockEnforcer := authMocks.NewMockEnforcer()

	action := entityManagementInterfaces.ActionConfig{
		Name:        "verb",
		Method:      http.MethodPost,
		Scope:       entityManagementInterfaces.ActionScopeItem,
		Handler:     sentinelHandlerFactory,
		Role:        access.RoleMember,
		Access:      access.AccessRule{Type: access.SelfScoped},
		Description: "Run verb on the widget",
	}

	service.SetEntityActionAccesses("ActionWidget", []entityManagementInterfaces.ActionConfig{action}, mockEnforcer)

	perm, found := access.RouteRegistry.Lookup(http.MethodPost, "/api/v1/action-widgets/:id/verb")
	assert.True(t, found, "expected a RoutePermission registered for the action route")
	assert.Equal(t, access.RoleMember, perm.Role)
	assert.Equal(t, access.SelfScoped, perm.Access.Type)
	assert.Equal(t, "Run verb on the widget", perm.Description)
	assert.Equal(t, "ActionWidget", perm.Category)
}

// ---------------------------------------------------------------------------
// 6. Action routes mount even when the entity has NO SwaggerConfig (CRUD stays
//    swagger-gated, but declared actions are always mounted + reachable).
// ---------------------------------------------------------------------------

func TestActionRoutes_MountWithoutSwaggerConfig(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionMountWidget"
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{itemAction("verb")}, nil), // NIL SwaggerConfig
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	engine := gin.New()
	apiGroup := engine.Group("/api/v1")
	permissive := func(c *gin.Context) { c.Next() }

	srg := swagger.NewSwaggerRouteGenerator(nil)
	srg.RegisterDocumentedRoutes(apiGroup, permissive)

	// (a) The route is registered on the engine at the item-scoped path.
	routeFound := false
	for _, r := range engine.Routes() {
		if r.Method == http.MethodPost && r.Path == "/api/v1/action-mount-widgets/:id/verb" {
			routeFound = true
		}
	}
	assert.True(t, routeFound, "expected POST /api/v1/action-mount-widgets/:id/verb to be mounted; routes: %v", engine.Routes())

	// (b) A request actually reaches the action handler.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/action-mount-widgets/00000000-0000-0000-0000-000000000001/verb", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, actionSentinelStatus, rec.Code)
	assert.Equal(t, "action-reached", rec.Body.String())
}

// ---------------------------------------------------------------------------
// 7. GenerateOpenAPISpec includes the action path for entities carrying actions.
// ---------------------------------------------------------------------------

func TestGenerateOpenAPISpec_IncludesActionPaths(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionSpecWidget"
	swaggerCfg := entityManagementInterfaces.NewEntitySwaggerConfig(entityName, "action-spec-widgets")
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{itemAction("verb")}, &swaggerCfg),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	dg := swagger.NewDocumentationGenerator()
	spec := dg.GenerateOpenAPISpec()

	paths, ok := spec["paths"].(map[string]any)
	assert.True(t, ok, "spec must contain a paths map")

	_, hasActionPath := paths["/action-spec-widgets/{id}/verb"]
	assert.True(t, hasActionPath, "expected action path key /action-spec-widgets/{id}/verb in generated spec; got paths: %v", keysOf(paths))
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ===========================================================================
// MR-B review fixes — RED contract for the two Important findings plus the
// coverage the reviewer asked to pin. Helpers/consts above are reused; the
// blocks below only ADD tests (nothing above is modified).
// ===========================================================================

// collectionAction is the collection-scoped twin of itemAction: it mounts under
// the entity base path directly (no /:id) and writes the same sentinel so a
// mounted route can be proven reachable.
func collectionAction(name string) entityManagementInterfaces.ActionConfig {
	a := itemAction(name)
	a.Scope = entityManagementInterfaces.ActionScopeCollection
	return a
}

// injectIdentity mirrors the authorization package's setupTestRouter identity
// seam: it pre-populates userId / userRoles so Layer2Enforcement resolves the
// caller from context (no live Casdoor / JWT needed).
func injectIdentity(userID string, roles []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	}
}

// ---------------------------------------------------------------------------
// A. Registration-time validation (fail-fast). A declared action missing any of
//    the required fields (Name, Method, Role, Access.Type) or with a nil Handler
//    must make registration PANIC — closing the fail-open hole where a
//    misdeclared action booted silently. Driven through the two production entry
//    points (SetEntityActionAccesses and the RegisterTypedEntity path) rather
//    than the validator symbol directly, so the outcome is what's pinned.
// ---------------------------------------------------------------------------

func TestSetEntityActionAccesses_InvalidActionConfig_Panics(t *testing.T) {
	valid := func() entityManagementInterfaces.ActionConfig { return itemAction("verb") }

	cases := []struct {
		name    string
		mutate  func(a *entityManagementInterfaces.ActionConfig)
		wantErr bool
	}{
		{"missing Name", func(a *entityManagementInterfaces.ActionConfig) { a.Name = "" }, true},
		{"missing Method", func(a *entityManagementInterfaces.ActionConfig) { a.Method = "" }, true},
		{"missing Role", func(a *entityManagementInterfaces.ActionConfig) { a.Role = "" }, true},
		{"missing Access.Type", func(a *entityManagementInterfaces.ActionConfig) { a.Access = access.AccessRule{} }, true},
		{"nil Handler", func(a *entityManagementInterfaces.ActionConfig) { a.Handler = nil }, true},
		{"fully valid", func(a *entityManagementInterfaces.ActionConfig) {}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := ems.NewEntityRegistrationService()
			mockEnforcer := authMocks.NewMockEnforcer()
			action := valid()
			tc.mutate(&action)

			call := func() {
				service.SetEntityActionAccesses("ActionWidget", []entityManagementInterfaces.ActionConfig{action}, mockEnforcer)
			}
			if tc.wantErr {
				require.Panics(t, call, "a misdeclared action must fail-fast (panic) at registration")
			} else {
				require.NotPanics(t, call, "a fully valid action must register without panicking")
			}
		})
	}
}

// The RegisterTypedEntity path (RegisterEntityActions) must also fail-fast, so a
// misdeclared action can never be stored+mounted silently.
func TestRegisterTypedEntity_InvalidAction_Panics(t *testing.T) {
	stubGlobalEnforcer(t)
	service := ems.NewEntityRegistrationService()

	bad := itemAction("verb")
	bad.Name = "" // missing required field

	require.Panics(t, func() {
		ems.RegisterTypedEntity(service, "ActionInvalidWidget", actionTestRegistration([]entityManagementInterfaces.ActionConfig{bad}, nil))
	}, "RegisterTypedEntity must panic when a declared action is invalid")
}

// ---------------------------------------------------------------------------
// B. Method normalization. A lowercase declared method ("post") must be
//    normalized to canonical uppercase everywhere observable.
// ---------------------------------------------------------------------------

// B1: the Casbin triple and the RouteRegistry byRoute key are uppercase, so
// Lookup("POST", ...) — the method Layer2Enforcement uses — finds the rule.
func TestSetEntityActionAccesses_NormalizesMethodToUppercase(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	mockEnforcer := authMocks.NewMockEnforcer()

	action := itemAction("verb")
	action.Method = "post" // lowercase — must be normalized

	service.SetEntityActionAccesses("ActionWidget", []entityManagementInterfaces.ActionConfig{action}, mockEnforcer)

	wantPath := "/api/v1/action-widgets/:id/verb"

	// Casbin policy carries the uppercase method.
	foundPolicy := false
	for _, call := range mockEnforcer.AddPolicyCalls {
		if len(call) == 3 && call[1] == wantPath && call[2] == http.MethodPost {
			foundPolicy = true
		}
	}
	assert.True(t, foundPolicy, "expected Casbin policy method normalized to POST; got calls: %v", mockEnforcer.AddPolicyCalls)

	// RouteRegistry is keyed by the uppercase method.
	_, foundUpper := access.RouteRegistry.Lookup(http.MethodPost, wantPath)
	assert.True(t, foundUpper, "expected RouteRegistry.Lookup(POST, %s) to hit after normalization", wantPath)

	// And NOT by the lowercase spelling.
	_, foundLower := access.RouteRegistry.Lookup("post", wantPath)
	assert.False(t, foundLower, "RouteRegistry must not retain the lowercase method key")
}

// B2: the mounted gin route uses the uppercase method. gin panics on a
// non-uppercase method, so an un-normalized "post" makes mounting blow up;
// after normalization the route mounts as POST and is reachable.
func TestActionRoutes_NormalizeMethodOnMount(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionNormalizeWidget"
	action := itemAction("verb")
	action.Method = "post" // lowercase
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{action}, nil),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	engine := gin.New()
	apiGroup := engine.Group("/api/v1")
	permissive := func(c *gin.Context) { c.Next() }
	srg := swagger.NewSwaggerRouteGenerator(nil)

	require.NotPanics(t, func() {
		srg.RegisterDocumentedRoutes(apiGroup, permissive)
	}, "mounting a lowercase-method action must not panic (method must be normalized to POST before router.Handle)")

	routeFound := false
	for _, r := range engine.Routes() {
		if r.Method == http.MethodPost && r.Path == "/api/v1/action-normalize-widgets/:id/verb" {
			routeFound = true
		}
	}
	assert.True(t, routeFound, "expected POST /api/v1/action-normalize-widgets/:id/verb mounted; routes: %v", engine.Routes())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/action-normalize-widgets/00000000-0000-0000-0000-000000000001/verb", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, actionSentinelStatus, rec.Code)
	assert.Equal(t, "action-reached", rec.Body.String())
}

// ---------------------------------------------------------------------------
// C. Collection-scope coverage — a collection action mounts under
//    /<plural>/<name> (no /:id) and appears in the generated spec.
// ---------------------------------------------------------------------------

func TestActionRoutes_CollectionScope_MountsAndReachable(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionCollectionWidget"
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{collectionAction("verb")}, nil), // NIL SwaggerConfig
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	engine := gin.New()
	apiGroup := engine.Group("/api/v1")
	permissive := func(c *gin.Context) { c.Next() }
	srg := swagger.NewSwaggerRouteGenerator(nil)
	srg.RegisterDocumentedRoutes(apiGroup, permissive)

	// Collection scope mounts directly under the base path — no /:id segment.
	wantPath := "/api/v1/action-collection-widgets/verb"
	routeFound := false
	for _, r := range engine.Routes() {
		if r.Method == http.MethodPost && r.Path == wantPath {
			routeFound = true
		}
	}
	assert.True(t, routeFound, "expected collection action mounted at POST %s; routes: %v", wantPath, engine.Routes())

	req := httptest.NewRequest(http.MethodPost, wantPath, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, actionSentinelStatus, rec.Code)
	assert.Equal(t, "action-reached", rec.Body.String())
}

func TestGenerateOpenAPISpec_IncludesCollectionActionPath(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionCollectionSpecWidget"
	swaggerCfg := entityManagementInterfaces.NewEntitySwaggerConfig(entityName, "action-collection-spec-widgets")
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{collectionAction("verb")}, &swaggerCfg),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	dg := swagger.NewDocumentationGenerator()
	spec := dg.GenerateOpenAPISpec()

	paths, ok := spec["paths"].(map[string]any)
	assert.True(t, ok, "spec must contain a paths map")

	// Collection scope → no {id} in the documented path.
	_, hasPath := paths["/action-collection-spec-widgets/verb"]
	assert.True(t, hasPath, "expected collection action path /action-collection-spec-widgets/verb; got paths: %v", keysOf(paths))
}

// ---------------------------------------------------------------------------
// D. Enforcement is actually applied to action routes.
// ---------------------------------------------------------------------------

// D1: the mount chain prepends the auth middleware — a rejecting auth middleware
// stops the request at 401 and the action handler is never reached.
func TestActionRoutes_AuthMiddlewareGatesHandler(t *testing.T) {
	stubGlobalEnforcer(t)

	entityName := "ActionAuthGateWidget"
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{itemAction("verb")}, nil),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	engine := gin.New()
	apiGroup := engine.Group("/api/v1")
	rejectingAuth := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
	srg := swagger.NewSwaggerRouteGenerator(nil)
	srg.RegisterDocumentedRoutes(apiGroup, rejectingAuth)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/action-auth-gate-widgets/00000000-0000-0000-0000-000000000001/verb", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "rejecting auth middleware must gate the action")
	assert.NotEqual(t, "action-reached", rec.Body.String(), "action handler must NOT run when auth rejects")
}

// D2: an action route carrying a Layer 2 Access rule is enforced by
// Layer2Enforcement. Uses AdminOnly: a non-admin caller is denied (403, handler
// not reached), an admin caller is bypassed (handler reached). This proves the
// action's RoutePermission is looked up by the same METHOD:path the route mounts
// at (byRoute key == ctx.FullPath()).
func TestActionRoutes_Layer2Enforcement_AdminOnly(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	stubGlobalEnforcer(t)

	entityName := "ActionEnforcedWidget"
	adminAction := itemAction("verb")
	adminAction.Access = access.AccessRule{Type: access.AdminOnly}
	ems.RegisterTypedEntity(
		ems.GlobalEntityRegistrationService,
		entityName,
		actionTestRegistration([]entityManagementInterfaces.ActionConfig{adminAction}, nil),
	)
	t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity(entityName) })

	// Real production AdminOnly enforcer (loader/checker unused by that rule).
	access.RegisterBuiltinEnforcers(nil, nil)

	permissive := func(c *gin.Context) { c.Next() }
	actionPath := "/api/v1/action-enforced-widgets/00000000-0000-0000-0000-000000000001/verb"

	build := func(userID string, roles []string) *httptest.ResponseRecorder {
		engine := gin.New()
		apiGroup := engine.Group("/api/v1")
		apiGroup.Use(injectIdentity(userID, roles))
		apiGroup.Use(access.Layer2Enforcement())
		srg := swagger.NewSwaggerRouteGenerator(nil)
		srg.RegisterDocumentedRoutes(apiGroup, permissive)

		req := httptest.NewRequest(http.MethodPost, actionPath, nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	// Non-admin → denied by Layer2, handler not reached.
	denied := build("member-user", []string{"member"})
	assert.Equal(t, http.StatusForbidden, denied.Code, "non-admin must be denied on an AdminOnly action")
	assert.NotEqual(t, "action-reached", denied.Body.String(), "action handler must NOT run when Layer 2 denies")

	// Admin → bypass, handler reached.
	allowed := build("admin-user", []string{"administrator"})
	assert.Equal(t, actionSentinelStatus, allowed.Code, "admin must be allowed through to the action handler")
	assert.Equal(t, "action-reached", allowed.Body.String())
}
