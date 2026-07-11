package entityManagement_tests

// RED tests for MR L — make OwnershipConfig actually register the write-side
// ownership hooks (doc/code drift fix).
//
// Today the write-side ownership hooks (BeforeCreate forces owner;
// BeforeUpdate/BeforeDelete verify owner) are hand-registered in four module
// Init*Hooks funcs (terminal, scenario, auth, payment). The registration
// carries the SAME OwnershipConfig that the entity already declares (or should
// declare) at registration time, so the manual wiring is redundant with the
// documented contract in CLAUDE.md:
//
//	OwnershipConfig{Operations: ["create","update","delete"]}
//	  → "auto-generates hooks. No hand-written hook files needed."
//
// MR L closes that drift by adding a single pass, ems.RegisterOwnershipHooks(db),
// that walks every registered entity's stored OwnershipConfig and registers a
// generic ownership hook for its WRITE operations (create/update/delete),
// exactly mirroring what the manual NewOwnershipHook calls do today. The "read"
// op is request-time read-scoping and must NOT produce a write hook.
//
// These tests are RED because ems.RegisterOwnershipHooks does not exist yet
// (compile failure on the missing symbol). They assert USER-OBSERVABLE hook
// behaviour, not mock calls:
//
//   - WriteOpEntity_ForcesOwnerOnCreate: after the auto-registration pass, a
//     forged owner on a create is overwritten with the authenticated caller —
//     the same anti-impersonation contract the ScenarioSession forgery test
//     pins, proving the auto-registered hook fires identically to the old
//     manual one.
//   - ReadOnlyEntity_RegistersNoWriteHook: an entity whose OwnershipConfig only
//     declares "read" must get NO create/update/delete hook (guards against
//     over-registration; e.g. UserTerminalKey stays read-only).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	authRegistration "soli/formations/src/auth/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	paymentRegistration "soli/formations/src/payment/entityRegistration"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"
)

// wiredOwnershipModel is a throwaway entity with a settable string owner field,
// so the reflection-based BeforeCreate ownership hook can force it.
type wiredOwnershipModel struct {
	ID     string `gorm:"primaryKey"`
	UserID string
}

// newWiringTestDB returns a fresh in-memory SQLite DB. The BeforeCreate owner-
// forcing path is pure reflection over ctx.NewEntity and never touches the DB,
// but NewOwnershipHook needs a *gorm.DB for its update/delete entity loader.
func newWiringTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open in-memory SQLite DB")
	return db
}

// swapGlobalsForWiring installs fresh, isolated global registration + hook
// registries for the duration of a test, restoring the originals afterwards.
// RegisterOwnershipHooks reads the entity OwnershipConfigs from
// GlobalEntityRegistrationService and registers the resulting hooks into
// GlobalHookRegistry, so both globals must be isolated to keep the test
// hermetic and free of cross-test hook-name collisions.
func swapGlobalsForWiring(t *testing.T) {
	t.Helper()
	origSvc := ems.GlobalEntityRegistrationService
	origReg := hooks.GlobalHookRegistry
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry = hooks.NewHookRegistry()
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = origSvc
		hooks.GlobalHookRegistry = origReg
	})
}

// TestRegisterOwnershipHooks_WriteOpEntity_ForcesOwnerOnCreate — an entity that
// declares {read, create} in its OwnershipConfig must, after the auto pass, have
// a BeforeCreate ownership hook that forces the owner field to the authenticated
// caller (overwriting a forged value). Drives the REAL hook chain via
// ExecuteHooks and asserts the mutated entity, not a mock call.
//
// RED today: ems.RegisterOwnershipHooks does not exist (compile failure).
func TestRegisterOwnershipHooks_WriteOpEntity_ForcesOwnerOnCreate(t *testing.T) {
	db := newWiringTestDB(t)
	swapGlobalsForWiring(t)

	const entityName = "WiredOwnershipEntity"
	ems.GlobalEntityRegistrationService.RegisterOwnershipConfig(entityName, &access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"read", "create"}, // read = request-time scope; create = write hook
		AdminBypass: true,
	})

	// The production change under test: walk registered OwnershipConfigs and
	// auto-register the write-side ownership hooks.
	ems.RegisterOwnershipHooks(db)

	// A create hook must now exist for this entity.
	createHooks := hooks.GlobalHookRegistry.GetHooks(entityName, hooks.BeforeCreate)
	require.NotEmpty(t, createHooks,
		"RegisterOwnershipHooks must register a BeforeCreate ownership hook for an entity declaring the create op")

	// Behavioural drive: a member forges another user's id on create; the
	// auto-registered hook must overwrite it with the authenticated caller.
	entity := &wiredOwnershipModel{ID: "row-1", UserID: "forged-user-B"}
	ctx := &hooks.HookContext{
		EntityName: entityName,
		HookType:   hooks.BeforeCreate,
		NewEntity:  entity,
		UserID:     "caller-A",
		UserRoles:  []string{"member"},
	}
	require.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(ctx),
		"BeforeCreate ownership hook must not error for a normal member create")

	assert.Equal(t, "caller-A", entity.UserID,
		"auto-registered create hook must force the owner to the authenticated caller, not the forged value")
	assert.NotEqual(t, "forged-user-B", entity.UserID,
		"forged owner must never survive the create (anti-impersonation contract)")
}

// TestRegisterOwnershipHooks_ReadOnlyEntity_RegistersNoWriteHook — an entity
// whose OwnershipConfig only declares "read" must get NO write hook. This is the
// contract that keeps read-only entities (e.g. UserTerminalKey) from gaining a
// spurious create/update/delete guard, and mirrors operationsToHookTypes
// ignoring "read".
//
// RED today: ems.RegisterOwnershipHooks does not exist (compile failure).
func TestRegisterOwnershipHooks_ReadOnlyEntity_RegistersNoWriteHook(t *testing.T) {
	db := newWiringTestDB(t)
	swapGlobalsForWiring(t)

	const entityName = "ReadOnlyWiredEntity"
	ems.GlobalEntityRegistrationService.RegisterOwnershipConfig(entityName, &access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"read"}, // read-only: no write hook expected
		AdminBypass: true,
	})

	ems.RegisterOwnershipHooks(db)

	assert.Empty(t, hooks.GlobalHookRegistry.GetHooks(entityName, hooks.BeforeCreate),
		"read-only OwnershipConfig must not register a BeforeCreate hook")
	assert.Empty(t, hooks.GlobalHookRegistry.GetHooks(entityName, hooks.BeforeUpdate),
		"read-only OwnershipConfig must not register a BeforeUpdate hook")
	assert.Empty(t, hooks.GlobalHookRegistry.GetHooks(entityName, hooks.BeforeDelete),
		"read-only OwnershipConfig must not register a BeforeDelete hook")
}

// TestRegisterOwnershipHooks_RealEntities_WireExpectedWriteHooks — the
// silent-regression guard flagged during the invariant map. It registers the
// REAL entity registrations that must carry write ownership hooks, runs the
// auto pass, and asserts the exact write hooks the manual NewOwnershipHook calls
// register today. It is RED on TWO counts until MR L lands correctly:
//
//  1. ems.RegisterOwnershipHooks does not exist yet (compile failure); AND
//  2. the 3 payment registrations (BillingAddress/PaymentMethod/UserSubscription)
//     currently declare NO OwnershipConfig at all — their write protection lives
//     only in the manual InitPaymentHooks calls that MR L removes. If the
//     implementer adds RegisterOwnershipHooks but forgets to add
//     OwnershipConfig{create,update,delete} to those 3 registrations, THIS test
//     fails (empty hook set) — catching the silent loss of payment ownership
//     enforcement that the direct-construction payment tests cannot see.
//
// The table encodes the exact per-entity write hooks (behaviour-preserving map
// from each entity's declared write ops). For each entity we assert the expected
// write hooks are present AND that no other write hook is registered, so an
// implementer who over-registers (e.g. all of C/U/D regardless of declared ops)
// is also caught.
func TestRegisterOwnershipHooks_RealEntities_WireExpectedWriteHooks(t *testing.T) {
	db := newWiringTestDB(t)
	swapGlobalsForWiring(t)

	svc := ems.GlobalEntityRegistrationService
	terminalRegistration.RegisterTerminal(svc)
	scenarioRegistration.RegisterScenarioSession(svc)
	authRegistration.RegisterUserSettings(svc)
	paymentRegistration.RegisterBillingAddress(svc)
	paymentRegistration.RegisterPaymentMethod(svc)
	paymentRegistration.RegisterUserSubscription(svc)

	ems.RegisterOwnershipHooks(db)

	// entityName → the exact set of write hooks it must have after the pass.
	expected := map[string][]hooks.HookType{
		"Terminal":         {hooks.BeforeCreate},
		"ScenarioSession":  {hooks.BeforeCreate},
		"UserSetting":      {hooks.BeforeUpdate},
		"BillingAddress":   {hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
		"PaymentMethod":    {hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
		"UserSubscription": {hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
	}

	allWriteTypes := []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}

	for entityName, want := range expected {
		wantSet := map[hooks.HookType]bool{}
		for _, ht := range want {
			wantSet[ht] = true
		}
		for _, ht := range allWriteTypes {
			registered := hooks.GlobalHookRegistry.GetHooks(entityName, ht)
			if wantSet[ht] {
				assert.NotEmpty(t, registered,
					"%s must have a %s ownership hook wired by RegisterOwnershipHooks", entityName, ht)
			} else {
				assert.Empty(t, registered,
					"%s must NOT have a %s ownership hook (not in its declared write ops)", entityName, ht)
			}
		}
	}
}
