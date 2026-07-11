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
