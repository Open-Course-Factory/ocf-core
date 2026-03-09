package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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
	// Set AllowedBackends and IncusUIEnabled on the organization
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)

	// Make org-owner1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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

	// Create an org with AllowedBackends containing "backend1" and IncusUIEnabled
	org := createTestOrgForHistory(t, db, "some-creator")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)

	// Make manager1 a manager member of the org
	createTestOrgMember(t, db, org.ID, "manager1", orgModels.OrgRoleManager)

	// Start a mock tt-backend
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

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

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

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

	// Create an org with AllowedBackends and IncusUIEnabled
	org := createTestOrgForHistory(t, db, "owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1","backend2"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

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

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

	authorized := controller.IsUserAuthorizedForBackend(
		"nobody",
		[]string{"member"},
		"some-backend",
	)

	assert.False(t, authorized,
		"IsUserAuthorizedForBackend should return false for users with no org membership")
}

// --------------------------------------------------------------------------
// HTML rewriting / monkey-patching tests
// --------------------------------------------------------------------------

// TestIncusUIProxy_HTMLRewriting_InjectsMonkeyPatch verifies that HTML responses
// have the monkey-patching script injected in <head>.
func TestIncusUIProxy_HTMLRewriting_InjectsMonkeyPatch(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	mockHTML := `<!doctype html><html><head><title>Incus UI</title></head><body><div id="app"></div></body></html>`

	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "test-backend", "ui/")

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should contain the monkey-patching script with the correct proxy prefix
	assert.Contains(t, body, `<script>(function()`)
	assert.Contains(t, body, `/api/v1/incus-ui/test-backend`)

	// Script should be inside <head>, before the original content
	headIdx := strings.Index(body, "<head>")
	scriptIdx := strings.Index(body, "<script>(function()")
	titleIdx := strings.Index(body, "<title>")
	assert.Greater(t, scriptIdx, headIdx, "script should be after <head>")
	assert.Less(t, scriptIdx, titleIdx, "script should be before <title>")
}

// TestIncusUIProxy_HTMLRewriting_RewritesAssetPaths verifies that absolute
// asset paths in HTML are rewritten to go through the proxy prefix.
func TestIncusUIProxy_HTMLRewriting_RewritesAssetPaths(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	mockHTML := `<!doctype html><html><head>` +
		`<script src="/ui/assets/index-abc123.js"></script>` +
		`<link rel="stylesheet" href="/ui/assets/index-xyz789.css">` +
		`<link rel="manifest" href="/manifest.json">` +
		`</head><body></body></html>`

	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "my-backend", "ui/")

	body := w.Body.String()
	prefix := "/api/v1/incus-ui/my-backend"

	assert.Contains(t, body, `"`+prefix+`/ui/assets/index-abc123.js"`,
		"JS asset path should be rewritten with proxy prefix")
	assert.Contains(t, body, `"`+prefix+`/ui/assets/index-xyz789.css"`,
		"CSS asset path should be rewritten with proxy prefix")
	assert.Contains(t, body, `"`+prefix+`/manifest.json"`,
		"manifest.json path should be rewritten with proxy prefix")

	// Should NOT contain the original absolute paths
	assert.NotContains(t, body, `"/ui/assets/index-abc123.js"`)
	assert.NotContains(t, body, `"/manifest.json"`)
}

// TestIncusUIProxy_CSSPassthrough verifies that CSS responses
// are passed through without modification.
func TestIncusUIProxy_CSSPassthrough(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	cssContent := `.app { background: #fff; }`

	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cssContent))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "test-backend", "ui/assets/index.css")

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.Equal(t, cssContent, body,
		"CSS responses should not be modified")
	assert.NotContains(t, body, "<script>",
		"Monkey-patch script should not be injected in non-HTML responses")
}

// TestIncusUIProxy_JSRewriting verifies that JavaScript responses have
// their /ui/assets/ paths rewritten through the proxy prefix (for Vite's
// __vite__mapDeps dynamic import preload paths).
func TestIncusUIProxy_JSRewriting(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	jsContent := `const deps=["/ui/assets/Foo-abc123.js","/ui/assets/Bar-def456.css"];fetch("/1.0/instances")`

	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(jsContent))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "test-backend", "ui/assets/index.js")

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// /ui/assets/ paths should be rewritten through the proxy
	assert.Contains(t, body, `"/api/v1/incus-ui/test-backend/ui/assets/Foo-abc123.js"`,
		"JS /ui/assets/ paths should be rewritten")
	assert.Contains(t, body, `"/api/v1/incus-ui/test-backend/ui/assets/Bar-def456.css"`,
		"JS /ui/assets/ CSS dep paths should be rewritten")
	// Non-asset paths should NOT be rewritten (monkey-patch handles those at runtime)
	assert.Contains(t, body, `fetch("/1.0/instances")`,
		"Non-asset paths in JS should remain unchanged")
	// No monkey-patch script injection in JS
	assert.NotContains(t, body, "<script>",
		"Monkey-patch script should not be injected in JS responses")
}

// TestIncusUIProxy_AcceptEncodingIdentity verifies that the proxy sets
// Accept-Encoding: identity to prevent compressed upstream responses.
func TestIncusUIProxy_AcceptEncodingIdentity(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	var capturedAcceptEncoding string
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAcceptEncoding = r.Header.Get("Accept-Encoding")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	makeIncusUIRequest(router, "test-backend", "ui/")

	assert.Equal(t, "identity", capturedAcceptEncoding,
		"Proxy should set Accept-Encoding: identity to prevent compressed responses")
}

// TestIncusUIProxy_MonkeyPatchScript_PatchesFetchXHRWebSocket verifies that the
// generated monkey-patching script contains patches for all three request types.
func TestIncusUIProxy_MonkeyPatchScript_PatchesFetchXHRWebSocket(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	mockHTML := `<!doctype html><html><head></head><body></body></html>`
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "default", "ui/")

	body := w.Body.String()

	// Verify all three request types are patched
	assert.Contains(t, body, "window.fetch=function",
		"Script should patch window.fetch")
	assert.Contains(t, body, "XMLHttpRequest.prototype.open=function",
		"Script should patch XMLHttpRequest.prototype.open")
	assert.Contains(t, body, "window.WebSocket=function",
		"Script should patch WebSocket constructor")

	// Verify the proxy prefix is embedded correctly
	assert.Contains(t, body, `"/api/v1/incus-ui/default"`,
		"Script should contain the correct proxy prefix")
}

// --------------------------------------------------------------------------
// incus_ui_enabled and protectedBackends tests
// --------------------------------------------------------------------------

// TestIncusUIProxy_IncusUIDisabled_OrgOwnerDenied verifies that an org owner
// is denied access when the org has AllowedBackends but IncusUIEnabled is false.
func TestIncusUIProxy_IncusUIDisabled_OrgOwnerDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends but IncusUIEnabled=false (default)
	org := createTestOrgForHistory(t, db, "org-owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)

	// Make org-owner1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend (should NOT be reached)
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

	// Org owner with non-admin system role
	router := newIncusUIRouter("org-owner1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend1", "some/path")

	// Should be denied because incus_ui_enabled is false
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Org owner should be denied when incus_ui_enabled is false")
}

// TestIncusUIProxy_IncusUIEnabled_OrgOwnerGranted verifies that an org owner
// is granted access when the org has AllowedBackends AND IncusUIEnabled is true.
func TestIncusUIProxy_IncusUIEnabled_OrgOwnerGranted(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends AND IncusUIEnabled=true
	org := createTestOrgForHistory(t, db, "org-owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)

	// Make org-owner1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend returning 200
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, nil)

	// Org owner with non-admin system role
	router := newIncusUIRouter("org-owner1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "backend1", "some/path")

	// Should be granted access (proxied, not 403)
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Org owner should be granted access when incus_ui_enabled is true")
}

// TestIncusUIProxy_ProtectedBackend_NonAdminDenied verifies that a non-admin
// user is denied access to a protected backend even if their org's AllowedBackends
// includes it and IncusUIEnabled is true.
func TestIncusUIProxy_ProtectedBackend_NonAdminDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an org with AllowedBackends containing the protected backend and IncusUIEnabled=true
	org := createTestOrgForHistory(t, db, "org-owner1")
	err := db.Model(org).Update("allowed_backends", `["protected-backend"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)

	// Make org-owner1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "org-owner1", orgModels.OrgRoleOwner)

	// Start a mock tt-backend (should NOT be reached)
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, map[string]bool{"protected-backend": true})

	// Org owner with non-admin system role
	router := newIncusUIRouter("org-owner1", []string{"member"}, controller)
	w := makeIncusUIRequest(router, "protected-backend", "some/path")

	// Should be denied — protected backend blocks non-admins
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Non-admin should be denied access to protected backends even if in org's AllowedBackends")
}

// TestIncusUIProxy_ProtectedBackend_AdminGranted verifies that a system admin
// can access a protected backend.
func TestIncusUIProxy_ProtectedBackend_AdminGranted(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Start a mock tt-backend returning 200
	mockTTBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockTTBackend.Close()

	controller := terminalController.NewIncusUIController(db, mockTTBackend.URL, map[string]bool{"protected-backend": true})

	// System admin with "administrator" role
	router := newIncusUIRouter("admin-user", []string{"administrator"}, controller)
	w := makeIncusUIRequest(router, "protected-backend", "some/path")

	// Admin should access protected backends
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should access protected backends")
}

// TestIncusUIProxy_IsUserAuthorizedForBackend_IncusUIDisabled verifies that
// IsUserAuthorizedForBackend returns false when IncusUIEnabled is false,
// even if the backend is in AllowedBackends.
func TestIncusUIProxy_IsUserAuthorizedForBackend_IncusUIDisabled(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org with AllowedBackends but IncusUIEnabled=false (default)
	org := createTestOrgForHistory(t, db, "owner1")
	err := db.Model(org).Update("allowed_backends", `["backend1"]`).Error
	require.NoError(t, err)
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

	authorized := controller.IsUserAuthorizedForBackend(
		"owner1",
		[]string{"member"},
		"backend1",
	)

	assert.False(t, authorized,
		"IsUserAuthorizedForBackend should return false when incus_ui_enabled is false")
}

// TestIncusUIProxy_IsUserAuthorizedForBackend_ProtectedBackend verifies that
// IsUserAuthorizedForBackend returns false for a non-admin user requesting a
// protected backend, even if their org has it in AllowedBackends with IncusUIEnabled.
func TestIncusUIProxy_IsUserAuthorizedForBackend_ProtectedBackend(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org with AllowedBackends containing the protected backend and IncusUIEnabled=true
	org := createTestOrgForHistory(t, db, "owner1")
	err := db.Model(org).Update("allowed_backends", `["sys-default"]`).Error
	require.NoError(t, err)
	db.Model(org).Update("incus_ui_enabled", true)
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	controller := terminalController.NewIncusUIController(db, "http://unused", map[string]bool{"sys-default": true})

	authorized := controller.IsUserAuthorizedForBackend(
		"owner1",
		[]string{"member"},
		"sys-default",
	)

	assert.False(t, authorized,
		"IsUserAuthorizedForBackend should deny non-admin access to protected backends")
}
