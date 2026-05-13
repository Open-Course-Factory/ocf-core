// Package observability provides aggregated counters + a hook-error pass-through
// for admin observability. Counters are in-memory atomic — they reset on process
// restart. Logs remain the authoritative trail; counters exist so operators can
// read aggregated failure rates without scraping.
package observability

import "sync/atomic"

// observabilityMetrics aggregates failure signals from across ocf-core's
// async / best-effort paths (Stripe sync, scenario session setup, terminal
// cleanup). All fields are exported so test helpers can reset them.
type observabilityMetrics struct {
	// Stripe sync (per operation kind)
	StripeCreateSuccess  atomic.Uint64
	StripeCreateFailure  atomic.Uint64
	StripeUpdateSuccess  atomic.Uint64
	StripeUpdateFailure  atomic.Uint64
	StripeArchiveSuccess atomic.Uint64
	StripeArchiveFailure atomic.Uint64

	// Stripe sync panic recovery
	StripeSyncPanic atomic.Uint64

	// Stripe sync queue (issue #327)
	// - Retry: bumped on every MarkFailure
	// - Exhausted: bumped when a row transitions to terminal state=failed
	// - PendingDepth: gauge updated by the worker after each pass
	StripeQueueRetry        atomic.Uint64
	StripeQueueExhausted    atomic.Uint64
	StripeQueuePendingDepth atomic.Uint64

	// Scenario session setup failures
	ScenarioSetupPanic           atomic.Uint64
	ScenarioSetupFailed          atomic.Uint64
	TerminalStopOnCleanupFailure atomic.Uint64
}

// Metrics is the global observability metrics singleton, accessed by both
// instrumentation sites (Add) and the admin endpoint (Load).
var Metrics = &observabilityMetrics{}
