// tests/terminalTrainer/terminalRename_test.go
//
// Issue #628: the "rename" action of My Sessions (ocf-front TerminalMySessions.vue)
// sends PATCH /api/v1/terminals/:id {name}. These drive that request through the
// production chain — the real AuthManagement (Layer 1, a real casbin enforcer
// loaded by the REAL Terminal registration), the ownership hooks wired the way
// main.go wires them, and the generic EditEntity handler — and assert the row in
// the database.
package terminalTrainer_tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	authController "soli/formations/src/auth"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	controller "soli/formations/src/entityManagement/routes"
	groupModels "soli/formations/src/groups/models"
	terminalRegistration "soli/formations/src/terminalTrainer/entityRegistration"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	testTools "soli/formations/tests/testTools"
)

// terminalRenameRouter mounts PATCH /api/v1/terminals/:id behind the real
// AuthManagement, the bearer token being taken verbatim as the caller's user id.
// Members and the administrator get their casbin role binding.
func terminalRenameRouter(t *testing.T, db *gorm.DB, members []string, admins []string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, thisFile, _, _ := runtime.Caller(0)
	enforcer, err := casdoor.NewEnforcer(filepath.Join(filepath.Dir(thisFile), "../../src/configuration/keymatch_model.conf"))
	require.NoError(t, err)

	authDB := testTools.MemoryDB(&authModels.TokenBlacklist{}, &authModels.BlockedAccount{}, &auditModels.AuditLog{})
	prevDB, prevParser, prevEnforcer := sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer
	prevSvc, prevHooks := ems.GlobalEntityRegistrationService, hooks.GlobalHookRegistry
	sqldb.DB = authDB
	casdoor.JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
		return &casdoorsdk.Claims{User: casdoorsdk.User{Id: token}}, nil
	}
	casdoor.Enforcer = enforcer
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	hooks.GlobalHookRegistry = hooks.NewHookRegistry()
	t.Cleanup(func() {
		sqldb.DB, casdoor.JwtTokenParser, casdoor.Enforcer = prevDB, prevParser, prevEnforcer
		ems.GlobalEntityRegistrationService, hooks.GlobalHookRegistry = prevSvc, prevHooks
		access.RouteRegistry.Reset()
	})

	// Startup order of main.go: register the entity, then wire its ownership hooks.
	terminalRegistration.RegisterTerminal(ems.GlobalEntityRegistrationService)
	ems.RegisterOwnershipHooks(db)

	for _, u := range members {
		_, err := enforcer.AddGroupingPolicy(u, string(authModels.Member))
		require.NoError(t, err)
	}
	for _, u := range admins {
		_, err := enforcer.AddGroupingPolicy(u, string(authModels.Admin))
		require.NoError(t, err)
	}

	gc := controller.NewGenericController(db, nil)
	r := gin.New()
	r.PATCH("/api/v1/terminals/:id", authController.NewAuthMiddleware(authDB).AuthManagement(), func(c *gin.Context) { gc.EditEntity(c) })
	return r
}

func patchTerminalAs(r *gin.Engine, userID, terminalID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/terminals/"+terminalID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func reloadTerminal(t *testing.T, db *gorm.DB, id string) models.Terminal {
	t.Helper()
	var got models.Terminal
	require.NoError(t, db.First(&got, "id = ?", id).Error)
	return got
}

func TestTerminalRename_OwnerMember_RenamesOwnTerminal(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)
	term, err := createTestTerminal(db, "owner-1", "running", nil)
	require.NoError(t, err)

	w := patchTerminalAs(r, "owner-1", term.ID.String(), `{"name":"My lab"}`)

	assert.Equal(t, http.StatusNoContent, w.Code, "the owner must be able to rename their terminal; body=%s", w.Body.String())
	assert.Equal(t, "My lab", reloadTerminal(t, db, term.ID.String()).Name)
}

// Guard for the fix: opening PATCH to members must not let one rename another's terminal.
func TestTerminalRename_OtherMember_CannotRename(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1", "intruder-1"}, nil)
	term, err := createTestTerminal(db, "owner-1", "running", nil)
	require.NoError(t, err)

	w := patchTerminalAs(r, "intruder-1", term.ID.String(), `{"name":"pwned"}`)

	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "Test Terminal", reloadTerminal(t, db, term.ID.String()).Name)
}

// The owner's rename must not carry lifecycle fields: state is synced from
// tt-backend and drives budget accounting, persistence_mode is chosen at launch.
func TestTerminalRename_OwnerSendsStateAndPersistence_OnlyNameChanges(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)
	term, err := createTestTerminal(db, "owner-1", "stopped", nil)
	require.NoError(t, err)
	before := reloadTerminal(t, db, term.ID.String())

	w := patchTerminalAs(r, "owner-1", term.ID.String(), `{"name":"My lab","state":"running","persistence_mode":"persistent"}`)

	require.Less(t, w.Code, 300, "body=%s", w.Body.String())
	after := reloadTerminal(t, db, term.ID.String())
	assert.Equal(t, "My lab", after.Name)
	assert.Equal(t, before.State, after.State, "state must not be writable through PATCH")
	assert.Equal(t, before.PersistenceMode, after.PersistenceMode, "persistence_mode must not be writable through PATCH")
}

// Same contract for the administrator, who passes Layer 1 today: the generic
// PATCH writes state and persistence_mode straight to the row.
func TestTerminalRename_AdminSendsStateAndPersistence_Ignored(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, nil, []string{"admin-1"})
	term, err := createTestTerminal(db, "owner-1", "stopped", nil)
	require.NoError(t, err)
	before := reloadTerminal(t, db, term.ID.String())

	patchTerminalAs(r, "admin-1", term.ID.String(), `{"state":"running","persistence_mode":"persistent"}`)

	after := reloadTerminal(t, db, term.ID.String())
	assert.Equal(t, before.State, after.State, "state must not be writable through PATCH")
	assert.Equal(t, before.PersistenceMode, after.PersistenceMode, "persistence_mode must not be writable through PATCH")
}

func TestTerminalRename_Admin_RenamesAnyTerminal(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, nil, []string{"admin-1"})
	term, err := createTestTerminal(db, "owner-1", "running", nil)
	require.NoError(t, err)

	w := patchTerminalAs(r, "admin-1", term.ID.String(), `{"name":"Renamed by admin"}`)

	assert.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "Renamed by admin", reloadTerminal(t, db, term.ID.String()).Name)
}

func TestTerminalRename_MissingTerminal_Returns404(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)

	w := patchTerminalAs(r, "owner-1", uuid.New().String(), `{"name":"x"}`)

	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

func TestTerminalRename_SoftDeletedTerminal_Returns404(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)
	term, err := createTestTerminal(db, "owner-1", "running", nil)
	require.NoError(t, err)
	require.NoError(t, db.Delete(term).Error)

	w := patchTerminalAs(r, "owner-1", term.ID.String(), `{"name":"x"}`)

	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	var got models.Terminal
	require.NoError(t, db.Unscoped().First(&got, "id = ?", term.ID).Error)
	assert.Equal(t, "Test Terminal", got.Name)
}

// Current contract, not a product decision: the generic PATCH has no length or
// emptiness validation, so an empty name is stored as is.
func TestTerminalRename_EmptyName_StoredAsIs(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)
	term, err := createTestTerminal(db, "owner-1", "running", nil)
	require.NoError(t, err)

	w := patchTerminalAs(r, "owner-1", term.ID.String(), `{"name":""}`)

	assert.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "", reloadTerminal(t, db, term.ID.String()).Name)
}

// Update ownership is owner-only: managing the learner's class (which allows
// stopping their terminal through the participant routes) does not allow a rename.
func TestTerminalRename_ClassManager_CannotRenameLearnerTerminal(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"learner-1", "manager-1"}, nil)
	g := createTestGroup(t, db, "manager-1")
	addGroupMember(t, db, g.ID, "learner-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, g.ID, "manager-1", groupModels.GroupMemberRoleManager)
	term, err := createTestTerminal(db, "learner-1", "running", nil)
	require.NoError(t, err)

	w := patchTerminalAs(r, "manager-1", term.ID.String(), `{"name":"by-manager"}`)

	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "Test Terminal", reloadTerminal(t, db, term.ID.String()).Name)
}

// Ended sessions stay in My Sessions history, so they stay renamable.
func TestTerminalRename_EndedTerminals_CanBeRenamed(t *testing.T) {
	db := freshTestDB(t)
	r := terminalRenameRouter(t, db, []string{"owner-1"}, nil)
	for _, state := range []string{"stopped", "deleted", "revoked"} {
		term, err := createTestTerminal(db, "owner-1", state, nil)
		require.NoError(t, err)
		w := patchTerminalAs(r, "owner-1", term.ID.String(), `{"name":"r-`+state+`"}`)
		assert.Equal(t, http.StatusNoContent, w.Code, "%s: body=%s", state, w.Body.String())
		assert.Equal(t, "r-"+state, reloadTerminal(t, db, term.ID.String()).Name)
	}
	expired, err := createTestTerminal(db, "owner-1", "running", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	w := patchTerminalAs(r, "owner-1", expired.ID.String(), `{"name":"r-expired"}`)
	assert.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "r-expired", reloadTerminal(t, db, expired.ID.String()).Name)
}

// Member lost POST on /terminals/:id: the single-segment POST routes the front
// calls must still pass Layer 1 through their own policies.
func TestTerminalRename_MemberPostRoutes_StillAllowedAtLayer1(t *testing.T) {
	db := freshTestDB(t)
	terminalRenameRouter(t, db, nil, nil)
	terminalController.RegisterTerminalPermissions(casdoor.Enforcer)

	for _, path := range []string{"/api/v1/terminals/start-composed-session", "/api/v1/terminals/sync-all"} {
		ok, err := casdoor.Enforcer.Enforce(string(authModels.Member), path, http.MethodPost)
		require.NoError(t, err)
		assert.True(t, ok, "member POST %s must stay allowed", path)
	}
	ok, err := casdoor.Enforcer.Enforce(string(authModels.Member), "/api/v1/terminals", http.MethodPost)
	require.NoError(t, err)
	assert.False(t, ok, "the generic create stays closed to members: sessions start through start-composed-session")
}
