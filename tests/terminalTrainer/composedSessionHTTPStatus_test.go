package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// ==========================================
// Minimal mock for TerminalTrainerService
// ==========================================

// mockTerminalTrainerService implements services.TerminalTrainerService.
// Only StartComposedSession is set up; all other methods panic if called.
type mockTerminalTrainerService struct {
	mock.Mock
}

func (m *mockTerminalTrainerService) StartComposedSession(userID string, input dto.CreateComposedSessionInput, planInterface any) (*dto.TerminalSessionResponse, error) {
	args := m.Called(userID, input, planInterface)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TerminalSessionResponse), args.Error(1)
}

// --- Stubs for remaining interface methods (not exercised by these tests) ---

func (m *mockTerminalTrainerService) CreateUserKey(userID, userName string) error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetUserKey(userID string) (*models.UserTerminalKey, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) DisableUserKey(userID string) error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetSessionInfo(sessionID string) (*models.Terminal, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetTerminalByUUID(terminalUUID string) (*models.Terminal, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetActiveUserSessions(userID string) (*[]models.Terminal, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) StopSession(sessionID string) error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) HasTerminalAccess(sessionID, userID string, requiredLevel string) (bool, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) SyncAllActiveSessions() error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetRepository() repositories.TerminalRepository {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) CleanupExpiredSessions() error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetTerms() (string, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetBackends() ([]dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetBackendsForOrganization(orgID uuid.UUID) ([]dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetBackendsForContext(orgID uuid.UUID, userID string) ([]dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) IsBackendOnline(backendName string) (bool, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) BulkCreateTerminalsForGroup(groupID string, requestingUserID string, userRoles []string, request dto.BulkCreateTerminalsRequest, planInterface any) (*dto.BulkCreateTerminalsResponse, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetEnumService() services.TerminalTrainerEnumService {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) DeleteSessionCommandHistory(sessionID string) error {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) DeleteAllUserCommandHistory(apiKey string) (int64, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetOrganizationTerminalSessions(orgID uuid.UUID) (*[]models.Terminal, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetOrgTerminalUsage(orgID uuid.UUID) (*dto.OrgTerminalUsageResponse, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetGroupCommandHistory(groupID string, userID string, since *int64, format string, limit, offset int, includeStopped bool, search string) ([]byte, string, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetGroupCommandHistoryStats(groupID string, userID string, includeStopped bool) ([]byte, string, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetUserConsentStatus(userID string) (consentHandled bool, source string, err error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) IsUserAuthorizedForSession(userID string, terminal *models.Terminal, isAdmin bool) bool {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) IsUserOrgManagerOrAdmin(userID string, orgID uuid.UUID, isAdmin bool) bool {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetDistributions(backend string) ([]dto.TTDistribution, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetCatalogSizes() ([]dto.TTSize, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetCatalogFeatures() ([]dto.TTFeature, error) {
	panic("not implemented")
}
func (m *mockTerminalTrainerService) GetSessionOptions(plan *paymentModels.SubscriptionPlan, distribution string, backend string) (*dto.SessionOptionsResponse, error) {
	panic("not implemented")
}

// ==========================================
// Router helper
// ==========================================

// setupComposedSessionRouter builds a gin router that injects a subscription plan and
// calls the StartComposedSession controller. The mock service replaces the real one.
func setupComposedSessionRouter(svc services.TerminalTrainerService, plan *paymentModels.SubscriptionPlan) *gin.Engine {
	db := sharedTestDB
	gin.SetMode(gin.TestMode)
	router := gin.New()

	ctrl := terminalController.NewTerminalControllerWithService(db, svc)

	// Simulate auth middleware (JWT already validated upstream)
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-id")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	// Simulate payment/subscription middleware injecting the plan
	router.Use(func(c *gin.Context) {
		if plan != nil {
			c.Set("subscription_plan", plan)
			c.Set("has_active_subscription", true)
		}
		c.Next()
	})

	router.POST("/terminals/start-composed-session", ctrl.StartComposedSession)

	return router
}

// validComposedSessionBody returns a JSON body with all required fields.
func validComposedSessionBody() []byte {
	body, _ := json.Marshal(map[string]interface{}{
		"distribution": "ubuntu-24.04",
		"size":         "S",
		"terms":        "accepted",
	})
	return body
}

// defaultTestPlan returns a minimal subscription plan for injection into context.
func defaultTestPlan() *paymentModels.SubscriptionPlan {
	plan := &paymentModels.SubscriptionPlan{
		Name:                      "Pro",
		IsActive:                  true,
		AllowedMachineSizes:       []string{"XS", "S", "M"},
		MaxConcurrentTerminals:    5,
		MaxSessionDurationMinutes: 60,
	}
	plan.ID = uuid.New()
	return plan
}

// ==========================================
// HTTP status code tests
// ==========================================

// TestStartComposedSession_PlanLimitError_ShouldReturn403 verifies that when the service
// returns a plan-limit error the controller must respond with 403, not 500.
//
// Currently FAILS because the controller returns 500 for all service errors.
func TestStartComposedSession_PlanLimitError_ShouldReturn403(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	// Service returns a plan-limit error (contains "not allowed" and "plan_limit")
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("size 'XL' is not allowed: plan_limit"))

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The controller currently returns 500 — this test is expected to FAIL until the fix lands.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"plan-limit rejection (not allowed / plan_limit) must return 403, not 500")

	svc.AssertExpectations(t)
}

// TestStartComposedSession_ValidationError_ShouldReturn400 verifies that when the service
// returns a validation error (invalid size, unknown distribution, etc.) the controller
// must respond with 400, not 500.
//
// Currently FAILS because the controller returns 500 for all service errors.
func TestStartComposedSession_ValidationError_ShouldReturn400(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	// Service returns a validation error (size not found in catalog)
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("size 'INVALID' not found in catalog"))

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The controller currently returns 500 — this test is expected to FAIL until the fix lands.
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"validation error (size not found in catalog) must return 400, not 500")

	svc.AssertExpectations(t)
}

// TestStartComposedSession_FeatureNotAllowedError_ShouldReturn403 verifies that when the
// service rejects a feature that is plan-disabled, the controller returns 403, not 500.
//
// Currently FAILS because the controller returns 500 for all service errors.
func TestStartComposedSession_FeatureNotAllowedError_ShouldReturn403(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	// Service returns a feature plan-limit error
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("feature 'network' is not allowed: plan_disabled"))

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The controller currently returns 500 — this test is expected to FAIL until the fix lands.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"plan-disabled feature rejection must return 403, not 500")

	svc.AssertExpectations(t)
}

// TestStartComposedSession_ServerError_ShouldReturn500 verifies that genuine server errors
// (e.g. tt-backend unreachable, DB failure) correctly return 500.
//
// This test is expected to PASS already (current behaviour).
func TestStartComposedSession_ServerError_ShouldReturn500(t *testing.T) {
	svc := &mockTerminalTrainerService{}
	plan := defaultTestPlan()

	// Service returns a genuine server/infrastructure error
	svc.On("StartComposedSession", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("connection refused: tt-backend unavailable"))

	router := setupComposedSessionRouter(svc, plan)

	req := httptest.NewRequest("POST", "/terminals/start-composed-session", bytes.NewReader(validComposedSessionBody()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"genuine server errors must still return 500")

	svc.AssertExpectations(t)
}
