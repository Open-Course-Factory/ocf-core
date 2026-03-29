package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"

	"gorm.io/gorm"
)

// setupOrgTestRouterWithUserAndRoles creates a test router with org-level scenario
// routes registered and custom userId + userRoles in the Gin context.
func setupOrgTestRouterWithUserAndRoles(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	// Organization-level scenario routes (the new endpoints under test)
	orgScenarios := api.Group("/organizations/:id/scenarios")
	orgScenarios.GET("", controller.OrgListScenarios)
	orgScenarios.POST("/import-json", controller.OrgImportJSON)
	orgScenarios.POST("/upload", controller.OrgUploadScenario)
	orgScenarios.GET("/:scenarioId/export", controller.OrgExportScenario)
	orgScenarios.DELETE("/:scenarioId", controller.OrgDeleteScenario)

	// Group-level combined listing (the new endpoint under test)
	groupScenarios := api.Group("/groups/:groupId/scenarios")
	groupScenarios.GET("", controller.ListGroupAvailableScenarios)

	return r
}

// --- Helpers ---

func createTestOrg(t *testing.T, db *gorm.DB, ownerUserID string) uuid.UUID {
	t.Helper()
	orgID, err := uuid.NewV7()
	require.NoError(t, err)
	org := &orgModels.Organization{
		Name:             "Test Org",
		DisplayName:      "Test Org",
		OwnerUserID:      ownerUserID,
		OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers:       100,
		IsActive:         true,
	}
	org.ID = orgID
	require.NoError(t, db.Omit("Metadata").Create(org).Error)
	return orgID
}

func addOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role orgModels.OrganizationMemberRole) {
	t.Helper()
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
}

func createTestScenarioForOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name:           name,
		Title:          "Test: " + name,
		Description:    "Test scenario for org management",
		InstanceType:   "ubuntu:22.04",
		SourceType:     "seed",
		CreatedByID:    "test-creator",
		OrganizationID: &orgID,
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Do step 1"},
		},
	}
	require.NoError(t, db.Create(scenario).Error)
	return scenario
}

func createTestScenarioNoOrg(t *testing.T, db *gorm.DB, name string) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name:         name,
		Title:        "Test: " + name,
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "test-creator",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Do step 1"},
		},
	}
	require.NoError(t, db.Create(scenario).Error)
	return scenario
}

func createTestGroupInOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID, ownerUserID string) uuid.UUID {
	t.Helper()
	groupID, err := uuid.NewV7()
	require.NoError(t, err)
	group := &groupModels.ClassGroup{
		Name:           "Test Group",
		DisplayName:    "Test Group",
		OwnerUserID:    ownerUserID,
		OrganizationID: &orgID,
		MaxMembers:     50,
		IsActive:       true,
	}
	group.ID = groupID
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	return groupID
}

func addGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) {
	t.Helper()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID:   groupID,
		UserID:    userID,
		Role:      role,
		InvitedBy: "test",
		JoinedAt:  time.Now(),
		IsActive:  true,
	}).Error)
}

func createScenarioAssignment(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, groupID *uuid.UUID, orgID *uuid.UUID, scope string) *models.ScenarioAssignment {
	t.Helper()
	assignment := &models.ScenarioAssignment{
		ScenarioID:     scenarioID,
		GroupID:        groupID,
		OrganizationID: orgID,
		Scope:          scope,
		CreatedByID:    "test-creator",
		IsActive:       true,
	}
	require.NoError(t, db.Create(assignment).Error)
	return assignment
}

// ============================================================================
// OrgListScenarios — GET /organizations/:orgId/scenarios
// ============================================================================

func TestOrgListScenarios_ReturnsOrgScenarios(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-list-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	// Create 2 scenarios belonging to this org
	createTestScenarioForOrg(t, db, orgID, "org-scenario-1")
	createTestScenarioForOrg(t, db, orgID, "org-scenario-2")

	// Create 1 scenario NOT belonging to this org (should not appear)
	createTestScenarioNoOrg(t, db, "unrelated-scenario")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 2, "should return only the 2 org scenarios")
}

func TestOrgListScenarios_EmptyList(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-list-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 0, "should return empty list for org with no scenarios")
}

func TestOrgListScenarios_Forbidden_NonManager(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-list-003"
	memberID := "org-member-list-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	router := setupOrgTestRouterWithUserAndRoles(db, memberID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOrgListScenarios_BadRequest_InvalidOrgID(t *testing.T) {
	db := freshTestDB(t)

	router := setupOrgTestRouterWithUserAndRoles(db, "some-user", []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/not-a-uuid/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOrgListScenarios_AdminBypass(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-list-004"
	adminID := "platform-admin-list-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	createTestScenarioForOrg(t, db, orgID, "admin-visible-scenario")

	// Admin is NOT an org member but should still have access
	router := setupOrgTestRouterWithUserAndRoles(db, adminID, []string{"Administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 1)
}

// ============================================================================
// OrgImportJSON — POST /organizations/:orgId/scenarios/import-json
// ============================================================================

func TestOrgImportJSON_Success(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-import-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	payload := map[string]any{
		"title":         "Org-Imported Scenario",
		"instance_type": "ubuntu:22.04",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Org-Imported Scenario", response["title"])
	assert.Equal(t, orgID.String(), response["organization_id"], "scenario should have the org ID set")
}

func TestOrgImportJSON_NoAutoAssignment(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-import-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	payload := map[string]any{
		"title":         "No-Assignment Scenario",
		"instance_type": "ubuntu:22.04",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// Unlike GroupImportJSON, OrgImportJSON should NOT create a ScenarioAssignment
	var assignmentCount int64
	db.Model(&models.ScenarioAssignment{}).Count(&assignmentCount)
	assert.Equal(t, int64(0), assignmentCount, "org-level import should NOT auto-create assignments")
}

func TestOrgImportJSON_Forbidden_NonManager(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-import-003"
	memberID := "org-member-import-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	router := setupOrgTestRouterWithUserAndRoles(db, memberID, []string{"Member"})

	payload := map[string]any{
		"title":         "Blocked Import",
		"instance_type": "ubuntu:22.04",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOrgImportJSON_BadRequest_InvalidInput(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-import-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	// Missing required "title" and "steps"
	payload := map[string]any{}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOrgImportJSON_ManagerCanImport(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-import-005"
	managerID := "org-manager-import-005"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, managerID, orgModels.OrgRoleManager)

	router := setupOrgTestRouterWithUserAndRoles(db, managerID, []string{"Member"})

	payload := map[string]any{
		"title":         "Manager-Imported Scenario",
		"instance_type": "ubuntu:22.04",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/organizations/"+orgID.String()+"/scenarios/import-json", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ============================================================================
// OrgDeleteScenario — DELETE /organizations/:orgId/scenarios/:scenarioId
// ============================================================================

func TestOrgDeleteScenario_Success(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-delete-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	scenario := createTestScenarioForOrg(t, db, orgID, "delete-me")

	// Create an assignment that should be cleaned up on delete
	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	createScenarioAssignment(t, db, scenario.ID, &groupID, &orgID, "group")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify scenario is deleted from DB
	var scenarioCount int64
	db.Model(&models.Scenario{}).Where("id = ?", scenario.ID).Count(&scenarioCount)
	assert.Equal(t, int64(0), scenarioCount, "scenario should be deleted")

	// Verify assignments are cleaned up
	var assignmentCount int64
	db.Model(&models.ScenarioAssignment{}).Where("scenario_id = ?", scenario.ID).Count(&assignmentCount)
	assert.Equal(t, int64(0), assignmentCount, "assignments should be cleaned up on scenario delete")
}

func TestOrgDeleteScenario_Forbidden_NonManager(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-delete-002"
	memberID := "org-member-delete-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	scenario := createTestScenarioForOrg(t, db, orgID, "no-delete")

	router := setupOrgTestRouterWithUserAndRoles(db, memberID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	// Verify scenario is NOT deleted
	var scenarioCount int64
	db.Model(&models.Scenario{}).Where("id = ?", scenario.ID).Count(&scenarioCount)
	assert.Equal(t, int64(1), scenarioCount, "scenario should still exist")
}

func TestOrgDeleteScenario_NotFound_WrongOrg(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-delete-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	// Create a scenario that belongs to a DIFFERENT org
	otherOrgID := createTestOrg(t, db, "other-owner-003")
	scenario := createTestScenarioForOrg(t, db, otherOrgID, "wrong-org-scenario")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOrgDeleteScenario_NotFound_NonexistentScenario(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-delete-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	fakeScenarioID := uuid.New()

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+fakeScenarioID.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ============================================================================
// OrgExportScenario — GET /organizations/:orgId/scenarios/:scenarioId/export
// ============================================================================

func TestOrgExportScenario_JSON_Success(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-export-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	scenario := createTestScenarioForOrg(t, db, orgID, "export-me")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String()+"/export?format=json", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Test: export-me", response["title"])
}

func TestOrgExportScenario_KillerCoda_Success(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-export-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	scenario := createTestScenarioForOrg(t, db, orgID, "export-archive")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String()+"/export?format=killerkoda", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/zip")
}

func TestOrgExportScenario_Forbidden_NonManager(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-export-003"
	memberID := "org-member-export-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	scenario := createTestScenarioForOrg(t, db, orgID, "no-export")

	router := setupOrgTestRouterWithUserAndRoles(db, memberID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOrgExportScenario_NotFound_WrongOrg(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-export-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	// Scenario belongs to different org
	otherOrgID := createTestOrg(t, db, "other-owner-export-004")
	scenario := createTestScenarioForOrg(t, db, otherOrgID, "other-org-export")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ============================================================================
// ListGroupAvailableScenarios — GET /groups/:groupId/scenarios
// ============================================================================

func TestListGroupAvailableScenarios_ReturnsCombinedWithSource(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-list-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	// Create org-level scenario (assigned at org level)
	orgScenario := createTestScenarioForOrg(t, db, orgID, "org-level-scenario")
	createScenarioAssignment(t, db, orgScenario.ID, nil, &orgID, "org")

	// Create group-level scenario (assigned directly to the group)
	groupScenario := createTestScenarioNoOrg(t, db, "group-level-scenario")
	createScenarioAssignment(t, db, groupScenario.ID, &groupID, nil, "group")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 2, "should return both org-level and group-level scenarios")

	// Check that each has a "source" field
	sources := make(map[string]bool)
	for _, s := range response {
		source, ok := s["source"].(string)
		require.True(t, ok, "each scenario should have a 'source' field")
		sources[source] = true
	}
	assert.True(t, sources["org"], "should have a scenario with source 'org'")
	assert.True(t, sources["group"], "should have a scenario with source 'group'")
}

func TestListGroupAvailableScenarios_OrgScenarioHasOrgSource(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-list-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	orgScenario := createTestScenarioForOrg(t, db, orgID, "org-only")
	createScenarioAssignment(t, db, orgScenario.ID, nil, &orgID, "org")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "org", response[0]["source"])
}

func TestListGroupAvailableScenarios_GroupScenarioHasGroupSource(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-list-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	groupScenario := createTestScenarioNoOrg(t, db, "group-only")
	createScenarioAssignment(t, db, groupScenario.ID, &groupID, nil, "group")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "group", response[0]["source"])
}

func TestListGroupAvailableScenarios_Forbidden_NonTeacher(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-list-004"
	memberID := "group-member-list-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, groupID, memberID, groupModels.GroupMemberRoleMember)

	router := setupOrgTestRouterWithUserAndRoles(db, memberID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListGroupAvailableScenarios_FiltersAlreadyAssigned(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-list-005"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	// Create 2 org-level scenarios
	s1 := createTestScenarioForOrg(t, db, orgID, "org-scenario-available")
	createScenarioAssignment(t, db, s1.ID, nil, &orgID, "org")

	s2 := createTestScenarioForOrg(t, db, orgID, "org-scenario-already-assigned")
	createScenarioAssignment(t, db, s2.ID, nil, &orgID, "org")

	// Also assign s2 directly to the group — it should still show with source "group"
	// or be deduplicated. The key behavior is that org-level scenarios already assigned
	// to the group show with source "group" rather than "org"
	createScenarioAssignment(t, db, s2.ID, &groupID, nil, "group")

	router := setupOrgTestRouterWithUserAndRoles(db, ownerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/groups/"+groupID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// s1 should appear as "org" source, s2 should appear as "group" source (direct assignment takes precedence)
	// We just verify we get exactly 2 results (no duplicates)
	assert.Len(t, response, 2, "should not duplicate scenarios that exist at both levels")
}
