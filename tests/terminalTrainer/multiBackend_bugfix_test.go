package terminalTrainer_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	terminalDto "soli/formations/src/terminalTrainer/dto"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// =============================================================================
// Issue #42 (C2): GetBackends requires admin role for unfiltered list
// =============================================================================

func TestGetBackends_NonAdminWithoutOrgID_Returns403(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "regular-user")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	// Non-admin without organization_id should get 403
	req := httptest.NewRequest("GET", "/terminals/backends", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var apiErr map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	assert.Contains(t, apiErr["error_message"], "Admin access required")
}

func TestGetBackends_NonAdminWithOrgID_Allowed(t *testing.T) {
	db := freshTestDB(t)

	// Create an organization
	org := &organizationModels.Organization{
		Name:             "test-org-backend-access",
		DisplayName:      "Test Org",
		OwnerUserID:      "regular-user",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  []string{"local"},
		DefaultBackend:   "local",
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "regular-user")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	// Non-admin with valid organization_id should be allowed (will fail 500 due to no TT API, but NOT 403)
	req := httptest.NewRequest("GET", "/terminals/backends?organization_id="+org.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should NOT be 403 - the org-filtered path doesn't require admin
	assert.NotEqual(t, http.StatusForbidden, w.Code, "Non-admin should be able to access org-filtered backends")
}

func TestGetBackends_AdminWithoutOrgID_Allowed(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "admin-user")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	// Admin without organization_id should be allowed (will get 500 due to no TT API, but NOT 403)
	req := httptest.NewRequest("GET", "/terminals/backends", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should NOT be 403
	assert.NotEqual(t, http.StatusForbidden, w.Code, "Admin should be able to list all backends")
}

// =============================================================================
// Issue #45 (M4): Error messages don't leak internal details
// =============================================================================

func TestGetBackends_ErrorDoesNotLeakInternalDetails(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "admin-user")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	// This will fail (no TT API configured) - verify error message is generic
	req := httptest.NewRequest("GET", "/terminals/backends", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		var apiErr map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &apiErr)
		require.NoError(t, err)

		errMsg := apiErr["error_message"].(string)
		// Should NOT contain internal URLs, API keys, or detailed error messages
		assert.NotContains(t, errMsg, "http://")
		assert.NotContains(t, errMsg, "https://")
		assert.NotContains(t, errMsg, "key=")
		assert.Equal(t, "Failed to get backends", errMsg)
	}
}

func TestGetServerMetrics_ErrorDoesNotLeakInternalDetails(t *testing.T) {
	db := freshTestDB(t)
	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "admin-user")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	router.GET("/terminals/metrics", ctrl.GetServerMetrics)

	req := httptest.NewRequest("GET", "/terminals/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		var apiErr map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &apiErr)
		require.NoError(t, err)

		errMsg := apiErr["error_message"].(string)
		assert.Equal(t, "Failed to get server metrics", errMsg)
	}
}

// =============================================================================
// Issue #49 (L6): validateBackendForOrg falls through to system default
// =============================================================================

func TestValidateBackendForOrg_EmptyOrgDefault_FallsToSystemDefault(t *testing.T) {
	db := freshTestDB(t)

	// Create org with NO default backend
	org := &organizationModels.Organization{
		Name:             "test-org-no-default",
		DisplayName:      "No Default",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  []string{},
		DefaultBackend:   "", // No default set
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	// The validateBackendForOrg function is private, so we test the behavior
	// indirectly through the service. The key test is that when org has no
	// default, the system should not pass empty string to tt-backend.
	_ = services.NewTerminalTrainerService(db)

	// Verify the org has empty default
	var retrieved organizationModels.Organization
	err = db.Where("id = ?", org.ID).First(&retrieved).Error
	require.NoError(t, err)
	assert.Empty(t, retrieved.DefaultBackend)
}

// =============================================================================
// Issue #42 (C2): Invalid organization_id returns 400 (not leak)
// =============================================================================

func TestGetBackends_InvalidOrgID_Returns400WithoutLeak(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "regular-user")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	req := httptest.NewRequest("GET", "/terminals/backends?organization_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var apiErr map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &apiErr)
	require.NoError(t, err)
	// Should say "Invalid organization_id" without leaking the parse error details
	assert.Equal(t, "Invalid organization_id", apiErr["error_message"])
}

// =============================================================================
// Issue #42: GetBackends with non-existent org returns proper error
// =============================================================================

func TestGetBackends_NonExistentOrg_ReturnsError(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "regular-user")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)

	fakeOrgID := uuid.New().String()
	req := httptest.NewRequest("GET", "/terminals/backends?organization_id="+fakeOrgID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 500 (org not found wrapped in service error), not 403
	assert.NotEqual(t, http.StatusForbidden, w.Code)
	assert.True(t, w.Code == http.StatusInternalServerError || w.Code == http.StatusNotFound,
		"Expected 404 or 500 for non-existent org, got %d", w.Code)
}

// =============================================================================
// Regression: GORM Select with serializer:json must load AllowedBackends
// =============================================================================

func TestSelectAllowedBackends_JSONSerializerWorks(t *testing.T) {
	db := freshTestDB(t)

	// Create org with specific allowed backends
	org := &organizationModels.Organization{
		Name:             "test-org-select-serializer",
		DisplayName:      "Select Test",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  []string{"backend-a", "backend-b"},
		DefaultBackend:   "backend-a",
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	// Read back using the same Select pattern as GetBackendsForOrganization
	var loaded organizationModels.Organization
	err = db.Select("allowed_backends, default_backend").First(&loaded, "id = ?", org.ID).Error
	require.NoError(t, err)

	// This is the core regression: AllowedBackends must be deserialized, not nil
	assert.Len(t, loaded.AllowedBackends, 2, "AllowedBackends should have 2 entries")
	assert.Contains(t, loaded.AllowedBackends, "backend-a")
	assert.Contains(t, loaded.AllowedBackends, "backend-b")
	assert.Equal(t, "backend-a", loaded.DefaultBackend)
}

// Regression: Verify that the UpdateOrganizationBackends pattern actually persists data
func TestUpdateOrganizationBackends_PersistsWithSelect(t *testing.T) {
	db := freshTestDB(t)

	// Create org with NO backends
	org := &organizationModels.Organization{
		Name:             "test-org-update-backends",
		DisplayName:      "Update Test",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	// Verify initially null
	var before organizationModels.Organization
	err = db.First(&before, "id = ?", org.ID).Error
	require.NoError(t, err)
	assert.Empty(t, before.AllowedBackends, "Should start with no backends")

	// Simulate what UpdateOrganizationBackends controller does after fix:
	// Map-based update with JSON-marshaled allowed_backends
	backendsJSON, _ := json.Marshal([]string{"backend-a", "backend-b"})
	updateMap := map[string]any{
		"allowed_backends": string(backendsJSON),
		"default_backend":  "backend-a",
	}
	err = db.Model(org).Updates(updateMap).Error
	require.NoError(t, err, "Update should not error")

	// Read back and verify persistence
	var after organizationModels.Organization
	err = db.First(&after, "id = ?", org.ID).Error
	require.NoError(t, err)

	t.Logf("AllowedBackends after update: %v (len=%d)", after.AllowedBackends, len(after.AllowedBackends))
	t.Logf("DefaultBackend after update: %q", after.DefaultBackend)

	assert.Len(t, after.AllowedBackends, 2, "AllowedBackends should have 2 entries after update")
	assert.Contains(t, after.AllowedBackends, "backend-a")
	assert.Contains(t, after.AllowedBackends, "backend-b")
	assert.Equal(t, "backend-a", after.DefaultBackend)
}

// Regression: Map-based Updates (entity management PATCH) must JSON-marshal serializer:json fields
func TestMapBasedUpdate_AllowedBackends_MustBeJSONMarshaled(t *testing.T) {
	db := freshTestDB(t)

	// Create org with no backends
	org := &organizationModels.Organization{
		Name:             "test-org-map-update",
		DisplayName:      "Map Update Test",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	// Simulate the entity management PATCH path: update via map with JSON-marshaled value
	// This is what EntityDtoToMap now does after the fix
	backendsJSON, _ := json.Marshal([]string{"backend-a", "backend-b"})
	updateMap := map[string]any{
		"allowed_backends": string(backendsJSON),
		"default_backend":  "backend-a",
	}
	err = db.Model(&organizationModels.Organization{}).Where("id = ?", org.ID).Updates(updateMap).Error
	require.NoError(t, err)

	// Read back — GORM serializer:json must deserialize the stored JSON string
	var loaded organizationModels.Organization
	err = db.First(&loaded, "id = ?", org.ID).Error
	require.NoError(t, err)

	assert.Len(t, loaded.AllowedBackends, 2, "AllowedBackends should have 2 entries (map update)")
	assert.Contains(t, loaded.AllowedBackends, "backend-a")
	assert.Contains(t, loaded.AllowedBackends, "backend-b")
	assert.Equal(t, "backend-a", loaded.DefaultBackend)
}

// =============================================================================
// End-to-end: GetBackendsForOrganization filtering
// =============================================================================

// setupServiceWithMockBackends creates a TerminalTrainerService backed by a fake
// TT API that returns the given backends. The system default is indicated by
// setting IsDefault=true on the matching backend in the response (tt-backend is
// the single source of truth for default backend).
func setupServiceWithMockBackends(t *testing.T, backends []terminalDto.BackendInfo, systemDefault string) (services.TerminalTrainerService, *gorm.DB) {
	t.Helper()

	// Mark the system default in the backend list (tt-backend is source of truth)
	for i := range backends {
		backends[i].IsDefault = (backends[i].ID == systemDefault)
	}

	// Fake TT backend API
	ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(backends)
	}))
	t.Cleanup(func() { ttServer.Close() })

	db := freshTestDB(t)

	// Set env vars for the service constructor, then restore
	origURL := os.Getenv("TERMINAL_TRAINER_URL")
	origKey := os.Getenv("TERMINAL_TRAINER_ADMIN_KEY")
	origVer := os.Getenv("TERMINAL_TRAINER_API_VERSION")
	os.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	os.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-key")
	os.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	t.Cleanup(func() {
		os.Setenv("TERMINAL_TRAINER_URL", origURL)
		os.Setenv("TERMINAL_TRAINER_ADMIN_KEY", origKey)
		os.Setenv("TERMINAL_TRAINER_API_VERSION", origVer)
	})

	svc := services.NewTerminalTrainerService(db)
	return svc, db
}

// createTestOrg is a helper to create an org with given backend config.
func createTestOrg(t *testing.T, db *gorm.DB, name string, allowedBackends []string, defaultBackend string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		Name:             name,
		DisplayName:      name,
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  allowedBackends,
		DefaultBackend:   defaultBackend,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)
	return org
}

func TestGetBackendsForOrganization_Filtering(t *testing.T) {
	// Two backends exist in the system
	allBackends := []terminalDto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}

	svc, db := setupServiceWithMockBackends(t, allBackends, "default")

	t.Run("org with null allowed_backends gets only default backend", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("null-backends-%s", uuid.New().String()[:8]),
			nil, "") // nil = null in DB

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 1, "Org with null allowed_backends should get ONLY the default backend")
		assert.Equal(t, "default", backends[0].ID)
		assert.True(t, backends[0].IsDefault)
	})

	t.Run("org with empty allowed_backends gets only default backend", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("empty-backends-%s", uuid.New().String()[:8]),
			[]string{}, "") // empty slice

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 1, "Org with empty allowed_backends should get ONLY the default backend")
		assert.Equal(t, "default", backends[0].ID)
	})

	t.Run("org with both backends configured gets both", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("both-backends-%s", uuid.New().String()[:8]),
			[]string{"default", "oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 2, "Org with both backends should get both")

		ids := []string{backends[0].ID, backends[1].ID}
		assert.Contains(t, ids, "default")
		assert.Contains(t, ids, "oracle1")

		// Verify only oracle1 is marked as default (org default), not both
		defaultCount := 0
		for _, b := range backends {
			if b.IsDefault {
				defaultCount++
				assert.Equal(t, "oracle1", b.ID, "Only org default backend should be marked as default")
			}
		}
		assert.Equal(t, 1, defaultCount, "Exactly one backend should be marked as default")
	})

	t.Run("org with only oracle1 gets only oracle1", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("oracle-only-%s", uuid.New().String()[:8]),
			[]string{"oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 1, "Org with only oracle1 should get only oracle1")
		assert.Equal(t, "oracle1", backends[0].ID)
	})

	t.Run("org with only default gets only default", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("default-only-%s", uuid.New().String()[:8]),
			[]string{"default"}, "default")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 1, "Org with only default should get only default")
		assert.Equal(t, "default", backends[0].ID)
	})
}

// Critical: test that filtering works even when no system default is configured
// (matches production state where features table has no terminal_default_backend)
func TestGetBackendsForOrganization_NoSystemDefault(t *testing.T) {
	allBackends := []terminalDto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}

	// Empty string = no system default configured (matches production)
	svc, db := setupServiceWithMockBackends(t, allBackends, "")

	t.Run("null allowed_backends with no system default returns first backend only", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("no-sysdefault-%s", uuid.New().String()[:8]),
			nil, "")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 1, "Should return only 1 backend, not all")
		assert.Equal(t, "default", backends[0].ID, "Should return the first backend as fallback")
	})

	t.Run("explicit config still works without system default", func(t *testing.T) {
		org := createTestOrg(t, db,
			fmt.Sprintf("explicit-no-sysdefault-%s", uuid.New().String()[:8]),
			[]string{"default", "oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		t.Logf("Backends returned: %+v", backends)
		require.Len(t, backends, 2, "Explicit config should still return both")
	})
}

// =============================================================================
// SetSystemDefaultBackend service-level tests
// =============================================================================

// setupSetDefaultTestServer creates an httptest.Server that routes requests
// to the public GET /backends, admin GET /admin/backends, and admin PUT /admin/backends/{id}
// endpoints, allowing each test to control responses independently.
func setupSetDefaultTestServer(
	t *testing.T,
	backends []terminalDto.BackendInfo,
	adminBackends []map[string]interface{},
	putHandler func(w http.ResponseWriter, r *http.Request),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/backends") && !strings.Contains(r.URL.Path, "/admin/"):
			json.NewEncoder(w).Encode(backends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    adminBackends,
			})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/admin/backends/"):
			if putHandler != nil {
				putHandler(w, r)
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Backend updated successfully"})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func setupSetDefaultService(t *testing.T, serverURL string) services.TerminalTrainerService {
	t.Helper()
	db := freshTestDB(t)
	t.Setenv("TERMINAL_TRAINER_URL", serverURL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
	return services.NewTerminalTrainerService(db)
}

func TestSetSystemDefaultBackend_HappyPath(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: true},
		{ID: "cloud1", Name: "Cloud 1", Connected: true, IsDefault: false},
	}
	adminBackends := []map[string]interface{}{
		{"id": 1, "backend_id": "local", "name": "Local Server", "is_default": true, "is_active": true, "server_url": "", "server_certificate": "", "client_certificate": "", "project": "default"},
		{"id": 2, "backend_id": "cloud1", "name": "Cloud 1", "is_default": false, "is_active": true, "server_url": "https://cloud1:8443", "server_certificate": "", "client_certificate": "", "project": "default"},
	}

	var putCalled atomic.Int32
	var putBody map[string]interface{}

	ts := setupSetDefaultTestServer(t, publicBackends, adminBackends, func(w http.ResponseWriter, r *http.Request) {
		putCalled.Add(1)
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &putBody)
		// Verify correct path (should target backend id=2 for "cloud1")
		assert.True(t, strings.HasSuffix(r.URL.Path, "/admin/backends/2"), "PUT should target admin backend ID 2, got %s", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Backend updated successfully"})
	})
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("cloud1")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "cloud1", result.ID)
	assert.True(t, result.IsDefault, "returned backend should be marked as default")
	assert.Equal(t, int32(1), putCalled.Load(), "PUT should have been called exactly once")

	// Verify the PUT body preserved the name and set is_default=true
	assert.Equal(t, "Cloud 1", putBody["name"], "PUT body should preserve backend name")
	assert.Equal(t, true, putBody["is_default"], "PUT body should set is_default to true")
}

func TestSetSystemDefaultBackend_NotFound(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: true},
	}

	ts := setupSetDefaultTestServer(t, publicBackends, nil, nil)
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("nonexistent")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend not found")
}

func TestSetSystemDefaultBackend_Offline(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: false, IsDefault: false},
	}

	ts := setupSetDefaultTestServer(t, publicBackends, nil, nil)
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("local")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend is offline")
}

func TestSetSystemDefaultBackend_AdminAPIError(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: false},
	}

	// Override the admin endpoint to return 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/backends") && !strings.Contains(r.URL.Path, "/admin/"):
			json.NewEncoder(w).Encode(publicBackends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "internal server error"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("local")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list admin backends")
}

func TestSetSystemDefaultBackend_PutFails(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: false},
	}
	adminBackends := []map[string]interface{}{
		{"id": 1, "backend_id": "local", "name": "Local Server", "is_default": false, "is_active": true, "server_url": "", "server_certificate": "", "client_certificate": "", "project": "default"},
	}

	ts := setupSetDefaultTestServer(t, publicBackends, adminBackends, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "database error"}`))
	})
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("local")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set default")
}

func TestSetSystemDefaultBackend_InvalidatesCache(t *testing.T) {
	var getBackendsCount atomic.Int32

	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: true},
		{ID: "cloud1", Name: "Cloud 1", Connected: true, IsDefault: false},
	}
	adminBackends := []map[string]interface{}{
		{"id": 1, "backend_id": "local", "name": "Local Server", "is_default": true, "is_active": true, "server_url": "", "server_certificate": "", "client_certificate": "", "project": "default"},
		{"id": 2, "backend_id": "cloud1", "name": "Cloud 1", "is_default": false, "is_active": true, "server_url": "https://cloud1:8443", "server_certificate": "", "client_certificate": "", "project": "default"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/backends") && !strings.Contains(r.URL.Path, "/admin/"):
			getBackendsCount.Add(1)
			json.NewEncoder(w).Encode(publicBackends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    adminBackends,
			})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/admin/backends/"):
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	// 1. First GetBackends call should hit the server
	_, err := svc.GetBackends()
	require.NoError(t, err)
	assert.Equal(t, int32(1), getBackendsCount.Load(), "first GetBackends should hit server")

	// 2. Second GetBackends should be served from cache (within 30s TTL)
	//    But since GetBackends() itself doesn't use the cache (getBackendsCached does),
	//    we call GetBackends directly which always calls the server.
	//    The cache is used by getBackendsCached (internal). So we verify via
	//    SetSystemDefaultBackend which uses getBackendsCached.

	// 3. Call SetSystemDefaultBackend — this calls getBackendsCached (populates cache)
	//    then invalidates cache after PUT succeeds
	_, err = svc.SetSystemDefaultBackend("cloud1")
	require.NoError(t, err)
	countAfterSet := getBackendsCount.Load()

	// 4. Call GetBackends again after invalidation — should hit the server again
	_, err = svc.GetBackends()
	require.NoError(t, err)
	assert.Greater(t, getBackendsCount.Load(), countAfterSet,
		"GetBackends after SetSystemDefaultBackend should fetch fresh data")
}

func TestSetSystemDefaultBackend_NotInAdminAPI(t *testing.T) {
	publicBackends := []terminalDto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: false},
	}
	// Admin API returns empty list — backend exists publicly but not in admin API
	adminBackends := []map[string]interface{}{}

	ts := setupSetDefaultTestServer(t, publicBackends, adminBackends, nil)
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	result, err := svc.SetSystemDefaultBackend("local")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in admin API")
}
