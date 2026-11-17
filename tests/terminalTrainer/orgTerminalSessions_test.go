package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestOrgTerminalSessions_RouteParamConsistency verifies that the
// GetOrganizationTerminalSessions handler reads the organization ID from a
// route parameter named "id" (not "orgId"), ensuring consistency with every
// other route under /api/v1/organizations/:id/*.
//
// This test is expected to FAIL (red phase) while the handler still reads
// ctx.Param("orgId"), because the test registers the route with ":id".
func TestOrgTerminalSessions_RouteParamConsistency(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create an organization with an owner
	org := createTestOrgForHistory(t, db, "trainer1")
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleOwner)

	// Create a terminal associated with the org
	createTestTerminalWithOrg(t, db, "student1", &org.ID)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", "trainer1")
		c.Set("userRoles", []string{"trainer"})
		c.Next()
	})

	// Register the route with :id — the convention used by all other org routes
	router.GET("/organizations/:id/terminal-sessions", controller.GetOrganizationTerminalSessions)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// If the handler reads ctx.Param("orgId") instead of ctx.Param("id"),
	// the orgID will be empty → uuid.Parse("") fails → 400 Bad Request.
	// We expect anything BUT a 400 "Invalid organization ID" error.
	assert.NotEqual(t, http.StatusBadRequest, w.Code,
		"Handler should read :id param, not :orgId — got 400 which means param was empty")

	if w.Code == http.StatusBadRequest {
		var resp map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		t.Logf("Response: %s", w.Body.String())
		assert.NotEqual(t, "Invalid organization ID", resp["error_message"],
			"Handler is reading the wrong route parameter name (orgId instead of id)")
	}
}

// A plain member of the organization may not list everyone's sessions.
func TestOrgTerminalSessions_PlainMember_Forbidden(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "trainer1")
	createTestOrgMember(t, db, org.ID, "trainer1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)
	createTestTerminalWithOrg(t, db, "trainer1", &org.ID)

	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "student1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-sessions", controller.GetOrganizationTerminalSessions)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), "sessions")
}
