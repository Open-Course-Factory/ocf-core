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

// ============================================
// Layer 1: Repository Tests (database queries)
// ============================================

func TestGetTerminalSessionsByUserIDAndOrg(t *testing.T) {
	db := freshTestDB(t)
	repo := repositories.NewTerminalRepository(db)

	userKey, err := createTestUserKey(db, "user1")
	require.NoError(t, err)

	orgID1 := uuid.New()
	orgID2 := uuid.New()

	mkTerminal := func(sessionID, status string, orgID *uuid.UUID, backend string) *models.Terminal {
		return &models.Terminal{
			SessionID:         sessionID,
			UserID:            "user1",
			Name:              sessionID,
			Status:            status,
			ExpiresAt:         time.Now().Add(time.Hour),
			InstanceType:      "test",
			MachineSize:       "S",
			Backend:           backend,
			OrganizationID:    orgID,
			UserTerminalKeyID: userKey.ID,
		}
	}

	require.NoError(t, db.Create(mkTerminal("session-org1-a", "active", &orgID1, "local")).Error)
	require.NoError(t, db.Create(mkTerminal("session-org2-a", "active", &orgID2, "cloud1")).Error)
	require.NoError(t, db.Create(mkTerminal("session-no-org", "active", nil, "")).Error)

	t.Run("filter by org1 returns only org1 terminals", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID1, false)
		require.NoError(t, err)
		require.Len(t, *terminals, 1)
		assert.Equal(t, "session-org1-a", (*terminals)[0].SessionID)
	})

	t.Run("filter by org2 returns only org2 terminals", func(t *testing.T) {
		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID2, false)
		require.NoError(t, err)
		require.Len(t, *terminals, 1)
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
		require.NoError(t, db.Create(mkTerminal("session-org1-stopped", "stopped", &orgID1, "local")).Error)

		terminals, err := repo.GetTerminalSessionsByUserIDAndOrg("user1", &orgID1, true)
		require.NoError(t, err)
		require.Len(t, *terminals, 1)
		assert.Equal(t, "active", (*terminals)[0].Status)
	})
}

// NOTE: TestTerminalBackendFieldPersistence (lines 153-183 in original) was DELETED.
// It exercised only the GORM happy path of saving/retrieving Backend + OrganizationID
// fields — anti-pattern #3 (testing the framework). The `CreateTerminalSession`
// soft-delete + reinit logic is covered by tests/terminalTrainer/syncSoftDelete_test.go.

// ============================================
// Layer 2: Service Tests (business logic)
// ============================================

// NOTE: TestValidateSessionAccess_BackendOffline and
// TestValidateBackendForOrg_NoOrg_AllowsAny were DELETED. Both ended with
// `_ = isValid; _ = reason` — they asserted nothing about the production
// behavior they claimed to cover (anti-pattern: test of nothing).

func TestValidateSessionAccess_NoBackend_Passes(t *testing.T) {
	db := freshTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Terminal without a backend (backward compat) should pass — backend check is skipped.
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)
	require.NoError(t, err)
	assert.True(t, isValid)
	assert.Equal(t, "active", reason)
}

// ============================================
// Layer 3: Controller/HTTP Tests
// ============================================

// userSessionsRouter sets up the GET /terminals/user-sessions route with the
// given auth context (userId + roles).
func userSessionsRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctrl := terminalController.NewTerminalController(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.GET("/terminals/user-sessions", ctrl.GetUserSessions)
	return router
}

func TestGetUserSessions_OrgFilter(t *testing.T) {
	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, "test-user-org")
	require.NoError(t, err)

	orgID := uuid.New()
	terminalWithOrg := &models.Terminal{
		SessionID:         "session-with-org",
		UserID:            "test-user-org",
		Name:              "With Org",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "M",
		Backend:           "cloud-eu-1",
		OrganizationID:    &orgID,
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(terminalWithOrg).Error)

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
	require.NoError(t, db.Create(terminalWithoutOrg).Error)

	router := userSessionsRouter(db, "test-user-org", []string{"user"})

	cases := []struct {
		name           string
		query          string
		wantStatus     int
		wantCount      int
		wantSessionID  string // empty = no specific check
		checkPassthrough bool // assert Backend + OrganizationID fields propagate
	}{
		{
			name:             "with organization_id returns only org terminals",
			query:            "?organization_id=" + orgID.String(),
			wantStatus:       http.StatusOK,
			wantCount:        1,
			wantSessionID:    "session-with-org",
			checkPassthrough: true,
		},
		{
			name:       "without organization_id returns all user terminals",
			query:      "",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "invalid organization_id returns 400",
			query:      "?organization_id=invalid",
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/terminals/user-sessions"+tc.query, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus != http.StatusOK {
				return
			}

			var terminals []dto.TerminalOutput
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &terminals))
			require.Len(t, terminals, tc.wantCount)
			if tc.wantSessionID != "" {
				assert.Equal(t, tc.wantSessionID, terminals[0].SessionID)
			}
			if tc.checkPassthrough {
				assert.Equal(t, "cloud-eu-1", terminals[0].Backend)
				require.NotNil(t, terminals[0].OrganizationID)
				assert.Equal(t, orgID, *terminals[0].OrganizationID)
			}
		})
	}
}

// ============================================
// System Default Backend Tests
// ============================================

func TestSetDefaultBackend_AdminOnly(t *testing.T) {
	db := freshTestDB(t)
	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)

	type want struct {
		// One of these will hold:
		exactStatus    int
		statusOneOf    []int
		errMsgContains string
	}

	cases := []struct {
		name  string
		roles []string
		want  want
	}{
		{
			name:  "non-admin gets 403",
			roles: []string{"member"},
			want:  want{exactStatus: http.StatusForbidden, errMsgContains: "Admin access required"},
		},
		{
			// Without TT API configured, service returns 500 ("not configured")
			// or 404 ("not found"). The test asserts ONE of those — not 200 or 403.
			name:  "admin with unknown backend gets 404 or 500 (not 200, not 403)",
			roles: []string{"administrator"},
			want:  want{statusOneOf: []int{http.StatusNotFound, http.StatusInternalServerError}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("userId", "u")
				c.Set("userRoles", tc.roles)
				c.Next()
			})
			router.PATCH("/terminals/backends/:backendId/set-default", ctrl.SetDefaultBackend)

			req := httptest.NewRequest("PATCH", "/terminals/backends/nonexistent/set-default", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tc.want.exactStatus != 0 {
				assert.Equal(t, tc.want.exactStatus, w.Code)
			}
			if len(tc.want.statusOneOf) > 0 {
				assert.Contains(t, tc.want.statusOneOf, w.Code,
					"expected one of %v, got %d", tc.want.statusOneOf, w.Code)
			}
			if tc.want.errMsgContains != "" {
				var apiErr map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
				assert.Equal(t, tc.want.errMsgContains, apiErr["error_message"])
			}
		})
	}
}

// TestGetBackends_PassesThroughIsDefault verifies that the service passes
// through the is_default field from tt-backend (single source of truth) without
// overwriting it. Covers both: a default is set, and no default is set.
func TestGetBackends_PassesThroughIsDefault(t *testing.T) {
	cases := []struct {
		name           string
		ttResponse     []dto.BackendInfo
		wantDefaultIDs []string // backends expected to be marked as default
	}{
		{
			name: "is_default=true is preserved",
			ttResponse: []dto.BackendInfo{
				{ID: "local", Name: "Local Backend", Connected: true, IsDefault: true},
				{ID: "cloud-eu-1", Name: "Cloud EU", Connected: true, IsDefault: false},
			},
			wantDefaultIDs: []string{"local"},
		},
		{
			name: "no default is preserved (all is_default=false)",
			ttResponse: []dto.BackendInfo{
				{ID: "local", Name: "Local Backend", Connected: true, IsDefault: false},
				{ID: "cloud-eu-1", Name: "Cloud EU", Connected: true, IsDefault: false},
			},
			wantDefaultIDs: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.ttResponse)
			}))
			defer ttServer.Close()

			db := freshTestDB(t)
			t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
			t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-key")
			t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

			svc := services.NewTerminalTrainerService(db)
			backends, err := svc.GetBackends()
			require.NoError(t, err)
			require.Len(t, backends, len(tc.ttResponse))

			gotDefaults := []string{}
			for _, b := range backends {
				if b.IsDefault {
					gotDefaults = append(gotDefaults, b.ID)
				}
			}
			assert.ElementsMatch(t, tc.wantDefaultIDs, gotDefaults)
		})
	}
}

// ============================================
// Organization Backend Assignment Tests
// ============================================

func TestOrganization_BackendFieldsPersistence(t *testing.T) {
	db := freshTestDB(t)

	cases := []struct {
		name             string
		allowedBackends  []string
		defaultBackend   string
		wantAllowed      []string // nil/empty checked via assert.Empty
		wantDefault      string
	}{
		{
			name:            "configured allowed_backends + default are persisted",
			allowedBackends: []string{"local", "cloud-eu-1"},
			defaultBackend:  "local",
			wantAllowed:     []string{"local", "cloud-eu-1"},
			wantDefault:     "local",
		},
		{
			name:            "empty allowed_backends + empty default round-trip",
			allowedBackends: []string{},
			defaultBackend:  "",
			wantAllowed:     nil, // assert.Empty
			wantDefault:     "",
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			org := &organizationModels.Organization{
				Name:             fmt.Sprintf("test-org-fields-%d", i),
				DisplayName:      tc.name,
				OwnerUserID:      "owner1",
				IsActive:         true,
				OrganizationType: organizationModels.OrgTypeTeam,
				MaxGroups:        10,
				MaxMembers:       50,
				AllowedBackends:  tc.allowedBackends,
				DefaultBackend:   tc.defaultBackend,
			}
			require.NoError(t, db.Omit("Metadata").Create(org).Error)

			var retrieved organizationModels.Organization
			require.NoError(t, db.Where("id = ?", org.ID).First(&retrieved).Error)

			if tc.wantAllowed == nil {
				assert.Empty(t, retrieved.AllowedBackends)
			} else {
				assert.Equal(t, tc.wantAllowed, retrieved.AllowedBackends)
			}
			assert.Equal(t, tc.wantDefault, retrieved.DefaultBackend)
		})
	}
}

// orgBackendsRouter mounts the org-backends PUT and GET routes with the given
// auth context (userId + roles).
func orgBackendsRouter(db *gorm.DB, userID string, roles []string) (*gin.Engine, *organizationModels.Organization) {
	gin.SetMode(gin.TestMode)
	org := &organizationModels.Organization{
		Name:             fmt.Sprintf("test-org-%s", uuid.New().String()[:8]),
		DisplayName:      "Test Org",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		MaxGroups:        10,
		MaxMembers:       50,
		AllowedBackends:  []string{"local", "cloud-eu-1"},
		DefaultBackend:   "local",
	}
	if err := db.Omit("Metadata").Create(org).Error; err != nil {
		panic(err)
	}

	orgService := orgServices.NewOrganizationService(db)
	importService := orgServices.NewImportService(db)
	ctrl := orgController.NewOrganizationController(orgService, importService, db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.PUT("/organizations/:id/backends", ctrl.UpdateOrganizationBackends)
	router.GET("/organizations/:id/backends", ctrl.GetOrganizationBackends)
	return router, org
}

func TestUpdateOrganizationBackends(t *testing.T) {
	cases := []struct {
		name            string
		roles           []string
		input           orgDto.UpdateOrganizationBackendsInput
		wantStatus      int
		wantBodyContain string // substring expected in response body
	}{
		{
			name:       "non-admin gets 403",
			roles:      []string{"member"},
			input:      orgDto.UpdateOrganizationBackendsInput{AllowedBackends: []string{"local"}, DefaultBackend: "local"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:            "admin can update backends",
			roles:           []string{"administrator"},
			input:           orgDto.UpdateOrganizationBackendsInput{AllowedBackends: []string{"local", "cloud-eu-1"}, DefaultBackend: "local"},
			wantStatus:      http.StatusOK,
			wantBodyContain: `"default_backend":"local"`,
		},
		{
			name:            "default_backend not in allowed_backends returns 400",
			roles:           []string{"administrator"},
			input:           orgDto.UpdateOrganizationBackendsInput{AllowedBackends: []string{"local", "cloud-eu-1"}, DefaultBackend: "nonexistent-backend"},
			wantStatus:      http.StatusBadRequest,
			wantBodyContain: "default_backend must be in allowed_backends",
		},
		{
			name:       "empty default_backend with non-empty allowed is valid",
			roles:      []string{"administrator"},
			input:      orgDto.UpdateOrganizationBackendsInput{AllowedBackends: []string{"local", "cloud-eu-1"}, DefaultBackend: ""},
			wantStatus: http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			router, org := orgBackendsRouter(db, "test-user", tc.roles)

			body, _ := json.Marshal(tc.input)
			req := httptest.NewRequest("PUT", "/organizations/"+org.ID.String()+"/backends", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantBodyContain != "" {
				assert.Contains(t, w.Body.String(), tc.wantBodyContain)
			}
		})
	}
}

func TestGetOrganizationBackends(t *testing.T) {
	db := freshTestDB(t)
	router, org := orgBackendsRouter(db, "some-user", []string{"member"})

	t.Run("returns org backend config", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/backends", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.Equal(t, "local", result["default_backend"])

		backends, ok := result["allowed_backends"].([]interface{})
		require.True(t, ok)
		assert.Len(t, backends, 2)
	})

	t.Run("returns 404 for unknown org", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/organizations/"+uuid.New().String()+"/backends", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// =============================================================================
// GetBackends — controller-level authorization & error contract
// =============================================================================

// getBackendsRouter mounts GET /terminals/backends with the given auth context.
func getBackendsRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctrl := terminalController.NewTerminalController(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.GET("/terminals/backends", ctrl.GetBackends)
	return router
}

// TestGetBackends_AuthAndContract covers the controller's auth gate and error
// contract for GET /terminals/backends:
//   - non-admin without org_id → 403
//   - non-admin with valid org_id → not 403 (org-filtered path is allowed)
//   - admin without org_id → not 403 (admin can list all)
//   - non-admin with invalid org_id → 400 without leak
//   - non-admin with valid-format but non-existent org_id → 404 or 500, not 403
//
// All "no TT API configured" paths must return a generic error message that
// does NOT leak URLs, keys, or internal error details.
func TestGetBackends_AuthAndContract(t *testing.T) {
	type orgSpec int
	const (
		noOrg orgSpec = iota
		realOrg
		invalidUUID
		nonExistentOrg
	)

	cases := []struct {
		name            string
		roles           []string
		org             orgSpec
		wantStatus      int    // 0 = use wantNotStatus
		wantNotStatus   int    // assert NOT this status
		wantErrContains string // substring of error_message (when set)
		wantErrEqual    string // exact error_message (when set)
	}{
		{
			name:            "non-admin without org_id → 403",
			roles:           []string{"member"},
			org:             noOrg,
			wantStatus:      http.StatusForbidden,
			wantErrContains: "Admin access required",
		},
		{
			name:          "non-admin with valid org_id → not 403",
			roles:         []string{"member"},
			org:           realOrg,
			wantNotStatus: http.StatusForbidden,
		},
		{
			name:          "admin without org_id → not 403",
			roles:         []string{"administrator"},
			org:           noOrg,
			wantNotStatus: http.StatusForbidden,
		},
		{
			name:         "non-admin with invalid org_id → 400 without leak",
			roles:        []string{"member"},
			org:          invalidUUID,
			wantStatus:   http.StatusBadRequest,
			wantErrEqual: "Invalid organization_id",
		},
		{
			name:          "non-admin with non-existent org_id → not 403",
			roles:         []string{"member"},
			org:           nonExistentOrg,
			wantNotStatus: http.StatusForbidden,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			router := getBackendsRouter(db, "u", tc.roles)

			path := "/terminals/backends"
			switch tc.org {
			case realOrg:
				org := &organizationModels.Organization{
					Name:             "test-org-backend-access",
					DisplayName:      "Test Org",
					OwnerUserID:      "u",
					IsActive:         true,
					OrganizationType: organizationModels.OrgTypeTeam,
					MaxGroups:        10,
					MaxMembers:       50,
					AllowedBackends:  []string{"local"},
					DefaultBackend:   "local",
				}
				require.NoError(t, db.Omit("Metadata").Create(org).Error)
				path += "?organization_id=" + org.ID.String()
			case invalidUUID:
				path += "?organization_id=not-a-uuid"
			case nonExistentOrg:
				path += "?organization_id=" + uuid.New().String()
			}

			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tc.wantStatus != 0 {
				assert.Equal(t, tc.wantStatus, w.Code)
			}
			if tc.wantNotStatus != 0 {
				assert.NotEqual(t, tc.wantNotStatus, w.Code,
					"expected status to NOT be %d, got %d", tc.wantNotStatus, w.Code)
			}
			if tc.wantErrEqual != "" || tc.wantErrContains != "" {
				var apiErr map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
				if tc.wantErrEqual != "" {
					assert.Equal(t, tc.wantErrEqual, apiErr["error_message"])
				}
				if tc.wantErrContains != "" {
					assert.Contains(t, apiErr["error_message"], tc.wantErrContains)
				}
			}
		})
	}
}

// TestGetBackends_GenericErrorOnFailure verifies that when the controller
// returns 500 (TT API not configured), the body does NOT leak URLs, keys, or
// internal error details — only the generic message.
func TestGetBackends_GenericErrorOnFailure(t *testing.T) {
	db := freshTestDB(t)
	router := getBackendsRouter(db, "admin-user", []string{"administrator"})

	req := httptest.NewRequest("GET", "/terminals/backends", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Skipf("expected 500 (TT API unconfigured), got %d — skipping leak check", w.Code)
	}
	var apiErr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
	errMsg := apiErr["error_message"].(string)
	assert.NotContains(t, errMsg, "http://")
	assert.NotContains(t, errMsg, "https://")
	assert.NotContains(t, errMsg, "key=")
	assert.Equal(t, "Failed to get backends", errMsg)
}

func TestGetServerMetrics_GenericErrorOnFailure(t *testing.T) {
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

	if w.Code != http.StatusInternalServerError {
		t.Skipf("expected 500 (TT API unconfigured), got %d — skipping leak check", w.Code)
	}
	var apiErr map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &apiErr))
	assert.Equal(t, "Failed to get server metrics", apiErr["error_message"])
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

	for i := range backends {
		backends[i].IsDefault = (backends[i].ID == systemDefault)
	}

	ttServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(backends)
	}))
	t.Cleanup(func() { ttServer.Close() })

	db := freshTestDB(t)

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
	require.NoError(t, db.Omit("Metadata").Create(org).Error)
	return org
}

func TestGetBackendsForOrganization_Filtering(t *testing.T) {
	allBackends := []dto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}
	svc, db := setupServiceWithMockBackends(t, allBackends, "default")

	cases := []struct {
		name              string
		allowedBackends   []string
		defaultBackend    string
		wantIDs           []string // expected backend IDs returned (order-insensitive)
		wantDefaultID     string   // ID expected to be marked is_default (empty = none required)
		wantDefaultCount  int      // number of backends marked is_default
	}{
		{
			name:             "null allowed_backends → only system default",
			allowedBackends:  nil,
			defaultBackend:   "",
			wantIDs:          []string{"default"},
			wantDefaultID:    "default",
			wantDefaultCount: 1,
		},
		{
			name:             "empty allowed_backends → only system default",
			allowedBackends:  []string{},
			defaultBackend:   "",
			wantIDs:          []string{"default"},
			wantDefaultID:    "default",
			wantDefaultCount: 1,
		},
		{
			name:             "both backends configured → both, org default marked",
			allowedBackends:  []string{"default", "oracle1"},
			defaultBackend:   "oracle1",
			wantIDs:          []string{"default", "oracle1"},
			wantDefaultID:    "oracle1",
			wantDefaultCount: 1,
		},
		{
			name:            "only oracle1 configured → only oracle1",
			allowedBackends: []string{"oracle1"},
			defaultBackend:  "oracle1",
			wantIDs:         []string{"oracle1"},
		},
		{
			name:            "only default configured → only default",
			allowedBackends: []string{"default"},
			defaultBackend:  "default",
			wantIDs:         []string{"default"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			org := createTestOrgWithBackends(t, db,
				fmt.Sprintf("filter-%s", uuid.New().String()[:8]),
				tc.allowedBackends, tc.defaultBackend)

			backends, err := svc.GetBackendsForOrganization(org.ID)
			require.NoError(t, err)
			require.Len(t, backends, len(tc.wantIDs))

			gotIDs := make([]string, len(backends))
			defaultCount := 0
			for i, b := range backends {
				gotIDs[i] = b.ID
				if b.IsDefault {
					defaultCount++
					if tc.wantDefaultID != "" {
						assert.Equal(t, tc.wantDefaultID, b.ID,
							"only org default backend should be marked as default")
					}
				}
			}
			assert.ElementsMatch(t, tc.wantIDs, gotIDs)
			if tc.wantDefaultCount > 0 {
				assert.Equal(t, tc.wantDefaultCount, defaultCount,
					"exactly %d backend(s) should be marked as default", tc.wantDefaultCount)
			}
		})
	}
}

// TestGetBackendsForOrganization_NoSystemDefault verifies behavior when no
// backend is marked is_default by tt-backend (matches production state where
// the features table has no terminal_default_backend).
func TestGetBackendsForOrganization_NoSystemDefault(t *testing.T) {
	allBackends := []dto.BackendInfo{
		{ID: "default", Name: "Default Backend", Connected: true},
		{ID: "oracle1", Name: "Oracle Cloud", Connected: true},
	}
	svc, db := setupServiceWithMockBackends(t, allBackends, "")

	t.Run("null allowed_backends + no system default → first backend only", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("no-sysdefault-%s", uuid.New().String()[:8]),
			nil, "")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)
		require.Len(t, backends, 1, "should return only 1 backend, not all")
		assert.Equal(t, "default", backends[0].ID, "should fall back to first backend")
	})

	t.Run("explicit allow-list still works without system default", func(t *testing.T) {
		org := createTestOrgWithBackends(t, db,
			fmt.Sprintf("explicit-no-sysdefault-%s", uuid.New().String()[:8]),
			[]string{"default", "oracle1"}, "oracle1")

		backends, err := svc.GetBackendsForOrganization(org.ID)
		require.NoError(t, err)
		assert.Len(t, backends, 2, "explicit config should still return both")
	})
}

// =============================================================================
// SetSystemDefaultBackend — service-level behavior
// =============================================================================

// setupSetDefaultTestServer creates an httptest.Server that routes requests
// to the public GET /backends, admin GET /admin/backends, and admin PUT
// /admin/backends/{id} endpoints.
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
			_ = json.NewEncoder(w).Encode(backends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    adminBackends,
			})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/admin/backends/"):
			if putHandler != nil {
				putHandler(w, r)
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Backend updated successfully"})
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
		_ = json.Unmarshal(body, &putBody)
		// Verify correct path (should target backend id=2 for "cloud1")
		assert.True(t, strings.HasSuffix(r.URL.Path, "/admin/backends/2"), "PUT should target admin backend ID 2, got %s", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Backend updated successfully"})
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

// TestSetSystemDefaultBackend_PreflightErrors covers the validation errors
// raised before the admin PUT is issued: backend not found, backend offline,
// backend missing from the admin API. Each row varies the public backend list
// or admin backend list to trigger one specific error path.
func TestSetSystemDefaultBackend_PreflightErrors(t *testing.T) {
	cases := []struct {
		name           string
		publicBackends []dto.BackendInfo
		adminBackends  []map[string]interface{} // nil = use default (no admin path)
		targetID       string
		wantErrSubstr  string
	}{
		{
			name: "backend not found in public list",
			publicBackends: []dto.BackendInfo{
				{ID: "local", Name: "Local Server", Connected: true, IsDefault: true},
			},
			targetID:      "nonexistent",
			wantErrSubstr: "backend not found",
		},
		{
			name: "backend is offline",
			publicBackends: []dto.BackendInfo{
				{ID: "local", Name: "Local Server", Connected: false, IsDefault: false},
			},
			targetID:      "local",
			wantErrSubstr: "backend is offline",
		},
		{
			name: "backend exists publicly but missing from admin API",
			publicBackends: []dto.BackendInfo{
				{ID: "local", Name: "Local Server", Connected: true, IsDefault: false},
			},
			adminBackends: []map[string]interface{}{}, // empty admin list
			targetID:      "local",
			wantErrSubstr: "not found in admin API",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := setupSetDefaultTestServer(t, tc.publicBackends, tc.adminBackends, nil)
			defer ts.Close()

			svc := setupSetDefaultService(t, ts.URL)

			result, err := svc.SetSystemDefaultBackend(tc.targetID)
			assert.Nil(t, result)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrSubstr)
		})
	}
}

func TestSetSystemDefaultBackend_AdminAPIError(t *testing.T) {
	publicBackends := []dto.BackendInfo{
		{ID: "local", Name: "Local Server", Connected: true, IsDefault: false},
	}

	// Admin endpoint returns 500 — different from the standard helper.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/backends") && !strings.Contains(r.URL.Path, "/admin/"):
			_ = json.NewEncoder(w).Encode(publicBackends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "internal server error"}`))
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
		_, _ = w.Write([]byte(`{"error": "database error"}`))
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
			_ = json.NewEncoder(w).Encode(publicBackends)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/admin/backends"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    adminBackends,
			})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/admin/backends/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	svc := setupSetDefaultService(t, ts.URL)

	// 1. First GetBackends call should hit the server.
	_, err := svc.GetBackends()
	require.NoError(t, err)
	assert.Equal(t, int32(1), getBackendsCount.Load(), "first GetBackends should hit server")

	// 2. SetSystemDefaultBackend populates cache (preflight) then invalidates after PUT.
	_, err = svc.SetSystemDefaultBackend("cloud1")
	require.NoError(t, err)
	countAfterSet := getBackendsCount.Load()

	// 3. Next GetBackends should refetch (cache was invalidated).
	_, err = svc.GetBackends()
	require.NoError(t, err)
	assert.Greater(t, getBackendsCount.Load(), countAfterSet,
		"GetBackends after SetSystemDefaultBackend should fetch fresh data")
}
