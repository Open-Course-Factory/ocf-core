package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	orgModels "soli/formations/src/organizations/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestOrgTerminalSessions_RouteParamConsistency verifies that the
// GetOrganizationTerminalSessions handler reads the organization ID from a
// route parameter named "id" (not "orgId"), ensuring consistency with every
// other route under /api/v1/organizations/:id/*.
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

	// A handler reading ctx.Param("orgId") gets an empty id and answers 400;
	// the owner must get the list.
	assert.Equal(t, http.StatusOK, w.Code)
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
}
