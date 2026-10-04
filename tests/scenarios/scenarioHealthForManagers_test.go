package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

func setupScenarioHealthRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	scenarioController.RegisterScenarioPermissions(mocks.NewMockEnforcer())
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
	api.GET("/scenarios/:id/health", scenarioController.NewScenarioController(db).GetOneScenarioHealth)
	return r
}

func getScenarioHealth(t *testing.T, db *gorm.DB, userID string, scenarioID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenarios/"+scenarioID.String()+"/health", nil)
	setupScenarioHealthRouter(t, db, userID, []string{"member"}).ServeHTTP(w, req)
	return w
}

// A teacher sees what is wrong with their own scenario — the same findings
// the platform operators' page lists — without needing the operator role.
func TestScenarioHealthForOne_ManagerSeesFindings_OthersAreRefused(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "health-org-owner")
	addOrgMember(t, db, orgID, "health-manager", orgModels.OrgRoleManager)
	// One step without a verify script: a warning finding.
	scenario := createPlatformScenario(t, db, "health-one", "health-author", &orgID)

	class := groupModels.ClassGroup{Name: "health-class", DisplayName: "Class", OwnerUserID: "health-org-owner", OrganizationID: &orgID}
	require.NoError(t, db.Omit("Metadata").Create(&class).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: class.ID, UserID: "health-learner", Role: groupModels.GroupMemberRoleMember, IsActive: true, JoinedAt: time.Now(),
	}).Error)

	otherOrg := createTestOrg(t, db, "health-other-owner")
	addOrgMember(t, db, otherOrg, "health-other-manager", orgModels.OrgRoleManager)

	w := getScenarioHealth(t, db, "health-manager", scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var health services.ScenarioHealth
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &health))
	assert.Equal(t, scenario.ID.String(), health.ScenarioID)
	require.NotEmpty(t, health.Findings)
	assert.Equal(t, services.HealthNoVerification, health.Findings[0].Code)
	assert.Equal(t, services.HealthWarning, health.Findings[0].Severity)

	assert.Equal(t, http.StatusForbidden, getScenarioHealth(t, db, "health-learner", scenario.ID).Code)
	assert.Equal(t, http.StatusForbidden, getScenarioHealth(t, db, "health-other-manager", scenario.ID).Code)
}

// A healthy scenario answers with an empty findings list, never null.
func TestScenarioHealthForOne_HealthyScenario_HasEmptyFindings(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "healthy-org-owner")
	addOrgMember(t, db, orgID, "healthy-manager", orgModels.OrgRoleManager)
	scenario := createPlatformScenario(t, db, "healthy-one", "healthy-author", &orgID)
	require.NoError(t, db.Exec("UPDATE scenario_steps SET verify_script = 'exit 0' WHERE scenario_id = ?", scenario.ID).Error)

	w := getScenarioHealth(t, db, "healthy-manager", scenario.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"findings":[]`)
}
