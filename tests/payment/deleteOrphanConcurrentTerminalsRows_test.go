// Tests for the one-shot startup migration that deletes orphan
// `concurrent_terminals` rows from usage_metrics.
//
// Context: the dual-mode quota cleanup turned the CPU/RAM budget engine into
// the authoritative quota gate. The legacy `concurrent_terminals` usage
// metric is dead infrastructure — it is no longer seeded, never enforces a
// limit, and its live value is recomputed via the budget engine. Any rows
// still present in usage_metrics are leftovers from previous deployments and
// should be scrubbed so they cannot resurface through the generic
// /usage-metrics entity endpoint.
package payment_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/initialization"
	paymentModels "soli/formations/src/payment/models"
)

func TestDeleteOrphanConcurrentTerminalsRows_DeletesOnlyConcurrentTerminals(t *testing.T) {
	db := freshTestDB(t)

	subID := uuid.New()
	now := time.Now()

	// Row 1: a concurrent_terminals row left over from a previous deployment.
	// Must be deleted.
	orphan := paymentModels.UsageMetrics{
		UserID:         "user-orphan",
		SubscriptionID: subID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   5,
		LimitValue:     5,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("seed orphan concurrent_terminals row: %v", err)
	}

	// Row 2: control — a healthy metric of a different type. Must NOT be touched.
	courses := paymentModels.UsageMetrics{
		UserID:         "user-courses",
		SubscriptionID: subID,
		MetricType:     "courses_created",
		CurrentValue:   3,
		LimitValue:     10,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&courses).Error; err != nil {
		t.Fatalf("seed control courses_created row: %v", err)
	}

	// Run the migration.
	initialization.DeleteOrphanConcurrentTerminalsRows(db)

	// The orphan must be gone.
	var afterOrphan paymentModels.UsageMetrics
	err := db.First(&afterOrphan, "id = ?", orphan.ID).Error
	assert.Error(t, err, "concurrent_terminals row should have been deleted")

	// The control must remain untouched.
	var afterCourses paymentModels.UsageMetrics
	if err := db.First(&afterCourses, "id = ?", courses.ID).Error; err != nil {
		t.Fatalf("reload courses control row: %v", err)
	}
	assert.Equal(t, int64(3), afterCourses.CurrentValue,
		"courses_created control row must not be touched")
	assert.Equal(t, "courses_created", afterCourses.MetricType)
}

func TestDeleteOrphanConcurrentTerminalsRows_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)

	subID := uuid.New()
	now := time.Now()

	orphan := paymentModels.UsageMetrics{
		UserID:         "user-idem",
		SubscriptionID: subID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   2,
		LimitValue:     5,
		PeriodStart:    now,
		PeriodEnd:      now.Add(30 * 24 * time.Hour),
		LastUpdated:    now,
	}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First run: deletes the orphan.
	initialization.DeleteOrphanConcurrentTerminalsRows(db)

	var count int64
	db.Model(&paymentModels.UsageMetrics{}).
		Where("metric_type = ?", "concurrent_terminals").
		Count(&count)
	assert.Equal(t, int64(0), count, "first run should delete the orphan row")

	// Second run: must be a no-op and must not error.
	initialization.DeleteOrphanConcurrentTerminalsRows(db)

	db.Model(&paymentModels.UsageMetrics{}).
		Where("metric_type = ?", "concurrent_terminals").
		Count(&count)
	assert.Equal(t, int64(0), count, "second run must remain at 0 (idempotent)")
}
