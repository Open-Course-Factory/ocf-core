package organizations_tests

// Tests for GET /organizations/me/memberships — returns the authenticated
// user's organization memberships with their role per org. The frontend uses
// this to decide which "Create Scenario" endpoint to call (admin/org/group)
// without N round-trips per org.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/organizations/controller"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMyMembershipsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Organization{}, &models.OrganizationMember{}))
	return db
}

// setupMyMembershipsRouter wires the controller and injects a fixed userId.
func setupMyMembershipsRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	orgService := services.NewOrganizationService(db)
	importService := services.NewImportService(db)
	orgCtrl := controller.NewOrganizationController(orgService, importService, db)

	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})
	api.GET("/organizations/me/memberships", orgCtrl.GetMyOrganizationMemberships)
	return r
}

func seedOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role models.OrganizationMemberRole, active bool) {
	t.Helper()
	m := &models.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       active,
	}
	require.NoError(t, db.Omit("Metadata").Create(m).Error)
	if !active {
		// GORM applies the column default (true) when the zero value (false)
		// is passed at create time. Override with an explicit update.
		require.NoError(t, db.Model(m).Update("is_active", false).Error)
	}
}

func TestGetMyOrganizationMemberships_ReturnsAllRoles(t *testing.T) {
	db := setupMyMembershipsDB(t)
	userID := "user-multi-orgs"

	org1, org2, org3 := uuid.New(), uuid.New(), uuid.New()
	seedOrgMember(t, db, org1, userID, models.OrgRoleOwner, true)
	seedOrgMember(t, db, org2, userID, models.OrgRoleManager, true)
	seedOrgMember(t, db, org3, userID, models.OrgRoleMember, true)

	// Membership for another user — must NOT be returned
	seedOrgMember(t, db, org1, "other-user", models.OrgRoleManager, true)

	router := setupMyMembershipsRouter(db, userID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 3, "should return only the 3 memberships of the authenticated user")

	// Build a map of orgID → role for assertion
	roleByOrg := make(map[string]string, len(response))
	for _, m := range response {
		roleByOrg[m["organization_id"].(string)] = m["role"].(string)
	}
	assert.Equal(t, "owner", roleByOrg[org1.String()])
	assert.Equal(t, "manager", roleByOrg[org2.String()])
	assert.Equal(t, "member", roleByOrg[org3.String()])
}

func TestGetMyOrganizationMemberships_ExcludesInactive(t *testing.T) {
	db := setupMyMembershipsDB(t)
	userID := "user-mixed-active"

	orgActive, orgInactive := uuid.New(), uuid.New()
	seedOrgMember(t, db, orgActive, userID, models.OrgRoleManager, true)
	seedOrgMember(t, db, orgInactive, userID, models.OrgRoleOwner, false)

	router := setupMyMembershipsRouter(db, userID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1, "inactive memberships must be excluded")
	assert.Equal(t, orgActive.String(), response[0]["organization_id"])
}

func TestGetMyOrganizationMemberships_EmptyForNonMember(t *testing.T) {
	db := setupMyMembershipsDB(t)
	router := setupMyMembershipsRouter(db, "user-with-no-orgs")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/me/memberships", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Len(t, response, 0)
}

func TestGetMyOrganizationMemberships_UnauthenticatedReturns401(t *testing.T) {
	db := setupMyMembershipsDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	orgService := services.NewOrganizationService(db)
	importService := services.NewImportService(db)
	orgCtrl := controller.NewOrganizationController(orgService, importService, db)

	api := r.Group("/api/v1")
	// Note: no middleware setting userId
	api.GET("/organizations/me/memberships", orgCtrl.GetMyOrganizationMemberships)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/organizations/me/memberships", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
