// tests/payment/orgSubscriptionUniqueConstraint_test.go
//
// Tests the DB-level partial unique index that enforces "at most one active
// subscription per organization". This is the canonical defense against
// multi-pod races (e.g. admin assign + Stripe webhook firing at the same time,
// both passing the in-process deactivate check before inserting their new rows).
//
// The partial predicate is:
//
//	UNIQUE (organization_id)
//	WHERE status = 'active' AND deleted_at IS NULL
//
// It spanned ('active','trialing') until #439 retired the trialing status.
// Note it has never covered past_due, so an org can hold an active and a
// past_due subscription simultaneously and both entitle — a pre-existing gap
// that removing trialing neither caused nor closed.
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

// TestOrgSubscription_PartialUniqueIndex_IgnoresTerminalStatuses guards the
// narrowed predicate (#439). The index used to span active AND trialing, so a
// trialing row occupied an organization's single slot. Now only 'active' does:
// a row in a terminal status must not block a fresh active subscription, or an
// organization whose subscription lapsed could never be given a new one.
func TestOrgSubscription_PartialUniqueIndex_IgnoresTerminalStatuses(t *testing.T) {
	for _, status := range []string{"cancelled", "incomplete", "unpaid"} {
		t.Run(status, func(t *testing.T) {
			db := freshTestDB(t)
			planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

			_, err := insertOrgSubWithStatus(t, db, orgID, planID, status)
			require.NoError(t, err, "first %s insert should succeed", status)

			_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
			require.NoError(t, err,
				"a %s row must not occupy the organization's active slot", status)
		})
	}
}

// TestOrgSubscription_TrialingNoLongerOccupiesTheActiveSlot pins the specific
// consequence of #439 on any row that somehow still carries the retired status:
// it is inert with respect to the index. MigrateTrialingStatusToActive converts
// such rows at startup, so this is the belt to that migration's braces.
func TestOrgSubscription_TrialingNoLongerOccupiesTheActiveSlot(t *testing.T) {
	db := freshTestDB(t)
	planID, orgID := seedOrgAndPlanForUniqueTest(t, db)

	_, err := insertOrgSubWithStatus(t, db, orgID, planID, "trialing")
	require.NoError(t, err, "a legacy trialing row should still insert")

	_, err = insertOrgSubWithStatus(t, db, orgID, planID, "active")
	require.NoError(t, err,
		"the retired trialing status must no longer block an active subscription")
}
