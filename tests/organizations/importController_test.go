package organizations_tests

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	orgController "soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
)

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

// A platform administrator passes Layer 2 for any org id, so a missing org must be a 404, not a 500.
func TestImportController_MissingOrganization_NotFound(t *testing.T) {
	db := newOffboardingDB(t)
	ctrl := orgController.NewOrganizationController(services.NewOrganizationService(db), &recordingImportService{}, nil, nil, db)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "admin-1"); c.Next() })
	router.POST("/organizations/:id/import", ctrl.ImportOrganizationData)

	rec := postImport(t, router, uuid.New(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
