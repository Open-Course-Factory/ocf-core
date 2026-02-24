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

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	terminalServices "soli/formations/src/terminalTrainer/services"
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

// TestHistoryAccess_AdminDifferentOrg_Denied verifies that an administrator from
// Org A cannot access command history of a session belonging to Org B.
// In a multi-tenant SaaS, admin access must be scoped to the admin's own organizations.
func TestHistoryAccess_AdminDifferentOrg_Denied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create orgA with student1
	orgA := createTestOrgForHistory(t, db, "orgA-creator")
	createTestOrgMember(t, db, orgA.ID, "student1", orgModels.OrgRoleMember)

	// Create orgB with admin1 as a member
	orgB := createTestOrgForHistory(t, db, "orgB-creator")
	createTestOrgMember(t, db, orgB.ID, "admin1", orgModels.OrgRoleOwner)

	// Create a terminal session owned by student1, associated with orgA
	terminal := createTestTerminalWithOrg(t, db, "student1", &orgA.ID)

	// admin1 (administrator role, but belongs to orgB not orgA) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "admin1", []string{"administrator"})

	// Admin from a different org should be DENIED access
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Administrator from a different organization should NOT be able to access session history")
}

// TestHistoryAccess_AdminSameOrg_Allowed verifies that an administrator who belongs
// to the same organization as the session can access command history.
func TestHistoryAccess_AdminSameOrg_Allowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org with student1 and admin1
	org := createTestOrgForHistory(t, db, "org-creator")
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)
	createTestOrgMember(t, db, org.ID, "admin1", orgModels.OrgRoleOwner)

	// Create a terminal session owned by student1, associated with org
	terminal := createTestTerminalWithOrg(t, db, "student1", &org.ID)

	// admin1 (administrator role, member of the same org) requests student1's command history
	w := makeHistoryRequest(t, db, terminal.SessionID, "admin1", []string{"administrator"})

	// Admin from the same org should be ALLOWED access (not 403).
	// May get 500 due to no real TT backend, but must NOT be 403.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Administrator from the same organization should be able to access session history")
}

// TestHistoryAccess_AdminSessionWithoutOrg_Denied verifies that an administrator
// cannot access history of a session that has no organization (personal session).
func TestHistoryAccess_AdminSessionWithoutOrg_Denied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create org with admin1
	org := createTestOrgForHistory(t, db, "org-creator")
	createTestOrgMember(t, db, org.ID, "admin1", orgModels.OrgRoleOwner)

	// Create a terminal session owned by student1 WITHOUT any organization
	terminal := createTestTerminalWithOrg(t, db, "student1", nil)

	// admin1 (administrator role) requests student1's personal session history
	w := makeHistoryRequest(t, db, terminal.SessionID, "admin1", []string{"administrator"})

	// Admin should be DENIED access to sessions without org association (only owner can access)
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Administrator should NOT be able to access personal session history of another user")
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

// setupTestDBWithGroups creates an in-memory SQLite database with terminal,
// organization, and group tables for group command history tests.
func setupTestDBWithGroups(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDBWithOrgs(t)

	// Also migrate group models needed for group command history tests
	err := db.AutoMigrate(&groupModels.ClassGroup{}, &groupModels.GroupMember{})
	require.NoError(t, err)

	return db
}

// createTestGroupForHistory creates a test group under an organization for history access tests.
func createTestGroupForHistory(t *testing.T, db *gorm.DB, ownerUserID string, orgID *uuid.UUID) *groupModels.ClassGroup {
	t.Helper()
	group := &groupModels.ClassGroup{
		Name:           "test-group-" + uuid.New().String()[:8],
		DisplayName:    "Test Group",
		OwnerUserID:    ownerUserID,
		OrganizationID: orgID,
		MaxMembers:     50,
		IsActive:       true,
	}
	err := db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	return group
}

// createTestGroupMember creates a member within a group with a given role.
func createTestGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) *groupModels.GroupMember {
	t.Helper()
	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err := db.Omit("Metadata").Create(member).Error
	require.NoError(t, err)

	return member
}

// TestGetGroupCommandHistory_ScopedToOrganization verifies that GetGroupCommandHistory
// only returns terminals scoped to the group's organization. A student who has terminals
// in a different organization (or with no org) should NOT have those terminals exposed
// through the group command history.
//
// BUG C3: Without the fix, the terminal query fetches ALL terminals for member user IDs
// without scoping to the group's organization, leaking personal terminal histories.
func TestGetGroupCommandHistory_ScopedToOrganization(t *testing.T) {
	db := setupTestDBWithGroups(t)

	// Create two organizations
	orgA := createTestOrgForHistory(t, db, "org-owner-a")
	orgB := createTestOrgForHistory(t, db, "org-owner-b")

	// Create a group under org A with trainer1 as owner and student1 as member
	group := createTestGroupForHistory(t, db, "trainer1", &orgA.ID)
	createTestGroupMember(t, db, group.ID, "trainer1", groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, "student1", groupModels.GroupMemberRoleMember)

	// Create terminals for student1 that should NOT appear in group history:
	// - one under org B (different org)
	// - one with no org (personal terminal)
	createTestTerminalWithOrg(t, db, "student1", &orgB.ID)
	createTestTerminalWithOrg(t, db, "student1", nil)

	// Call GetGroupCommandHistory as trainer1 (group owner)
	// With the fix: query scoped to orgA finds 0 terminals -> returns empty result successfully
	// Without the fix: query finds orgB + nil terminals -> tries HTTP call to TT backend -> fails
	service := terminalServices.NewTerminalTrainerService(db)
	body, contentType, err := service.GetGroupCommandHistory(
		group.ID.String(), "trainer1", nil, "json", 50, 0, true, "",
	)

	// The method should return successfully with empty results (no orgA terminals exist)
	require.NoError(t, err, "GetGroupCommandHistory should not fail when no matching terminals exist for the org")
	assert.Equal(t, "application/json", contentType)

	// Parse the JSON response and verify it returns 0 commands
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "Response should be valid JSON")

	commands, ok := result["commands"].([]interface{})
	require.True(t, ok, "Response should contain 'commands' array")
	assert.Equal(t, 0, len(commands),
		"Group command history should return 0 commands when student only has terminals in other orgs")

	total, ok := result["total"].(float64)
	require.True(t, ok, "Response should contain 'total' field")
	assert.Equal(t, float64(0), total,
		"Total should be 0 when no terminals match the group's organization")
}

// TestGetGroupCommandHistoryStats_ScopedToOrganization verifies that GetGroupCommandHistoryStats
// only returns statistics for terminals scoped to the group's organization.
//
// BUG C3: Without the fix, the terminal query fetches ALL terminals for member user IDs
// without scoping to the group's organization, leaking personal terminal histories.
func TestGetGroupCommandHistoryStats_ScopedToOrganization(t *testing.T) {
	db := setupTestDBWithGroups(t)

	// Create two organizations
	orgA := createTestOrgForHistory(t, db, "org-owner-a")
	orgB := createTestOrgForHistory(t, db, "org-owner-b")

	// Create a group under org A with trainer1 as owner and student1 as member
	group := createTestGroupForHistory(t, db, "trainer1", &orgA.ID)
	createTestGroupMember(t, db, group.ID, "trainer1", groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, "student1", groupModels.GroupMemberRoleMember)

	// Create terminals for student1 that should NOT appear in group history stats:
	// - one under org B (different org)
	// - one with no org (personal terminal)
	createTestTerminalWithOrg(t, db, "student1", &orgB.ID)
	createTestTerminalWithOrg(t, db, "student1", nil)

	// Call GetGroupCommandHistoryStats as trainer1 (group owner)
	// With the fix: query scoped to orgA finds 0 terminals -> returns empty stats successfully
	// Without the fix: query finds orgB + nil terminals -> tries HTTP call to TT backend -> fails
	service := terminalServices.NewTerminalTrainerService(db)
	body, contentType, err := service.GetGroupCommandHistoryStats(
		group.ID.String(), "trainer1", true,
	)

	// The method should return successfully with empty stats (no orgA terminals exist)
	require.NoError(t, err, "GetGroupCommandHistoryStats should not fail when no matching terminals exist for the org")
	assert.Equal(t, "application/json", contentType)

	// Parse the JSON response and verify it returns empty statistics
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "Response should be valid JSON")

	summary, ok := result["summary"].(map[string]interface{})
	require.True(t, ok, "Response should contain 'summary' object")
	assert.Equal(t, float64(0), summary["total_sessions"],
		"Stats should show 0 sessions when student only has terminals in other orgs")
	assert.Equal(t, float64(0), summary["total_commands"],
		"Stats should show 0 commands when student only has terminals in other orgs")

	students, ok := result["students"].([]interface{})
	require.True(t, ok, "Response should contain 'students' array")
	assert.Equal(t, 0, len(students),
		"Stats should show 0 students when no terminals match the group's organization")
}
