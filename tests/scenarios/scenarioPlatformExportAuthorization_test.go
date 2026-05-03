package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"

	"gorm.io/gorm"
)

// setupPlatformExportTestRouter mounts ONLY the two platform-level scenario
// export routes (GET /scenarios/:id/export and POST /scenarios/export) behind
// Layer-2 enforcement. It deliberately skips the AuthManagement (Casbin)
// middleware: the Layer-1 contract is covered by
// tests/authorization/scenario_export_permissions_test.go, while these tests
// focus on the controller-level canManageScenario gate that the fix introduces.
func setupPlatformExportTestRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mockEnforcer)
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

	controller := scenarioController.NewScenarioController(db)
	api.GET("/scenarios/:id/export", controller.ExportScenario)
	api.POST("/scenarios/export", controller.ExportScenarios)

	return r
}

// createPlatformScenario inserts a scenario row owned by `creatorID` and
// optionally scoped to `orgID` (nil = no org).
func createPlatformScenario(t *testing.T, db *gorm.DB, name, creatorID string, orgID *uuid.UUID) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name:           name,
		Title:          "Test: " + name,
		Description:    "Platform export authorization test",
		InstanceType:   "ubuntu:22.04",
		SourceType:     "seed",
		CreatedByID:    creatorID,
		OrganizationID: orgID,
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Do step 1"},
		},
	}
	require.NoError(t, db.Create(scenario).Error)
	return scenario
}

// ============================================================================
// GET /api/v1/scenarios/:id/export
// ============================================================================

// TestPlatformExport_AsCreatorMember_Allowed — a Member who created the
// scenario must be able to export it from the platform-level endpoint. Today
// this fails with 403 "Administrator role required" because the route is
// gated as AdminOnly at Layer 2.
func TestPlatformExport_AsCreatorMember_Allowed(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-member-export-001"

	scenario := createPlatformScenario(t, db, "creator-owned", creatorID, nil)

	router := setupPlatformExportTestRouter(t, db, creatorID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"creator should be able to export their own scenario from the platform endpoint")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Test: creator-owned", response["title"])
}

// TestPlatformExport_AsOrgManager_Allowed — an org manager must be able to
// export a scenario belonging to their organization, even if they did not
// create it.
func TestPlatformExport_AsOrgManager_Allowed(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "org-owner-platform-export-002"
	managerID := "org-manager-platform-export-002"
	otherCreatorID := "other-creator-platform-export-002"

	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, managerID, orgModels.OrgRoleManager)

	scenario := createPlatformScenario(t, db, "org-scenario", otherCreatorID, &orgID)

	router := setupPlatformExportTestRouter(t, db, managerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"org manager should be able to export scenarios from their organization "+
			"via the platform endpoint")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Test: org-scenario", response["title"])
}

// TestPlatformExport_AsGroupManager_Allowed — a group manager must be able to
// export a scenario assigned to their group, even when they did not create it
// and the scenario has no org scope.
func TestPlatformExport_AsGroupManager_Allowed(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-platform-export-003"
	managerID := "group-manager-platform-export-003"
	otherCreatorID := "other-creator-platform-export-003"

	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, groupID, managerID, groupModels.GroupMemberRoleManager)

	scenario := createPlatformScenario(t, db, "group-scenario", otherCreatorID, nil)
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")

	router := setupPlatformExportTestRouter(t, db, managerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"group manager should be able to export scenarios assigned to their group "+
			"via the platform endpoint")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Test: group-scenario", response["title"])
}

// TestPlatformExport_AsUnrelatedMember_Forbidden — a Member with no
// relationship to the scenario (not creator, not in its org, not in any
// group it's assigned to) must NOT be able to export it. Regression guard:
// must hold both today (AdminOnly rejects all non-admins) and after the fix
// (canManageScenario rejects unrelated users).
func TestPlatformExport_AsUnrelatedMember_Forbidden(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-platform-export-004"
	strangerID := "stranger-platform-export-004"

	scenario := createPlatformScenario(t, db, "private-scenario", creatorID, nil)

	router := setupPlatformExportTestRouter(t, db, strangerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"unrelated member must NOT be able to export an arbitrary scenario")
}

// TestPlatformExport_AsAdmin_Allowed — platform administrator must always be
// able to export any scenario. Regression guard: must hold today (AdminOnly
// admin bypass) and after the fix (canManageScenario admin bypass).
func TestPlatformExport_AsAdmin_Allowed(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-platform-export-005"
	adminID := "platform-admin-platform-export-005"

	scenario := createPlatformScenario(t, db, "admin-can-export", creatorID, nil)

	router := setupPlatformExportTestRouter(t, db, adminID, []string{"Administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenarios/"+scenario.ID.String()+"/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"platform administrator must always be able to export any scenario")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Test: admin-can-export", response["title"])
}

// ============================================================================
// POST /api/v1/scenarios/export (bulk)
// ============================================================================

// TestPlatformExportBulk_AsCreatorMember_OnlyOwnScenarios — a Member who
// passes a list mixing scenarios they manage and scenarios they do NOT manage
// must get a 403 (whole request rejected). The fix should iterate the input
// IDs and reject if ANY scenario is not manageable by the user.
func TestPlatformExportBulk_AsCreatorMember_OnlyOwnScenarios(t *testing.T) {
	db := freshTestDB(t)
	creatorID := "creator-bulk-export-006"
	otherCreatorID := "other-creator-bulk-export-006"

	ownScenario := createPlatformScenario(t, db, "own-scenario", creatorID, nil)
	foreignScenario := createPlatformScenario(t, db, "foreign-scenario", otherCreatorID, nil)

	router := setupPlatformExportTestRouter(t, db, creatorID, []string{"Member"})

	payload := map[string]any{
		"ids": []string{ownScenario.ID.String(), foreignScenario.ID.String()},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/export", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"bulk export must reject the whole request when the caller cannot manage "+
			"every scenario in the list")
}
