package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// newIncusUIRouter sets up a gin test router with mock auth middleware and
// the IncusUIController handler registered at the expected route.
func newIncusUIRouter(userID string, userRoles []string, controller *terminalController.IncusUIController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware: sets userId and userRoles in context
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})

	router.Any("/api/v1/incus-ui/:backendId/*path", controller.ProxyIncusUI)
	return router
}

// makeIncusUIRequest sends a GET request to the Incus UI proxy route and returns the response.
func makeIncusUIRequest(router *gin.Engine, backendID, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/v1/incus-ui/"+backendID+"/"+path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --------------------------------------------------------------------------
// Test cases
// --------------------------------------------------------------------------

// TestIncusUIProxy_AdminCanAccessAnyBackend verifies that a system administrator
// can access any backendId through the Incus UI proxy.
// In the RED phase the stub returns 501, so this test expects != 501 and != 403.
func TestIncusUIProxy_AdminCanAccessAnyBackend(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Start a mock tt-backend that returns 200 for any request
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// System admin with "administrator" role
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "any-backend-id", "some/path")

	// Admin should be authorized — we expect the proxy to forward (200) or at least
	// not return 403 Forbidden or 501 Not Implemented.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"System administrator should not be denied access to any backend")
	assert.NotEqual(t, http.StatusNotImplemented, w.Code,
		"ProxyIncusUI should be implemented and proxy the request for admins")
}

// TestIncusUIProxy_OrgOwnerCanAccessAllowedBackend verifies that an organization
// owner can access a backend listed in their org's AllowedBackends.
func TestIncusUIProxy_OrgOwnerCanAccessAllowedBackend(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends containing "backend1"
	org := createTestOrgForHistory(t, db, "org-owner1")
	// Set AllowedBackends on the organization
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)

	// Make org-owner1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// Org owner with non-admin system role
	router := newIncusUIRouter("org-owner1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend1", "some/path")

	// Org owner should be authorized for backend1
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Org owner should be allowed to access a backend in their org's AllowedBackends")
	assert.NotEqual(t, http.StatusNotImplemented, w.Code,
		"ProxyIncusUI should be implemented and proxy the request for authorized org owners")
}

// TestIncusUIProxy_OrgManagerCanAccessAllowedBackend verifies that an organization
// manager can access a backend listed in their org's AllowedBackends.
func TestIncusUIProxy_OrgManagerCanAccessAllowedBackend(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends containing "backend1"
	org := createTestOrgForHistory(t, db, "some-creator")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)

	// Make manager1 a manager member of the org
	createTestOrgMember(t, db, org.ID, "manager1", orgModels.OrgRoleManager)

	// Start a mock tt-backend
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// Org manager with non-admin system role
	router := newIncusUIRouter("manager1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend1", "some/path")

	// Org manager should be authorized for backend1
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Org manager should be allowed to access a backend in their org's AllowedBackends")
	assert.NotEqual(t, http.StatusNotImplemented, w.Code,
		"ProxyIncusUI should be implemented and proxy the request for authorized org managers")
}

// TestIncusUIProxy_OrgOwnerDeniedUnallowedBackend verifies that an organization
// owner is denied access to a backend NOT listed in their org's AllowedBackends.
func TestIncusUIProxy_OrgOwnerDeniedUnallowedBackend(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends containing only "backend1"
	org := createTestOrgForHistory(t, db, "org-owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)

	// Make org-owner1 an owner
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend (should NOT be reached)
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// Org owner with non-admin system role tries to access "backend2" (not allowed)
	router := newIncusUIRouter("org-owner1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend2", "some/path")

	// Should be denied — backend2 is not in org's AllowedBackends
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Org owner should be denied access to a backend NOT in their org's AllowedBackends")

	// Verify error message
	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp["error_message"], "forbidden",
		"Response should indicate forbidden access")
}

// TestIncusUIProxy_OrgMemberDenied verifies that a regular organization member
// (not owner or manager) is denied access to the Incus UI proxy.
func TestIncusUIProxy_OrgMemberDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends containing "backend1"
	org := createTestOrgForHistory(t, db, "some-creator")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)

	// Make regular-member a regular member (not owner/manager)
	createTestOrgMember(t, db, org.ID, "regular-member", orgModels.OrgRoleMember)

	// Start a mock tt-backend (should NOT be reached)
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// Regular member with non-admin system role
	router := newIncusUIRouter("regular-member", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend1", "some/path")

	// Regular member should be denied access
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Regular org member (not owner/manager) should be denied access to the Incus UI proxy")
}

// TestIncusUIProxy_NonOrgUserDenied verifies that a user with no organization
// membership at all is denied access to the Incus UI proxy.
func TestIncusUIProxy_NonOrgUserDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// No org membership for "lone-user"

	// Start a mock tt-backend (should NOT be reached)
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL)

	// User with no org membership and non-admin role
	router := newIncusUIRouter("lone-user", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "some-backend", "some/path")

	// Non-org user should be denied access
	assert.Equal(t, http.StatusForbidden, w.Code,
		"User with no organization membership should be denied access to the Incus UI proxy")
}

// TestIncusUIProxy_IsUserAuthorizedForBackend_AdminAlwaysTrue verifies the
// authorization helper returns true for system admins regardless of backend.
func TestIncusUIProxy_IsUserAuthorizedForBackend_AdminAlwaysTrue(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	controller := terminalController.NewIncusUIController(db, "http://unused")

	authorized := controller.IsUserAuthorizedForBackend(
		"admin-user",
		[]string{"administrator"},
		"any-backend-id",
	)

	assert.True(t, authorized,
		"IsUserAuthorizedForBackend should return true for system administrators")
}

// TestIncusUIProxy_IsUserAuthorizedForBackend_OrgOwnerAllowed verifies the
// authorization helper returns true for an org owner when the backend is in AllowedBackends.
func TestIncusUIProxy_IsUserAuthorizedForBackend_OrgOwnerAllowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends
	org := createTestOrgForHistory(t, db, "owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1","backend2"]`).Error
	require.NoError(t, err)
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	controller := terminalController.NewIncusUIController(db, "http://unused")

	authorized := controller.IsUserAuthorizedForBackend(
		"owner1",
		[]string{"member"},
		"backend1",
	)

	assert.True(t, authorized,
		"IsUserAuthorizedForBackend should return true for org owner with backend in AllowedBackends")
}

// TestIncusUIProxy_IsUserAuthorizedForBackend_NonMemberDenied verifies the
// authorization helper returns false for a user with no org membership.
func TestIncusUIProxy_IsUserAuthorizedForBackend_NonMemberDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	controller := terminalController.NewIncusUIController(db, "http://unused")

	authorized := controller.IsUserAuthorizedForBackend(
		"nobody",
		[]string{"member"},
		"some-backend",
	)

	assert.False(t, authorized,
		"IsUserAuthorizedForBackend should return false for users with no org membership")
}
