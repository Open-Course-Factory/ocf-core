// tests/entityManagement/archivable_test.go
//
// Contract for the generic archiving capability (#488): an entity that opts in
// with `Archivable: true` gets archive/unarchive item actions, four lifecycle
// hook points, an explicit-column archived_at write, and a default list scope
// that hides archived rows unless the caller asks for them.
package entityManagement_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/swagger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// ArchivableThing embeds Archivable NEXT TO BaseModel, the way a product entity
// opts in.
type ArchivableThing struct {
	entityManagementModels.BaseModel
	entityManagementModels.Archivable
	Name string `json:"name"`
}

type ArchivableThingInput struct {
	Name string `json:"name"`
}

type ArchivableThingOutput struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// PlainThing has no Archivable column: registering it as Archivable must panic,
// and include_archived on its list must be a harmless no-op.
type PlainThing struct {
	entityManagementModels.BaseModel
	Name string `json:"name"`
}

type PlainThingOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

const (
	archivableEntityName = "ArchivableThing"
	archivableBasePath   = "/api/v1/archivable-things"
	plainEntityName      = "PlainThing"
)

func archivableThingRegistration(memberMethods string) entityManagementInterfaces.TypedEntityRegistration[ArchivableThing, ArchivableThingInput, ArchivableThingInput, ArchivableThingOutput] {
	swaggerCfg := entityManagementInterfaces.NewEntitySwaggerConfig(archivableEntityName, "archivable-things")
	return entityManagementInterfaces.TypedEntityRegistration[ArchivableThing, ArchivableThingInput, ArchivableThingInput, ArchivableThingOutput]{
		Converters: entityManagementInterfaces.TypedEntityConverters[ArchivableThing, ArchivableThingInput, ArchivableThingInput, ArchivableThingOutput]{
			ModelToDto: func(e *ArchivableThing) (ArchivableThingOutput, error) {
				return ArchivableThingOutput{ID: e.ID.String(), Name: e.Name, ArchivedAt: e.ArchivedAt}, nil
			},
			DtoToModel: func(dto ArchivableThingInput) *ArchivableThing {
				return &ArchivableThing{Name: dto.Name}
			},
		},
		Roles: entityManagementInterfaces.EntityRoles{
			Roles: map[string]string{
				access.RoleMember:        memberMethods,
				access.RoleAdministrator: "(GET|POST|PATCH|DELETE)",
			},
		},
		SwaggerConfig: &swaggerCfg,
		Archivable:    true,
	}
}

func plainThingRegistration() entityManagementInterfaces.TypedEntityRegistration[PlainThing, ArchivableThingInput, ArchivableThingInput, PlainThingOutput] {
	swaggerCfg := entityManagementInterfaces.NewEntitySwaggerConfig(plainEntityName, "plain-things")
	return entityManagementInterfaces.TypedEntityRegistration[PlainThing, ArchivableThingInput, ArchivableThingInput, PlainThingOutput]{
		Converters: entityManagementInterfaces.TypedEntityConverters[PlainThing, ArchivableThingInput, ArchivableThingInput, PlainThingOutput]{
			ModelToDto: func(e *PlainThing) (PlainThingOutput, error) {
				return PlainThingOutput{ID: e.ID.String(), Name: e.Name}, nil
			},
			DtoToModel: func(dto ArchivableThingInput) *PlainThing {
				return &PlainThing{Name: dto.Name}
			},
		},
		Roles: entityManagementInterfaces.EntityRoles{
			Roles: map[string]string{access.RoleMember: "(GET|POST|PATCH|DELETE)"},
		},
		SwaggerConfig: &swaggerCfg,
	}
}

// archivableTestEnv registers ArchivableThing in the GLOBAL registration service
// (the generic handlers read it), mounts the generated routes on a gin engine
// with a permissive auth middleware, and hands back the DB the handlers use.
type archivableTestEnv struct {
	db       *gorm.DB
	engine   *gin.Engine
	enforcer *authMocks.MockEnforcer
}

func setupArchivableEnv(t *testing.T, memberMethods string) *archivableTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mockEnforcer := authMocks.NewMockEnforcer()
	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	access.RouteRegistry.Reset()

	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.DisableAllHooks(false)
	t.Cleanup(func() {
		casdoor.Enforcer = origEnforcer
		hooks.GlobalHookRegistry.ClearAllHooks()
		access.RouteRegistry.Reset()
		ems.GlobalEntityRegistrationService.UnregisterEntity(archivableEntityName)
		ems.GlobalEntityRegistrationService.UnregisterEntity(plainEntityName)
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ArchivableThing{}, &PlainThing{}))

	ems.RegisterTypedEntity(ems.GlobalEntityRegistrationService, archivableEntityName, archivableThingRegistration(memberMethods))
	ems.RegisterTypedEntity(ems.GlobalEntityRegistrationService, plainEntityName, plainThingRegistration())

	engine := gin.New()
	engine.Use(injectIdentity("user-1", []string{access.RoleMember}))
	permissive := func(c *gin.Context) { c.Next() }
	swagger.NewSwaggerRouteGenerator(db).RegisterDocumentedRoutes(engine.Group("/api/v1"), permissive, permissive)

	return &archivableTestEnv{db: db, engine: engine, enforcer: mockEnforcer}
}

func (env *archivableTestEnv) createThing(t *testing.T, name string) ArchivableThing {
	t.Helper()
	thing := ArchivableThing{Name: name}
	require.NoError(t, env.db.Create(&thing).Error)
	return thing
}

func (env *archivableTestEnv) do(method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.engine.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func (env *archivableTestEnv) reload(t *testing.T, id uuid.UUID) ArchivableThing {
	t.Helper()
	var row ArchivableThing
	require.NoError(t, env.db.First(&row, "id = ?", id).Error)
	return row
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) (names []string, total int64) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data  []ArchivableThingOutput `json:"data"`
		Total int64                   `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	for _, item := range body.Data {
		names = append(names, item.Name)
	}
	return names, body.Total
}

// recordingHook captures the contexts it was called with, and optionally fails.
type recordingHook struct {
	name      string
	hookTypes []hooks.HookType
	fail      error
	seen      []*hooks.HookContext
}

func (h *recordingHook) GetName() string                { return h.name }
func (h *recordingHook) GetEntityName() string          { return archivableEntityName }
func (h *recordingHook) GetHookTypes() []hooks.HookType { return h.hookTypes }
func (h *recordingHook) IsEnabled() bool                { return true }
func (h *recordingHook) GetPriority() int               { return 50 }
func (h *recordingHook) Execute(ctx *hooks.HookContext) error {
	h.seen = append(h.seen, ctx)
	return h.fail
}

func registerRecordingHook(t *testing.T, name string, fail error, types ...hooks.HookType) *recordingHook {
	t.Helper()
	h := &recordingHook{name: name, hookTypes: types, fail: fail}
	require.NoError(t, hooks.GlobalHookRegistry.RegisterHook(h))
	return h
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestRegisterTypedEntity_Archivable_MountsArchiveAndUnarchiveActions(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|POST|PATCH|DELETE)")

	mounted := map[string]bool{}
	for _, r := range env.engine.Routes() {
		mounted[r.Method+" "+r.Path] = true
	}
	assert.True(t, mounted["POST "+archivableBasePath+"/:id/archive"], "routes: %v", env.engine.Routes())
	assert.True(t, mounted["POST "+archivableBasePath+"/:id/unarchive"], "routes: %v", env.engine.Routes())

	assert.True(t, ems.GlobalEntityRegistrationService.IsArchivable(archivableEntityName))
	assert.False(t, ems.GlobalEntityRegistrationService.IsArchivable(plainEntityName))
	assert.False(t, ems.GlobalEntityRegistrationService.IsArchivable("NoSuchEntity"))

	paths := swagger.NewDocumentationGenerator().GenerateOpenAPISpec()["paths"].(map[string]any)
	_, hasArchive := paths["/archivable-things/{id}/archive"]
	_, hasUnarchive := paths["/archivable-things/{id}/unarchive"]
	assert.True(t, hasArchive, "spec paths: %v", keysOf(paths))
	assert.True(t, hasUnarchive, "spec paths: %v", keysOf(paths))
}

func TestRegisterTypedEntity_Archivable_DerivesPolicyFromPatchPermission(t *testing.T) {
	cases := []struct {
		name          string
		memberMethods string
		wantMember    bool
		wantAccess    access.AccessRuleType
	}{
		{"member may PATCH", "(GET|PATCH)", true, access.Public},
		{"member may not PATCH", "(GET)", false, access.AdminOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupArchivableEnv(t, tc.memberMethods)
			mockEnforcer := env.enforcer

			archivePath := archivableBasePath + "/:id/archive"
			memberPolicy := false
			for _, call := range mockEnforcer.AddPolicyCalls {
				if len(call) == 3 && call[0] == access.RoleMember && call[1] == archivePath && call[2] == http.MethodPost {
					memberPolicy = true
				}
			}
			assert.Equal(t, tc.wantMember, memberPolicy, "member Casbin policy on %s; calls: %v", archivePath, mockEnforcer.AddPolicyCalls)

			for _, action := range []string{"archive", "unarchive"} {
				perm, found := access.RouteRegistry.Lookup(http.MethodPost, archivableBasePath+"/:id/"+action)
				require.True(t, found, "RoutePermission for %s", action)
				assert.Equal(t, tc.wantAccess, perm.Access.Type)
				assert.Equal(t, archivableEntityName, perm.Category)
			}
		})
	}
}

func TestRegisterTypedEntity_Archivable_PanicsWhenModelLacksArchivedAt(t *testing.T) {
	stubGlobalEnforcer(t)
	service := ems.NewEntityRegistrationService()

	reg := plainThingRegistration()
	reg.Archivable = true

	require.Panics(t, func() {
		ems.RegisterTypedEntity(service, "PlainThingArchivable", reg)
	}, "an Archivable registration whose model lacks the Archivable struct must fail at boot")
}

func TestArchivableEntitiesWithoutBeforeArchiveHook_ListsTheUnguardedOnes(t *testing.T) {
	setupArchivableEnv(t, "(GET|PATCH)")

	assert.Equal(t, []string{archivableEntityName}, ems.GlobalEntityRegistrationService.ArchivableEntitiesWithoutBeforeArchiveHook())

	registerRecordingHook(t, "guard", nil, hooks.BeforeArchive)
	assert.Empty(t, ems.GlobalEntityRegistrationService.ArchivableEntitiesWithoutBeforeArchiveHook())
}

// ---------------------------------------------------------------------------
// Archive / unarchive
// ---------------------------------------------------------------------------

func TestArchiveEntity_StampsArchivedAt_AndUnarchiveClearsIt(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "to-archive")

	rec := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out ArchivableThingOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, thing.ID.String(), out.ID)
	require.NotNil(t, out.ArchivedAt, "response DTO must report the stamp")

	row := env.reload(t, thing.ID)
	require.NotNil(t, row.ArchivedAt)
	assert.WithinDuration(t, time.Now(), *row.ArchivedAt, 5*time.Second)

	rec = env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/unarchive")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var restored ArchivableThingOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &restored))
	assert.Nil(t, restored.ArchivedAt, rec.Body.String())
	assert.Nil(t, env.reload(t, thing.ID).ArchivedAt, "nil must CLEAR the column, not be skipped as a zero value")
}

func TestArchiveEntity_UnknownId_Returns404(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")

	rec := env.do(http.MethodPost, archivableBasePath+"/"+uuid.New().String()+"/archive")
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestArchiveEntity_BeforeArchiveHookError_Refuses409_AndLeavesRowUntouched(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "guarded")
	registerRecordingHook(t, "refuse", errors.New("still has running sessions"), hooks.BeforeArchive)
	after := registerRecordingHook(t, "after", nil, hooks.AfterArchive)

	rec := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Nil(t, env.reload(t, thing.ID).ArchivedAt, "a refused archive must not touch the row")
	assert.Empty(t, after.seen, "AfterArchive must not run when BeforeArchive refused")
}

func TestArchiveEntity_BeforeArchiveHookPermissionDenied_Refuses403(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "not-yours")
	registerRecordingHook(t, "deny", entityErrors.NewUnauthorizedError("user-1", archivableEntityName, "archive"), hooks.BeforeArchive)

	rec := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")

	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Nil(t, env.reload(t, thing.ID).ArchivedAt)
}

func TestArchiveEntity_RunsAfterArchiveHook_WithLoadedEntity(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "observed")
	before := registerRecordingHook(t, "before", nil, hooks.BeforeArchive)
	after := registerRecordingHook(t, "after", nil, hooks.AfterArchive)

	rec := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, before.seen, 1)
	require.Len(t, after.seen, 1)
	for _, ctx := range []*hooks.HookContext{before.seen[0], after.seen[0]} {
		loaded, ok := ctx.NewEntity.(*ArchivableThing)
		require.True(t, ok, "NewEntity must be the loaded model, got %T", ctx.NewEntity)
		assert.Equal(t, thing.ID, loaded.ID)
		assert.Nil(t, ctx.OldEntity)
		assert.Equal(t, thing.ID, ctx.EntityID)
		assert.Equal(t, "user-1", ctx.UserID)
		assert.Equal(t, []string{access.RoleMember}, ctx.UserRoles)
	}
	assert.Equal(t, hooks.BeforeArchive, before.seen[0].HookType)
	assert.Equal(t, hooks.AfterArchive, after.seen[0].HookType)
	assert.Nil(t, before.seen[0].NewEntity.(*ArchivableThing).ArchivedAt, "BeforeArchive sees the row before the stamp")
	assert.NotNil(t, after.seen[0].NewEntity.(*ArchivableThing).ArchivedAt, "AfterArchive sees the reloaded, stamped row")
}

func TestUnarchiveEntity_RunsUnarchiveHooks(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "restored")
	stamp := time.Now().Add(-time.Hour)
	require.NoError(t, env.db.Model(&thing).Update("archived_at", &stamp).Error)
	before := registerRecordingHook(t, "before", nil, hooks.BeforeUnarchive)
	after := registerRecordingHook(t, "after", nil, hooks.AfterUnarchive)
	archiveOnly := registerRecordingHook(t, "archive-only", nil, hooks.BeforeArchive, hooks.AfterArchive)

	rec := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/unarchive")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	assert.Len(t, before.seen, 1)
	assert.Len(t, after.seen, 1)
	assert.Empty(t, archiveOnly.seen, "archive hooks must not fire on unarchive")
	assert.Nil(t, env.reload(t, thing.ID).ArchivedAt)
}

func TestArchiveEntity_IsIdempotent(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	thing := env.createThing(t, "twice")
	before := registerRecordingHook(t, "before", nil, hooks.BeforeArchive)
	after := registerRecordingHook(t, "after", nil, hooks.AfterArchive)

	first := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstStamp := env.reload(t, thing.ID).ArchivedAt
	require.NotNil(t, firstStamp)

	time.Sleep(20 * time.Millisecond)
	second := env.do(http.MethodPost, archivableBasePath+"/"+thing.ID.String()+"/archive")
	assert.Equal(t, http.StatusOK, second.Code, second.Body.String())

	assert.Equal(t, *firstStamp, *env.reload(t, thing.ID).ArchivedAt, "second archive keeps the first stamp")
	assert.Len(t, before.seen, 1, "cascades must not re-run")
	assert.Len(t, after.seen, 1, "cascades must not re-run")

	var out ArchivableThingOutput
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &out))
	assert.Equal(t, firstStamp.UTC(), out.ArchivedAt.UTC())
}

// ---------------------------------------------------------------------------
// Read scope
// ---------------------------------------------------------------------------

func seedOneArchivedOneLive(t *testing.T, env *archivableTestEnv) {
	t.Helper()
	env.createThing(t, "live")
	archived := env.createThing(t, "archived")
	stamp := time.Now()
	require.NoError(t, env.db.Model(&archived).Update("archived_at", &stamp).Error)
}

func TestGetEntities_Archivable_HidesArchivedRowsByDefault(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	seedOneArchivedOneLive(t, env)

	names, total := decodeList(t, env.do(http.MethodGet, archivableBasePath))

	assert.Equal(t, []string{"live"}, names)
	assert.Equal(t, int64(1), total, "total must exclude archived rows too")
}

func TestGetEntities_Archivable_IncludeArchivedShowsThem(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	seedOneArchivedOneLive(t, env)

	names, total := decodeList(t, env.do(http.MethodGet, archivableBasePath+"?include_archived=true"))

	assert.ElementsMatch(t, []string{"live", "archived"}, names)
	assert.Equal(t, int64(2), total)
}

func TestGetEntities_Archivable_CursorPaginationAppliesTheSameScope(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	seedOneArchivedOneLive(t, env)

	names, total := decodeList(t, env.do(http.MethodGet, archivableBasePath+"?cursor=&limit=10"))
	assert.Equal(t, []string{"live"}, names)
	assert.Equal(t, int64(1), total)

	names, total = decodeList(t, env.do(http.MethodGet, archivableBasePath+"?cursor=&limit=10&include_archived=true"))
	assert.ElementsMatch(t, []string{"live", "archived"}, names)
	assert.Equal(t, int64(2), total)
}

func TestGetEntity_Archivable_StillReturnsAnArchivedRow(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	archived := env.createThing(t, "archived")
	stamp := time.Now()
	require.NoError(t, env.db.Model(&archived).Update("archived_at", &stamp).Error)

	rec := env.do(http.MethodGet, archivableBasePath+"/"+archived.ID.String())

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out ArchivableThingOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotNil(t, out.ArchivedAt, "get-by-id never hides an archived row; it carries archived_at")
}

func TestGetEntities_NonArchivableEntity_IgnoresIncludeArchived(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	require.NoError(t, env.db.Create(&PlainThing{Name: "plain"}).Error)

	for _, query := range []string{"", "?include_archived=true", "?include_archived=false"} {
		rec := env.do(http.MethodGet, "/api/v1/plain-things"+query)
		require.Equal(t, http.StatusOK, rec.Code, "query %q: %s", query, rec.Body.String())
		var body struct {
			Data  []PlainThingOutput `json:"data"`
			Total int64              `json:"total"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, int64(1), body.Total, "query %q", query)
	}
}

func TestGenerateOpenAPISpec_Archivable_ListDocumentsIncludeArchived(t *testing.T) {
	setupArchivableEnv(t, "(GET|PATCH)")
	paths := swagger.NewDocumentationGenerator().GenerateOpenAPISpec()["paths"].(map[string]any)

	hasParam := func(basePath string) bool {
		op, _ := paths[basePath].(map[string]any)["get"].(map[string]any)
		params, _ := op["parameters"].([]map[string]any)
		for _, p := range params {
			if p["name"] == "include_archived" && p["in"] == "query" {
				return true
			}
		}
		return false
	}
	assert.True(t, hasParam("/archivable-things"), "archivable list must document include_archived")
	assert.False(t, hasParam("/plain-things"), "non-archivable list must not")
}

// ---------------------------------------------------------------------------
// Canonical scope for hand-written queries
// ---------------------------------------------------------------------------

func TestNotArchivedScope_FiltersHandWrittenQueries(t *testing.T) {
	env := setupArchivableEnv(t, "(GET|PATCH)")
	seedOneArchivedOneLive(t, env)

	var rows []ArchivableThing
	require.NoError(t, env.db.Scopes(entityManagementModels.NotArchived("archivable_things")).Find(&rows).Error)

	require.Len(t, rows, 1)
	assert.Equal(t, "live", rows[0].Name)
	assert.False(t, rows[0].IsArchived())
}
