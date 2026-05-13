// tests/payment/stripeSyncWorker_test.go
//
// Failing tests for the StripeSyncWorker — the background drainer that polls
// the StripeSyncQueue and calls the StripeService for each pending row
// (issue #327, MR-O).
//
// The implementer must add:
//   - src/payment/services/stripeSyncWorker.go with:
//       NewStripeSyncWorker(queue StripeSyncQueue, stripeService StripeService) *StripeSyncWorker
//       (*StripeSyncWorker).Start(ctx context.Context)
//       (*StripeSyncWorker).ProcessOnce(ctx context.Context) error
//       (*StripeSyncWorker).Shutdown(timeout time.Duration)
//
// Contract:
//   - ProcessOnce drains all pending rows from the queue in a single pass
//     (calls ListPending, then for each row decodes PlanSnapshot, calls the
//     matching StripeService method, and MarkSuccess / MarkFailure).
//   - On Stripe error, MarkFailure is called and the row remains pending (until
//     StripeSyncMaxAttempts is reached, at which point the queue transitions
//     it to terminal failed — that logic lives in MarkFailure, not the worker).
//   - On panic inside Stripe call, the worker recovers, calls MarkFailure with
//     a panic-flavored error, and continues processing the next row.
//   - Shutdown waits for the background loop to exit within the given timeout.
//     This requires the loop's sleep between polls to be cancellation-aware
//     (a select on ctx.Done(), NOT a bare time.Sleep).
package payment_tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

// TestStripeSyncWorker_ProcessOnce_DrainsPendingRows seeds three pending rows
// (one per operation kind) and verifies ProcessOnce calls the matching Stripe
// method for each and clears them from the pending queue.
func TestStripeSyncWorker_ProcessOnce_DrainsPendingRows(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)
	fake := &fakeStripeService{}
	worker := paymentServices.NewStripeSyncWorker(queue, fake)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "P",
		PriceAmount:     100,
		StripeProductID: stringPtr("prod_test"),
	}

	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue create failed: %v", err)
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationUpdate, plan); err != nil {
		t.Fatalf("Enqueue update failed: %v", err)
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationArchive, plan); err != nil {
		t.Fatalf("Enqueue archive failed: %v", err)
	}

	fake.On("CreateSubscriptionPlanInStripe", mock.Anything).Return(nil)
	fake.On("UpdateSubscriptionPlanInStripe", mock.Anything).Return(nil)
	fake.On("ArchiveSubscriptionPlanInStripe", mock.Anything).Return(nil)

	err := worker.ProcessOnce(context.Background())
	assert.NoError(t, err)

	pending, err := queue.ListPending(10)
	assert.NoError(t, err)
	assert.Empty(t, pending, "all rows should be drained")

	fake.AssertExpectations(t)
}

// TestStripeSyncWorker_ProcessOnce_MarksFailureOnStripeError verifies that
// when the Stripe call returns an error, the row is marked as failure and
// remains pending (retryable) on attempt < StripeSyncMaxAttempts.
func TestStripeSyncWorker_ProcessOnce_MarksFailureOnStripeError(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)
	fake := &fakeStripeService{}
	worker := paymentServices.NewStripeSyncWorker(queue, fake)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	fake.On("CreateSubscriptionPlanInStripe", mock.Anything).Return(errors.New("stripe is sad"))

	_ = worker.ProcessOnce(context.Background())

	pending, _ := queue.ListPending(10)
	if assert.Len(t, pending, 1, "failed row should remain pending (retryable)") {
		assert.Equal(t, 1, pending[0].Attempts)
		assert.Equal(t, "stripe is sad", pending[0].LastError)
	}
}

// TestStripeSyncWorker_ProcessOnce_TransitionsToFailedAfterMaxAttempts runs
// ProcessOnce StripeSyncMaxAttempts times against a row whose Stripe call
// always fails. After the last failure, the row should be terminally failed
// (no longer in pending).
func TestStripeSyncWorker_ProcessOnce_TransitionsToFailedAfterMaxAttempts(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)
	fake := &fakeStripeService{}
	worker := paymentServices.NewStripeSyncWorker(queue, fake)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	fake.On("CreateSubscriptionPlanInStripe", mock.Anything).Return(errors.New("persistent"))

	for i := 0; i < paymentModels.StripeSyncMaxAttempts; i++ {
		_ = worker.ProcessOnce(context.Background())
	}

	pending, _ := queue.ListPending(10)
	assert.Empty(t, pending, "row should be transitioned out of pending after %d failures", paymentModels.StripeSyncMaxAttempts)
}

// TestStripeSyncWorker_ProcessOnce_RecoversFromPanic verifies a panic inside
// the Stripe call does NOT propagate out of ProcessOnce. The row should be
// marked as a failure (still retryable on the first panic).
func TestStripeSyncWorker_ProcessOnce_RecoversFromPanic(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)
	fake := &fakeStripeService{}
	worker := paymentServices.NewStripeSyncWorker(queue, fake)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	fake.On("CreateSubscriptionPlanInStripe", mock.Anything).Run(func(args mock.Arguments) {
		panic("simulated stripe SDK nil-deref")
	}).Return(nil)

	assert.NotPanics(t, func() {
		_ = worker.ProcessOnce(context.Background())
	})

	pending, _ := queue.ListPending(10)
	if assert.Len(t, pending, 1, "panicked row should be marked as failure, still retryable") {
		assert.Equal(t, 1, pending[0].Attempts)
		assert.NotEmpty(t, pending[0].LastError, "LastError should be populated by the panic recovery path")
	}
}

// TestStripeSyncWorker_Shutdown_ReturnsWithinTimeout ensures the background
// loop's poll sleep is cancellation-aware (select on ctx.Done(), not a bare
// time.Sleep). If the implementer uses time.Sleep(pollInterval) at the top of
// the loop, Shutdown will block until the next tick — this test fails.
func TestStripeSyncWorker_Shutdown_ReturnsWithinTimeout(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)
	fake := &fakeStripeService{}
	worker := paymentServices.NewStripeSyncWorker(queue, fake)

	worker.Start(context.Background())
	// Let the goroutine reach its select on ctx.Done() / tick.
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		worker.Shutdown(2 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		// success — Shutdown returned promptly
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not return within 3s — goroutine likely blocked on time.Sleep instead of select")
	}
}
