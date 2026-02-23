package terminalTrainer_tests

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

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// setupTestDBWithOrgs creates an in-memory SQLite database with terminal and organization tables
func setupTestDBWithOrgs(t *testing.T) *gorm.DB {
	db := setupTestDB(t)

	// Also migrate organization models needed for org-based access checks
	err := db.AutoMigrate(&orgModels.Organization{}, &orgModels.OrganizationMember{})
	require.NoError(t, err)

	return db
}

// createTestTerminalWithOrg creates a test terminal associated with an organization
func createTestTerminalWithOrg(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID) *models.Terminal {
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "test-session-" + uuid.New().String(),
		UserID:            userID,
		Name:              "Test Terminal",
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    orgID,
	}
	err = db.Create(terminal).Error
	require.NoError(t, err)

	return terminal
}

// createTestOrgForHistory creates a test organization for history access tests
func createTestOrgForHistory(t *testing.T, db *gorm.DB, ownerUserID string) *orgModels.Organization {
	t.Helper()
	org := &orgModels.Organization{
		Name:             "test-org-" + uuid.New().String()[:8],
		DisplayName:      "Test Organization",
		OwnerUserID:      ownerUserID,
		OrganizationType: orgModels.OrgTypeTeam,
		IsActive:         true,
		MaxGroups:        30,
		MaxMembers:       100,
	}
	err := db.Omit("Metadata").Create(org).Error
	require.NoError(t, err)

	return org
}

// createTestOrgMember creates an organization member with a given role
func createTestOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role orgModels.OrganizationMemberRole) *orgModels.OrganizationMember {
	t.Helper()
	member := &orgModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err := db.Omit("Metadata").Create(member).Error
	require.NoError(t, err)

	return member
}

// makeHistoryRequest creates a gin router and sends a GET request for session history
func makeHistoryRequest(t *testing.T, db *gorm.DB, sessionID string, userID string, userRoles []string) *httptest.ResponseRecorder {
	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})

	router.GET("/terminals/:id/history", controller.GetSessionHistory)

	req := httptest.NewRequest("GET", "/terminals/"+sessionID+"/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w
}

// TestIsSessionOwnerOrOrgManager_OrgOwnerCanAccessStudentHistory verifies that an
// organization owner can access the command history of a terminal session belonging
// to a student in their organization.
//
// Currently this test FAILS (red phase) because isSessionOwnerOrAdmin only checks
// for session owner or admin role, not organization ownership.
func TestIsSessionOwnerOrOrgManager_OrgOwnerCanAccessStudentHistory(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org owned by trainer1
	org := createTestOrgForHistory(t, db, "trainer1")

	// Make trainer1 an owner member of the org
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleOwner)

	// Make student1 a member of the org
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	// Create a terminal session owned by student1, associated with org
	terminal := createTestTerminalWithOrg(t, db, "student1", &org.ID)

	// trainer1 (org owner, non-admin role) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "trainer1", []string{"user"})

	// Org owner should NOT get a 403 Forbidden - they should be allowed access.
	// The request may fail with 500 (because there's no real TT backend to call),
	// but it must NOT be 403.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Org owner should be able to access student's command history (got 403 Forbidden)")

	// If we get a 403, let's verify it's the specific "Only session owner or admin"
	// message to confirm it's the access check that blocked us.
	if w.Code == http.StatusForbidden {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		t.Logf("Response body: %s", w.Body.String())
		assert.NotContains(t, response["error_message"], "Only session owner or admin",
			"Access check should have been expanded to include org owners")
	}
}

// TestIsSessionOwnerOrOrgManager_OrgManagerCanAccessStudentHistory verifies that an
// organization manager can access the command history of a terminal session belonging
// to a student in their organization.
//
// Currently this test FAILS (red phase) because isSessionOwnerOrAdmin only checks
// for session owner or admin role, not organization manager status.
func TestIsSessionOwnerOrOrgManager_OrgManagerCanAccessStudentHistory(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org owned by someone else
	org := createTestOrgForHistory(t, db, "org-creator")

	// Make trainer1 a manager of the org
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleManager)

	// Make student1 a member of the org
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	// Create a terminal session owned by student1, associated with org
	terminal := createTestTerminalWithOrg(t, db, "student1", &org.ID)

	// trainer1 (org manager, non-admin role) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "trainer1", []string{"user"})

	// Org manager should NOT get a 403 Forbidden - they should be allowed access.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Org manager should be able to access student's command history (got 403 Forbidden)")

	if w.Code == http.StatusForbidden {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		t.Logf("Response body: %s", w.Body.String())
		assert.NotContains(t, response["error_message"], "Only session owner or admin",
			"Access check should have been expanded to include org managers")
	}
}

// TestIsSessionOwnerOrOrgManager_OrgMemberCannotAccessStudentHistory verifies that a
// regular organization member (not owner or manager) cannot access the command history
// of another member's terminal session.
func TestIsSessionOwnerOrOrgManager_OrgMemberCannotAccessStudentHistory(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org
	org := createTestOrgForHistory(t, db, "org-creator")

	// Make trainer1 just a regular member of the org
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleMember)

	// Make student1 a member of the org
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	// Create a terminal session owned by student1, associated with org
	terminal := createTestTerminalWithOrg(t, db, "student1", &org.ID)

	// trainer1 (regular member, not owner/manager) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "trainer1", []string{"user"})

	// Regular org member should be DENIED access
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Regular org member should NOT be able to access another member's command history")
}

// TestIsSessionOwnerOrOrgManager_DifferentOrgManagerDenied verifies that a manager
// of a different organization cannot access the command history of a session belonging
// to another organization.
func TestIsSessionOwnerOrOrgManager_DifferentOrgManagerDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org1 with student1
	org1 := createTestOrgForHistory(t, db, "org1-creator")
	createTestOrgMember(t, db, org1.ID, "student1", orgModels.OrgRoleMember)

	// Create org2 with trainer1 as manager
	org2 := createTestOrgForHistory(t, db, "org2-creator")
	createTestOrgMember(t, db, org2.ID, "trainer1", orgModels.OrgRoleManager)

	// Create a terminal session owned by student1, associated with org1
	terminal := createTestTerminalWithOrg(t, db, "student1", &org1.ID)

	// trainer1 (manager of org2, NOT org1) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "trainer1", []string{"user"})

	// Manager of a different org should be DENIED access
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Manager of a different org should NOT be able to access the session's command history")
}

// TestIsSessionOwnerOrOrgManager_SessionWithoutOrg verifies that when a terminal
// session has no organization association (OrganizationID is nil), an org manager
// cannot access its history (falls back to owner/admin only).
func TestIsSessionOwnerOrOrgManager_SessionWithoutOrg(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org with trainer1 as manager
	org := createTestOrgForHistory(t, db, "org-creator")
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleManager)

	// Create a terminal session owned by student1 WITHOUT an organization
	terminal := createTestTerminalWithOrg(t, db, "student1", nil)

	// trainer1 (manager of some org) requests student1's command history
	// which has no org association
	w := makeHistoryRequest(t, db, terminal.SessionID, "trainer1", []string{"user"})

	// Should be DENIED - no org link means owner/admin only
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Org manager should NOT be able to access a session that has no organization association")
}
