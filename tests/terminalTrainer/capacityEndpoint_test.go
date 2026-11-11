// HTTP-level tests for GET /api/v1/terminals/capacity-check.
package terminalTrainer_tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authErrors "soli/formations/src/auth/errors"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// metricsAwareMockService is a TerminalTrainerService stub that returns a
// programmable ServerMetricsResponse. Used to drive the capacity-check
// endpoint in unit tests without standing up a fake tt-backend.
type metricsAwareMockService struct {
	metrics    *dto.ServerMetricsResponse
	metricsErr error
	// policy, when set, replaces the single-default-host policy.
	policy *services.BackendPolicy
	// gotCandidates records what LaunchCapacity was asked about.
	gotCandidates []string
}

func (m *metricsAwareMockService) GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error) {
	if m.metricsErr != nil {
		return nil, m.metricsErr
	}
	return m.metrics, nil
}

// --- Stubs for the rest of the interface (panic if called) ----------------

func (m *metricsAwareMockService) CreateUserKey(userID, userName string) error {
	panic("not implemented")
}
func (m *metricsAwareMockService) SyncUserKeyBudget(userID string) error {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetUserKey(userID string) (*models.UserTerminalKey, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) DisableUserKey(userID string) error { panic("not implemented") }
func (m *metricsAwareMockService) GetSessionInfo(sessionID string) (*models.Terminal, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetTerminalByUUID(terminalUUID string) (*models.Terminal, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetActiveUserSessions(userID string) (*[]models.Terminal, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) StopSession(sessionID string) error  { panic("not implemented") }
func (m *metricsAwareMockService) StartSession(sessionID string, _ *paymentModels.SubscriptionPlan) error { panic("not implemented") }
func (m *metricsAwareMockService) DeleteSession(sessionID string) error {
	panic("not implemented")
}
func (m *metricsAwareMockService) HasTerminalAccess(sessionID, userID string) (bool, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) SyncAllActiveSessions() error { panic("not implemented") }
func (m *metricsAwareMockService) GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetRepository() repositories.TerminalRepository {
	panic("not implemented")
}
func (m *metricsAwareMockService) CleanupExpiredSessions() error { panic("not implemented") }
func (m *metricsAwareMockService) GetTerms() (string, error)     { panic("not implemented") }
func (m *metricsAwareMockService) GetBackends() ([]dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetBackendsForContext(orgID uuid.UUID, userID string) ([]dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) BackendPolicyFor(*uuid.UUID, *paymentModels.SubscriptionPlan) (services.BackendPolicy, error) {
	if m.policy != nil {
		return *m.policy, nil
	}
	return services.BackendPolicy{Allowed: []string{""}}, nil
}

// LaunchCapacity treats every candidate as one host carrying m's metrics.
func (m *metricsAwareMockService) LaunchCapacity(candidates []string, _ string, plan *paymentModels.SubscriptionPlan, size string) services.CapacityResult {
	m.gotCandidates = candidates
	metrics, _ := m.GetServerMetrics(true, "")
	return services.PlacementCapacity([]services.HostLoad{{Backend: candidates[0], Metrics: metrics}}, plan, size)
}
func (m *metricsAwareMockService) RankBackends(candidates []string) []string { return candidates }
func (m *metricsAwareMockService) IsBackendOnline(backendName string) (bool, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) BulkCreateTerminalsForGroup(groupID string, requestingUserID string, userRoles []string, request dto.BulkCreateTerminalsRequest, planInterface any) (*dto.BulkCreateTerminalsResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetEnumService() services.TerminalTrainerEnumService {
	panic("not implemented")
}
func (m *metricsAwareMockService) ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetSessionCommandHistoryAdmin(sessionUUID string, limit, offset int) ([]byte, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) DeleteSessionCommandHistory(sessionID string) error {
	panic("not implemented")
}
func (m *metricsAwareMockService) DeleteAllUserCommandHistory(apiKey string) (int64, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetOrganizationTerminalSessions(orgID uuid.UUID) (*[]models.Terminal, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetOrgTerminalUsage(orgID uuid.UUID) (*dto.OrgTerminalUsageResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetUserTerminalUsage(userID string, orgID *uuid.UUID) (*dto.MyTerminalUsageResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetGroupCommandHistory(groupID string, userID string, since *int64, format string, limit, offset int, includeStopped bool, search string) ([]byte, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetGroupCommandHistoryStats(groupID string, userID string, includeStopped bool) ([]byte, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetUserConsentStatus(userID string) (bool, string, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) IsUserAuthorizedForSession(userID string, terminal *models.Terminal, isAdmin bool) bool {
	panic("not implemented")
}
func (m *metricsAwareMockService) IsUserOrgManagerOrAdmin(userID string, orgID uuid.UUID, isAdmin bool) bool {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetDistributions(backend string) ([]dto.TTDistribution, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetOfferedDistributions(backend string) ([]dto.TTDistribution, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) SetWithheldDistributions([]string) error {
	return nil
}
func (m *metricsAwareMockService) GetCatalogSizes() ([]dto.TTSize, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) FetchRawSizes(ctx context.Context) ([]dto.TTSize, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetCatalogFeatures() ([]dto.TTFeature, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) GetSessionOptions(plan *paymentModels.SubscriptionPlan, distribution string, backend string) (*dto.SessionOptionsResponse, error) {
	panic("not implemented")
}
func (m *metricsAwareMockService) EnrichSessionOptionsBudget(opts *dto.SessionOptionsResponse, plan *paymentModels.SubscriptionPlan, userID string) {
}
func (m *metricsAwareMockService) StartComposedSession(userID string, input dto.CreateComposedSessionInput, planInterface any) (*dto.TerminalSessionResponse, error) {
	panic("not implemented")
}

// --- Helper: build a router with optional auth simulation -----------------

// setupCapacityRouter wires the GET /terminals/capacity-check endpoint with
// optional auth/plan simulation. When `simulateAuth` is false, no
// userId/plan is injected — exercising the unauthenticated path.
func setupCapacityRouter(svc services.TerminalTrainerService, plan *paymentModels.SubscriptionPlan, simulateAuth bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	if simulateAuth {
		router.Use(func(c *gin.Context) {
			c.Set("userId", "test-user-id")
			c.Set("userRoles", []string{"member"})
			c.Next()
		})
		router.Use(func(c *gin.Context) {
			if plan != nil {
				c.Set("subscription_plan", plan)
				c.Set("has_active_subscription", true)
			}
			c.Next()
		})
	} else {
		// Unauthenticated path: emulate the auth middleware aborting
		// with 401 when no userId is resolved. The capacity endpoint
		// itself never runs in this path.
		router.Use(func(c *gin.Context) {
			c.JSON(http.StatusUnauthorized, &authErrors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "unauthorized",
			})
			c.Abort()
		})
	}

	ctrl := terminalController.NewTerminalControllerWithService(sharedTestDB, svc)
	router.GET("/terminals/capacity-check", ctrl.CapacityCheck)
	return router
}

// ==========================================
// Endpoint tests
// ==========================================

func TestCapacityCheck_ReturnsOKForSmallSize(t *testing.T) {
	svc := &metricsAwareMockService{
		// Plenty of RAM: 10 GB available, 50% used → total 20 GB.
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 10.0, RAMPercent: 50.0},
	}
	plan := &paymentModels.SubscriptionPlan{}
	router := setupCapacityRouter(svc, plan, true)

	req := httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?distribution=ubuntu&size=XS", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got services.CapacityResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, services.CapacityStatusOK, got.Status)
}

func TestCapacityCheck_ReturnsCriticalForOversizedRequest(t *testing.T) {
	svc := &metricsAwareMockService{
		// Tight RAM: 1.7 GB available, 83% used → total ≈ 10 GB, reserve 0.5 GB.
		// Requesting L (2 GB) leaves 1.7 - 2 = -0.3 GB → critical.
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 1.7, RAMPercent: 83.0},
	}
	plan := &paymentModels.SubscriptionPlan{}
	router := setupCapacityRouter(svc, plan, true)

	req := httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?distribution=ubuntu&size=L", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "endpoint is a query, must always return 200")
	var got services.CapacityResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, services.CapacityStatusCritical, got.Status)
	assert.Equal(t, "insufficient_ram_for_size", got.Reason)
}

func TestCapacityCheck_RequiresAuth(t *testing.T) {
	svc := &metricsAwareMockService{
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 10.0, RAMPercent: 50.0},
	}
	router := setupCapacityRouter(svc, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?size=XS", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"capacity-check must reject unauthenticated callers — would otherwise leak server metrics")
}

func TestCapacityCheck_ReturnsUnknownWhenMetricsUnavailable(t *testing.T) {
	svc := &metricsAwareMockService{
		metricsErr: errors.New("tt-backend unreachable"),
	}
	plan := &paymentModels.SubscriptionPlan{}
	router := setupCapacityRouter(svc, plan, true)

	req := httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?size=XS", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got services.CapacityResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, services.CapacityStatusUnknown, got.Status)
}

func TestCapacityCheck_AsksAboutEveryAllowedHost(t *testing.T) {
	svc := &metricsAwareMockService{
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 10.0, RAMPercent: 50.0},
		policy:  &services.BackendPolicy{Allowed: []string{"a", "b"}, Default: "a"},
	}
	router := setupCapacityRouter(svc, &paymentModels.SubscriptionPlan{}, true)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?size=XS", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"a", "b"}, svc.gotCandidates, "no backend requested: a launch may land on any allowed host")
}

func TestCapacityCheck_RequestedBackendNarrowsToIt(t *testing.T) {
	svc := &metricsAwareMockService{
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 10.0, RAMPercent: 50.0},
		policy:  &services.BackendPolicy{Allowed: []string{"a", "b"}, Default: "a"},
	}
	router := setupCapacityRouter(svc, &paymentModels.SubscriptionPlan{}, true)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?size=XS&backend=b", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"b"}, svc.gotCandidates)
}

func TestCapacityCheck_BackendOutsidePolicyIsUnknown(t *testing.T) {
	svc := &metricsAwareMockService{
		metrics: &dto.ServerMetricsResponse{RAMAvailableGB: 10.0, RAMPercent: 50.0},
		policy:  &services.BackendPolicy{Allowed: []string{"a"}, Default: "a"},
	}
	router := setupCapacityRouter(svc, &paymentModels.SubscriptionPlan{}, true)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/terminals/capacity-check?size=XS&backend=z", nil))

	require.Equal(t, http.StatusOK, w.Code, "endpoint is a query, must always return 200")
	var got services.CapacityResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, services.CapacityStatusUnknown, got.Status)
	assert.Equal(t, "backend_not_allowed", got.Reason)
	assert.Nil(t, svc.gotCandidates, "no host is read for a backend the launch would refuse")
}

// BuildComplete satisfies TerminalTrainerService. These tests never exercise
// the provisioning window, so the stub reports success without recording.
func (m *metricsAwareMockService) BuildComplete(sessionID string) error { return nil }

// Exposed-ports stubs satisfy TerminalTrainerService. These tests never
// exercise port exposure.
func (m *metricsAwareMockService) CreateExposedPort(sessionID string, containerPort int) (*dto.ExposedPortResponse, error) {
	return nil, nil
}
func (m *metricsAwareMockService) ListExposedPorts(sessionID string) ([]dto.ExposedPortResponse, error) {
	return nil, nil
}
func (m *metricsAwareMockService) DeleteExposedPort(sessionID string, exposedPortID uuid.UUID) error {
	return nil
}
func (m *metricsAwareMockService) GetActiveExposedPortsForTraefik() ([]models.ExposedPort, error) {
	return nil, nil
}
