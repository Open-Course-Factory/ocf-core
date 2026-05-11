// tests/payment/usageMetricsDrift_test.go
//
// Documents the contract introduced in the QuotaService consolidation
// (#311): for concurrent_terminals, the stored usage_metrics.current_value
// is no longer authoritative. Every read goes through QuotaService, which
// recomputes from the terminals table. This test makes the contract
// explicit so future refactors don't silently re-introduce the
// materialized counter.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestUsageMetricsDrift_ConcurrentTerminalsLiveRecalcDominates verifies that
// a wildly wrong usage_metrics.current_value (5 here) is ignored by the
// quota-check path — only the live count from the terminals table matters.
// Documents the post-#311 contract: drifted metric counters never leak into
// user-facing quota decisions for concurrent_terminals.
func TestUsageMetricsDrift_ConcurrentTerminalsLiveRecalcDominates(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "drift-user"

	// Plan: 3 concurrent terminals max
	plan := createPlan(t, db, "DriftPlan", 5, 3)
	sub := createUserSubscription(t, db, userID, plan)

	// Persist a drifted usage_metrics row: current_value=5 (would mean the
	// user is at-or-over the 3-terminal limit). With the materialized
	// counter, this would block creation. With the live-recalc contract,
	// it must NOT influence the decision because there are zero actual
	// terminals.
	now := time.Now()
	driftedMetric := &paymentModels.UsageMetrics{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:         userID,
		SubscriptionID: sub.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   5, // intentionally wrong — no actual terminals exist
		LimitValue:     3,
		PeriodStart:    now.AddDate(0, 0, -1),
		PeriodEnd:      now.AddDate(0, 1, 0),
		LastUpdated:    now,
	}
	assert.NoError(t, db.Create(driftedMetric).Error)

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed, "live count (0) dominates stored counter (5)")
	assert.Equal(t, int64(0), check.CurrentUsage, "must report live count, not stored drift value")
	assert.Equal(t, int64(3), check.Limit)
	assert.Equal(t, int64(3), check.RemainingUsage)
}

// TestUsageMetricsDrift_GetUserUsage_IgnoresStoredValue documents the same
// contract from the GetUserUsage side: even a heavily-drifted stored row
// must not appear in the live count.
func TestUsageMetricsDrift_GetUserUsage_IgnoresStoredValue(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "drift-usage-user"

	plan := createPlan(t, db, "DriftPlan2", 5, 10)
	sub := createUserSubscription(t, db, userID, plan)

	now := time.Now()
	assert.NoError(t, db.Create(&paymentModels.UsageMetrics{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:         userID,
		SubscriptionID: sub.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   42, // intentionally absurd
		LimitValue:     10,
		PeriodStart:    now.AddDate(0, 0, -1),
		PeriodEnd:      now.AddDate(0, 1, 0),
		LastUpdated:    now,
	}).Error)

	// Insert 2 real terminals — the truth.
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "stopped")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	usage, err := svc.GetUserUsage(userID, nil, "concurrent_terminals")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), usage, "GetUserUsage must reflect live terminal count, not the drifted stored value")
}
