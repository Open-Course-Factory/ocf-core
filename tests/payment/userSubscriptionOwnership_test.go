// tests/payment/userSubscriptionOwnership_test.go
package payment_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	casbinUtils "soli/formations/src/auth/casbin"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
)

var userSubscriptionOwnershipConfig = casbinUtils.OwnershipConfig{
	OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
}

// createTestSubscriptionPlan inserts a SubscriptionPlan for FK reference
func createTestSubscriptionPlan(t *testing.T) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Test Plan",
		PriceAmount:     1000,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	err := sharedTestDB.Create(plan).Error
	require.NoError(t, err)
	return plan
}

// createTestUserSubscription inserts a UserSubscription owned by the given userID
func createTestUserSubscription(t *testing.T, userID string, plan *models.SubscriptionPlan) *models.UserSubscription {
	t.Helper()
	sub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	err := sharedTestDB.Create(sub).Error
	require.NoError(t, err)
	return sub
}

// ============================================================================
// UserSubscription Ownership Hook — BeforeCreate Tests
// ============================================================================

func TestUserSubscriptionOwnership_BeforeCreate_SetsUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	userID := "user-creator-123"
	sub := &models.UserSubscription{
		SubscriptionPlanID: plan.ID,
		Status:             "incomplete",
	}

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeCreate,
		NewEntity:  sub,
		UserID:     userID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, userID, sub.UserID, "BeforeCreate should set UserID from authenticated user")
}

func TestUserSubscriptionOwnership_BeforeCreate_AdminCanSetAnyUserID(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	adminID := "admin-user-789"
	targetUserID := "target-user-456"
	sub := &models.UserSubscription{
		UserID:             targetUserID,
		SubscriptionPlanID: plan.ID,
		Status:             "incomplete",
	}

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeCreate,
		NewEntity:  sub,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, targetUserID, sub.UserID, "Admin should be able to set any UserID")
}

// ============================================================================
// UserSubscription Ownership Hook — BeforeUpdate Tests
// ============================================================================

func TestUserSubscriptionOwnership_BeforeUpdate_OwnerCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeUpdate,
		EntityID:   sub.ID,
		OldEntity:  sub,
		NewEntity:  map[string]any{"status": "cancelled"},
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to update their own subscription")
}

func TestUserSubscriptionOwnership_BeforeUpdate_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeUpdate,
		EntityID:   sub.ID,
		OldEntity:  sub,
		NewEntity:  map[string]any{"status": "cancelled"},
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from updating subscription")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestUserSubscriptionOwnership_BeforeUpdate_AdminCanUpdate(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeUpdate,
		EntityID:   sub.ID,
		OldEntity:  sub,
		NewEntity:  map[string]any{"status": "cancelled"},
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to update any subscription")
}

// ============================================================================
// UserSubscription Ownership Hook — BeforeDelete Tests
// (Member can't DELETE per Casbin, but hook should still enforce for safety)
// ============================================================================

func TestUserSubscriptionOwnership_BeforeDelete_OwnerCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeDelete,
		EntityID:   sub.ID,
		NewEntity:  sub,
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to delete their own subscription")
}

func TestUserSubscriptionOwnership_BeforeDelete_NonOwnerBlocked(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeDelete,
		EntityID:   sub.ID,
		NewEntity:  sub,
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from deleting subscription")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestUserSubscriptionOwnership_BeforeDelete_AdminCanDelete(t *testing.T) {
	_ = freshTestDB(t)
	hook := hooks.NewOwnershipHook(sharedTestDB, "UserSubscription", userSubscriptionOwnershipConfig)

	plan := createTestSubscriptionPlan(t)
	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	sub := createTestUserSubscription(t, ownerID, plan)

	ctx := &hooks.HookContext{
		EntityName: "UserSubscription",
		HookType:   hooks.BeforeDelete,
		EntityID:   sub.ID,
		NewEntity:  sub,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to delete any subscription")
}
