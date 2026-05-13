package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"

	"soli/formations/src/observability"
	"soli/formations/src/payment/models"
)

// stripeSyncWorkerPollInterval controls how often the worker polls the queue
// for pending rows. 30s matches the documented "worst-case ~2.5 minutes to
// give up after StripeSyncMaxAttempts (5)" trade-off in models/stripeSync.go.
const stripeSyncWorkerPollInterval = 30 * time.Second

// stripeSyncWorkerBatchSize caps the number of pending rows drained per pass.
// Tunable; production Stripe ops never burst this hard, and a single 30s pass
// is the right granularity if they ever do.
const stripeSyncWorkerBatchSize = 50

// StripeSyncWorker drains the StripeSyncQueue: polls pending rows on a fixed
// interval, dispatches each to the appropriate StripeService method, and
// marks success or failure. Survives process restarts via the durable queue.
//
// Concurrency model: a single goroutine — mirrors tt-backend's cleanupRunner
// pattern. Stripe API calls are serial; a single durable queue + single
// drainer is the simplest correct shape.
type StripeSyncWorker struct {
	queue         StripeSyncQueue
	stripeService StripeService

	cancel   context.CancelFunc
	wg       sync.WaitGroup
	shutOnce sync.Once
}

// NewStripeSyncWorker constructs a worker. Call Start(ctx) to begin polling
// and Shutdown(timeout) to stop gracefully.
func NewStripeSyncWorker(queue StripeSyncQueue, stripeService StripeService) *StripeSyncWorker {
	return &StripeSyncWorker{queue: queue, stripeService: stripeService}
}

// Start spawns the polling goroutine. Returns immediately. The loop's wait
// uses a select on ctx.Done() / ticker (NOT time.Sleep) so Shutdown returns
// promptly.
func (w *StripeSyncWorker) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(stripeSyncWorkerPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.ProcessOnce(ctx); err != nil {
					slog.Error("stripe sync worker pass error", "err", err)
				}
			}
		}
	}()
}

// Shutdown cancels the polling goroutine and waits for it to exit, up to
// timeout. Idempotent.
func (w *StripeSyncWorker) Shutdown(timeout time.Duration) {
	w.shutOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		slog.Warn("stripe sync worker shutdown timed out", "timeout", timeout)
	}
}

// ProcessOnce drains the current batch of pending rows synchronously. Used
// by tests and by Start's polling loop. Returns the first DB-level error
// encountered (per-row Stripe errors are recorded via MarkFailure and do
// NOT cause ProcessOnce to return an error).
func (w *StripeSyncWorker) ProcessOnce(ctx context.Context) error {
	rows, err := w.queue.ListPending(stripeSyncWorkerBatchSize)
	if err != nil {
		return fmt.Errorf("list pending stripe syncs: %w", err)
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.processRow(row)
	}

	// Update the pending-depth gauge after the pass so the observability
	// endpoint surfaces a current value rather than a stale one. A separate
	// query (not len(rows)) because some rows may have been failed-out of
	// pending during the pass.
	if remaining, err := w.queue.ListPending(stripeSyncWorkerBatchSize); err == nil {
		observability.Metrics.StripeQueuePendingDepth.Store(uint64(len(remaining)))
	}

	return nil
}

// processRow handles one queue row: decodes the snapshot, dispatches to the
// correct Stripe method, and records success/failure. Panic-safe: any panic
// from the Stripe SDK is recovered and converted to a MarkFailure.
func (w *StripeSyncWorker) processRow(row *models.StripeSync) {
	var plan models.SubscriptionPlan
	if err := json.Unmarshal([]byte(row.PlanSnapshot), &plan); err != nil {
		w.markFailure(row.ID, fmt.Errorf("decode plan snapshot: %w", err))
		return
	}

	// recover() must be unconditional and at the top so a panic from any
	// downstream Stripe call (including SDK nil-derefs) is caught.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("stripe sync worker panicked",
				"row_id", row.ID,
				"operation", row.Operation,
				"panic", r,
				"stack", string(debug.Stack()))
			observability.Metrics.StripeSyncPanic.Add(1)
			w.markFailure(row.ID, fmt.Errorf("panic recovered: %v", r))
		}
	}()

	var syncErr error
	switch row.Operation {
	case models.StripeSyncOperationCreate:
		syncErr = w.stripeService.CreateSubscriptionPlanInStripe(&plan)
	case models.StripeSyncOperationUpdate:
		syncErr = w.stripeService.UpdateSubscriptionPlanInStripe(&plan)
	case models.StripeSyncOperationArchive:
		if plan.StripeProductID != nil {
			syncErr = w.stripeService.ArchiveSubscriptionPlanInStripe(*plan.StripeProductID)
		}
	default:
		syncErr = fmt.Errorf("unsupported operation: %s", row.Operation)
	}

	if syncErr != nil {
		w.markFailure(row.ID, syncErr)
		w.bumpOperationFailureCounter(row.Operation)
		return
	}

	if err := w.queue.MarkSuccess(row.ID); err != nil {
		slog.Error("mark stripe sync succeeded failed", "row_id", row.ID, "err", err)
		return
	}
	w.bumpOperationSuccessCounter(row.Operation)
}

// markFailure records a per-row failure and bumps observability counters. When
// the row transitions to terminal state=failed (attempts == StripeSyncMaxAttempts),
// also bumps StripeQueueExhausted for operator visibility.
func (w *StripeSyncWorker) markFailure(id uuid.UUID, err error) {
	if mfErr := w.queue.MarkFailure(id, err); mfErr != nil {
		slog.Error("mark stripe sync failed failed", "row_id", id, "original_err", err, "mark_err", mfErr)
		return
	}
	observability.Metrics.StripeQueueRetry.Add(1)

	// If the row just transitioned to terminal failed state, bump the
	// exhausted counter for operator visibility. Single-row re-read; cheap.
	if rows, lookupErr := w.queue.ListByID(id); lookupErr == nil && len(rows) > 0 && rows[0].State == models.StripeSyncStateFailed {
		observability.Metrics.StripeQueueExhausted.Add(1)
	}
}

func (w *StripeSyncWorker) bumpOperationSuccessCounter(operation string) {
	switch operation {
	case models.StripeSyncOperationCreate:
		observability.Metrics.StripeCreateSuccess.Add(1)
	case models.StripeSyncOperationUpdate:
		observability.Metrics.StripeUpdateSuccess.Add(1)
	case models.StripeSyncOperationArchive:
		observability.Metrics.StripeArchiveSuccess.Add(1)
	}
}

func (w *StripeSyncWorker) bumpOperationFailureCounter(operation string) {
	switch operation {
	case models.StripeSyncOperationCreate:
		observability.Metrics.StripeCreateFailure.Add(1)
	case models.StripeSyncOperationUpdate:
		observability.Metrics.StripeUpdateFailure.Add(1)
	case models.StripeSyncOperationArchive:
		observability.Metrics.StripeArchiveFailure.Add(1)
	}
}
