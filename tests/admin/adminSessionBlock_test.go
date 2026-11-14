package admin_test

// PUT /admin/users/:id/session-block (#580): a platform administrator blocks a
// user from starting any session (scope "global", scope_id NULL). Blocking
// also disables the user's terminal key (DisableUserTerminalKey);
// stop_running stops every running session of the user, keeping their disks.
//
// Handler assumed: adminUsersRoutes.NewSetSessionBlockHandler(db).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/admin/routes/adminUsersRoutes"
	auditModels "soli/formations/src/audit/models"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testTools "soli/formations/tests/testTools"
)

type asbFixture struct {
	db          *gorm.DB
	mu          sync.Mutex
	stops       []string
	keyDisabled []string
	keyEnabled  []string
}

func newASBFixture(t *testing.T) *asbFixture {
	t.Helper()
	db := freshTestDB(t)
	require.NoError(t, testTools.CreateSessionBlocksTable(db))
	require.NoError(t, testTools.Truncate(db, "session_blocks"))

	f := &asbFixture{db: db}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			f.mu.Lock()
			f.stops = append(f.stops, parts[len(parts)-2])
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"state":"stopped","idle_until":%d}`, time.Now().Add(30*time.Minute).Unix())
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	original := adminUsersRoutes.DisableUserTerminalKey
	adminUsersRoutes.DisableUserTerminalKey = func(_ *gorm.DB, userID string) error {
		f.mu.Lock()
		f.keyDisabled = append(f.keyDisabled, userID)
		f.mu.Unlock()
		return nil
	}
	t.Cleanup(func() { adminUsersRoutes.DisableUserTerminalKey = original })

	// A global unblock gives the terminal key back: re-enable or regenerate
	// whichever key exists, through the existing key service.
	originalEnable := adminUsersRoutes.EnableUserTerminalKey
	adminUsersRoutes.EnableUserTerminalKey = func(_ *gorm.DB, userID string) error {
		f.mu.Lock()
		f.keyEnabled = append(f.keyEnabled, userID)
		f.mu.Unlock()
		return nil
	}
	t.Cleanup(func() { adminUsersRoutes.EnableUserTerminalKey = originalEnable })

	// The route refuses an unknown target (404): the fixture's users exist.
	originalGet := adminUsersRoutes.GetCasdoorUserByID
	adminUsersRoutes.GetCasdoorUserByID = func(userID string) (*casdoorsdk.User, error) {
		if userID == uidBob || userID == uidCarol {
			return fakeUser(userID, userID, userID, userID+"@example.com", ""), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { adminUsersRoutes.GetCasdoorUserByID = originalGet })
	return f
}

func (f *asbFixture) stopped() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stops...)
}

func (f *asbFixture) disabledKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keyDisabled...)
}

func (f *asbFixture) enabledKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keyEnabled...)
}

func asbTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state terminalModels.TerminalState) string {
	t.Helper()
	key := &terminalModels.UserTerminalKey{UserID: userID, APIKey: "key-" + uuid.NewString(), KeyName: "k", IsActive: true}
	require.NoError(t, db.Create(key).Error)
	term := &terminalModels.Terminal{
		SessionID: "asb-session-" + uuid.NewString(), UserID: userID, State: state,
		PersistenceMode: terminalModels.PersistenceModePersistent, ExpiresAt: time.Now().Add(time.Hour),
		MachineSize: "S", OrganizationID: orgID, UserTerminalKeyID: key.ID,
	}
	require.NoError(t, db.Create(term).Error)
	return term.SessionID
}

func asbPut(t *testing.T, f *asbFixture, admin bool, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	r := newAdminRouter(http.MethodPut, "/admin/users/:id/session-block", adminUsersRoutes.NewSetSessionBlockHandler(f.db), admin)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+target+"/session-block", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func asbGlobalBlocks(t *testing.T, db *gorm.DB, userID string) []terminalModels.SessionBlock {
	t.Helper()
	var rows []terminalModels.SessionBlock
	require.NoError(t, db.Where("user_id = ? AND scope_type = ?", userID, terminalModels.SessionBlockScopeGlobal).Find(&rows).Error)
	return rows
}

func TestAdminSessionBlock_BlockDisablesKeyUpdateAndUnblock(t *testing.T) {
	f := newASBFixture(t)

	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": "first"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows := asbGlobalBlocks(t, f.db, uidBob)
	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].ScopeID, "a global block has no scope id")
	assert.Equal(t, "first", rows[0].Reason)
	assert.Equal(t, uidAdminOp, rows[0].BlockedBy)
	assert.Equal(t, []string{uidBob}, f.disabledKeys(), "a global block disables the terminal key")

	w = asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": "second"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	rows = asbGlobalBlocks(t, f.db, uidBob)
	require.Len(t, rows, 1, "re-blocking updates the block")
	assert.Equal(t, "second", rows[0].Reason)

	w = asbPut(t, f, true, uidBob, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, asbGlobalBlocks(t, f.db, uidBob))

	var set []auditModels.AuditLog
	require.NoError(t, f.db.Where("event_type = ?", "user.session_block.set").Find(&set).Error)
	require.Len(t, set, 2)
	// Contract change (final round, F): identities in typed columns only,
	// has_reason instead of the reason, status "requested".
	for _, row := range set {
		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(row.Metadata), &meta))
		for _, key := range []string{"actor_user_id", "target_user_id", "impersonator_id", "reason"} {
			assert.NotContains(t, meta, key)
		}
		assert.Equal(t, true, meta["has_reason"])
		assert.Equal(t, "requested", row.Status)
		assert.Contains(t, row.Metadata, terminalModels.SessionBlockScopeGlobal)
	}
	var cleared int64
	require.NoError(t, f.db.Model(&auditModels.AuditLog{}).Where("event_type = ?", "user.session_block.cleared").Count(&cleared).Error)
	assert.Equal(t, int64(1), cleared)
	assert.Empty(t, f.stopped(), "stop_running defaults to false")
}

func TestAdminSessionBlock_StopRunning_StopsEveryRunningSession(t *testing.T) {
	f := newASBFixture(t)
	orgA, orgB := uuid.New(), uuid.New()
	a := asbTerminal(t, f.db, uidBob, &orgA, terminalModels.StateRunning)
	b := asbTerminal(t, f.db, uidBob, &orgB, terminalModels.StateRunning)
	personal := asbTerminal(t, f.db, uidBob, nil, terminalModels.StateRunning)
	asbTerminal(t, f.db, uidBob, nil, terminalModels.StateStopped)
	other := asbTerminal(t, f.db, uidCarol, &orgA, terminalModels.StateRunning)

	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.ElementsMatch(t, []string{a, b, personal}, f.stopped(), "every running session of the user is stopped, nothing else")

	var rows []terminalModels.Terminal
	require.NoError(t, f.db.Where("session_id IN ?", []string{a, b, personal}).Find(&rows).Error)
	for _, row := range rows {
		assert.Equal(t, terminalModels.StateStopped, row.State, "stopped (disk kept), not revoked or deleted")
	}
	var otherRow terminalModels.Terminal
	require.NoError(t, f.db.First(&otherRow, "session_id = ?", other).Error)
	assert.Equal(t, terminalModels.StateRunning, otherRow.State)
}

func TestAdminSessionBlock_NonAdmin_403(t *testing.T) {
	f := newASBFixture(t)
	w := asbPut(t, f, false, uidBob, map[string]any{"blocked": true, "reason": "r"})
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, asbGlobalBlocks(t, f.db, uidBob))
	assert.Empty(t, f.disabledKeys())
}

func TestAdminSessionBlock_ReasonOver500Chars_400(t *testing.T) {
	f := newASBFixture(t)
	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": strings.Repeat("x", 501)})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, asbGlobalBlocks(t, f.db, uidBob))
}

func TestAdminSessionBlock_AuditWriteFails_NothingDone(t *testing.T) {
	f := newASBFixture(t)
	running := asbTerminal(t, f.db, uidBob, nil, terminalModels.StateRunning)
	name := "asb:fail_audit_" + uuid.NewString()
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "audit_logs" {
			_ = tx.AddError(errors.New("audit store unavailable"))
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Create().Remove(name) })

	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": "r", "stop_running": true})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, asbGlobalBlocks(t, f.db, uidBob), "audit before acting: no audit, no block")
	assert.Empty(t, f.disabledKeys())
	assert.Empty(t, f.stopped())
	var row terminalModels.Terminal
	require.NoError(t, f.db.First(&row, "session_id = ?", running).Error)
	assert.Equal(t, terminalModels.StateRunning, row.State)
}

func TestAdminSessionBlockRoute_DeclaredAdminOnly(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })
	adminUsersRoutes.RegisterPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("PUT", "/api/v1/admin/users/:id/session-block")
	require.True(t, found, "route must be declared in the RouteRegistry")
	assert.Equal(t, access.RoleAdministrator, perm.Role)
	assert.Equal(t, access.AdminOnly, perm.Access.Type)
}

func TestAdminSessionBlock_UnblockReenablesTheTerminalKey(t *testing.T) {
	f := newASBFixture(t)

	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": true, "reason": "r"})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, f.enabledKeys(), "blocking never re-enables the key")

	w = asbPut(t, f, true, uidBob, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{uidBob}, f.enabledKeys(), "a global unblock re-enables the user's terminal key")
	assert.Equal(t, []string{uidBob}, f.disabledKeys(), "the unblock does not disable the key again")
}

// =============================================================================
// Final round (#580)
// =============================================================================

func TestAdminSessionBlock_UnknownTarget_404NothingDone(t *testing.T) {
	f := newASBFixture(t)
	w := asbPut(t, f, true, uidNobody, map[string]any{"blocked": true, "reason": "r"})
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, asbGlobalBlocks(t, f.db, uidNobody))
	assert.Empty(t, f.disabledKeys())
	var audits int64
	require.NoError(t, f.db.Model(&auditModels.AuditLog{}).Count(&audits).Error)
	assert.Zero(t, audits)
}

func TestAdminSessionBlock_UnblockWithoutBlock_NoKeyEnableNoClearedAudit(t *testing.T) {
	f := newASBFixture(t)
	w := asbPut(t, f, true, uidBob, map[string]any{"blocked": false})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, f.enabledKeys(), "no block was removed: the terminal key is left as it is")
	var cleared int64
	require.NoError(t, f.db.Model(&auditModels.AuditLog{}).Where("event_type = ?", "user.session_block.cleared").Count(&cleared).Error)
	assert.Zero(t, cleared, "nothing was cleared: no cleared row")
}
