package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	ttDto "soli/formations/src/terminalTrainer/dto"
	ttModels "soli/formations/src/terminalTrainer/models"
	ttRepos "soli/formations/src/terminalTrainer/repositories"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// --- Capturing mock for TerminalTrainerService ---
// This mock captures the CreateComposedSessionInput passed to StartComposedSession,
// so we can assert on the fields sent to the terminal trainer.

type capturingTTService struct {
	keys             map[string]*ttModels.UserTerminalKey
	capturedInputs   []ttDto.CreateComposedSessionInput
	capturedUserIDs  []string
}

func newCapturingTTService() *capturingTTService {
	return &capturingTTService{
		keys:            make(map[string]*ttModels.UserTerminalKey),
		capturedInputs:  make([]ttDto.CreateComposedSessionInput, 0),
		capturedUserIDs: make([]string, 0),
	}
}

func (m *capturingTTService) addKey(userID string) {
	m.keys[userID] = &ttModels.UserTerminalKey{UserID: userID, APIKey: "key-" + userID, KeyName: "test", IsActive: true}
}

// --- Methods actually called by BulkStartScenario ---

func (m *capturingTTService) GetUserKey(userID string) (*ttModels.UserTerminalKey, error) {
	if k, ok := m.keys[userID]; ok {
		return k, nil
	}
	return nil, assert.AnError
}

func (m *capturingTTService) CreateUserKey(userID, userName string) error {
	m.addKey(userID)
	return nil
}

func (m *capturingTTService) GetTerms() (string, error) {
	return "test-terms", nil
}

// --- Stubs for the rest of the interface (not called by BulkStartScenario) ---

func (m *capturingTTService) DisableUserKey(string) error { return nil }
func (m *capturingTTService) GetSessionInfo(string) (*ttModels.Terminal, error) { return nil, nil }
func (m *capturingTTService) GetTerminalByUUID(string) (*ttModels.Terminal, error) {
	return nil, nil
}
func (m *capturingTTService) GetActiveUserSessions(string) (*[]ttModels.Terminal, error) {
	return nil, nil
}
func (m *capturingTTService) StopSession(string) error { return nil }
func (m *capturingTTService) HasTerminalAccess(string, string, string) (bool, error) {
	return false, nil
}
func (m *capturingTTService) GetAllSessionsFromAPI(string) (*ttDto.TerminalTrainerSessionsResponse, error) {
	return nil, nil
}
func (m *capturingTTService) SyncUserSessions(string) (*ttDto.SyncAllSessionsResponse, error) {
	return nil, nil
}
func (m *capturingTTService) SyncAllActiveSessions() error { return nil }
func (m *capturingTTService) GetSessionInfoFromAPI(string) (*ttDto.TerminalTrainerSessionInfo, error) {
	return nil, nil
}
func (m *capturingTTService) GetRepository() ttRepos.TerminalRepository { return nil }
func (m *capturingTTService) CleanupExpiredSessions() error             { return nil }
func (m *capturingTTService) GetServerMetrics(bool, string) (*ttDto.ServerMetricsResponse, error) {
	return nil, nil
}
func (m *capturingTTService) GetBackends() ([]ttDto.BackendInfo, error) { return nil, nil }
func (m *capturingTTService) GetBackendsForOrganization(uuid.UUID) ([]ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *capturingTTService) GetBackendsForContext(uuid.UUID, string) ([]ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *capturingTTService) IsBackendOnline(string) (bool, error) { return true, nil }
func (m *capturingTTService) SetSystemDefaultBackend(string) (*ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *capturingTTService) BulkCreateTerminalsForGroup(string, string, []string, ttDto.BulkCreateTerminalsRequest, any) (*ttDto.BulkCreateTerminalsResponse, error) {
	return nil, nil
}
func (m *capturingTTService) GetEnumService() ttServices.TerminalTrainerEnumService { return nil }
func (m *capturingTTService) ValidateSessionAccess(string, bool) (bool, string, error) {
	return true, "", nil
}
func (m *capturingTTService) GetSessionCommandHistory(string, *int64, string, int, int) ([]byte, string, error) {
	return nil, "", nil
}
func (m *capturingTTService) DeleteSessionCommandHistory(string) error      { return nil }
func (m *capturingTTService) DeleteAllUserCommandHistory(string) (int64, error) { return 0, nil }
func (m *capturingTTService) GetOrganizationTerminalSessions(uuid.UUID) (*[]ttModels.Terminal, error) {
	return nil, nil
}
func (m *capturingTTService) GetOrgTerminalUsage(uuid.UUID) (*ttDto.OrgTerminalUsageResponse, error) {
	return nil, nil
}
func (m *capturingTTService) GetGroupCommandHistory(string, string, *int64, string, int, int, bool, string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *capturingTTService) GetGroupCommandHistoryStats(string, string, bool) ([]byte, string, error) {
	return nil, "", nil
}
func (m *capturingTTService) GetUserConsentStatus(string) (bool, string, error) {
	return true, "mock", nil
}
func (m *capturingTTService) IsUserAuthorizedForSession(string, *ttModels.Terminal, bool) bool {
	return true
}
func (m *capturingTTService) IsUserOrgManagerOrAdmin(string, uuid.UUID, bool) bool { return true }
func (m *capturingTTService) GetDistributions(string) ([]ttDto.TTDistribution, error) {
	return nil, nil
}
func (m *capturingTTService) GetCatalogSizes() ([]ttDto.TTSize, error) { return nil, nil }
func (m *capturingTTService) GetCatalogFeatures() ([]ttDto.TTFeature, error) { return nil, nil }
func (m *capturingTTService) GetSessionOptions(*paymentModels.SubscriptionPlan, string, string) (*ttDto.SessionOptionsResponse, error) {
	return nil, nil
}
func (m *capturingTTService) StartComposedSession(userID string, input ttDto.CreateComposedSessionInput, _ any) (*ttDto.TerminalSessionResponse, error) {
	m.capturedUserIDs = append(m.capturedUserIDs, userID)
	m.capturedInputs = append(m.capturedInputs, input)
	return &ttDto.TerminalSessionResponse{SessionID: "terminal-" + userID, Status: "running"}, nil
}

// --- Test ---

// TestBulkStartScenario_PassesOrganizationID verifies that when a scenario has an
// OrganizationID set, the BulkStartScenario method passes it through to the
// CreateComposedSessionInput when calling StartComposedSession on the terminal trainer.
func TestBulkStartScenario_PassesOrganizationID(t *testing.T) {
	db := setupTestDB(t)

	// Create an organization ID for the scenario
	orgID := uuid.New()

	// Create scenario WITH OrganizationID and steps
	scenario := models.Scenario{
		Name:           "org-backend-test",
		Title:          "Org Backend Test",
		InstanceType:   "ubuntu:22.04",
		CreatedByID:    "creator1",
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	// Verify the scenario was saved with the org ID
	var savedScenario models.Scenario
	require.NoError(t, db.First(&savedScenario, "id = ?", scenario.ID).Error)
	require.NotNil(t, savedScenario.OrganizationID, "scenario should have OrganizationID set")
	require.Equal(t, orgID, *savedScenario.OrganizationID)

	// Create a subscription plan and user subscription so plan resolution works
	plan := paymentModels.SubscriptionPlan{
		Name:                        "Test Plan",
		IsActive:                    true,
		MaxConcurrentUsers:          10,
		MaxSessionDurationMinutes:   240,
		AllowedMachineSizes:         []string{"all"},
		CommandHistoryRetentionDays: 30,
	}
	require.NoError(t, db.Create(&plan).Error)

	// Create group with 1 member
	groupID := uuid.New()
	userID := "org-backend-user1"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: userID, Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create active subscription for the user
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}).Error)

	// Set up capturing mock
	ttMock := newCapturingTTService()
	ttMock.addKey(userID)
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	// Call BulkStartScenario WITH an instanceType (distribution) so terminal creation triggers
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, "ubuntu:22.04", "", 0, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify: StartComposedSession was called exactly once
	require.Len(t, ttMock.capturedInputs, 1, "StartComposedSession should have been called once")
	require.Len(t, ttMock.capturedUserIDs, 1, "StartComposedSession should have been called for 1 user")
	assert.Equal(t, userID, ttMock.capturedUserIDs[0])

	// THE KEY ASSERTION: OrganizationID must be passed through from the scenario
	capturedInput := ttMock.capturedInputs[0]
	assert.Equal(t, orgID.String(), capturedInput.OrganizationID,
		"CreateComposedSessionInput.OrganizationID should match the scenario's OrganizationID, "+
			"but got %q (empty means the field was not set)", capturedInput.OrganizationID)

	// Also verify other fields were set correctly
	assert.Equal(t, "test-terms", capturedInput.Terms)
	assert.Equal(t, "ubuntu:22.04", capturedInput.Distribution)

	// Verify session was created successfully
	assert.Equal(t, 1, result.Created, "should report 1 created session")
	assert.Empty(t, result.Errors, "should have no errors")
}
