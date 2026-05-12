// tests/payment/stripeSyncQueue_test.go
// Failing tests for the StripeSyncQueue service (issue #326).
//
// The implementer must add:
//   - src/payment/models/stripeSync.go     — StripeSync GORM model
//   - src/payment/services/stripeSyncQueue.go — StripeSyncQueue interface + impl
//
// Expected constants on the models package:
//   - StripeSyncMaxAttempts (int = 5)
//   - StripeSyncStatePending   ("pending")
//   - StripeSyncStateSucceeded ("succeeded")
//   - StripeSyncStateFailed    ("failed")
//   - StripeSyncOperationCreate  ("create")
//   - StripeSyncOperationUpdate  ("update")
//   - StripeSyncOperationArchive ("archive")
//
// Expected fields on the StripeSync model:
//   - ID            uuid.UUID
//   - PlanID        uuid.UUID
//   - Operation     string
//   - State         string
//   - Attempts      int
//   - LastError     string
//   - LastAttemptAt *time.Time
//   - PlanSnapshot  string (JSON of the SubscriptionPlan at enqueue time)
//
// Expected constructor: paymentServices.NewStripeSyncQueue(db *gorm.DB) StripeSyncQueue.
package payment_tests

import (
	"encoding/json"
	"errors"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
)

// TestStripeSyncQueue_Enqueue_StoresPendingRowWithDeserializablePlanSnapshot verifies
// that enqueueing a sync operation persists a pending row whose PlanSnapshot can be
// deserialized back into the original SubscriptionPlan.
func TestStripeSyncQueue_Enqueue_StoresPendingRowWithDeserializablePlanSnapshot(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)

	planID := uuid.New()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: planID},
		Name:        "Test Plan",
		PriceAmount: 1000,
	}

	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	pending, err := queue.ListPending(10)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending row, got %d", len(pending))
	}

	row := pending[0]
	if row.State != paymentModels.StripeSyncStatePending {
		t.Errorf("state should be pending, got %q", row.State)
	}
	if row.Operation != paymentModels.StripeSyncOperationCreate {
		t.Errorf("operation should be 'create', got %q", row.Operation)
	}
	if row.PlanID != planID {
		t.Errorf("plan_id mismatch: want %s, got %s", planID, row.PlanID)
	}
	if row.Attempts != 0 {
		t.Errorf("attempts should be 0 on fresh enqueue, got %d", row.Attempts)
	}

	// Snapshot must be a deserializable SubscriptionPlan with the same fields.
	var restored paymentModels.SubscriptionPlan
	if err := json.Unmarshal([]byte(row.PlanSnapshot), &restored); err != nil {
		t.Fatalf("PlanSnapshot is not valid JSON of SubscriptionPlan: %v", err)
	}
	if restored.ID != planID || restored.Name != plan.Name || restored.PriceAmount != plan.PriceAmount {
		t.Errorf("restored plan differs from enqueued: %+v vs %+v", restored, plan)
	}
}

// TestStripeSyncQueue_MarkSuccess_RemovesFromPending verifies that marking a row
// as succeeded transitions its state and excludes it from future ListPending results.
func TestStripeSyncQueue_MarkSuccess_RemovesFromPending(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	pending, _ := queue.ListPending(10)
	if len(pending) != 1 {
		t.Fatalf("setup: expected 1 pending, got %d", len(pending))
	}
	rowID := pending[0].ID

	if err := queue.MarkSuccess(rowID); err != nil {
		t.Fatalf("MarkSuccess failed: %v", err)
	}

	afterSuccess, _ := queue.ListPending(10)
	if len(afterSuccess) != 0 {
		t.Errorf("expected 0 pending after MarkSuccess, got %d", len(afterSuccess))
	}

	// Also verify the row state directly.
	var row paymentModels.StripeSync
	db.First(&row, "id = ?", rowID)
	if row.State != paymentModels.StripeSyncStateSucceeded {
		t.Errorf("state should be succeeded, got %q", row.State)
	}
	if row.LastAttemptAt == nil {
		t.Error("LastAttemptAt should be set on MarkSuccess")
	}
}

// TestStripeSyncQueue_MarkFailure_KeepsRetryableBeforeMaxAttempts verifies that
// a single failure increments the attempt counter, records the error, but keeps
// the row in pending state so a future worker pass can retry it.
func TestStripeSyncQueue_MarkFailure_KeepsRetryableBeforeMaxAttempts(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationUpdate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	pending, _ := queue.ListPending(10)
	if len(pending) != 1 {
		t.Fatalf("setup: expected 1 pending, got %d", len(pending))
	}
	rowID := pending[0].ID

	if err := queue.MarkFailure(rowID, errors.New("stripe is sad")); err != nil {
		t.Fatalf("MarkFailure failed: %v", err)
	}

	afterOneFailure, _ := queue.ListPending(10)
	if len(afterOneFailure) != 1 {
		t.Errorf("expected 1 pending after 1 failure (retryable), got %d", len(afterOneFailure))
	}

	var row paymentModels.StripeSync
	db.First(&row, "id = ?", rowID)
	if row.Attempts != 1 {
		t.Errorf("attempts should be 1, got %d", row.Attempts)
	}
	if row.State != paymentModels.StripeSyncStatePending {
		t.Errorf("state should be pending after 1 failure (retryable), got %q", row.State)
	}
	if row.LastError != "stripe is sad" {
		t.Errorf("LastError should be 'stripe is sad', got %q", row.LastError)
	}
	if row.LastAttemptAt == nil {
		t.Error("LastAttemptAt should be set")
	}
}

// TestStripeSyncQueue_MarkFailure_TransitionsToFailedAfterMaxAttempts verifies that
// once the configured MaxAttempts has been hit, the row leaves the pending pool
// and is marked terminally failed.
func TestStripeSyncQueue_MarkFailure_TransitionsToFailedAfterMaxAttempts(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationArchive, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	pending, _ := queue.ListPending(10)
	if len(pending) != 1 {
		t.Fatalf("setup: expected 1 pending, got %d", len(pending))
	}
	rowID := pending[0].ID

	// Fail MaxAttempts times.
	for i := 0; i < paymentModels.StripeSyncMaxAttempts; i++ {
		if err := queue.MarkFailure(rowID, errors.New("persistent fail")); err != nil {
			t.Fatalf("MarkFailure attempt %d failed: %v", i+1, err)
		}
	}

	// ListPending must NOT return it now.
	afterAll, _ := queue.ListPending(10)
	if len(afterAll) != 0 {
		t.Errorf("expected 0 pending after %d failures, got %d", paymentModels.StripeSyncMaxAttempts, len(afterAll))
	}

	// Row state should be terminal failed.
	var row paymentModels.StripeSync
	db.First(&row, "id = ?", rowID)
	if row.State != paymentModels.StripeSyncStateFailed {
		t.Errorf("state should be failed after %d attempts, got %q", paymentModels.StripeSyncMaxAttempts, row.State)
	}
	if row.Attempts != paymentModels.StripeSyncMaxAttempts {
		t.Errorf("attempts should be %d, got %d", paymentModels.StripeSyncMaxAttempts, row.Attempts)
	}
}
