package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	ttDto "soli/formations/src/terminalTrainer/dto"
	ttModels "soli/formations/src/terminalTrainer/models"
	ttRepos "soli/formations/src/terminalTrainer/repositories"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// --- Mock for TerminalTrainerService (minimal: only methods used by BulkStartScenario) ---

type mockTTService struct {
	keys map[string]*ttModels.UserTerminalKey // pre-populated user keys
}

func newMockTTService() *mockTTService {
	return &mockTTService{keys: make(map[string]*ttModels.UserTerminalKey)}
}

func (m *mockTTService) addKey(userID string) {
	m.keys[userID] = &ttModels.UserTerminalKey{UserID: userID, APIKey: "key-" + userID, KeyName: "test", IsActive: true}
}

// --- Methods actually called by BulkStartScenario ---

func (m *mockTTService) GetUserKey(userID string) (*ttModels.UserTerminalKey, error) {
	if k, ok := m.keys[userID]; ok {
		return k, nil
	}
	return nil, assert.AnError
}

func (m *mockTTService) CreateUserKey(userID, userName string) error {
	m.addKey(userID)
	return nil
}

func (m *mockTTService) GetTerms() (string, error) {
	return "test-terms", nil
}

// --- Stubs for the rest of the interface (not called by BulkStartScenario) ---

func (m *mockTTService) DisableUserKey(string) error { return nil }
func (m *mockTTService) StartSessionWithPlan(userID string, sessionInput ttDto.CreateTerminalSessionInput, _ any) (*ttDto.TerminalSessionResponse, error) {
	return &ttDto.TerminalSessionResponse{SessionID: "terminal-" + userID, Status: "running"}, nil
}
func (m *mockTTService) GetSessionInfo(string) (*ttModels.Terminal, error) { return nil, nil }
func (m *mockTTService) GetTerminalByUUID(string) (*ttModels.Terminal, error) {
	return nil, nil
}
func (m *mockTTService) GetActiveUserSessions(string) (*[]ttModels.Terminal, error) {
	return nil, nil
}
func (m *mockTTService) StopSession(string) error { return nil }
func (m *mockTTService) ShareTerminal(string, string, string, string, *time.Time) error {
	return nil
}
func (m *mockTTService) ShareTerminalWithGroup(string, string, uuid.UUID, string, *time.Time) error {
	return nil
}
func (m *mockTTService) RevokeTerminalAccess(string, string, string) error { return nil }
func (m *mockTTService) GetTerminalShares(string, string) (*[]ttModels.TerminalShare, error) {
	return nil, nil
}
func (m *mockTTService) GetSharedTerminals(string) (*[]ttModels.Terminal, error) { return nil, nil }
func (m *mockTTService) GetSharedTerminalsWithHidden(string, bool) (*[]ttModels.Terminal, error) {
	return nil, nil
}
func (m *mockTTService) HasTerminalAccess(string, string, string) (bool, error) {
	return false, nil
}
func (m *mockTTService) GetSharedTerminalInfo(string, string) (*ttDto.SharedTerminalInfo, error) {
	return nil, nil
}
func (m *mockTTService) HideTerminal(string, string) error   { return nil }
func (m *mockTTService) UnhideTerminal(string, string) error { return nil }
func (m *mockTTService) GetAllSessionsFromAPI(string) (*ttDto.TerminalTrainerSessionsResponse, error) {
	return nil, nil
}
func (m *mockTTService) SyncUserSessions(string) (*ttDto.SyncAllSessionsResponse, error) {
	return nil, nil
}
func (m *mockTTService) SyncAllActiveSessions() error { return nil }
func (m *mockTTService) GetSessionInfoFromAPI(string) (*ttDto.TerminalTrainerSessionInfo, error) {
	return nil, nil
}
func (m *mockTTService) GetRepository() ttRepos.TerminalRepository { return nil }
func (m *mockTTService) CleanupExpiredSessions() error             { return nil }
func (m *mockTTService) GetInstanceTypes(string) ([]ttDto.InstanceType, error) {
	return nil, nil
}
func (m *mockTTService) GetSizes() ([]string, error) { return nil, nil }
func (m *mockTTService) GetServerMetrics(bool, string) (*ttDto.ServerMetricsResponse, error) {
	return nil, nil
}
func (m *mockTTService) GetBackends() ([]ttDto.BackendInfo, error) { return nil, nil }
func (m *mockTTService) GetBackendsForOrganization(uuid.UUID) ([]ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *mockTTService) GetBackendsForContext(uuid.UUID, string) ([]ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *mockTTService) IsBackendOnline(string) (bool, error) { return true, nil }
func (m *mockTTService) SetSystemDefaultBackend(string) (*ttDto.BackendInfo, error) {
	return nil, nil
}
func (m *mockTTService) FixTerminalHidePermissions(string) (*ttDto.FixPermissionsResponse, error) {
	return nil, nil
}
func (m *mockTTService) BulkCreateTerminalsForGroup(string, string, []string, ttDto.BulkCreateTerminalsRequest, any) (*ttDto.BulkCreateTerminalsResponse, error) {
	return nil, nil
}
func (m *mockTTService) GetEnumService() ttServices.TerminalTrainerEnumService { return nil }
func (m *mockTTService) ValidateSessionAccess(string, bool) (bool, string, error) {
	return true, "", nil
}
func (m *mockTTService) GetSessionCommandHistory(string, *int64, string, int, int) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockTTService) DeleteSessionCommandHistory(string) error      { return nil }
func (m *mockTTService) DeleteAllUserCommandHistory(string) (int64, error) { return 0, nil }
func (m *mockTTService) GetOrganizationTerminalSessions(uuid.UUID) (*[]ttModels.Terminal, error) {
	return nil, nil
}
func (m *mockTTService) GetGroupCommandHistory(string, string, *int64, string, int, int, bool, string) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockTTService) GetGroupCommandHistoryStats(string, string, bool) ([]byte, string, error) {
	return nil, "", nil
}
func (m *mockTTService) GetUserConsentStatus(string) (bool, string, error) {
	return true, "mock", nil
}
func (m *mockTTService) IsUserAuthorizedForSession(string, *ttModels.Terminal, bool) bool {
	return true
}
func (m *mockTTService) IsUserOrgManagerOrAdmin(string, uuid.UUID, bool) bool { return true }

// --- Tests ---

// TestBulkStartScenario_ReplacesExistingActiveSessions verifies that when members
// already have "active" sessions for the same scenario, BulkStartScenario should
// abandon those old sessions and create new ones (reporting via result.Replaced).
func TestBulkStartScenario_ReplacesExistingActiveSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create scenario with steps
	scenario := models.Scenario{
		Name: "replace-active", Title: "Replace Active", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	// Create group with 2 members
	groupID := uuid.New()
	users := []string{"replace-active-s1", "replace-active-s2"}
	for _, uid := range users {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	// Create existing ACTIVE sessions for both users
	oldSessionIDs := make([]uuid.UUID, 0, 2)
	for _, uid := range users {
		session := models.ScenarioSession{
			ScenarioID: scenario.ID, UserID: uid, Status: "active",
			CurrentStep: 0, StartedAt: time.Now().Add(-time.Hour),
		}
		require.NoError(t, db.Create(&session).Error)
		oldSessionIDs = append(oldSessionIDs, session.ID)
	}

	// Set up mocks
	ttMock := newMockTTService()
	for _, uid := range users {
		ttMock.addKey(uid)
	}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	// Call BulkStartScenario with empty instanceType (no terminal creation)
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, "", "", 0, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify: old sessions should be abandoned
	for _, oldID := range oldSessionIDs {
		var oldSession models.ScenarioSession
		require.NoError(t, db.First(&oldSession, "id = ?", oldID).Error)
		assert.Equal(t, "abandoned", oldSession.Status,
			"old active session %s should have been abandoned", oldID)
	}

	// Verify: new sessions should have been created (one per user)
	for _, uid := range users {
		var sessions []models.ScenarioSession
		db.Where("user_id = ? AND scenario_id = ? AND status = ?", uid, scenario.ID, "active").Find(&sessions)
		assert.Len(t, sessions, 1,
			"user %s should have exactly 1 new active session", uid)
		// The new session should be different from the old one
		if len(sessions) > 0 {
			found := false
			for _, oldID := range oldSessionIDs {
				if sessions[0].ID == oldID {
					found = true
				}
			}
			assert.False(t, found, "the active session should be a new one, not the old one")
		}
	}

	// Verify counters
	assert.Equal(t, 2, result.Replaced, "should report 2 replaced sessions")
	assert.Equal(t, 2, result.Created, "should report 2 created sessions")
	assert.Equal(t, 0, result.Skipped, "should not skip any members")
	assert.Empty(t, result.Errors, "should have no errors")
}

// TestBulkStartScenario_ReplacesInProgressSessions verifies that "in_progress"
// sessions are also replaced (not just "active" ones).
func TestBulkStartScenario_ReplacesInProgressSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create scenario with steps
	scenario := models.Scenario{
		Name: "replace-progress", Title: "Replace InProgress", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	// Create group with 1 member
	groupID := uuid.New()
	userID := "replace-progress-s1"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: userID, Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create existing IN_PROGRESS session
	oldSession := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: userID, Status: "in_progress",
		CurrentStep: 1, StartedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, db.Create(&oldSession).Error)

	// Set up mocks
	ttMock := newMockTTService()
	ttMock.addKey(userID)
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	// Call BulkStartScenario
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, "", "", 0, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify: old in_progress session should be abandoned
	var oldSessionUpdated models.ScenarioSession
	require.NoError(t, db.First(&oldSessionUpdated, "id = ?", oldSession.ID).Error)
	assert.Equal(t, "abandoned", oldSessionUpdated.Status,
		"old in_progress session should have been abandoned")

	// Verify: a new active session was created
	var activeSessions []models.ScenarioSession
	db.Where("user_id = ? AND scenario_id = ? AND status = ?", userID, scenario.ID, "active").Find(&activeSessions)
	assert.Len(t, activeSessions, 1, "should have exactly 1 new active session")
	if len(activeSessions) > 0 {
		assert.NotEqual(t, oldSession.ID, activeSessions[0].ID, "the new session should be different from the old one")
	}

	// Verify counters
	assert.Equal(t, 1, result.Replaced, "should report 1 replaced session")
	assert.Equal(t, 1, result.Created, "should report 1 created session")
	assert.Equal(t, 0, result.Skipped, "should not skip any members")
	assert.Empty(t, result.Errors, "should have no errors")
}
