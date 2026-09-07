// tests/scenarios/managementInHandlerChecks_test.go
//
// #295: defense in depth on two management handlers. Both routes are gated by
// Layer 2 today (GroupRole / OrgRole, MinRole manager), so these tests mount
// the RAW controller without that middleware: they pin what the handler
// refuses on its own, should a refactor weaken Layer 2 or remount the route.
//
//  1. GroupExportScenario verified the assignment but never asked whether the
//     caller may manage the scenario; every other export runs CanManageScenario.
//  2. OrgListScenarios accepted any active membership, while only managers may
//     list; the in-handler rule read wider than the real one.
package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

// setupRawManagementRouter mounts the two handlers with the caller injected
// and NO Layer 2 middleware.
func setupRawManagementRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	ctrl := scenarioController.NewScenarioManagementController(db)
	api.GET("/organizations/:id/scenarios", ctrl.OrgListScenarios)
	api.GET("/groups/:groupId/scenarios/:scenarioId/export", ctrl.GroupExportScenario)
	return r
}

func getRaw(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(w, req)
	return w
}

// --- GroupExportScenario ------------------------------------------------------

func TestGroupExportScenario_RawController_PlainGroupMember_Forbidden(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner-295")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner-295")
	addGroupMember(t, db, groupID, "plain-student-295", groupModels.GroupMemberRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "group-export-295")
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")

	router := setupRawManagementRouter(db, "plain-student-295", []string{"member"})
	w := getRaw(router, "/api/v1/groups/"+groupID.String()+"/scenarios/"+scenario.ID.String()+"/export")

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a plain group member must not bulk-export the scenario even without Layer 2. Body: %s", w.Body.String())
}

func TestGroupExportScenario_RawController_GroupManager_Allowed(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner-295")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner-295")
	addGroupMember(t, db, groupID, "group-manager-295", groupModels.GroupMemberRoleManager)
	scenario := createTestScenarioForOrg(t, db, orgID, "group-export-ok-295")
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")

	router := setupRawManagementRouter(db, "group-manager-295", []string{"member"})
	w := getRaw(router, "/api/v1/groups/"+groupID.String()+"/scenarios/"+scenario.ID.String()+"/export")

	assert.Equal(t, http.StatusOK, w.Code, "Body: %s", w.Body.String())
}

// --- OrgListScenarios ---------------------------------------------------------

func TestOrgListScenarios_RawController_PlainActiveMember_Forbidden(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner-295")
	addOrgMember(t, db, orgID, "org-owner-295", orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, "plain-member-295", orgModels.OrgRoleMember)
	createTestScenarioForOrg(t, db, orgID, "org-list-295")

	router := setupRawManagementRouter(db, "plain-member-295", []string{"member"})
	w := getRaw(router, "/api/v1/organizations/"+orgID.String()+"/scenarios")

	assert.Equal(t, http.StatusForbidden, w.Code,
		"only a manager may list the org's scenarios; the in-handler rule must say the same as Layer 2. Body: %s", w.Body.String())
}

func TestOrgListScenarios_RawController_Manager_Allowed(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "org-owner-295")
	addOrgMember(t, db, orgID, "org-owner-295", orgModels.OrgRoleOwner)
	addOrgMember(t, db, orgID, "org-manager-295", orgModels.OrgRoleManager)
	createTestScenarioForOrg(t, db, orgID, "org-list-ok-295")

	router := setupRawManagementRouter(db, "org-manager-295", []string{"member"})
	w := getRaw(router, "/api/v1/organizations/"+orgID.String()+"/scenarios")

	assert.Equal(t, http.StatusOK, w.Code, "Body: %s", w.Body.String())
}
