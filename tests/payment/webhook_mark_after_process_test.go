// tests/payment/webhook_mark_after_process_test.go
//
// Structural regression tests that verify the webhook controller processes
// events SYNCHRONOUSLY and marks them as processed ONLY AFTER success.
//
// The critical invariant:
//   - Success: process -> mark as processed -> return 200
//   - Failure: process -> DON'T mark -> return 500 (Stripe retries)
//
// These tests read the actual source code to detect dangerous patterns,
// preventing accidental reversion to the "mark-before-process" flow.
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

// TestWebhook_ProcessBeforeMarkOrder verifies that ProcessWebhook is called
// BEFORE markEventProcessed in the webhook handler source code.
func TestWebhook_ProcessBeforeMarkOrder(t *testing.T) {
	source := readWebhookControllerSource(t)

	// Find positions in the full source
	processIdx := strings.Index(source, "ProcessWebhook(payload")
	markIdx := strings.Index(source, "markEventProcessed(event.ID)")

	require.Greater(t, processIdx, 0, "ProcessWebhook call should exist in source")
	require.Greater(t, markIdx, 0, "markEventProcessed call should exist in source")

	assert.Less(t, processIdx, markIdx,
		"SECURITY: ProcessWebhook must be called BEFORE markEventProcessed. "+
			"The mark-before-process pattern causes silent data loss: if processing "+
			"fails after marking, Stripe won't retry because the event appears 'processed'. "+
			"Subscriptions may never activate after payment.")
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

// TestWebhook_FailureDoesNotMark verifies that when processing fails,
// the handler returns BEFORE calling markEventProcessed. This ensures
// failed events are not marked as processed.
func TestWebhook_FailureDoesNotMark(t *testing.T) {
	source := readWebhookControllerSource(t)
	handlerBody := extractHandlerBody(t, source)

	// After ProcessWebhook fails, there should be a "return" before markEventProcessed
	processIdx := strings.Index(handlerBody, "ProcessWebhook(payload")
	require.Greater(t, processIdx, 0)

	// Find the error handling block after ProcessWebhook
	afterProcess := handlerBody[processIdx:]
	returnIdx := strings.Index(afterProcess, "return\n")
	markIdx := strings.Index(afterProcess, "markEventProcessed")

	require.Greater(t, returnIdx, 0, "Should have a return statement after error check")
	require.Greater(t, markIdx, 0, "Should have markEventProcessed after success path")

	assert.Less(t, returnIdx, markIdx,
		"SECURITY: When ProcessWebhook fails, the handler must return BEFORE "+
			"calling markEventProcessed. The 'return' in the error path must come "+
			"before the mark call in the success path.")
}
