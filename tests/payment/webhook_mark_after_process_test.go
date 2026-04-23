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

// TestWebhook_ReservationUsesOnConflictDoNothing verifies the reservation is
// atomic (ON CONFLICT DO NOTHING) rather than a check-then-act pattern.
func TestWebhook_ReservationUsesOnConflictDoNothing(t *testing.T) {
	source := readWebhookControllerSource(t)

	assert.True(t, strings.Contains(source, "OnConflict{DoNothing: true}"),
		"SECURITY: reserveEvent must use clause.OnConflict{DoNothing: true} "+
			"to atomically claim the event. A plain Create or a SELECT-then-"+
			"INSERT is racy — two concurrent pods could both observe 'not "+
			"processed' and both insert.")
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

// TestWebhook_FailureReleasesReservation verifies that when ProcessWebhook
// fails, the handler releases the reservation so Stripe's retry can
// re-enter the critical section. Skipping the release would cause retries
// to be silently swallowed as duplicates.
func TestWebhook_FailureReleasesReservation(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	processIdx := strings.Index(handlerBody, "ProcessWebhook(payload")
	require.Greater(t, processIdx, 0, "ProcessWebhook call should exist")

	afterProcess := handlerBody[processIdx:]

	releaseIdx := strings.Index(afterProcess, "releaseReservation(event.ID")
	require.Greater(t, releaseIdx, 0,
		"Handler must call releaseReservation in the failure path so Stripe's "+
			"retry is not swallowed as a duplicate.")

	// The release must precede the 500 return (i.e. appear before the error
	// response is sent).
	errResponseIdx := strings.Index(afterProcess, "StatusInternalServerError")
	require.Greater(t, errResponseIdx, 0)

	assert.Less(t, releaseIdx, errResponseIdx,
		"SECURITY: releaseReservation must be called BEFORE returning 500. "+
			"Otherwise the reservation row persists and Stripe's retry is "+
			"swallowed as 'already processed', leaving the underlying event "+
			"permanently un-applied.")
}
