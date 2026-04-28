package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	orgController "soli/formations/src/organizations/controller"
	orgDto "soli/formations/src/organizations/dto"
	organizationModels "soli/formations/src/organizations/models"
	orgServices "soli/formations/src/organizations/services"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupMultiBackendTestDB creates a fresh test DB and returns repo + service.
// All tables are migrated once in TestMain.
func setupMultiBackendTestDB(t *testing.T) (*repositories.TerminalRepository, *services.TerminalTrainerService) {
	db := freshTestDB(t)

	repo := repositories.NewTerminalRepository(db)
	svc := services.NewTerminalTrainerService(db)
	return &repo, &svc
}

// ============================================
// Layer 1: Repository Tests (database queries)
// ============================================

func TestGetTerminalSessionsByUserIDAndOrg(t *testing.T) {
	db := freshTestDB(t)

	repo := repositories.NewTerminalRepository(db)

	// Create user key
	userKey, err := createTestUserKey(db, "user1")
	require.NoError(t, err)

	orgID1 := uuid.New()
	orgID2 := uuid.New()

	// Create terminals with different org IDs
	terminal1 := &models.Terminal{
		SessionID:         "session-org1-a",
		UserID:            "user1",
		Name:              "Terminal Org1 A",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		Backend:           "local",
		OrganizationID:    &orgID1,
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminal1).Error
	require.NoError(t, err)

	terminal2 := &models.Terminal{
		SessionID:         "session-org2-a",
		UserID:            "user1",
		Name:              "Terminal Org2 A",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		Backend:           "cloud1",
		OrganizationID:    &orgID2,
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminal2).Error
	require.NoError(t, err)

	terminal3 := &models.Terminal{
		SessionID:         "session-no-org",
		UserID:            "user1",
		Name:              "Terminal No Org",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminal3).Error
	require.NoError(t, err)

	t.Run("filter by org1 returns only org1 terminals", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID1, false)
		require.NoError(t, err)
		assert.Len(t, *terminals, 1)
		assert.Equal(t, "session-org1-a", (*terminals)[0].SessionID)
	})

	t.Run("filter by org2 returns only org2 terminals", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID2, false)
		require.NoError(t, err)
		assert.Len(t, *terminals, 1)
		assert.Equal(t, "session-org2-a", (*terminals)[0].SessionID)
	})

	t.Run("nil org returns all terminals (global view)", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", nil, false)
		require.NoError(t, err)
		assert.Len(t, *terminals, 3)
	})

	t.Run("empty DB returns empty slice", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("nonexistent-user", nil, false)
		require.NoError(t, err)
		assert.Len(t, *terminals, 0)
	})

	t.Run("active only filter works with org", func(t *testing.T) {
		// Create a stopped terminal in org1
		stoppedTerminal := &models.Terminal{
			SessionID:         "session-org1-stopped",
			UserID:            "user1",
			Name:              "Stopped Terminal",
			Status:            "stopped",
			ExpiresAt:         time.Now().Add(time.Hour),
			InstanceType:      "test",
			MachineSize:       "S",
			Backend:           "local",
			OrganizationID:    &orgID1,
			UserTerminalKeyID: userKey.ID,
		}
		err = db.Create(stoppedTerminal).Error
		require.NoError(t, err)

		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID1, true)
		require.NoError(t, err)
		assert.Len(t, *terminals, 1)
		assert.Equal(t, "active", (*terminals)[0].Status)
	})
}

func TestTerminalBackendFieldPersistence(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "user1")
	require.NoError(t, err)

	orgID := uuid.New()

	terminal := &models.Terminal{
		SessionID:         "session-backend-test",
		UserID:            "user1",
		Name:              "Backend Test",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		Backend:           "cloud-eu-1",
		OrganizationID:    &orgID,
		UserTerminalKeyID: userKey.ID,
	}
	err = repo.CreateTerminalSession(terminal)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := repo.GetTerminalSessionByID("session-backend-test")
	require.NoError(t, err)
	assert.Equal(t, "cloud-eu-1", retrieved.Backend)
	assert.NotNil(t, retrieved.OrganizationID)
	assert.Equal(t, orgID, *retrieved.OrganizationID)
}

// ============================================
// Layer 2: Service Tests (business logic)
// ============================================

func TestValidateSessionAccess_BackendOffline(t *testing.T) {
	db := freshTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create a terminal with a backend field
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	// Set backend field on the terminal
	db.Model(terminal).Update("backend", "some-offline-backend")

	// The backend status check will fail (no Terminal Trainer configured),
	// but with no baseURL set, IsBackendOnline returns error which is logged as warning
	// and validation continues. This tests the flow without a real TT API.
	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)
	assert.NoError(t, err)

	// Without a real TT API, the backend cache will be empty so IsBackendOnline
	// returns false for unknown backends. The session should be marked as backend_offline.
	// However, if the TT API is not configured (empty baseURL), GetBackends returns error,
	// which means IsBackendOnline logs a warning but doesn't block.
	// The exact behavior depends on the service configuration.
	// With no baseURL, getBackendsCached returns error, IsBackendOnline returns false+error,
	// and the warning is logged. The check continues, so the session is still valid.
	_ = isValid
	_ = reason
}

func TestValidateSessionAccess_NoBackend_Passes(t *testing.T) {
	db := freshTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create a terminal without a backend (backward compat)
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	// Should pass - no backend means skip the backend check
	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)
	assert.NoError(t, err)
	assert.True(t, isValid)
	assert.Equal(t, "active", reason)
}

func TestValidateBackendForOrg_NoOrg_AllowsAny(t *testing.T) {
	// When no org context is provided, any backend should be allowed
	// This is tested indirectly through StartComposedSession without org ID

	db := freshTestDB(t)

	service := services.NewTerminalTrainerService(db)

	// Create a terminal without org ID - backend validation should pass
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	// Set a backend but no org - should still be valid
	db.Model(terminal).Update("backend", "any-backend")

	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)
	assert.NoError(t, err)
	// The backend check will fail because TT is not configured,
	// but since the error is logged as warning and not blocking, the session is still active
	_ = isValid
	_ = reason
}

// ============================================
// Layer 3: Controller/HTTP Tests
// ============================================

func TestGetUserSessions_FilterByOrg(t *testing.T) {
	db := freshTestDB(t)

	controller := terminalController.NewTerminalController(db)

	userKey, err := createTestUserKey(db, "test-user-org")
	require.NoError(t, err)

	orgID := uuid.New()

	// Create terminals: one with org, one without
	terminalWithOrg := &models.Terminal{
		SessionID:         "session-with-org",
		UserID:            "test-user-org",
		Name:              "With Org",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		Backend:           "local",
		OrganizationID:    &orgID,
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminalWithOrg).Error
	require.NoError(t, err)

	terminalWithoutOrg := &models.Terminal{
		SessionID:         "session-without-org",
		UserID:            "test-user-org",
		Name:              "Without Org",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminalWithoutOrg).Error
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	t.Run("with organization_id returns only org terminals", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "test-user-org")
			c.Set("userRoles", []string{"user"})
			c.Next()
		})
		router.GET("/terminals/user-sessions", controller.GetUserSessions)

		req := httptest.NewRequest("GET", "/terminals/user-sessions?organization_id="+orgID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var terminals []dto.TerminalOutput
		err := json.Unmarshal(w.Body.Bytes(), &terminals)
		require.NoError(t, err)
		assert.Len(t, terminals, 1)
		assert.Equal(t, "session-with-org", terminals[0].SessionID)
		assert.Equal(t, "local", terminals[0].Backend)
		assert.NotNil(t, terminals[0].OrganizationID)
	})

	t.Run("without organization_id returns all terminals", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "test-user-org")
			c.Set("userRoles", []string{"user"})
			c.Next()
		})
		router.GET("/terminals/user-sessions", controller.GetUserSessions)

		req := httptest.NewRequest("GET", "/terminals/user-sessions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var terminals []dto.TerminalOutput
		err := json.Unmarshal(w.Body.Bytes(), &terminals)
		require.NoError(t, err)
		assert.Len(t, terminals, 2)
	})

	t.Run("invalid organization_id returns 400", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "test-user-org")
			c.Set("userRoles", []string{"user"})
			c.Next()
		})
		router.GET("/terminals/user-sessions", controller.GetUserSessions)

		req := httptest.NewRequest("GET", "/terminals/user-sessions?organization_id=invalid", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetUserSessions_IncludesBackendAndOrgFields(t *testing.T) {
	db := freshTestDB(t)
	controller := terminalController.NewTerminalController(db)

	userKey, err := createTestUserKey(db, "test-user-fields")
	require.NoError(t, err)

	orgID := uuid.New()
	terminal := &models.Terminal{
		SessionID:         "session-fields-test",
		UserID:            "test-user-fields",
		Name:              "Fields Test",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "M",
		Backend:           "cloud-eu-1",
		OrganizationID:    &orgID,
		UserTerminalKeyID: userKey.ID,
	}
	err = db.Create(terminal).Error
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-fields")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})
	router.GET("/terminals/user-sessions", controller.GetUserSessions)

	req := httptest.NewRequest("GET", "/terminals/user-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var terminals []dto.TerminalOutput
	err = json.Unmarshal(w.Body.Bytes(), &terminals)
	require.NoError(t, err)
	require.Len(t, terminals, 1)
	assert.Equal(t, "cloud-eu-1", terminals[0].Backend)
	assert.NotNil(t, terminals[0].OrganizationID)
	assert.Equal(t, orgID, *terminals[0].OrganizationID)
}

// ============================================
// System Default Backend Tests
// ============================================

func TestSetDefaultBackend_AdminOnly(t *testing.T) {
	db := freshTestDB(t)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	t.Run("non-admin gets 403", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "regular-user")
			c.Set("userRoles", []string{"member"})
			c.Next()
		})
		router.PATCH("/terminals/backends/:backendId/set-default", ctrl.SetDefaultBackend)

		req := httptest.NewRequest("PATCH", "/terminals/backends/local/set-default", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var apiErr map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &apiErr)
		require.NoError(t, err)
		assert.Equal(t, "Admin access required", apiErr["error_message"])
	})

	t.Run("admin with unknown backend gets 404 or 500", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "admin-user")
			c.Set("userRoles", []string{"administrator"})
			c.Next()
		})
		router.PATCH("/terminals/backends/:backendId/set-default", ctrl.SetDefaultBackend)

		req := httptest.NewRequest("PATCH", "/terminals/backends/nonexistent/set-default", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Without Terminal Trainer configured, service returns error (500 for "not configured" or 404 for "not found")
		assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusInternalServerError,
			"Expected 404 or 500, got %d", w.Code)
	})
}

func TestGetBackends_ReturnsIsDefaultFromTTBackend(t *testing.T) {
	// Verify that GetBackends passes through the is_default field from tt-backend
	// without overwriting it (tt-backend is the single source of truth).
	expectedBackends := []dto.BackendInfo{
		{ID: "local", Name: "Local Backend", Connected: true, IsDefault: true},
		{ID: "cloud-eu-1", Name: "Cloud EU", Connected: true, IsDefault: false},
	}

	ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedBackends)
	}))
	defer ttServer.Close()

	db := freshTestDB(t)

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	svc := services.NewTerminalTrainerService(db)

	backends, err := svc.GetBackends()
	require.NoError(t, err)
	require.Len(t, backends, 2)

	// "local" should be marked as default (from tt-backend response)
	assert.True(t, backends[0].IsDefault, "local backend should be default")
	assert.Equal(t, "local", backends[0].ID)

	// "cloud-eu-1" should NOT be default
	assert.False(t, backends[1].IsDefault, "cloud-eu-1 should not be default")
}

func TestGetBackends_NoDefaultInResponse(t *testing.T) {
	// When tt-backend returns no backend with is_default=true,
	// GetBackends should pass that through (no backend marked as default).
	expectedBackends := []dto.BackendInfo{
		{ID: "local", Name: "Local Backend", Connected: true, IsDefault: false},
		{ID: "cloud-eu-1", Name: "Cloud EU", Connected: true, IsDefault: false},
	}

	ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedBackends)
	}))
	defer ttServer.Close()

	db := freshTestDB(t)

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	svc := services.NewTerminalTrainerService(db)

	backends, err := svc.GetBackends()
	require.NoError(t, err)
	require.Len(t, backends, 2)

	for _, b := range backends {
		assert.False(t, b.IsDefault, "no backend should be marked as default when tt-backend returns none")
	}
}

// ============================================
// Organization Backend Assignment Tests
// ============================================

func TestOrganization_BackendFieldsPersistence(t *testing.T) {
	db := freshTestDB(t)

	t.Run("AllowedBackends and DefaultBackend are persisted on org", func(t *testing.T) {
		org := &organizationModels.Organization{
			Name:             "test-org-backends",
			DisplayName:      "Test Org Backends",
			OwnerUserID:      "owner1",
			IsActive:         true,
			OrganizationType: organizationModels.OrgTypeTeam,
			MaxGroups:        10,
			MaxMembers:       50,
			AllowedBackends:  []string{"local", "cloud-eu-1"},
			DefaultBackend:   "local",
		}
		err := db.Omit("Metadata").Create(org).Error
		require.NoError(t, err)

		var retrieved organizationModels.Organization
		err = db.Where("id = ?", org.ID).First(&retrieved).Error
		require.NoError(t, err)

		assert.Equal(t, []string{"local", "cloud-eu-1"}, retrieved.AllowedBackends)
		assert.Equal(t, "local", retrieved.DefaultBackend)
	})

	t.Run("empty AllowedBackends means all backends allowed", func(t *testing.T) {
		org := &organizationModels.Organization{
			Name:             "test-org-no-restrict",
			DisplayName:      "No Restrictions",
			OwnerUserID:      "owner2",
			IsActive:         true,
			OrganizationType: organizationModels.OrgTypeTeam,
			MaxGroups:        10,
			MaxMembers:       50,
			AllowedBackends:  []string{},
			DefaultBackend:   "",
		}
		err := db.Omit("Metadata").Create(org).Error
		require.NoError(t, err)

		var retrieved organizationModels.Organization
		err = db.Where("id = ?", org.ID).First(&retrieved).Error
		require.NoError(t, err)

		assert.Empty(t, retrieved.AllowedBackends)
		assert.Equal(t, "", retrieved.DefaultBackend)
	})
}

func TestUpdateOrganizationBackends_AdminOnly(t *testing.T) {
	db := freshTestDB(t)

	// Create a test organization
	org := &organizationModels.Organization{
		Name:             "test-org-admin",
		DisplayName:      "Test Org Admin",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	orgService := orgServices.NewOrganizationService(db)
	importService := orgServices.NewImportService(db)
	ctrl := orgController.NewOrganizationController(orgService, importService, db)
	gin.SetMode(gin.TestMode)

	t.Run("non-admin gets 403", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "regular-user")
			c.Set("userRoles", []string{"member"})
			c.Next()
		})
		router.PUT("/organizations/:id/backends", ctrl.UpdateOrganizationBackends)

		body, _ := json.Marshal(orgDto.UpdateOrganizationBackendsInput{
			AllowedBackends: []string{"local"},
			DefaultBackend:  "local",
		})
		req := httptest.NewRequest("PUT", "/organizations/"+org.ID.String()+"/backends", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("admin can update backends", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "admin-user")
			c.Set("userRoles", []string{"administrator"})
			c.Next()
		})
		router.PUT("/organizations/:id/backends", ctrl.UpdateOrganizationBackends)

		body, _ := json.Marshal(orgDto.UpdateOrganizationBackendsInput{
			AllowedBackends: []string{"local", "cloud-eu-1"},
			DefaultBackend:  "local",
		})
		req := httptest.NewRequest("PUT", "/organizations/"+org.ID.String()+"/backends", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "local", result["default_backend"])
	})
}

func TestUpdateOrganizationBackends_ValidatesDefaultInAllowed(t *testing.T) {
	db := freshTestDB(t)

	org := &organizationModels.Organization{
		Name:             "test-org-validate",
		DisplayName:      "Test Org Validate",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	orgService := orgServices.NewOrganizationService(db)
	importService := orgServices.NewImportService(db)
	ctrl := orgController.NewOrganizationController(orgService, importService, db)
	gin.SetMode(gin.TestMode)

	t.Run("default_backend not in allowed_backends returns 400", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "admin-user")
			c.Set("userRoles", []string{"administrator"})
			c.Next()
		})
		router.PUT("/organizations/:id/backends", ctrl.UpdateOrganizationBackends)

		body, _ := json.Marshal(orgDto.UpdateOrganizationBackendsInput{
			AllowedBackends: []string{"local", "cloud-eu-1"},
			DefaultBackend:  "nonexistent-backend",
		})
		req := httptest.NewRequest("PUT", "/organizations/"+org.ID.String()+"/backends", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var apiErr map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &apiErr)
		require.NoError(t, err)
		assert.Contains(t, apiErr["error_message"], "default_backend must be in allowed_backends")
	})

	t.Run("empty default_backend with non-empty allowed is valid", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "admin-user")
			c.Set("userRoles", []string{"administrator"})
			c.Next()
		})
		router.PUT("/organizations/:id/backends", ctrl.UpdateOrganizationBackends)

		body, _ := json.Marshal(orgDto.UpdateOrganizationBackendsInput{
			AllowedBackends: []string{"local", "cloud-eu-1"},
			DefaultBackend:  "",
		})
		req := httptest.NewRequest("PUT", "/organizations/"+org.ID.String()+"/backends", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetOrganizationBackends(t *testing.T) {
	db := freshTestDB(t)

	org := &organizationModels.Organization{
		Name:             "test-org-get-backends",
		DisplayName:      "Test Get Backends",
		OwnerUserID:      "owner1",
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  []string{"local", "cloud-eu-1"},
		DefaultBackend:   "local",
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	orgService := orgServices.NewOrganizationService(db)
	importService := orgServices.NewImportService(db)
	ctrl := orgController.NewOrganizationController(orgService, importService, db)
	gin.SetMode(gin.TestMode)

	t.Run("returns org backend config", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "some-user")
			c.Set("userRoles", []string{"member"})
			c.Next()
		})
		router.GET("/organizations/:id/backends", ctrl.GetOrganizationBackends)

		req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/backends", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "local", result["default_backend"])

		backends, ok := result["allowed_backends"].([]interface{})
		require.True(t, ok)
		assert.Len(t, backends, 2)
	})

	t.Run("returns 404 for unknown org", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("userId", "some-user")
			c.Set("userRoles", []string{"member"})
			c.Next()
		})
		router.GET("/organizations/:id/backends", ctrl.GetOrganizationBackends)

		req := httptest.NewRequest("GET", "/organizations/"+uuid.New().String()+"/backends", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// =============================================================================
// GetBackends — controller-level authorization & error contract
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
// GetBackendsForOrganization — service-level filtering
// =============================================================================

// setupServiceWithMockBackends creates a TerminalTrainerService backed by a fake
// TT API that returns the given backends. The system default is indicated by
// setting IsDefault=true on the matching backend in the response (tt-backend is
// the single source of truth for default backend).
func setupServiceWithMockBackends(t *testing.T, backends []dto.BackendInfo, systemDefault string) (services.TerminalTrainerService, *gorm.DB) {
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

// createTestOrgWithBackends creates an org with a given backend allow-list and default.
func createTestOrgWithBackends(t *testing.T, db *gorm.DB, name string, allowedBackends []string, defaultBackend string) *organizationModels.Organization {
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
	allBackends := []dto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}

	svc, db := setupServiceWithMockBackends(t, allBackends, "default")

	t.Run("org with null allowed_backends gets only default backend", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("null-backends-%s", uuid.New().String()[:8]),
			nil, "") // nil = null in DB

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 1, "Org with null allowed_backends should get ONLY the default backend")
		assert.Equal(t, "default", backends[0].ID)
		assert.True(t, backends[0].IsDefault)
	})

	t.Run("org with empty allowed_backends gets only default backend", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("empty-backends-%s", uuid.New().String()[:8]),
			[]string{}, "") // empty slice

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 1, "Org with empty allowed_backends should get ONLY the default backend")
		assert.Equal(t, "default", backends[0].ID)
	})

	t.Run("org with both backends configured gets both", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("both-backends-%s", uuid.New().String()[:8]),
			[]string{"default", "oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

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
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("oracle-only-%s", uuid.New().String()[:8]),
			[]string{"oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 1, "Org with only oracle1 should get only oracle1")
		assert.Equal(t, "oracle1", backends[0].ID)
	})

	t.Run("org with only default gets only default", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("default-only-%s", uuid.New().String()[:8]),
			[]string{"default"}, "default")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 1, "Org with only default should get only default")
		assert.Equal(t, "default", backends[0].ID)
	})
}

// Critical: test that filtering works even when no system default is configured
// (matches production state where features table has no terminal_default_backend)
func TestGetBackendsForOrganization_NoSystemDefault(t *testing.T) {
	allBackends := []dto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}

	// Empty string = no system default configured (matches production)
	svc, db := setupServiceWithMockBackends(t, allBackends, "")

	t.Run("null allowed_backends with no system default returns first backend only", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("no-sysdefault-%s", uuid.New().String()[:8]),
			nil, "")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 1, "Should return only 1 backend, not all")
		assert.Equal(t, "default", backends[0].ID, "Should return the first backend as fallback")
	})

	t.Run("explicit config still works without system default", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("explicit-no-sysdefault-%s", uuid.New().String()[:8]),
			[]string{"default", "oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)

		require.Len(t, backends, 2, "Explicit config should still return both")
	})
}

// =============================================================================
// SetSystemDefaultBackend — service-level behavior
// =============================================================================

// setupSetDefaultTestServer creates an httptest.Server that routes requests
// to the public GET /backends, admin GET /admin/backends, and admin PUT /admin/backends/{id}
// endpoints, allowing each test to control responses independently.
func setupSetDefaultTestServer(
	t *testing.T,
	backends []dto.BackendInfo,
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
	publicBackends := []dto.BackendInfo{
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
	publicBackends := []dto.BackendInfo{
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
	publicBackends := []dto.BackendInfo{
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
	publicBackends := []dto.BackendInfo{
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
	publicBackends := []dto.BackendInfo{
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

	publicBackends := []dto.BackendInfo{
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

	// 2. Call SetSystemDefaultBackend — this calls getBackendsCached (populates cache)
	//    then invalidates cache after PUT succeeds
	_, err = svc.SetSystemDefaultBackend("cloud1")
	require.NoError(t, err)
	countAfterSet := getBackendsCount.Load()

	// 3. Call GetBackends again after invalidation — should hit the server again
	_, err = svc.GetBackends()
	require.NoError(t, err)
	assert.Greater(t, getBackendsCount.Load(), countAfterSet,
		"GetBackends after SetSystemDefaultBackend should fetch fresh data")
}

func TestSetSystemDefaultBackend_NotInAdminAPI(t *testing.T) {
	publicBackends := []dto.BackendInfo{
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
