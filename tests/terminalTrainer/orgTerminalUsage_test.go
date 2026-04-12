package terminalTrainer_tests

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

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/models"
)

// makeOrgUsageRequest builds a Gin router and sends GET /organizations/:id/terminal-usage.
func makeOrgUsageRequest(t *testing.T, orgIDStr string, userID string, userRoles []string) *httptest.ResponseRecorder {
	t.Helper()
	db := setupTestDBWithOrgs(t)

	ctrl := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+orgIDStr+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- Authorization tests ---

// TestGetOrgTerminalUsage_InvalidOrgID returns 400 for a non-UUID path param.
func TestGetOrgTerminalUsage_InvalidOrgID(t *testing.T) {
	w := makeOrgUsageRequest(t, "not-a-uuid", "any-user", []string{"member"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetOrgTerminalUsage_MemberDenied verifies that a regular org member gets 403.
func TestGetOrgTerminalUsage_MemberDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "regular-member", orgModels.OrgRoleMember)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "regular-member")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestGetOrgTerminalUsage_NonMemberDenied verifies that a user not in the org gets 403.
func TestGetOrgTerminalUsage_NonMemberDenied(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "outsider")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestGetOrgTerminalUsage_ManagerAllowed verifies that an org manager gets a 200 response.
func TestGetOrgTerminalUsage_ManagerAllowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "manager1", orgModels.OrgRoleManager)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "manager1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Manager should get 200")
}

// TestGetOrgTerminalUsage_OwnerAllowed verifies that an org owner gets a 200 response.
func TestGetOrgTerminalUsage_OwnerAllowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Owner should get 200")
}

// TestGetOrgTerminalUsage_AdminAllowed verifies that a platform admin gets 200
// even without being an org member.
func TestGetOrgTerminalUsage_AdminAllowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	// admin is not added as org member — platform admin bypasses org membership check
	createTestOrgMember(t, db, org.ID, "admin1", orgModels.OrgRoleOwner)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "admin1")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Platform admin should get 200")
}

// --- Response shape tests ---

// TestGetOrgTerminalUsage_ResponseShape verifies the JSON response contains the expected fields.
func TestGetOrgTerminalUsage_ResponseShape(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "Response should be valid JSON")

	assert.Equal(t, org.ID.String(), resp["organization_id"], "organization_id must match")
	_, hasActive := resp["active_terminals"]
	assert.True(t, hasActive, "Response must contain active_terminals")
	_, hasMax := resp["max_terminals"]
	assert.True(t, hasMax, "Response must contain max_terminals")
	_, hasPlan := resp["plan_name"]
	assert.True(t, hasPlan, "Response must contain plan_name")
	_, hasFallback := resp["is_fallback"]
	assert.True(t, hasFallback, "Response must contain is_fallback")
	_, hasUsers := resp["users"]
	assert.True(t, hasUsers, "Response must contain users")
}

// TestGetOrgTerminalUsage_ActiveTerminalsAggregated verifies that active terminal counts
// are summed across all members and grouped by user correctly.
func TestGetOrgTerminalUsage_ActiveTerminalsAggregated(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	// Create two active terminals for student1 in the org
	userKey1, err := createTestUserKey(db, "student1")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		terminal := &models.Terminal{
			SessionID:         "active-session-" + uuid.New().String(),
			UserID:            "student1",
			Status:            "active",
			ExpiresAt:         time.Now().Add(1 * time.Hour),
			InstanceType:      "test",
			MachineSize:       "S",
			UserTerminalKeyID: userKey1.ID,
			OrganizationID:    &org.ID,
		}
		require.NoError(t, db.Create(terminal).Error)
	}

	// Create one stopped terminal for student1 in the org (should NOT be counted)
	stoppedTerminal := &models.Terminal{
		SessionID:         "stopped-session-" + uuid.New().String(),
		UserID:            "student1",
		Status:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey1.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(stoppedTerminal).Error)

	// Create an active terminal for student1 in a DIFFERENT org (should NOT be counted)
	otherOrg := createTestOrgForHistory(t, db, "other-owner")
	userKey2, err := createTestUserKey(db, "student1b")
	require.NoError(t, err)
	outsideTerminal := &models.Terminal{
		SessionID:         "outside-session-" + uuid.New().String(),
		UserID:            "student1",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey2.ID,
		OrganizationID:    &otherOrg.ID,
	}
	require.NoError(t, db.Create(outsideTerminal).Error)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Only the 2 active terminals in this org should be counted
	assert.Equal(t, float64(2), resp["active_terminals"],
		"active_terminals should count only active terminals in this org")

	// Users array should have exactly one entry (student1 with count 2)
	users, ok := resp["users"].([]interface{})
	require.True(t, ok, "users should be a JSON array")
	assert.Equal(t, 1, len(users), "There should be 1 user entry (student1)")
}

// TestGetOrgTerminalUsage_PlanLimitsFromSubscription verifies that max_terminals
// is populated from the organization's subscription plan.
func TestGetOrgTerminalUsage_PlanLimitsFromSubscription(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Create a subscription plan with a specific max_concurrent_terminals
	plan := &paymentModels.SubscriptionPlan{
		Name:                   "Pro",
		MaxConcurrentTerminals: 5,
		MaxCourses:             -1,
		IsActive:               true,
		IsCatalog:              true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create an org subscription
	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(orgSub).Error)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, float64(5), resp["max_terminals"],
		"max_terminals should come from the organization's subscription plan")
	assert.Equal(t, "Pro", resp["plan_name"],
		"plan_name should match the subscription plan name")
}
