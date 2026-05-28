// Tests for the idempotent startup migration that clamps drifted
// `concurrent_terminals` usage rows to zero.
//
// Context: the dual-mode quota cleanup left a decrement-only path that drove
// `usage_metrics.current_value` negative for users who deleted sessions after
// the cleanup landed (one user observed -9). Commit 577336a fixed the live
// recompute on the read path; this migration cleans up the persisted bad
// rows so any code path that still reads the model field directly (e.g. the
// generic `/usage-metrics` entity endpoint) never serves a negative value.
package payment_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/initialization"
	paymentModels "soli/formations/src/payment/models"
)

func TestClampNegativeConcurrentTerminalsUsage_ClampsOnlyNegativeRows(t *testing.T) {
	db := freshTestDB(t)

	subID := uuid.New()
	now := time.Now()

	// Row 1: the user's exact case — concurrent_terminals drifted to -9.
	driftedTerminals := paymentModels.UsageMetrics{
		UserID:         "user-drifted",
		SubscriptionID: subID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   -9,
		LimitValue:     5,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&driftedTerminals).Error; err != nil {
		t.Fatalf("seed drifted concurrent_terminals row: %v", err)
	}

	// Row 2: control — different metric type, also negative. Must NOT be touched
	// (negative `courses_created` is a separate bug and out of scope here).
	driftedCourses := paymentModels.UsageMetrics{
		UserID:         "user-courses",
		SubscriptionID: subID,
		MetricType:     "courses_created",
		CurrentValue:   -3,
		LimitValue:     10,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&driftedCourses).Error; err != nil {
		t.Fatalf("seed control courses_created row: %v", err)
	}

	// Row 3: control — concurrent_terminals with a positive value. Must NOT
	// be touched.
	healthyTerminals := paymentModels.UsageMetrics{
		UserID:         "user-healthy",
		SubscriptionID: subID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   4,
		LimitValue:     5,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&healthyTerminals).Error; err != nil {
		t.Fatalf("seed healthy concurrent_terminals row: %v", err)
	}

	// Run the migration.
	initialization.ClampNegativeConcurrentTerminalsUsage(db)

	// Re-read all three rows and assert the expected end state.
	var afterDriftedTerminals paymentModels.UsageMetrics
	if err := db.First(&afterDriftedTerminals, "id = ?", driftedTerminals.ID).Error; err != nil {
		t.Fatalf("reload drifted row: %v", err)
	}
	assert.Equal(t, int64(0), afterDriftedTerminals.CurrentValue,
		"concurrent_terminals row at -9 should be clamped to 0")

	var afterDriftedCourses paymentModels.UsageMetrics
	if err := db.First(&afterDriftedCourses, "id = ?", driftedCourses.ID).Error; err != nil {
		t.Fatalf("reload courses control row: %v", err)
	}
	assert.Equal(t, int64(-3), afterDriftedCourses.CurrentValue,
		"courses_created -3 must be left alone (different metric type)")

	var afterHealthyTerminals paymentModels.UsageMetrics
	if err := db.First(&afterHealthyTerminals, "id = ?", healthyTerminals.ID).Error; err != nil {
		t.Fatalf("reload healthy row: %v", err)
	}
	assert.Equal(t, int64(4), afterHealthyTerminals.CurrentValue,
		"positive concurrent_terminals value must be left alone")
}

func TestClampNegativeConcurrentTerminalsUsage_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)

	subID := uuid.New()
	now := time.Now()

	drifted := paymentModels.UsageMetrics{
		UserID:         "user-idem",
		SubscriptionID: subID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   -2,
		LimitValue:     5,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&drifted).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First run: clamps -2 → 0.
	initialization.ClampNegativeConcurrentTerminalsUsage(db)

	var afterFirst paymentModels.UsageMetrics
	if err := db.First(&afterFirst, "id = ?", drifted.ID).Error; err != nil {
		t.Fatalf("reload after first run: %v", err)
	}
	assert.Equal(t, int64(0), afterFirst.CurrentValue,
		"first run should clamp -2 to 0")

	// Second run: must be a no-op, and must not error.
	initialization.ClampNegativeConcurrentTerminalsUsage(db)

	var afterSecond paymentModels.UsageMetrics
	if err := db.First(&afterSecond, "id = ?", drifted.ID).Error; err != nil {
		t.Fatalf("reload after second run: %v", err)
	}
	assert.Equal(t, int64(0), afterSecond.CurrentValue,
		"second run must remain at 0 (idempotent)")
}
