package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/admin/routes/adminUsersRoutes"
	"soli/formations/src/auth/mocks"
	authServices "soli/formations/src/auth/services"
)

// ---------------------------------------------------------------------------
// Admin-created accounts (POST /admin/users) and forced email verification
// (POST /admin/users/:id/verify-email). Both bypass the self-service flows
// (ToS acceptance, verification token) that don't apply when an admin is
// the one acting.
// ---------------------------------------------------------------------------

func validCreateUserBody() adminUsersRoutes.AdminCreateUserInput {
	return adminUsersRoutes.AdminCreateUserInput{
		Username:    "newteacher",
		DisplayName: "New Teacher",
		Email:       "newteacher@example.com",
		Password:    "correct-horse-battery-staple",
		FirstName:   "New",
		LastName:    "Teacher",
	}
}

func TestCreateUserHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalCreate := adminUsersRoutes.CreateCasdoorUser
	originalGetByEmail := adminUsersRoutes.GetCasdoorUserByEmail
	originalExtras := adminUsersRoutes.ProvisionNewUserExtras
	originalNativeRole := adminUsersRoutes.AddCasdoorNativeRole
	t.Cleanup(func() {
		adminUsersRoutes.CreateCasdoorUser = originalCreate
		adminUsersRoutes.GetCasdoorUserByEmail = originalGetByEmail
		adminUsersRoutes.ProvisionNewUserExtras = originalExtras
		adminUsersRoutes.AddCasdoorNativeRole = originalNativeRole
	})
	// AddCasdoorNativeRole calls the Casdoor SDK's package-level client
	// directly, which panics (rather than erroring) when unconfigured —
	// always stub it out in tests.
	adminUsersRoutes.AddCasdoorNativeRole = func(_ casdoorsdk.User) error { return nil }

	var createdArg *casdoorsdk.User
	adminUsersRoutes.CreateCasdoorUser = func(user *casdoorsdk.User) error {
		createdArg = user
		return nil
	}
	adminUsersRoutes.GetCasdoorUserByEmail = func(email string) (*casdoorsdk.User, error) {
		return fakeUser("newteacher-id", "newteacher", "New Teacher", email, ""), nil
	}
	var extrasUserID, extrasUsername string
	adminUsersRoutes.ProvisionNewUserExtras = func(_ *gorm.DB, userID, username string) {
		extrasUserID = userID
		extrasUsername = username
	}

	body := validCreateUserBody()
	payload, _ := json.Marshal(body)

	r := newAdminRouter(http.MethodPost, "/admin/users",
		adminUsersRoutes.NewCreateUserHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	require.NotNil(t, createdArg)
	assert.Equal(t, "newteacher", createdArg.Name)
	assert.Equal(t, "newteacher@example.com", createdArg.Email)
	assert.Equal(t, "correct-horse-battery-staple", createdArg.Password)
	assert.False(t, createdArg.EmailVerified, "email_verified defaults to false unless requested")
	assert.NotEmpty(t, createdArg.Properties["tos_accepted_at"], "ToS acceptance is synthesized, not collected")
	assert.Equal(t, "true", createdArg.Properties["created_by_admin"])

	var got adminUsersRoutes.AdminCreateUserOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "newteacher-id", got.ID)

	require.GreaterOrEqual(t, enforcer.GetAddGroupingPolicyCallCount(), 1)
	assert.Equal(t, "newteacher-id", enforcer.AddGroupingPolicyCalls[0][0])
	assert.Equal(t, "member", enforcer.AddGroupingPolicyCalls[0][1])

	assert.Equal(t, "newteacher-id", extrasUserID, "starter resources must be provisioned for the new account")
	assert.Equal(t, "newteacher", extrasUsername)
}

func TestCreateUserHandler_EmailVerifiedFlagMarksAccountVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalCreate := adminUsersRoutes.CreateCasdoorUser
	originalGetByEmail := adminUsersRoutes.GetCasdoorUserByEmail
	originalExtras := adminUsersRoutes.ProvisionNewUserExtras
	originalNativeRole := adminUsersRoutes.AddCasdoorNativeRole
	t.Cleanup(func() {
		adminUsersRoutes.CreateCasdoorUser = originalCreate
		adminUsersRoutes.GetCasdoorUserByEmail = originalGetByEmail
		adminUsersRoutes.ProvisionNewUserExtras = originalExtras
		adminUsersRoutes.AddCasdoorNativeRole = originalNativeRole
	})
	// AddCasdoorNativeRole calls the Casdoor SDK's package-level client
	// directly, which panics (rather than erroring) when unconfigured —
	// always stub it out in tests.
	adminUsersRoutes.AddCasdoorNativeRole = func(_ casdoorsdk.User) error { return nil }

	var createdArg *casdoorsdk.User
	adminUsersRoutes.CreateCasdoorUser = func(user *casdoorsdk.User) error {
		createdArg = user
		return nil
	}
	adminUsersRoutes.GetCasdoorUserByEmail = func(email string) (*casdoorsdk.User, error) {
		return fakeUser("newteacher-id", "newteacher", "New Teacher", email, ""), nil
	}
	adminUsersRoutes.ProvisionNewUserExtras = func(_ *gorm.DB, userID, username string) {}

	body := validCreateUserBody()
	body.EmailVerified = true
	payload, _ := json.Marshal(body)

	r := newAdminRouter(http.MethodPost, "/admin/users",
		adminUsersRoutes.NewCreateUserHandler(enforcer, db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	require.NotNil(t, createdArg)
	assert.True(t, createdArg.EmailVerified, "email_verified:true in the request must mark the account verified at creation")
	assert.NotEmpty(t, createdArg.Properties["email_verified_at"])
}

func TestCreateUserHandler_NonAdmin_403(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	r := newAdminRouter(http.MethodPost, "/admin/users",
		adminUsersRoutes.NewCreateUserHandler(enforcer, db), false)

	payload, _ := json.Marshal(validCreateUserBody())
	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateUserHandler_CasdoorRejects_400(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	original := adminUsersRoutes.CreateCasdoorUser
	t.Cleanup(func() { adminUsersRoutes.CreateCasdoorUser = original })
	adminUsersRoutes.CreateCasdoorUser = func(user *casdoorsdk.User) error {
		return assert.AnError
	}

	r := newAdminRouter(http.MethodPost, "/admin/users",
		adminUsersRoutes.NewCreateUserHandler(enforcer, db), true)

	payload, _ := json.Marshal(validCreateUserBody())
	req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Verify email -----------------------------------------------------------

func TestVerifyEmailHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.MarkUserEmailVerified
	t.Cleanup(func() { adminUsersRoutes.MarkUserEmailVerified = original })
	var calledUserID string
	adminUsersRoutes.MarkUserEmailVerified = func(userID string) error {
		calledUserID = userID
		return nil
	}

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/verify-email",
		adminUsersRoutes.NewVerifyEmailHandler(db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidBob+"/verify-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, uidBob, calledUserID)
}

func TestVerifyEmailHandler_UnknownUser_404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.MarkUserEmailVerified
	t.Cleanup(func() { adminUsersRoutes.MarkUserEmailVerified = original })
	adminUsersRoutes.MarkUserEmailVerified = func(userID string) error {
		return authServices.ErrUserNotFound
	}

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/verify-email",
		adminUsersRoutes.NewVerifyEmailHandler(db), true)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidNobody+"/verify-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestVerifyEmailHandler_NonAdmin_403(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	r := newAdminRouter(http.MethodPost, "/admin/users/:id/verify-email",
		adminUsersRoutes.NewVerifyEmailHandler(db), false)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uidBob+"/verify-email", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- email_verified surfaced in the listing ---------------------------------

func TestBuildUserListings_SurfacesEmailVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	verified := fakeUser(uidAlice, "alice", "Alice", "alice@example.com", "")
	verified.EmailVerified = true
	unverified := fakeUser(uidBob, "bob", "Bob", "bob@example.com", "")
	unverified.EmailVerified = false

	listings, err := adminUsersRoutes.BuildUserListings(
		[]*casdoorsdk.User{verified, unverified}, db, noAdminFn())
	require.NoError(t, err)

	assert.True(t, findListing(t, listings, uidAlice).EmailVerified)
	assert.False(t, findListing(t, listings, uidBob).EmailVerified)
}
