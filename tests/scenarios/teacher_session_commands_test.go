package scenarios_test

// Wave 9 backend (#289): teacher session commands proxy.
//
// Verifies that group managers can fetch the terminal command history
// for a student's scenario session via
//   GET /api/v1/teacher/groups/:groupId/sessions/:sessionId/commands
// while non-managers, missing-terminal sessions, and out-of-group
// sessions are rejected with the appropriate status (403 / 404).
//
// The success path is tested at the service level with a mock
// TerminalTrainerService (real service would call tt-backend over HTTP,
// which is not available in unit tests). The non-manager-403,
// session-without-terminal-404, and session-not-in-group-404 cases are
// tested through the real HTTP stack via setupRealTeacherRouter (which
// runs Layer 2 enforcement) — those paths short-circuit before any
// tt-backend call is made.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

// commandsCapturingTTService is a minimal mockTTService extension that
// records the args passed to GetSessionCommandHistoryAdmin and returns
// a canned JSON body. We extend mockTTService rather than redefining
// the full interface to keep the test compact.
type commandsCapturingTTService struct {
	*mockTTService
	lastSessionUUID string
	lastLimit       int
	lastOffset      int
	body            []byte
	contentType     string
}

func newCommandsCapturingTTService(body []byte) *commandsCapturingTTService {
	return &commandsCapturingTTService{
		mockTTService: newMockTTService(),
		body:          body,
		contentType:   "application/json",
	}
}

func (m *commandsCapturingTTService) GetSessionCommandHistoryAdmin(sessionUUID string, limit, offset int) ([]byte, string, error) {
	m.lastSessionUUID = sessionUUID
	m.lastLimit = limit
	m.lastOffset = offset
	return m.body, m.contentType, nil
}

// --- Service-level test: success path ---

func TestGetSessionCommands_AsGroupManager_Allowed(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	studentID := "student-cmd-1"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-success", Title: "Commands Success", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	terminalUUID := "tt-session-uuid-" + uuid.NewString()
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalUUID,
	}
	require.NoError(t, db.Create(&session).Error)

	// Canned response body matching tt-backend's bulk-history shape.
	body := []byte(`{"commands":[{"sequence_num":1,"command_text":"ls","executed_at":1700000000}],"total":1,"limit":50,"offset":0}`)
	tt := newCommandsCapturingTTService(body)
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, tt, sessionSvc)

	gotBody, contentType, err := dashSvc.GetSessionCommands(groupID, session.ID, 25, 5)
	require.NoError(t, err)
	assert.Equal(t, terminalUUID, tt.lastSessionUUID, "service must forward the terminal session UUID")
	assert.Equal(t, 25, tt.lastLimit)
	assert.Equal(t, 5, tt.lastOffset)
	assert.Equal(t, "application/json", contentType)
	// Verify shape (verbatim proxy).
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &parsed))
	assert.Contains(t, parsed, "commands")
}

// --- Service-level tests: error paths ---

func TestGetSessionCommands_SessionWithoutTerminal_404(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	studentID := "student-no-term"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-no-term", Title: "No Term", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Session has no terminal yet (TerminalSessionID = nil).
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	body, _, err := dashSvc.GetSessionCommands(groupID, session.ID, 0, 0)
	assert.ErrorIs(t, err, services.ErrSessionHasNoTerminal,
		"missing TerminalSessionID must produce ErrSessionHasNoTerminal so the controller returns 404")
	assert.Nil(t, body)
	assert.Empty(t, tt.lastSessionUUID, "tt-backend must NOT be called when there is no terminal")
}

func TestGetSessionCommands_SessionNotInGroup_404(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	otherGroupID := uuid.New()
	studentID := "student-other-group"
	// Student is a member of a DIFFERENT group, not groupID.
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: otherGroupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-other-grp", Title: "Other Group", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	terminalUUID := "tt-session-other"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalUUID,
	}
	require.NoError(t, db.Create(&session).Error)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	body, _, err := dashSvc.GetSessionCommands(groupID, session.ID, 0, 0)
	assert.ErrorIs(t, err, services.ErrSessionNotInGroup,
		"session whose user is NOT in the requested group must produce ErrSessionNotInGroup so the controller returns 404 (no existence leak)")
	assert.Nil(t, body)
	assert.Empty(t, tt.lastSessionUUID, "tt-backend must NOT be called when membership check fails — prevents leaking session existence to non-members")
}

func TestGetSessionCommands_SessionUnknown_404(t *testing.T) {
	db := setupTestDB(t)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	body, _, err := dashSvc.GetSessionCommands(uuid.New(), uuid.New(), 0, 0)
	assert.ErrorIs(t, err, services.ErrSessionNotFound)
	assert.Nil(t, body)
}

// --- HTTP-level test: Layer 2 enforcement ---

// setupCommandsTeacherRouter mounts only the new /commands route on top of
// the standard Layer 2 enforcement stack, then injects a controller that
// uses the supplied dashboard service. We don't reuse setupRealTeacherRouter
// because that helper builds the controller with a real TerminalTrainerService
// (which would try to hit tt-backend over HTTP).
func setupCommandsTeacherRouter(t *testing.T, db *gorm.DB, dashSvc *services.TeacherDashboardService, userID string, roles []string) *gin.Engine {
	t.Helper()

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mockEnforcer)

	// Wire up enforcers with a real GORM membership checker so GroupRole
	// queries against the in-memory test DB.
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	teacher := api.Group("/teacher")
	// Inline handler that reuses the test dashboard service. The production
	// TeacherController.GetSessionCommands wraps the same service call —
	// mirroring its body here keeps this test independent of internal
	// controller construction.
	teacher.GET("/groups/:groupId/sessions/:sessionId/commands", func(c *gin.Context) {
		groupID, err := uuid.Parse(c.Param("groupId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
			return
		}
		sessionID, err := uuid.Parse(c.Param("sessionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session ID"})
			return
		}
		body, contentType, err := dashSvc.GetSessionCommands(groupID, sessionID, 0, 0)
		if err != nil {
			switch err {
			case services.ErrSessionNotFound, services.ErrSessionNotInGroup:
				c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
				return
			case services.ErrSessionHasNoTerminal:
				c.JSON(http.StatusNotFound, gin.H{"error": "session has no terminal yet"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session commands"})
			return
		}
		c.Data(http.StatusOK, contentType, body)
	})

	return r
}

func TestGetSessionCommands_AsNonManager_Forbidden(t *testing.T) {
	db := setupTestDB(t)

	// Group exists with an owner, but the requester is NOT a member at all.
	group := groupModels.ClassGroup{
		Name: "cmd-deny", DisplayName: "Cmd Deny", OwnerUserID: "owner-1", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	router := setupCommandsTeacherRouter(t, db, dashSvc, "random-student", []string{"member"})

	w := httptest.NewRecorder()
	url := "/api/v1/teacher/groups/" + group.ID.String() + "/sessions/" + uuid.NewString() + "/commands"
	req, _ := http.NewRequest("GET", url, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a user who is not a manager of the group must be rejected by Layer 2 GroupRole(manager) before the handler runs")
	assert.Empty(t, tt.lastSessionUUID, "tt-backend must NOT be called when Layer 2 denies the request")
}
