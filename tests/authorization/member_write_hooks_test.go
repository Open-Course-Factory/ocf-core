package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	authHooks "soli/formations/src/auth/hooks"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	courseHooks "soli/formations/src/courses/hooks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
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
	authHooks.InitAuthHooks(db)
	groupHooks.InitGroupHooks(db)
	organizationHooks.InitOrganizationHooks(db)
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
