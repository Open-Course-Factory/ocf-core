package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	courseHooks "soli/formations/src/courses/hooks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupHooks "soli/formations/src/groups/hooks"
	"soli/formations/src/initialization"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/payment"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
)

// bootProductionEntitiesAndHooks registers every entity and hook in the order
// main.go does, against a throwaway registry restored on cleanup.
func bootProductionEntitiesAndHooks(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	prevEnforcer, prevRegistry := casdoor.Enforcer, ems.GlobalEntityRegistrationService
	t.Cleanup(func() {
		casdoor.Enforcer, ems.GlobalEntityRegistrationService = prevEnforcer, prevRegistry
		hooks.GlobalHookRegistry.ClearAllHooks()
	})
	casdoor.Enforcer = mocks.NewMockEnforcer()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.ClearAllHooks()

	initialization.RegisterEntities()
	payment.InitPaymentEntities(db)
	courseHooks.InitCourseHooks(db)
	groupHooks.InitGroupHooks(db)
	organizationHooks.InitOrganizationHooks(db, nil)
	terminalHooks.InitTerminalHooks(db)
	scenarioHooks.InitScenarioHooks(db)
	ems.RegisterOwnershipHooks(db)
}

// TestMemberWritesWithoutBeforeHook_RealRegistry_ReportsNone asserts that
// every generic write a member may call runs at least one Before hook. Layer 2
// never enforces entity CRUD routes, so an unguarded one is open to every
// user.
func TestMemberWritesWithoutBeforeHook_RealRegistry_ReportsNone(t *testing.T) {
	bootProductionEntitiesAndHooks(t)

	assert.Empty(t, ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook(),
		"member-writable generic routes with no Before hook: add a hook (or an OwnershipConfig) for each")
}

// TestMemberWritesWithoutBeforeHook_GroupMemberRoleHookMissing_ReportsPatch
// replays the 0.67.2 hole: without its role-change hook, GroupMember PATCH let
// any member make themselves class owner.
func TestMemberWritesWithoutBeforeHook_GroupMemberRoleHookMissing_ReportsPatch(t *testing.T) {
	bootProductionEntitiesAndHooks(t)
	require.NoError(t, hooks.GlobalHookRegistry.UnregisterHook("group_member_role_change"))

	assert.Equal(t, []string{"GroupMember PATCH"}, ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}

// CreatorGrantThing is a minimal generic entity for the creator-grant cases.
type CreatorGrantThing struct {
	entityManagementModels.BaseModel
	UserID string `json:"user_id"`
}

type creatorGrantThingDto struct {
	UserID string `json:"user_id"`
}

// bootCreatorGrantThing registers only CreatorGrantThing, with every generic
// route mounted unless mountCreate is false, and turns ownershipOps into
// hooks. "create" is always guarded so only PATCH/DELETE are under test.
func bootCreatorGrantThing(t *testing.T, memberMethods string, mountCreate bool, ownershipOps ...string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	prevEnforcer, prevRegistry := casdoor.Enforcer, ems.GlobalEntityRegistrationService
	t.Cleanup(func() {
		casdoor.Enforcer, ems.GlobalEntityRegistrationService = prevEnforcer, prevRegistry
		hooks.GlobalHookRegistry.ClearAllHooks()
	})
	casdoor.Enforcer = mocks.NewMockEnforcer()
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry.ClearAllHooks()

	swaggerCfg := entityManagementInterfaces.NewEntitySwaggerConfig("CreatorGrantThing", "creator-grant-things")
	if !mountCreate {
		swaggerCfg.Create = nil
	}
	ems.RegisterTypedEntity(ems.GlobalEntityRegistrationService, "CreatorGrantThing",
		entityManagementInterfaces.TypedEntityRegistration[CreatorGrantThing, creatorGrantThingDto, creatorGrantThingDto, creatorGrantThingDto]{
			Converters: entityManagementInterfaces.TypedEntityConverters[CreatorGrantThing, creatorGrantThingDto, creatorGrantThingDto, creatorGrantThingDto]{
				ModelToDto: func(e *CreatorGrantThing) (creatorGrantThingDto, error) { return creatorGrantThingDto{UserID: e.UserID}, nil },
				DtoToModel: func(d creatorGrantThingDto) *CreatorGrantThing { return &CreatorGrantThing{UserID: d.UserID} },
			},
			Roles: entityManagementInterfaces.EntityRoles{Roles: map[string]string{
				access.RoleMember:        memberMethods,
				access.RoleAdministrator: "(GET|POST|PATCH|DELETE)",
			}},
			SwaggerConfig:   &swaggerCfg,
			OwnershipConfig: &access.OwnershipConfig{OwnerField: "UserID", Operations: append([]string{"create"}, ownershipOps...)},
		})
	ems.RegisterOwnershipHooks(db)
}

// TestMemberWritesWithoutBeforeHook_MemberCreatesNoUpdateDeleteHook_ReportsCreatorGrant
// covers the implicit per-row grant: after a generic create the creator may
// PATCH and DELETE that row, so both need a Before hook even when the declared
// member roles only say GET|POST.
func TestMemberWritesWithoutBeforeHook_MemberCreatesNoUpdateDeleteHook_ReportsCreatorGrant(t *testing.T) {
	bootCreatorGrantThing(t, "(GET|POST)", true)

	assert.Equal(t, []string{"CreatorGrantThing DELETE", "CreatorGrantThing PATCH"},
		ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}

func TestMemberWritesWithoutBeforeHook_MemberCreatesWithUpdateDeleteHooks_ReportsNone(t *testing.T) {
	bootCreatorGrantThing(t, "(GET|POST)", true, "update", "delete")

	assert.Empty(t, ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}

func TestMemberWritesWithoutBeforeHook_MemberCreatesWithUpdateHookOnly_ReportsDelete(t *testing.T) {
	bootCreatorGrantThing(t, "(GET|POST)", true, "update")

	assert.Equal(t, []string{"CreatorGrantThing DELETE"},
		ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}

func TestMemberWritesWithoutBeforeHook_CreateNotMounted_ReportsNone(t *testing.T) {
	bootCreatorGrantThing(t, "(GET|POST)", false)

	assert.Empty(t, ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}

func TestMemberWritesWithoutBeforeHook_CreateNotOpenToMember_ReportsNone(t *testing.T) {
	bootCreatorGrantThing(t, "(GET)", true)

	assert.Empty(t, ems.GlobalEntityRegistrationService.MemberWritesWithoutBeforeHook())
}
