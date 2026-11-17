package scenarios_test

// Tests for the OrgListScenarios authorization gap (issue #241).
//
// Security bug: GET /organizations/:id/scenarios queries WHERE organization_id = ?
// directly from the URL param without verifying the caller is a member of that org.
// Any authenticated user could enumerate scenarios from any org by guessing UUIDs.
//
// Fixed by an in-handler membership check on top of the Layer 2 OrgRole gate.
// These tests pin both guards, each on its own router:
//  1. A total outsider (no org membership at all) is denied (403)
//  2. A manager of org A is denied when listing org B's scenarios (403)
//  3. Only a manager+ of the requested org may list its scenarios (200)

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	scenarioController "soli/formations/src/scenarios/routes"

	"gorm.io/gorm"
)

// setupControllerOnlyRouter creates a router that exposes OrgListScenarios WITHOUT
// Layer 2 enforcement. This simulates the raw controller behaviour to prove that
// the handler itself has no in-handler membership check.
func setupControllerOnlyRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"Member"})
		c.Next()
	})
	// NOTE: No Layer2Enforcement middleware — testing the raw controller behaviour
	managementController := scenarioController.NewScenarioManagementController(db)
	api.GET("/organizations/:id/scenarios", managementController.OrgListScenarios)
	api.GET("/groups/:groupId/scenarios/:scenarioId/export", managementController.GroupExportScenario)
	return r
}

// setupLayer2Router creates a router WITH Layer 2 enforcement properly wired.
// This is the expected production configuration.
func setupLayer2Router(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
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

	managementController := scenarioController.NewScenarioManagementController(db)
	orgScenarios := api.Group("/organizations/:id/scenarios")
	orgScenarios.GET("", managementController.OrgListScenarios)
	return r
}

// ============================================================================
// TestOrgListScenarios_NonMember_ShouldBeDenied
//
// A user who has NO membership record for org X must NOT be able to list
// org X's scenarios: both the in-handler check and Layer 2 return 403.
// ============================================================================

// TestOrgListScenarios_NonMember_RawController_ShouldNotReturn200
// Verifies that the in-handler membership check (fix for issue #241) denies
// non-members even when Layer 2 middleware is not present.
func TestOrgListScenarios_NonMember_RawController_ShouldNotReturn200(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "auth-owner-001"
	outsiderID := "auth-outsider-001"

	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgID, "secret-scenario-001")

	// Outsider has NO membership in this org
	router := setupControllerOnlyRouter(db, outsiderID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	// After the in-handler fix, the raw controller must return 403 to non-members.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"the in-handler membership check must deny a non-member outsider even without Layer 2")
}

// TestOrgListScenarios_NonMember_ShouldBeDenied
// An outsider must get 403 from the fully-wired (Layer 2) stack.
func TestOrgListScenarios_NonMember_ShouldBeDenied(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "auth-owner-002"
	outsiderID := "auth-outsider-002"

	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgID, "secret-scenario-002")

	// Outsider has NO membership in this org
	router := setupLayer2Router(t, db, outsiderID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a user with no org membership must be denied access to org scenarios")
}

// ============================================================================
// TestOrgListScenarios_Member_ShouldSucceed
//
// A user who IS a manager of org X can list org X's scenarios (baseline).
// ============================================================================

func TestOrgListScenarios_Member_ShouldSucceed(t *testing.T) {
	db := freshTestDB(t)
	managerID := "auth-manager-003"

	orgID := createTestOrg(t, db, managerID)
	addOrgMember(t, db, orgID, managerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgID, "visible-scenario-003")

	router := setupLayer2Router(t, db, managerID, []string{"Member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"an org manager must be able to list org scenarios")

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 1, "manager should see the org scenario")
}

// ============================================================================
// TestOrgListScenarios_DifferentOrg_ShouldNotLeakData
//
// A user who is a manager of org A must NOT be able to list org B's scenarios
// by simply changing the URL param. This is the cross-org enumeration vector.
// ============================================================================

// TestOrgListScenarios_DifferentOrg_RawController_LeaksData
// Verifies that the in-handler membership check (fix for issue #241) denies
// cross-org access even when Layer 2 middleware is not present.
func TestOrgListScenarios_DifferentOrg_RawController_LeaksData(t *testing.T) {
	db := freshTestDB(t)
	orgAOwnerID := "auth-owner-a-004"
	orgBOwnerID := "auth-owner-b-004"

	orgAID := createTestOrg(t, db, orgAOwnerID)
	addOrgMember(t, db, orgAID, orgAOwnerID, orgModels.OrgRoleOwner)

	orgBID := createTestOrg(t, db, orgBOwnerID)
	addOrgMember(t, db, orgBID, orgBOwnerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgBID, "org-b-secret-scenario-004")

	// orgAOwner is only a member of org A, not org B
	router := setupControllerOnlyRouter(db, orgAOwnerID)

	w := httptest.NewRecorder()
	// orgAOwner requests org B's scenarios via URL manipulation
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgBID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	// After the in-handler fix, the raw controller must return 403 for cross-org access.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"the in-handler membership check must deny access to org B's scenarios for an org A manager")
}

// TestOrgListScenarios_DifferentOrg_ShouldNotLeakData
// With Layer 2 enforcement, a manager of org A must get 403 when targeting org B.
func TestOrgListScenarios_DifferentOrg_ShouldNotLeakData(t *testing.T) {
	db := freshTestDB(t)
	orgAOwnerID := "auth-owner-a-005"
	orgBOwnerID := "auth-owner-b-005"

	orgAID := createTestOrg(t, db, orgAOwnerID)
	addOrgMember(t, db, orgAID, orgAOwnerID, orgModels.OrgRoleOwner)

	orgBID := createTestOrg(t, db, orgBOwnerID)
	addOrgMember(t, db, orgBID, orgBOwnerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgBID, "org-b-secret-scenario-005")

	// orgAOwner is only a manager of org A
	router := setupLayer2Router(t, db, orgAOwnerID, []string{"Member"})

	w := httptest.NewRecorder()
	// orgAOwner tries to access org B's scenarios
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgBID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a manager of org A must be denied access to org B's scenarios")
}

// TestOrgListScenarios_NoLayer2_Manager_ShouldSucceed
// An org manager can list their org's scenarios through the raw controller:
// the in-handler check lets managers through.
func TestOrgListScenarios_NoLayer2_Manager_ShouldSucceed(t *testing.T) {
	db := freshTestDB(t)
	managerID := "auth-manager-008"

	orgID := createTestOrg(t, db, managerID)
	addOrgMember(t, db, orgID, managerID, orgModels.OrgRoleOwner)
	createTestScenarioForOrg(t, db, orgID, "manager-scenario-008")

	// Raw controller — no Layer 2
	router := setupControllerOnlyRouter(db, managerID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/"+orgID.String()+"/scenarios", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"an org manager must be able to list their org's scenarios via the controller itself")

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response, 1, "manager should see the org scenario")
}
