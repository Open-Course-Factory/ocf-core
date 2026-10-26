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
	"soli/formations/src/auth/casdoor"
	apiErrors "soli/formations/src/auth/errors"
	"soli/formations/src/auth/interfaces"
	"soli/formations/src/auth/mocks"
	authServices "soli/formations/src/auth/services"
	orgModels "soli/formations/src/organizations/models"
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
	originalProvision := adminUsersRoutes.ProvisionNewUser
	originalNativeRole := adminUsersRoutes.AddCasdoorNativeRole
	t.Cleanup(func() {
		adminUsersRoutes.CreateCasdoorUser = originalCreate
		adminUsersRoutes.GetCasdoorUserByEmail = originalGetByEmail
		adminUsersRoutes.ProvisionNewUser = originalProvision
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
	var provisioned *casdoorsdk.User
	var provisionedRole string
	adminUsersRoutes.ProvisionNewUser = func(_ interfaces.EnforcerInterface, _ *gorm.DB, user *casdoorsdk.User, defaultRole string) {
		provisioned = user
		provisionedRole = defaultRole
	}

	body := validCreateUserBody()
	body.DefaultRole = "trainer"
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

	require.NotNil(t, provisioned, "the new account must go through the shared post-creation setup")
	assert.Equal(t, "newteacher-id", provisioned.Id)
	assert.Equal(t, "trainer", provisionedRole)
}

func TestCreateUserHandler_EmailVerifiedFlagMarksAccountVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalCreate := adminUsersRoutes.CreateCasdoorUser
	originalGetByEmail := adminUsersRoutes.GetCasdoorUserByEmail
	originalProvision := adminUsersRoutes.ProvisionNewUser
	originalNativeRole := adminUsersRoutes.AddCasdoorNativeRole
	t.Cleanup(func() {
		adminUsersRoutes.CreateCasdoorUser = originalCreate
		adminUsersRoutes.GetCasdoorUserByEmail = originalGetByEmail
		adminUsersRoutes.ProvisionNewUser = originalProvision
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
	adminUsersRoutes.ProvisionNewUser = func(interfaces.EnforcerInterface, *gorm.DB, *casdoorsdk.User, string) {}

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
	var got apiErrors.APIError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "create_failed", got.ErrorMessage)
	assert.NotContains(t, w.Body.String(), assert.AnError.Error(), "the identity provider's raw error must not reach the client")
}

// The admin path must give the account what self-signup gives it — the
// personal organization was once silently missing here.
func TestCreateUserHandler_ProvisionsPersonalOrganization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)
	enforcer := mocks.NewMockEnforcer()

	originalCreate := adminUsersRoutes.CreateCasdoorUser
	originalGetByEmail := adminUsersRoutes.GetCasdoorUserByEmail
	originalNativeRole := adminUsersRoutes.AddCasdoorNativeRole
	originalEnforcer := casdoor.Enforcer
	t.Cleanup(func() {
		adminUsersRoutes.CreateCasdoorUser = originalCreate
		adminUsersRoutes.GetCasdoorUserByEmail = originalGetByEmail
		adminUsersRoutes.AddCasdoorNativeRole = originalNativeRole
		casdoor.Enforcer = originalEnforcer
	})
	casdoor.Enforcer = enforcer
	adminUsersRoutes.AddCasdoorNativeRole = func(_ casdoorsdk.User) error { return nil }
	adminUsersRoutes.CreateCasdoorUser = func(*casdoorsdk.User) error { return nil }
	adminUsersRoutes.GetCasdoorUserByEmail = func(email string) (*casdoorsdk.User, error) {
		u := fakeUser("newteacher-id", "newteacher", "New Teacher", email, "")
		// Verified, so the real setup skips the verification email (SMTP).
		u.EmailVerified = true
		return u, nil
	}

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

	var org orgModels.Organization
	require.NoError(t, db.Where("owner_user_id = ? AND organization_type = ?", "newteacher-id", orgModels.OrgTypePersonal).
		First(&org).Error, "an admin-created account must get its personal organization")
	var owners int64
	require.NoError(t, db.Model(&orgModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND role = ?", org.ID, "newteacher-id", orgModels.OrgRoleOwner).
		Count(&owners).Error)
	assert.EqualValues(t, 1, owners)

	require.GreaterOrEqual(t, enforcer.GetAddGroupingPolicyCallCount(), 1)
	assert.Equal(t, []any{"newteacher-id", "member"}, enforcer.AddGroupingPolicyCalls[0][:2])
}

func TestCreateUserHandler_RejectsInvalidInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.CreateCasdoorUser
	t.Cleanup(func() { adminUsersRoutes.CreateCasdoorUser = original })
	called := false
	adminUsersRoutes.CreateCasdoorUser = func(*casdoorsdk.User) error {
		called = true
		return nil
	}

	cases := map[string]struct {
		mutate func(*adminUsersRoutes.AdminCreateUserInput)
		code   string
	}{
		"short password": {func(in *adminUsersRoutes.AdminCreateUserInput) { in.Password = "short" }, "password_too_short"},
		"invalid email":  {func(in *adminUsersRoutes.AdminCreateUserInput) { in.Email = "not-an-email" }, "invalid_email"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := validCreateUserBody()
			tc.mutate(&body)
			payload, _ := json.Marshal(body)

			r := newAdminRouter(http.MethodPost, "/admin/users",
				adminUsersRoutes.NewCreateUserHandler(mocks.NewMockEnforcer(), db), true)
			req := httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var got apiErrors.APIError
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, tc.code, got.ErrorMessage)
		})
	}
	assert.False(t, called, "must refuse before creating anything in the identity provider")
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

// --- Set password directly (no email) ---------------------------------------

func TestSetPasswordHandler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetUserPassword
	t.Cleanup(func() { adminUsersRoutes.SetUserPassword = original })
	var calledUserID, calledPassword string
	adminUsersRoutes.SetUserPassword = func(userID, newPassword string) error {
		calledUserID = userID
		calledPassword = newPassword
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/password",
		adminUsersRoutes.NewSetPasswordHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetPasswordInput{Password: "correct-horse-battery-staple"})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, uidBob, calledUserID)
	assert.Equal(t, "correct-horse-battery-staple", calledPassword)
}

func TestSetPasswordHandler_TooShort_400(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetUserPassword
	t.Cleanup(func() { adminUsersRoutes.SetUserPassword = original })
	called := false
	adminUsersRoutes.SetUserPassword = func(userID, newPassword string) error {
		called = true
		return nil
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/password",
		adminUsersRoutes.NewSetPasswordHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetPasswordInput{Password: "short"})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, called, "must refuse before calling SetUserPassword")
}

func TestSetPasswordHandler_UnknownUser_404(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	original := adminUsersRoutes.SetUserPassword
	t.Cleanup(func() { adminUsersRoutes.SetUserPassword = original })
	adminUsersRoutes.SetUserPassword = func(userID, newPassword string) error {
		return authServices.ErrUserNotFound
	}

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/password",
		adminUsersRoutes.NewSetPasswordHandler(db), true)

	body, _ := json.Marshal(adminUsersRoutes.SetPasswordInput{Password: "correct-horse-battery-staple"})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidNobody+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSetPasswordHandler_NonAdmin_403(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	db := freshTestDB(t)

	r := newAdminRouter(http.MethodPut, "/admin/users/:id/password",
		adminUsersRoutes.NewSetPasswordHandler(db), false)

	body, _ := json.Marshal(adminUsersRoutes.SetPasswordInput{Password: "correct-horse-battery-staple"})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+uidBob+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
