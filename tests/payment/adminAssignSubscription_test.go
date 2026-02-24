// tests/payment/adminAssignSubscription_test.go
// Tests for the AdminAssignSubscription service method.
// These tests verify that admin-assigned subscriptions are created correctly,
// default duration is applied, existing subscriptions are replaced, etc.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestPlan creates a subscription plan for testing admin assignment.
// Uses RequiredRole="" to avoid Casdoor/Casbin calls during tests.
func createTestPlan(t *testing.T, name string, priceAmount int64) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		Description:            "Test plan for admin assign tests",
		PriceAmount:            priceAmount,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             -1,
		MaxConcurrentTerminals: 3,
		MaxConcurrentUsers:     1,
		IsActive:               true,
		RequiredRole:           "", // Empty to avoid Casdoor calls in tests
	}
	err := sharedTestDB.Create(plan).Error
	require.NoError(t, err, "Failed to create test plan %q", name)
	return plan
}

func TestAdminAssignSubscription_ValidInput_CreatesSubscription(t *testing.T) {
	db := freshTestDB(t)
	plan := createTestPlan(t, "Pro Plan", 1200)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()
	sub, err := svc.AdminAssignSubscription(userID, plan.ID, 30, "")
	require.NoError(t, err, "Admin assign should succeed with valid input")
	require.NotNil(t, sub)

	assert.Equal(t, userID, sub.UserID)
	assert.Equal(t, plan.ID, sub.SubscriptionPlanID)
	assert.Equal(t, "active", sub.Status)
	assert.False(t, sub.CurrentPeriodStart.IsZero(), "Period start should be set")
	assert.False(t, sub.CurrentPeriodEnd.IsZero(), "Period end should be set")

	// Verify the period is approximately 30 days
	expectedEnd := sub.CurrentPeriodStart.AddDate(0, 0, 30)
	assert.WithinDuration(t, expectedEnd, sub.CurrentPeriodEnd, time.Minute,
		"Period end should be 30 days after start")
}

func TestAdminAssignSubscription_DefaultDuration_365Days(t *testing.T) {
	db := freshTestDB(t)
	plan := createTestPlan(t, "Default Duration Plan", 0)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()
	sub, err := svc.AdminAssignSubscription(userID, plan.ID, 0, "")
	require.NoError(t, err, "Admin assign with durationDays=0 should default to 365")
	require.NotNil(t, sub)

	// Verify the period is approximately 365 days
	expectedEnd := sub.CurrentPeriodStart.AddDate(0, 0, 365)
	assert.WithinDuration(t, expectedEnd, sub.CurrentPeriodEnd, time.Minute,
		"Duration should default to 365 days when durationDays=0")
}

func TestAdminAssignSubscription_InvalidPlan_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()
	fakePlanID := uuid.New() // Non-existent plan

	sub, err := svc.AdminAssignSubscription(userID, fakePlanID, 30, "")
	assert.Error(t, err, "Should fail with non-existent plan ID")
	assert.Nil(t, sub)
	assert.Contains(t, err.Error(), "invalid plan ID")
}

func TestAdminAssignSubscription_ExistingSubscription_ReplacesOld(t *testing.T) {
	db := freshTestDB(t)
	plan := createTestPlan(t, "Replace Test Plan", 1200)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()

	// Create the first subscription
	firstSub, err := svc.AdminAssignSubscription(userID, plan.ID, 30, "admin-test-user")
	require.NoError(t, err)
	require.NotNil(t, firstSub)
	firstSubID := firstSub.ID

	// Assign a new subscription — the old one should be replaced
	secondSub, err := svc.AdminAssignSubscription(userID, plan.ID, 60, "admin-test-user")
	require.NoError(t, err)
	require.NotNil(t, secondSub)

	// Verify the new subscription is active
	assert.Equal(t, "active", secondSub.Status)
	assert.NotEqual(t, firstSubID, secondSub.ID, "Should be a new subscription, not the same one")

	// Verify the old subscription was marked as replaced
	var oldSub models.UserSubscription
	err = db.Unscoped().First(&oldSub, "id = ?", firstSubID).Error
	require.NoError(t, err, "Should find the old subscription")
	assert.Equal(t, "replaced", oldSub.Status, "Old subscription should have status 'replaced'")
	assert.NotNil(t, oldSub.CancelledAt, "Old subscription should have CancelledAt set")
}

func TestAdminAssignSubscription_SubscriptionType_IsAssigned(t *testing.T) {
	db := freshTestDB(t)
	plan := createTestPlan(t, "Type Test Plan", 500)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()
	sub, err := svc.AdminAssignSubscription(userID, plan.ID, 90, "")
	require.NoError(t, err)
	require.NotNil(t, sub)

	assert.Equal(t, "assigned", sub.SubscriptionType,
		"Admin-assigned subscriptions should have type 'assigned'")
}

// ==========================================
// BUG-EXPOSING TEST (should FAIL with current code — item #11)
// ==========================================

// TestAdminAssignSubscription_DurationExceedsMax_ReturnsError tests that passing
// an extremely large durationDays is rejected.
// BUG: The current code has no upper bound on durationDays. It accepts any positive
// integer, which could create subscriptions lasting thousands of years.
// A reasonable maximum (e.g., 3650 days = 10 years) should be enforced.
func TestAdminAssignSubscription_DurationExceedsMax_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	plan := createTestPlan(t, "Max Duration Plan", 1200)
	svc := services.NewSubscriptionService(db)

	userID := uuid.New().String()

	// Pass an absurdly large duration — 999999 days (about 2739 years)
	sub, err := svc.AdminAssignSubscription(userID, plan.ID, 999999, "")

	// BUG: Current code accepts any positive duration with no upper bound.
	// This test expects an error to be returned for unreasonable durations.
	// With current code, this assertion FAILS because the subscription is created
	// successfully with an end date in the year ~4765.
	assert.Error(t, err, "Should reject unreasonably large durations (no upper bound validation)")
	assert.Nil(t, sub, "No subscription should be created for invalid duration")
}
