// tests/payment/orgSubscriptionUniqueConstraint_test.go
//
// Tests the DB-level partial unique index that enforces "at most one
// active/trialing subscription per organization". This is the canonical
// defense against multi-pod races (e.g. admin assign + Stripe webhook firing
// at the same time, both passing the in-process deactivate check before
// inserting their new rows).
//
// The partial predicate is:
//
//	UNIQUE (organization_id)
//	WHERE status IN ('active', 'trialing') AND deleted_at IS NULL
package payment_tests

import (
	"strings"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// insertOrgSubWithStatus inserts a single org subscription with a chosen
// status (and optionally a deleted_at), bypassing the service so we can
// directly exercise the DB-level constraint.
func insertOrgSubWithStatus(
	t *testing.T,
	db *gorm.DB,
	orgID, planID uuid.UUID,
	status string,
) (uuid.UUID, error) {
	t.Helper()
	sub := &models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		SubscriptionPlanID: planID,
		Status:             status,
		StripeCustomerID:   "cus_test_" + uuid.NewString()[:8],
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
		Quantity:           1,
	}
	err := db.Create(sub).Error
	return sub.ID, err
}

// seedOrgAndPlanForUniqueTest creates a single plan and organization used by
// the partial-unique-index tests.
func seedOrgAndPlanForUniqueTest(t *testing.T, db *gorm.DB) (planID uuid.UUID, orgID uuid.UUID) {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "PlanUniqueIdxTest_" + uuid.NewString()[:8],
		Priority:        0,
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan.ID, uuid.New()
}

// isUniqueViolation returns true if the error looks like a unique-constraint
// violation (loose check so we work across SQLite + PostgreSQL).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

// TestOrgSubscription_PartialUniqueIndex_RejectsDuplicateActive verifies the
// core invariant: two rows with the same organization_id and status='active'
// cannot coexist (no deleted_at).
func TestOrgSubscription_PartialUniqueIndex_RejectsDuplicateActive(t *testing.T) {
	db := freshTestDB(t)
	planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

	_, err := insertOrgSubWithStatus(t, db, orgID, planID, "active")
	require.NoError(t, err, "first active insert should succeed")

	_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
	assert.Error(t, err, "second active insert for the same org must be rejected by the partial unique index")
	assert.True(t, isUniqueViolation(err),
		"expected a unique-constraint violation, got: %v", err)
}

// TestOrgSubscription_PartialUniqueIndex_AllowsCancelledThenActive verifies
// the partial predicate excludes 'cancelled' rows — assigning a new plan
// after a cancellation must succeed.
func TestOrgSubscription_PartialUniqueIndex_AllowsCancelledThenActive(t *testing.T) {
	db := freshTestDB(t)
	planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

	_, err := insertOrgSubWithStatus(t, db, orgID, planID, "cancelled")
	require.NoError(t, err, "first cancelled insert should succeed")

	_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
	require.NoError(t, err,
		"active insert must succeed when only cancelled rows exist for the org")
}

// TestOrgSubscription_PartialUniqueIndex_AllowsSoftDeletedThenActive verifies
// the partial predicate excludes soft-deleted rows. Without `deleted_at IS
// NULL` in the WHERE clause, the index would block reassigning a plan after
// a hard cancellation that left a soft-deleted row around.
func TestOrgSubscription_PartialUniqueIndex_AllowsSoftDeletedThenActive(t *testing.T) {
	db := freshTestDB(t)
	planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

	firstID, err := insertOrgSubWithStatus(t, db, orgID, planID, "active")
	require.NoError(t, err, "first active insert should succeed")

	// Soft-delete: GORM sets deleted_at; row stays in the table but the
	// partial unique index must ignore it.
	require.NoError(t, db.Delete(&models.OrganizationSubscription{}, "id = ?", firstID).Error)

	_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
	require.NoError(t, err,
		"active insert must succeed when only soft-deleted rows exist for the org")
}

// TestOrgSubscription_PartialUniqueIndex_RejectsTrialingThenActive guards the
// multi-status predicate: 'trialing' is treated as an active state and must
// collide with a subsequent 'active' insert for the same org.
func TestOrgSubscription_PartialUniqueIndex_RejectsTrialingThenActive(t *testing.T) {
	db := freshTestDB(t)
	planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

	_, err := insertOrgSubWithStatus(t, db, orgID, planID, "trialing")
	require.NoError(t, err, "first trialing insert should succeed")

	_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
	assert.Error(t, err,
		"active insert must be rejected when a trialing row already exists for the same org")
	assert.True(t, isUniqueViolation(err),
		"expected a unique-constraint violation, got: %v", err)
}
