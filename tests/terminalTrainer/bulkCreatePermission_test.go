package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	terminalController "soli/formations/src/terminalTrainer/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupBulkCreateTestDB creates an in-memory SQLite database with tables needed for bulk-create tests
func setupBulkCreateTestDB(t *testing.T) *gorm.DB {
	db := setupTestDB(t)

	// Create tables manually to avoid JSONB type issues with SQLite
	err := db.Exec(`CREATE TABLE IF NOT EXISTS class_groups (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		name TEXT NOT NULL,
		display_name TEXT NOT NULL,
		description TEXT,
		owner_user_id TEXT NOT NULL,
		organization_id TEXT,
		parent_group_id TEXT,
		subscription_plan_id TEXT,
		max_members INTEGER DEFAULT 50,
		expires_at DATETIME,
		casdoor_group_name TEXT,
		is_active BOOLEAN DEFAULT TRUE,
		metadata TEXT
	)`).Error
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE IF NOT EXISTS group_members (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		group_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT DEFAULT 'member',
		invited_by TEXT,
		joined_at DATETIME NOT NULL,
		is_active BOOLEAN DEFAULT TRUE,
		metadata TEXT
	)`).Error
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE IF NOT EXISTS subscription_plans (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		name TEXT,
		description TEXT,
		is_active BOOLEAN DEFAULT TRUE,
		max_concurrent_terminals INTEGER DEFAULT 1,
		max_session_duration_minutes INTEGER DEFAULT 60,
		price_amount INTEGER DEFAULT 0,
		currency TEXT DEFAULT 'eur',
		billing_interval TEXT DEFAULT 'month',
		max_concurrent_users INTEGER DEFAULT 1,
		max_courses INTEGER DEFAULT -1,
		priority INTEGER DEFAULT 0,
		trial_days INTEGER DEFAULT 0,
		network_access_enabled BOOLEAN DEFAULT FALSE,
		data_persistence_enabled BOOLEAN DEFAULT FALSE,
		data_persistence_gb INTEGER DEFAULT 0,
		use_tiered_pricing BOOLEAN DEFAULT FALSE,
		stripe_product_id TEXT,
		stripe_price_id TEXT,
		required_role TEXT,
		default_backend TEXT,
		features TEXT,
		allowed_machine_sizes TEXT,
		allowed_templates TEXT,
		allowed_backends TEXT,
		planned_features TEXT,
		pricing_tiers TEXT
	)`).Error
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE IF NOT EXISTS user_subscriptions (
		id TEXT PRIMARY KEY,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		owner_ids TEXT,
		user_id TEXT,
		subscription_plan_id TEXT,
		status TEXT DEFAULT 'active',
		stripe_subscription_id TEXT,
		stripe_customer_id TEXT,
		current_period_start DATETIME,
		current_period_end DATETIME,
		cancel_at_period_end BOOLEAN DEFAULT FALSE,
		canceled_at DATETIME,
		source TEXT DEFAULT 'manual',
		assigned_by TEXT,
		batch_id TEXT
	)`).Error
	require.NoError(t, err)

	return db
}

// createTestGroup creates a test class group using raw SQL to avoid JSONB issues with SQLite
func createTestGroup(t *testing.T, db *gorm.DB, ownerUserID string) *groupModels.ClassGroup {
	id := uuid.New()
	err := db.Exec(
		`INSERT INTO class_groups (id, created_at, updated_at, name, display_name, owner_user_id, max_members, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), time.Now(), time.Now(), "test-group", "Test Group", ownerUserID, 50, true,
	).Error
	require.NoError(t, err)

	var group groupModels.ClassGroup
	err = db.First(&group, "id = ?", id).Error
	require.NoError(t, err)
	return &group
}

// addGroupMember adds a member to a group using raw SQL to avoid JSONB issues with SQLite.
// Members are set as inactive by default to avoid triggering Casdoor API calls during terminal creation.
// The permission check iterates all members regardless of IsActive, so inactive members still pass the role check.
func addGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) {
	id := uuid.New()
	err := db.Exec(
		`INSERT INTO group_members (id, created_at, updated_at, group_id, user_id, role, is_active, joined_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), time.Now(), time.Now(), groupID.String(), userID, string(role), false, time.Now(),
	).Error
	require.NoError(t, err)
}

// createTestPlanAndSubscription creates a subscription plan and user subscription for testing
func createTestPlanAndSubscription(t *testing.T, db *gorm.DB, userID string) *paymentModels.SubscriptionPlan {
	planID := uuid.New()
	err := db.Exec(
		`INSERT INTO subscription_plans (id, created_at, updated_at, name, is_active, max_concurrent_terminals, max_session_duration_minutes) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		planID.String(), time.Now(), time.Now(), "Test Plan", true, 5, 60,
	).Error
	require.NoError(t, err)

	subID := uuid.New()
	err = db.Exec(
		`INSERT INTO user_subscriptions (id, created_at, updated_at, user_id, subscription_plan_id, status) VALUES (?, ?, ?, ?, ?, ?)`,
		subID.String(), time.Now(), time.Now(), userID, planID.String(), "active",
	).Error
	require.NoError(t, err)

	plan := &paymentModels.SubscriptionPlan{
		Name:                      "Test Plan",
		IsActive:                  true,
		MaxConcurrentTerminals:    5,
		MaxSessionDurationMinutes: 60,
	}
	plan.ID = planID
	return plan
}

func setupBulkCreateRouter(db *gorm.DB, userID string, userRoles []string, plan *paymentModels.SubscriptionPlan) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	ctrl := terminalController.NewTerminalController(db)

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})

	// Mock subscription middleware - inject plan into context
	router.Use(func(c *gin.Context) {
		if plan != nil {
			c.Set("subscription_plan", plan)
			c.Set("has_active_subscription", true)
		}
		c.Next()
	})

	router.POST("/class-groups/:groupId/bulk-create-terminals", ctrl.BulkCreateTerminalsForGroup)

	return router
}

func TestBulkCreate_GroupOwner_Allowed(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	group := createTestGroup(t, db, ownerID)
	plan := createTestPlanAndSubscription(t, db, ownerID)

	router := setupBulkCreateRouter(db, ownerID, []string{"member"}, plan)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Group owner should be allowed")

	var response dto.BulkCreateTerminalsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, 0, response.TotalMembers, "No active members, so 0 terminals created")
}

func TestBulkCreate_GroupAdmin_Allowed(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	adminID := "admin-user-id"
	group := createTestGroup(t, db, ownerID)
	addGroupMember(t, db, group.ID, adminID, groupModels.GroupMemberRoleAdmin)
	plan := createTestPlanAndSubscription(t, db, adminID)

	router := setupBulkCreateRouter(db, adminID, []string{"member"}, plan)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Group admin should be allowed")
}

func TestBulkCreate_SystemAdmin_Allowed(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	sysAdminID := "sys-admin-user-id"
	group := createTestGroup(t, db, ownerID)
	plan := createTestPlanAndSubscription(t, db, sysAdminID)

	// System admin is NOT the group owner and NOT a group member
	router := setupBulkCreateRouter(db, sysAdminID, []string{"administrator"}, plan)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "System administrator should be allowed to bulk create terminals")

	var response dto.BulkCreateTerminalsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.True(t, response.Success)
}

func TestBulkCreate_RegularMember_Denied(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	memberID := "regular-member-id"
	group := createTestGroup(t, db, ownerID)
	addGroupMember(t, db, group.ID, memberID, groupModels.GroupMemberRoleMember)
	plan := createTestPlanAndSubscription(t, db, memberID)

	router := setupBulkCreateRouter(db, memberID, []string{"member"}, plan)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "Regular member should be denied")
}

func TestBulkCreate_NonMember_Denied(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	outsiderID := "outsider-user-id"
	group := createTestGroup(t, db, ownerID)
	plan := createTestPlanAndSubscription(t, db, outsiderID)

	// User is not a member and not an admin
	router := setupBulkCreateRouter(db, outsiderID, []string{"member"}, plan)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "Non-member non-admin should be denied")
}

func TestBulkCreate_NoSubscription_Returns403(t *testing.T) {
	db := setupBulkCreateTestDB(t)
	ownerID := "owner-user-id"
	group := createTestGroup(t, db, ownerID)

	// No plan injected (nil) - simulates no active subscription
	router := setupBulkCreateRouter(db, ownerID, []string{"member"}, nil)

	body := `{"terms": "accepted"}`
	req := httptest.NewRequest("POST", "/class-groups/"+group.ID.String()+"/bulk-create-terminals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 when no subscription")
}
