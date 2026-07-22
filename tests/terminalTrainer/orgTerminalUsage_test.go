package terminalTrainer_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
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
	_, hasOccupying := resp["occupying_slots"]
	assert.True(t, hasOccupying, "Response must contain occupying_slots")
	_, hasPlan := resp["plan_name"]
	assert.True(t, hasPlan, "Response must contain plan_name")
	_, hasFallback := resp["is_fallback"]
	assert.True(t, hasFallback, "Response must contain is_fallback")
	_, hasUsers := resp["users"]
	assert.True(t, hasUsers, "Response must contain users")
}

// TestGetOrgTerminalUsage_ActiveTerminalsAggregated verifies that active terminal counts
// are summed across all members and grouped by user correctly.
//
// Scoping follows the canonical member-join SSOT (organization_members), the
// same predicate the enforced budget uses: terminals owned by a member of the
// org count; terminals owned by a non-member are excluded even if org-tagged.
// The "outside" terminal below is therefore owned by a NON-member so it stays
// out of this org's dashboard.
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
			State:            models.StateRunning,
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
		State:            models.StateStopped,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey1.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(stoppedTerminal).Error)

	// Create an active terminal owned by a NON-member but tagged to this org
	// (should NOT be counted: member-join excludes non-members).
	userKey2, err := createTestUserKey(db, "non-member")
	require.NoError(t, err)
	outsideTerminal := &models.Terminal{
		SessionID:         "outside-session-" + uuid.New().String(),
		UserID:            "non-member",
		State:            models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey2.ID,
		OrganizationID:    &org.ID,
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

// TestGetOrgTerminalUsage_OccupyingSlotsIncludesStopped verifies that the
// new OccupyingSlots field reports the quota-relevant count (active + stopped)
// while ActiveTerminals continues to report only running sessions. This is the
// SSOT rule documented in models.TerminalStatusesOccupyingSlot: a stopped
// session still occupies a slot until it is deleted.
func TestGetOrgTerminalUsage_OccupyingSlotsIncludesStopped(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	userKey, err := createTestUserKey(db, "student1")
	require.NoError(t, err)

	// 1 active terminal for student1
	activeTerminal := &models.Terminal{
		SessionID:         "active-session-" + uuid.New().String(),
		UserID:            "student1",
		State:            models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(activeTerminal).Error)

	// 1 stopped terminal for student1 — still occupies a slot per
	// models.OccupiesSlotScope (the SSOT rule).
	stoppedTerminal := &models.Terminal{
		SessionID:         "stopped-session-" + uuid.New().String(),
		UserID:            "student1",
		State:            models.StateStopped,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(stoppedTerminal).Error)

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

	// active_terminals = 1 (the running one)
	assert.Equal(t, float64(1), resp["active_terminals"],
		"active_terminals reports running-only sessions")
	// occupying_slots = 2 (active + stopped) — matches the quota rule
	assert.Equal(t, float64(2), resp["occupying_slots"],
		"occupying_slots must include stopped sessions per OccupiesSlotScope")

	// Per-user breakdown carries both counts as well.
	users, ok := resp["users"].([]interface{})
	require.True(t, ok, "users should be a JSON array")
	require.Equal(t, 1, len(users), "There should be 1 user entry (student1)")
	userEntry := users[0].(map[string]interface{})
	assert.Equal(t, "student1", userEntry["user_id"])
	assert.Equal(t, float64(1), userEntry["active_count"],
		"per-user active_count reports running-only sessions")
	assert.Equal(t, float64(2), userEntry["occupying_slots"],
		"per-user occupying_slots must include stopped sessions")
}

// TestGetOrgTerminalUsage_OccupyingSlotsAllStopped verifies that a user whose
// only sessions are stopped still appears in the breakdown with active_count=0
// and occupying_slots>0 — the field union must not drop users that have no
// running sessions.
func TestGetOrgTerminalUsage_OccupyingSlotsAllStopped(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student2", orgModels.OrgRoleMember)

	userKey, err := createTestUserKey(db, "student2")
	require.NoError(t, err)

	stoppedTerminal := &models.Terminal{
		SessionID:         "stopped-only-" + uuid.New().String(),
		UserID:            "student2",
		State:            models.StateStopped,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(stoppedTerminal).Error)

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

	assert.Equal(t, float64(0), resp["active_terminals"])
	assert.Equal(t, float64(1), resp["occupying_slots"])

	users, ok := resp["users"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 1, len(users),
		"user with only stopped sessions must still appear in the breakdown")
	userEntry := users[0].(map[string]interface{})
	assert.Equal(t, "student2", userEntry["user_id"])
	assert.Equal(t, float64(0), userEntry["active_count"])
	assert.Equal(t, float64(1), userEntry["occupying_slots"])
}

// TestGetOrgTerminalUsage_PlanLimitsFromSubscription verifies that the
// budget envelope is populated from the organization's subscription plan.
func TestGetOrgTerminalUsage_PlanLimitsFromSubscription(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	plan := &paymentModels.SubscriptionPlan{
		Name:        "Pro",
		MaxCPU:      8000, // 8 vCPU in mCPU
		MaxMemoryMB: 4096,
		IsActive:    true,
		IsCatalog:   true,
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

	quota, ok := resp["quota"].(map[string]interface{})
	require.True(t, ok, "response must carry budget quota envelope")
	assert.Equal(t, float64(8000), quota["max_cpu"], "quota max_cpu should come from the plan in mCPU")
	assert.Equal(t, float64(4096), quota["max_memory_mb"], "quota max_memory_mb should come from the plan")
	assert.Equal(t, "Pro", resp["plan_name"],
		"plan_name should match the subscription plan name")
}

// TestGetOrgTerminalUsage_PastExpiryNotCountedAsActive verifies the SSOT
// alignment between the "active" (running display) count and the slot count:
// both must exclude past-expiry rows. Without this guard, a session whose
// status='active' but whose expires_at is in the past inflates active_terminals
// while being correctly excluded from occupying_slots — producing inconsistent
// numbers in the same dashboard (e.g. "1 active / 0 occupying"). This mirrors
// the OccupiesSlotScope rule documented in src/terminalTrainer/models/terminal.go.
func TestGetOrgTerminalUsage_PastExpiryNotCountedAsActive(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	userKey, err := createTestUserKey(db, "student1")
	require.NoError(t, err)

	// A zombie terminal: status/state still say "active/running" but the
	// session's expires_at is in the past. The proxy session is long gone;
	// only the stale row remains.
	zombie := &models.Terminal{
		SessionID:         "zombie-active-" + uuid.New().String(),
		UserID:            "student1",
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(-30 * time.Minute), // past expiry
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

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

	// Both counts must exclude past-expiry rows for SSOT alignment.
	assert.Equal(t, float64(0), resp["active_terminals"],
		"active_terminals must exclude past-expiry rows (SSOT alignment with OccupiesSlotScope)")
	assert.Equal(t, float64(0), resp["occupying_slots"],
		"occupying_slots already excludes past-expiry rows via OccupiesSlotScope")
}

// TestGetOrgTerminalUsage_FutureExpiryCountedAsActive is the regression guard
// for the past-expiry fix: a still-valid running session MUST continue to be
// counted as active. Ensures the SSOT alignment does not over-correct.
func TestGetOrgTerminalUsage_FutureExpiryCountedAsActive(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	userKey, err := createTestUserKey(db, "student1")
	require.NoError(t, err)

	live := &models.Terminal{
		SessionID:         "live-active-" + uuid.New().String(),
		UserID:            "student1",
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour), // future
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(live).Error)

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

	assert.Equal(t, float64(1), resp["active_terminals"],
		"a future-expiry running session must still be counted as active")
	assert.Equal(t, float64(1), resp["occupying_slots"],
		"a future-expiry running session must still occupy a slot")
}

// TestGetOrgTerminalUsage_DisplayNameResolvedFromCasdoor verifies that the
// per-user breakdown resolves DisplayName + Email through the Casdoor lookup
// seam instead of hardcoding DisplayName to the user_id (which is what the
// previous implementation did — frontend renders `user.display_name` directly
// without enrichment, so users saw raw UUIDs).
//
// The Casdoor lookup is exposed via the package-level
// `services.LookupCasdoorUser` test seam (mirrors the pattern from
// `usersRoutes.LookupCasdoorUser`). Tests swap the function to inject fake
// user records without standing up a real Casdoor server.
func TestGetOrgTerminalUsage_DisplayNameResolvedFromCasdoor(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	userKey, err := createTestUserKey(db, "student1")
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "name-test-" + uuid.New().String(),
		UserID:            "student1",
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(terminal).Error)

	// Swap the Casdoor lookup seam to return a deterministic record for
	// student1. Restore it on test exit so other tests stay isolated.
	originalLookup := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		if id == "student1" {
			return &casdoorsdk.User{
				Id:          "student1",
				Name:        "alice",
				DisplayName: "Alice Liddell",
				Email:       "alice@example.com",
			}, nil
		}
		return nil, fmt.Errorf("user not found")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = originalLookup })

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

	users, ok := resp["users"].([]interface{})
	require.True(t, ok, "users should be a JSON array")
	require.Equal(t, 1, len(users))

	entry := users[0].(map[string]interface{})
	assert.Equal(t, "student1", entry["user_id"], "user_id must carry the Casdoor ID")
	assert.Equal(t, "Alice Liddell", entry["display_name"],
		"display_name must be resolved from Casdoor, not hardcoded to the user_id")
	assert.Equal(t, "alice@example.com", entry["email"],
		"email must be resolved from Casdoor")
}

// TestGetOrgTerminalUsage_DisplayNameFallbackChain verifies the fallback
// behaviour when Casdoor returns an incomplete record (no DisplayName) or
// fails the lookup entirely: DisplayName should chain through name -> email
// -> user_id so users always get a sensible label even when Casdoor data is
// missing.
func TestGetOrgTerminalUsage_DisplayNameFallbackChain(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student-noname", orgModels.OrgRoleMember)
	createTestOrgMember(t, db, org.ID, "student-missing", orgModels.OrgRoleMember)

	userKey1, err := createTestUserKey(db, "student-noname")
	require.NoError(t, err)
	userKey2, err := createTestUserKey(db, "student-missing")
	require.NoError(t, err)

	for _, uid := range []string{"student-noname", "student-missing"} {
		key := userKey1
		if uid == "student-missing" {
			key = userKey2
		}
		terminal := &models.Terminal{
			SessionID:         "fallback-" + uid + "-" + uuid.New().String(),
			UserID:            uid,
			State:             models.StateRunning,
			ExpiresAt:         time.Now().Add(1 * time.Hour),
			InstanceType:      "test",
			MachineSize:       "S",
			UserTerminalKeyID: key.ID,
			OrganizationID:    &org.ID,
		}
		require.NoError(t, db.Create(terminal).Error)
	}

	originalLookup := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		if id == "student-noname" {
			// No DisplayName, but has email -> email used as label.
			return &casdoorsdk.User{
				Id:    "student-noname",
				Email: "ghost@example.com",
			}, nil
		}
		// student-missing: lookup fails -> fallback to user_id.
		return nil, fmt.Errorf("not found")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = originalLookup })

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

	users, ok := resp["users"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 2, len(users))

	byUser := make(map[string]map[string]interface{})
	for _, u := range users {
		entry := u.(map[string]interface{})
		byUser[entry["user_id"].(string)] = entry
	}

	noname, ok := byUser["student-noname"]
	require.True(t, ok)
	assert.Equal(t, "ghost@example.com", noname["display_name"],
		"when DisplayName is empty, email must be used as label")

	missing, ok := byUser["student-missing"]
	require.True(t, ok)
	assert.Equal(t, "student-missing", missing["display_name"],
		"when Casdoor lookup fails, user_id must be used as final fallback")
	assert.Equal(t, "", missing["email"],
		"failed lookup must leave email empty (not invent a value)")
}
