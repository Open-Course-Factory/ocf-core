// tests/payment/webhook_mark_after_process_test.go
//
// Structural regression tests that verify the webhook controller processes
// events SYNCHRONOUSLY under the reserve-then-process flow introduced for
// GitLab issue #260.
//
// The critical invariants:
//   - Events are reserved BEFORE ProcessWebhook runs (prevents concurrent dup).
//   - On ProcessWebhook failure the reservation is released so Stripe's retry
//     is not swallowed as a duplicate; handler returns 500.
//   - On ProcessWebhook success the reservation row stays; handler returns 200.
//   - No async goroutine: everything runs synchronously under Stripe's 20s
//     response window.
//
// These tests read the actual source code to detect dangerous patterns,
// preventing accidental reversion to the "check-then-act" flow (SELECT then
// INSERT) or to a mark-before-process variant.
package payment_tests

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readWebhookControllerSource reads the webhook controller source file.
func readWebhookControllerSource(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../src/payment/routes/webHookController.go")
	require.NoError(t, err, "Failed to read webhook controller source")
	return string(content)
}

// extractHandlerBody extracts the HandleStripeWebhook method body from source.
func extractHandlerBody(t *testing.T, source string) string {
	t.Helper()
	handlerStart := strings.Index(source, "func (wc *webhookController) HandleStripeWebhook")
	require.Greater(t, handlerStart, 0, "HandleStripeWebhook should exist in source")

	handlerBody := source[handlerStart:]
	nextFunc := strings.Index(handlerBody[1:], "\nfunc ")
	if nextFunc > 0 {
		handlerBody = handlerBody[:nextFunc+1]
	}
	return handlerBody
}

// TestWebhook_ReserveBeforeProcessOrder verifies that the reservation is
// obtained BEFORE ProcessWebhook is invoked. The reverse order would recreate
// the race condition fixed in issue #260: two concurrent pods could both
// invoke the handler before either recorded a row.
func TestWebhook_ReserveBeforeProcessOrder(t *testing.T) {
	source := readWebhookControllerSource(t)

	reserveIdx := strings.Index(source, "reserveEvent(event.ID")
	processIdx := strings.Index(source, "ProcessWebhook(payload")

	require.Greater(t, reserveIdx, 0, "reserveEvent call should exist in source")
	require.Greater(t, processIdx, 0, "ProcessWebhook call should exist in source")

	assert.Less(t, reserveIdx, processIdx,
		"SECURITY: reserveEvent must be called BEFORE ProcessWebhook. "+
			"Reversing the order reintroduces the concurrent-duplicate race "+
			"(two deliveries can both pass the reservation check before either "+
			"records a row). Fix: atomic INSERT ... ON CONFLICT DO NOTHING "+
			"must run first, and only the caller with RowsAffected == 1 may "+
			"proceed to ProcessWebhook.")
}

// TestWebhook_ReservationIsAtomic verifies the reservation is performed
// inside a DB transaction (atomic SELECT-then-INSERT/UPDATE) rather than
// a plain check-then-act outside any transaction. The unique index on
// event_id resolves any racing concurrent INSERTs.
//
// This replaces the old "must use OnConflict{DoNothing}" check: the new
// status-aware reservation needs to also handle status=failed -> reserved
// transitions, which a plain ON CONFLICT DO NOTHING cannot express.
func TestWebhook_ReservationIsAtomic(t *testing.T) {
	source := readWebhookControllerSource(t)

	// Locate reserveEvent and verify it uses a transaction.
	reserveStart := strings.Index(source, "func (wc *webhookController) reserveEvent(")
	require.Greater(t, reserveStart, 0, "reserveEvent function must exist")

	reserveBody := source[reserveStart:]
	if next := strings.Index(reserveBody[1:], "\nfunc "); next > 0 {
		reserveBody = reserveBody[:next+1]
	}

	assert.True(t, strings.Contains(reserveBody, "wc.db.Transaction("),
		"SECURITY: reserveEvent must run its SELECT-then-INSERT/UPDATE inside "+
			"a DB transaction (wc.db.Transaction(...)) so the reservation is "+
			"atomic. A plain check-then-act outside a transaction is racy — "+
			"two concurrent pods could both observe 'not present' and both "+
			"insert. The unique index on event_id then resolves the loser.")
}

// TestWebhook_NoAsyncGoroutine verifies that the webhook handler does NOT
// use goroutines for processing. Async processing combined with immediate
// 200 responses means Stripe considers the event delivered even if processing
// crashes, panics, or the server restarts mid-processing.
func TestWebhook_NoAsyncGoroutine(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	assert.False(t, strings.Contains(handlerBody, "go func()"),
		"SECURITY: HandleStripeWebhook must NOT use async goroutines (go func()). "+
			"Stripe allows 20 seconds for webhook responses — plenty for DB operations. "+
			"Async processing means Stripe gets 200 immediately, won't retry on failure, "+
			"and webhook data is permanently lost if the goroutine crashes.")
}

// TestWebhook_FailureReturns500 verifies that the handler returns a 500 error
// when ProcessWebhook fails. This is critical: Stripe retries webhooks that
// receive 5xx responses, which is the recovery mechanism for transient failures.
func TestWebhook_FailureReturns500(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	assert.True(t, strings.Contains(handlerBody, "StatusInternalServerError"),
		"Handler must return 500 (StatusInternalServerError) when ProcessWebhook fails. "+
			"Stripe retries webhooks that receive 5xx responses — this is the recovery "+
			"mechanism for transient failures (DB outage, network issues, etc.).")
}

// TestWebhook_FailureMarksReservationFailed verifies that when ProcessWebhook
// fails, the handler marks the reservation as failed (status=failed) so
// Stripe's retry can re-claim and re-enter the critical section. Skipping
// this transition would either swallow retries as duplicates (if the row
// stayed in status=reserved) or duplicate side effects (if the row were
// hard-deleted but the delete itself failed).
func TestWebhook_FailureMarksReservationFailed(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	processIdx := strings.Index(handlerBody, "ProcessWebhook(payload")
	require.Greater(t, processIdx, 0, "ProcessWebhook call should exist")

	afterProcess := handlerBody[processIdx:]

	markFailedIdx := strings.Index(afterProcess, "markFailed(event.ID")
	require.Greater(t, markFailedIdx, 0,
		"Handler must call markFailed in the failure path so Stripe's "+
			"retry is not swallowed as a duplicate. The row must transition "+
			"reserved -> failed (re-reservable), NOT be hard-deleted.")

	// The mark must precede the 500 return (i.e. appear before the error
	// response is sent).
	errResponseIdx := strings.Index(afterProcess, "StatusInternalServerError")
	require.Greater(t, errResponseIdx, 0)

	assert.Less(t, markFailedIdx, errResponseIdx,
		"SECURITY: markFailed must be called BEFORE returning 500. "+
			"Otherwise the reservation row stays as 'reserved' and Stripe's "+
			"retry is swallowed as 'already reserved', leaving the underlying "+
			"event permanently un-applied.")
}

// TestWebhook_SuccessMarksReservationProcessed verifies that on a successful
// ProcessWebhook the reservation row is transitioned to status=processed
// (terminal state) so future deliveries short-circuit on the row.
func TestWebhook_SuccessMarksReservationProcessed(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	assert.True(t, strings.Contains(handlerBody, "markProcessed(event.ID"),
		"Handler must call markProcessed after ProcessWebhook succeeds so "+
			"the row transitions reserved -> processed. Without this, future "+
			"deliveries would see status=reserved and short-circuit, but "+
			"this 'soft success' would lose visibility into the terminal state.")
}
