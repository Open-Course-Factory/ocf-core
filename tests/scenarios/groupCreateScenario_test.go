package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
)

// ============================================================================
// GroupCreateScenario — POST /groups/:groupId/scenarios
//
// Mirrors OrgCreateScenario but is scoped to a group: the organization_id is
// derived from the group, and a ScenarioAssignment is auto-created so the
// new scenario is immediately visible to the group.
// ============================================================================

func TestGroupCreateScenario_Success(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-create-001"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	payload := map[string]any{
		"name":          "group-blank-scenario",
		"title":         "Group Blank Scenario",
		"instance_type": "ubuntu:22.04",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/groups/"+groupID.String()+"/scenarios", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Group Blank Scenario", response["title"])
	assert.Equal(t, orgID.String(), response["organization_id"], "scenario should inherit the group's org id")

	scenarioIDStr, _ := response["id"].(string)
	require.NotEmpty(t, scenarioIDStr, "response should contain the new scenario id")
	scenarioID, err := uuid.Parse(scenarioIDStr)
	require.NoError(t, err)

	// Verify ScenarioAssignment was auto-created for the group
	var assignment models.ScenarioAssignment
	err = db.Where("scenario_id = ? AND group_id = ?", scenarioID, groupID).First(&assignment).Error
	require.NoError(t, err, "expected an auto-created group ScenarioAssignment")
	assert.Equal(t, "group", assignment.Scope)
	assert.True(t, assignment.IsActive)
	assert.Equal(t, ownerID, assignment.CreatedByID)
}

func TestGroupCreateScenario_ManagerCanCreate(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-create-002"
	managerID := "group-manager-create-002"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, groupID, managerID, groupModels.GroupMemberRoleManager)

	router := setupOrgTestRouterWithUserAndRoles(t, db, managerID, []string{"Member"})

	payload := map[string]any{
		"name":          "manager-blank",
		"title":         "Manager-Created Scenario",
		"instance_type": "ubuntu:22.04",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/groups/"+groupID.String()+"/scenarios", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
}

func TestGroupCreateScenario_Forbidden_NonManager(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-create-003"
	memberID := "group-member-create-003"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, memberID, orgModels.OrgRoleMember)

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, groupID, memberID, groupModels.GroupMemberRoleMember)

	router := setupOrgTestRouterWithUserAndRoles(t, db, memberID, []string{"Member"})

	payload := map[string]any{
		"name":          "blocked",
		"title":         "Blocked",
		"instance_type": "ubuntu:22.04",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/groups/"+groupID.String()+"/scenarios", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	// Verify no scenario was created
	var scenarioCount int64
	db.Model(&models.Scenario{}).Count(&scenarioCount)
	assert.Equal(t, int64(0), scenarioCount, "no scenario should have been created")
}

func TestGroupCreateScenario_BodyOrgIDIgnored(t *testing.T) {
	// Even if a caller tries to retarget another org via the body, the path
	// (group → group.OrganizationID) is the source of truth.
	db := freshTestDB(t)
	ownerID := "group-owner-create-004"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	otherOrgID := createTestOrg(t, db, "other-owner-004")

	groupID := createTestGroupInOrg(t, db, orgID, ownerID)
	addGroupMember(t, db, groupID, ownerID, groupModels.GroupMemberRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	payload := map[string]any{
		"name":            "retarget-attempt",
		"title":           "Retarget Attempt",
		"instance_type":   "ubuntu:22.04",
		"organization_id": otherOrgID.String(),
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/groups/"+groupID.String()+"/scenarios", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, orgID.String(), response["organization_id"], "must use the group's org, not the body's")
	assert.NotEqual(t, otherOrgID.String(), response["organization_id"])
}

func TestGroupCreateScenario_BadRequest_InvalidGroupID(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "group-owner-create-005"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)

	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	payload := map[string]any{
		"name":          "x",
		"title":         "x",
		"instance_type": "ubuntu:22.04",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/groups/not-a-uuid/scenarios", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Layer 2 GroupRole enforcer rejects non-existent/invalid groups before
	// the controller can return 400 — so 403 is returned.
	assert.Equal(t, http.StatusForbidden, w.Code)
}
