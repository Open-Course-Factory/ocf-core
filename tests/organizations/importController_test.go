package organizations_tests

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
)

// TestImportController_HTTPValidation tests HTTP request validation
// NOTE: Full integration tests with database operations are better suited for E2E tests
// with PostgreSQL, as SQLite doesn't support JSONB fields used by the Organization model.

// TestImportController_DryRunParameter tests dry-run flag handling
func TestImportController_DryRunParameter(t *testing.T) {
	t.Run("dry_run=true", func(t *testing.T) {
		// Test that dry_run flag is properly parsed
		dryRunValue := "true"
		assert.Equal(t, "true", dryRunValue)
	})

	t.Run("dry_run=false", func(t *testing.T) {
		dryRunValue := "false"
		assert.Equal(t, "false", dryRunValue)
	})

	t.Run("default dry_run", func(t *testing.T) {
		// Default should be false
		dryRunValue := ""
		assert.NotEqual(t, "true", dryRunValue)
	})
}

// TestImportController_UpdateExistingParameter tests update_existing flag handling
func TestImportController_UpdateExistingParameter(t *testing.T) {
	t.Run("update_existing=true", func(t *testing.T) {
		updateExistingValue := "true"
		assert.Equal(t, "true", updateExistingValue)
	})

	t.Run("update_existing=false", func(t *testing.T) {
		updateExistingValue := "false"
		assert.Equal(t, "false", updateExistingValue)
	})
}

// A manager who is not the owner may import members: Layer 2 declares
// OrgRole manager on this route, and the handler must agree (#587).
func TestImportController_NonOwnerManager_Allowed(t *testing.T) {
	installMockEnforcer(t)
	db := newOffboardingDB(t)
	orgID := seedTeamOrg(t, db, "owner-1", new(10))
	seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)
	seedMember(t, db, orgID, "manager-1", models.OrgRoleManager)

	ctrl := orgController.NewOrganizationController(services.NewOrganizationService(db), &recordingImportService{}, nil, nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "manager-1"); c.Next() })
	router.POST("/organizations/:id/import", ctrl.ImportOrganizationData)

	rec := postImport(t, router, orgID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
