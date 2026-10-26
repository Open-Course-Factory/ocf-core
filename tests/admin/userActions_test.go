package admin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/admin/routes/adminUsersRoutes"
	"soli/formations/src/auth/mocks"
	authServices "soli/formations/src/auth/services"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// ---------------------------------------------------------------------------
// Per-user admin actions: send-password-reset-email, grant/revoke admin role,
// block/unblock account (forbidden). Every handler is admin-gated by
// requireAdmin + the "userRoles" context value the real AuthManagement
// middleware sets — adminCtxInjector (users_test.go) stands in for it here.
// ---------------------------------------------------------------------------

func nonAdminCtxInjector() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userId", uidAlice)
		c.Set("userRoles", []string{"member"})
		c.Next()
	}
}

func newAdminRouter(method, path string, handler gin.HandlerFunc, admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if admin {
		r.Use(adminCtxInjector())
	} else {
		r.Use(nonAdminCtxInjector())
	}
	r.Handle(method, path, handler)
	return r
}

// --- Send password reset email ---------------------------------------------

func TestSendPasswordResetHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	originalGet := adminUsersRoutes.GetCasdoorUserByID
	originalSend := adminUsersRoutes.SendPasswordResetEmail
	t.Cleanup(func() {
		adminUsersRoutes.GetCasdoorUserByID = originalGet
		adminUsersRoutes.SendPasswordResetEmail = originalSend
	})

	adminUsersRoutes.GetCasdoorUserByID = func(userID string) (*casdoorsdk.User, error) {
		assert.Equal(t, uidAlice, userID)
		return fakeUser(uidAlice, "alice", "Alice", "alice@example.com", ""), nil
	}
	var sentEmail, sentURL string
	adminUsersRoutes.SendPasswordResetEmail = func(_ *gorm.DB, email, resetURL string) error {
		sentEmail = email
		sentURL = resetURL
		return nil
	}

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/send-password-reset-email",
		adminUsersRoutes.NewSendPasswordResetHandler(db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidAlice+"/send-password-reset-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "alice@example.com", sentEmail, "must send to the target user's own email")
	assert.NotEmpty(t, sentURL, "must pass a reset URL")

	var got adminUsersRoutes.ActionResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Success)
}

func TestSendPasswordResetHandler_UnknownUser_404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	originalGet := adminUsersRoutes.GetCasdoorUserByID
	t.Cleanup(func() { adminUsersRoutes.GetCasdoorUserByID = originalGet })
	adminUsersRoutes.GetCasdoorUserByID = func(userID string) (*casdoorsdk.User, error) {
		return nil, nil
	}

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/send-password-reset-email",
		adminUsersRoutes.NewSendPasswordResetHandler(db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidNobody+"/send-password-reset-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSendPasswordResetHandler_NonAdmin_403(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/send-password-reset-email",
		adminUsersRoutes.NewSendPasswordResetHandler(db), false)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidBob+"/send-password-reset-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Grant / revoke admin role -----------------------------------------------

func TestGrantAdminRoleHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalGet := adminUsersRoutes.GetCasdoorUserByID
	t.Cleanup(func() { adminUsersRoutes.GetCasdoorUserByID = originalGet })
	adminUsersRoutes.GetCasdoorUserByID = func(userID string) (*casdoorsdk.User, error) {
		return fakeUser(userID, "bob", "Bob", "bob@example.com", ""), nil
	}

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/admin-role",
		adminUsersRoutes.NewGrantAdminRoleHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidBob+"/admin-role", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Equal(t, 1, enforcer.GetAddGroupingPolicyCallCount())
	call := enforcer.AddGroupingPolicyCalls[0]
	assert.Equal(t, uidBob, call[0])
	assert.Equal(t, "administrator", call[1])
}

func TestGrantAdminRoleHandler_UnknownUser_404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalGet := adminUsersRoutes.GetCasdoorUserByID
	t.Cleanup(func() { adminUsersRoutes.GetCasdoorUserByID = originalGet })
	adminUsersRoutes.GetCasdoorUserByID = func(string) (*casdoorsdk.User, error) { return nil, nil }

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/admin-role",
		adminUsersRoutes.NewGrantAdminRoleHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidNobody+"/admin-role", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, enforcer.GetAddGroupingPolicyCallCount(), "must not bind a role to an id nobody owns")
}

func TestGrantAdminRoleHandler_NonAdmin_403(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/admin-role",
		adminUsersRoutes.NewGrantAdminRoleHandler(enforcer, db), false)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidBob+"/admin-role", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, 0, enforcer.GetAddGroupingPolicyCallCount(), "must not touch the enforcer when unauthorized")
}

func TestRevokeAdminRoleHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	r := newAdminRouter(http.MethodDelete, "/admin/users/:id/admin-role",
		adminUsersRoutes.NewRevokeAdminRoleHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+uidBob+"/admin-role", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Equal(t, 1, enforcer.GetRemoveGroupingPolicyCallCount())
	call := enforcer.RemoveGroupingPolicyCalls[0]
	assert.Equal(t, uidBob, call[0])
	assert.Equal(t, "administrator", call[1])
}

func TestRevokeAdminRoleHandler_CannotRevokeOwnRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	// adminCtxInjector sets userId = uidAdminOp — target the same id.
	r := newAdminRouter(http.MethodDelete, "/admin/users/:id/admin-role",
		adminUsersRoutes.NewRevokeAdminRoleHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+uidAdminOp+"/admin-role", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, enforcer.GetRemoveGroupingPolicyCallCount(), "must refuse before touching the enforcer")
}

// --- Block / unblock account (forbidden) ------------------------------------

func TestSetForbiddenHandler_BlocksAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetAccountForbidden
	originalKeyDisable := adminUsersRoutes.DisableUserTerminalKey
	t.Cleanup(func() {
		adminUsersRoutes.SetAccountForbidden = original
		adminUsersRoutes.DisableUserTerminalKey = originalKeyDisable
	})

	var calledUserID string
	var calledForbidden bool
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error {
		calledUserID = userID
		calledForbidden = forbidden
		return nil
	}
	var keyDisabledFor string
	adminUsersRoutes.DisableUserTerminalKey = func(_ *gorm.DB, userID string) error {
		keyDisabledFor = userID
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: true})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, uidBob, calledUserID)
	assert.True(t, calledForbidden)
	assert.Equal(t, uidBob, keyDisabledFor,
		"blocking sign-in must also disable the account's tt-backend terminal key, "+
			"or a user already holding that key keeps every terminal capability it grants")
}

// A blocked user's already-issued tt-backend key must not survive the block
// even if tt-backend itself is unreachable — the account block (Casdoor) is
// the point of this endpoint and must still succeed.
func TestSetForbiddenHandler_BlocksAccountEvenIfKeyDisableFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetAccountForbidden
	originalKeyDisable := adminUsersRoutes.DisableUserTerminalKey
	t.Cleanup(func() {
		adminUsersRoutes.SetAccountForbidden = original
		adminUsersRoutes.DisableUserTerminalKey = originalKeyDisable
	})
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error { return nil }
	adminUsersRoutes.DisableUserTerminalKey = func(_ *gorm.DB, userID string) error {
		return fmt.Errorf("tt-backend unreachable")
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: true})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "a tt-backend key-disable failure must not fail the block itself; body=%s", w.Body.String())
}

// Unblocking must not try to disable the key — there is nothing to revoke
// for a sign-in being restored, and it would also be a pointless call for
// the common case (the account's key, if any, is already inactive from the
// original block).
func TestSetForbiddenHandler_UnblockDoesNotTouchTerminalKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	originalForbidden := adminUsersRoutes.SetAccountForbidden
	originalKeyDisable := adminUsersRoutes.DisableUserTerminalKey
	t.Cleanup(func() {
		adminUsersRoutes.SetAccountForbidden = originalForbidden
		adminUsersRoutes.DisableUserTerminalKey = originalKeyDisable
	})
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error { return nil }
	keyDisableCalled := false
	adminUsersRoutes.DisableUserTerminalKey = func(_ *gorm.DB, userID string) error {
		keyDisableCalled = true
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: false})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.False(t, keyDisableCalled)
}

// A running terminal must not outlive the block: TerminateUserTerminals
// revokes it, which also frees its slot and budget.
func TestSetForbiddenHandler_BlockRevokesRunningTerminals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	originalForbidden := adminUsersRoutes.SetAccountForbidden
	originalKeyDisable := adminUsersRoutes.DisableUserTerminalKey
	t.Cleanup(func() {
		adminUsersRoutes.SetAccountForbidden = originalForbidden
		adminUsersRoutes.DisableUserTerminalKey = originalKeyDisable
	})
	adminUsersRoutes.SetAccountForbidden = func(string, bool) error { return nil }
	adminUsersRoutes.DisableUserTerminalKey = func(*gorm.DB, string) error { return nil }

	key := terminalModels.UserTerminalKey{UserID: uidBob, APIKey: "key-bob", KeyName: "bob", IsActive: true}
	require.NoError(t, db.Create(&key).Error)
	term := terminalModels.Terminal{
		SessionID: "sess-bob", UserID: uidBob, State: terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour), UserTerminalKeyID: key.ID,
	}
	require.NoError(t, db.Create(&term).Error)

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)
	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: true})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var after terminalModels.Terminal
	require.NoError(t, db.First(&after, "id = ?", term.ID).Error)
	assert.Equal(t, terminalModels.StateRevoked, after.State)
}

func TestSetForbiddenHandler_UnknownUser_404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetAccountForbidden
	t.Cleanup(func() { adminUsersRoutes.SetAccountForbidden = original })
	adminUsersRoutes.SetAccountForbidden = func(userID string, _ bool) error {
		return fmt.Errorf("%w: %s", authServices.ErrUserNotFound, userID)
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)
	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: true})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidNobody+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSetForbiddenHandler_CannotBlockOwnAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetAccountForbidden
	t.Cleanup(func() { adminUsersRoutes.SetAccountForbidden = original })
	called := false
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error {
		called = true
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: true})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidAdminOp+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, called, "must refuse before calling SetAccountForbidden")
}

func TestSetForbiddenHandler_CanUnblockOwnAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetAccountForbidden
	t.Cleanup(func() { adminUsersRoutes.SetAccountForbidden = original })
	called := false
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error {
		called = true
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/forbidden",
		adminUsersRoutes.NewSetForbiddenHandler(db), true)

	// Unblocking (forbidden=false) one's own account is not the lockout risk
	// the guard exists for — only blocking (forbidden=true) is refused.
	body, _ := json.Marshal(adminUsersRoutes.SetForbiddenInput{Forbidden: false})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidAdminOp+"/forbidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.True(t, called)
}

// --- MemberID surfaced in the listing (groups need it for the frontend's
//     generic PATCH /group-members/:id role-change control) --------------

func TestBuildUserListings_SurfacesMemberIDForOrgAndGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	orgID := seedOrg(t, db, "OrgOne", uidAlice)
	seedOrgMember(t, db, orgID, uidAlice, orgModels.OrgRoleOwner, true)
	groupID := seedGroup(t, db, "GroupOne", uidAlice)
	seedGroupMember(t, db, groupID, uidAlice, groupModels.GroupMemberRoleManager, true)

	users := []*casdoorsdk.User{fakeUser(uidAlice, "alice", "Alice", "alice@example.com", "")}
	listings, err := adminUsersRoutes.BuildUserListings(users, db, noAdminFn())
	require.NoError(t, err)

	got := findListing(t, listings, uidAlice)
	require.Len(t, got.Organizations, 1)
	require.Len(t, got.Groups, 1)
	assert.NotEmpty(t, got.Organizations[0].MemberID, "org membership row id must be surfaced")
	assert.NotEmpty(t, got.Groups[0].MemberID, "group membership row id must be surfaced")
}
