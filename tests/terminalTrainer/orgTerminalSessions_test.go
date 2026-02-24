package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		t.Logf("Response: %s", w.Body.String())
		assert.NotEqual(t, "Invalid organization ID", resp["error_message"],
			"Handler is reading the wrong route parameter name (orgId instead of id)")
	}
}

// TestOrgTerminalSessions_ValidOrgID_NotBadRequest verifies that passing a valid
// UUID as the :id parameter does not result in a "Invalid organization ID" error.
// This confirms the handler correctly reads the route param.
func TestOrgTerminalSessions_ValidOrgID_NotBadRequest(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	orgID := uuid.New()

	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", "some-user")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})

	// Route uses :id (correct convention)
	router.GET("/organizations/:id/terminal-sessions", controller.GetOrganizationTerminalSessions)

	req := httptest.NewRequest("GET", "/organizations/"+orgID.String()+"/terminal-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should NOT be 400 — the UUID is valid, so param parsing should succeed.
	// May get 403 (not a member) or 200, but never 400 "Invalid organization ID".
	assert.NotEqual(t, http.StatusBadRequest, w.Code,
		"Valid UUID passed as :id should not produce 400 — handler may be reading wrong param name")
}
