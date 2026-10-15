package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/admin/routes/adminUsersRoutes"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
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
	t.Cleanup(func() { adminUsersRoutes.SetAccountForbidden = original })

	var calledUserID string
	var calledForbidden bool
	adminUsersRoutes.SetAccountForbidden = func(userID string, forbidden bool) error {
		calledUserID = userID
		calledForbidden = forbidden
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
