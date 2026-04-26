// tests/payment/webhookIdempotency_test.go
//
// Failing tests for the webhook idempotency race condition (GitLab issue #260).
//
// The current `HandleStripeWebhook` flow is:
//   1. isEventProcessed(event.ID)   -- SELECT
//   2. ProcessWebhook(...)          -- runs handler
//   3. markEventProcessed(event.ID) -- INSERT
//
// Between steps 1 and 3, a concurrent pod can also pass step 1 and also
// run step 2, resulting in two handler invocations and potentially duplicate
// side effects (duplicate UserSubscription rows, double-granted entitlements,
// etc.).
//
// The planned fix ("reserve-then-process") will replace the flow with a single
// INSERT ... ON CONFLICT DO NOTHING that atomically reserves the event_id.
// Only the pod that successfully reserves the row runs ProcessWebhook. The
// handlers themselves will also be made idempotent (check for existing
// UserSubscription with the same stripe_subscription_id before Create).
//
// These tests will fail on main; they drive the fix.
package payment_tests

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	paymentController "soli/formations/src/payment/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
	stripeWebhook "github.com/stripe/stripe-go/v82/webhook"
	"gorm.io/gorm"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// -----------------------------------------------------------------------------
// Mock StripeService fully implementing services.StripeService
// -----------------------------------------------------------------------------
//
// Used to drive the webhook controller end-to-end for concurrency tests.
// Only ValidateWebhookSignature and ProcessWebhook are exercised; the rest of
// the interface is stubbed so the mock satisfies services.StripeService and
// can be injected via NewWebhookControllerWithService.

type webhookTestStripeService struct {
	// Deterministic behavior for ValidateWebhookSignature.
	eventID     string
	validateErr error

	// Counter for how many times ProcessWebhook was invoked.
	processCalls atomic.Int32

	// Controls ProcessWebhook behavior.
	processSleep time.Duration // widens the race window
	processErr   error         // single-value behavior
	processErrs  []error       // per-call behavior; index = callN-1 (clamped)
}

func (m *webhookTestStripeService) ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	if m.validateErr != nil {
		return nil, m.validateErr
	}
	return &stripe.Event{
		ID:      m.eventID,
		Type:    stripe.EventType("customer.subscription.created"),
		Created: time.Now().Unix(),
	}, nil
}

func (m *webhookTestStripeService) ProcessWebhook(payload []byte, signature string) error {
	n := m.processCalls.Add(1)
	if m.processSleep > 0 {
		time.Sleep(m.processSleep)
	}
	if len(m.processErrs) > 0 {
		idx := int(n) - 1
		if idx >= len(m.processErrs) {
			idx = len(m.processErrs) - 1
		}
		return m.processErrs[idx]
	}
	return m.processErr
}

// --- unused interface methods (stubs) ----------------------------------------

func (m *webhookTestStripeService) CreateOrGetCustomer(userID, email, name string) (string, error) {
	return "", nil
}
func (m *webhookTestStripeService) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	return nil
}
func (m *webhookTestStripeService) CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput, replaceSubscriptionID *uuid.UUID) (*dto.CheckoutSessionOutput, error) {
	return nil, nil
}
func (m *webhookTestStripeService) CreateBulkCheckoutSession(userID string, input dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	return nil, nil
}
func (m *webhookTestStripeService) CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error) {
	return nil, nil
}
func (m *webhookTestStripeService) CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	return nil
}
func (m *webhookTestStripeService) UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	return nil
}
func (m *webhookTestStripeService) ArchiveSubscriptionPlanInStripe(productID string) error {
	return nil
}
func (m *webhookTestStripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	return nil
}
func (m *webhookTestStripeService) MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error {
	return nil
}
func (m *webhookTestStripeService) ReactivateSubscription(subscriptionID string) error {
	return nil
}
func (m *webhookTestStripeService) UpdateSubscription(subscriptionID, newPriceID, prorationBehavior string) (*stripe.Subscription, error) {
	return nil, nil
}
func (m *webhookTestStripeService) SyncExistingSubscriptions() (*services.SyncSubscriptionsResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) SyncUserSubscriptions(userID string) (*services.SyncSubscriptionsResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) SyncSubscriptionsWithMissingMetadata() (*services.SyncSubscriptionsResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID uuid.UUID) error {
	return nil
}
func (m *webhookTestStripeService) SyncUserInvoices(userID string) (*services.SyncInvoicesResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) CleanupIncompleteInvoices(input dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) SyncUserPaymentMethods(userID string) (*services.SyncPaymentMethodsResult, error) {
	return nil, nil
}
func (m *webhookTestStripeService) AttachPaymentMethod(paymentMethodID, customerID string) error {
	return nil
}
func (m *webhookTestStripeService) DetachPaymentMethod(paymentMethodID string) error {
	return nil
}
func (m *webhookTestStripeService) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	return nil
}
func (m *webhookTestStripeService) GetInvoice(invoiceID string) (*stripe.Invoice, error) {
	return nil, nil
}
func (m *webhookTestStripeService) SendInvoice(invoiceID string) error { return nil }
func (m *webhookTestStripeService) CreateSubscriptionWithQuantity(customerID string, plan *models.SubscriptionPlan, quantity int, paymentMethodID string) (*stripe.Subscription, error) {
	return nil, nil
}
func (m *webhookTestStripeService) UpdateSubscriptionQuantity(subscriptionID string, subscriptionItemID string, newQuantity int) (*stripe.Subscription, error) {
	return nil, nil
}
func (m *webhookTestStripeService) ImportPlansFromStripe() (*services.SyncPlansResult, error) {
	return nil, nil
}

// Compile-time check: satisfies services.StripeService.
var _ services.StripeService = (*webhookTestStripeService)(nil)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// buildWebhookRequest builds a Stripe-looking POST request for /webhooks/stripe.
// Headers match what the controller's basicSecurityChecks expects.
func buildWebhookRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Stripe/1.0")
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef") // mock service ignores
	return req
}

// buildSignedWebhookRequest builds a webhook request with a VALID Stripe
// signature computed against the given secret. Used by handler-level tests
// that invoke the real ValidateWebhookSignature.
func buildSignedWebhookRequest(t *testing.T, payload []byte, secret string) *http.Request {
	t.Helper()
	ts := time.Now()
	sig := stripeWebhook.ComputeSignature(ts, payload, secret)
	header := fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(sig))
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Stripe/1.0")
	req.Header.Set("Stripe-Signature", header)
	return req
}

// newRouterWithMockService wires the real webhook controller into gin using
// our fully-mocked StripeService (for concurrency tests).
func newRouterWithMockService(db *gorm.DB, stripeSvc services.StripeService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := paymentController.NewWebhookControllerWithService(db, stripeSvc)
	r.POST("/webhooks/stripe", ctrl.HandleStripeWebhook)
	return r
}

// newRouterWithRealService wires the real webhook controller with the REAL
// StripeService (so ProcessWebhook actually runs the handlers). The test
// must provide the webhook secret that matches signed payloads.
func newRouterWithRealService(t *testing.T, db *gorm.DB, webhookSecret string) *gin.Engine {
	t.Helper()
	prev := os.Getenv("STRIPE_WEBHOOK_SECRET")
	os.Setenv("STRIPE_WEBHOOK_SECRET", webhookSecret)
	t.Cleanup(func() { os.Setenv("STRIPE_WEBHOOK_SECRET", prev) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := paymentController.NewWebhookController(db)
	r.POST("/webhooks/stripe", ctrl.HandleStripeWebhook)
	return r
}

// countWebhookEvents returns the row count in webhook_events for a given event_id.
func countWebhookEvents(t *testing.T, db *gorm.DB, eventID string) int64 {
	t.Helper()
	var count int64
	err := db.Model(&models.WebhookEvent{}).
		Where("event_id = ?", eventID).
		Count(&count).Error
	require.NoError(t, err)
	return count
}

// -----------------------------------------------------------------------------
// TESTS
// -----------------------------------------------------------------------------

// TestWebhook_ConcurrentDuplicateEvent_OnlyProcessedOnce documents the core race.
//
// Two goroutines POST the same event_id simultaneously. ProcessWebhook sleeps
// briefly to widen the window between isEventProcessed (SELECT) and
// markEventProcessed (INSERT). Under the current flow both goroutines pass
// the SELECT and both call ProcessWebhook — duplicating whatever side effects
// ProcessWebhook has (e.g. UserSubscription rows, entitlements).
//
// After the fix, exactly one ProcessWebhook invocation is expected. The losing
// goroutine should observe the reservation and return 200 "already processed"
// without calling ProcessWebhook.
func TestWebhook_ConcurrentDuplicateEvent_OnlyProcessedOnce(t *testing.T) {
	// Use an isolated DB with MaxOpenConns=1 for this concurrent writer test.
	// We can't cap the shared DB globally because other tests (notably
	// TestCheckLimit_UsesContextPlan_SkipsPlanResolution) use internal GORM
	// transactions that would deadlock on a single-connection pool.
	db := cappedTestDB(t, "webhook_race_"+t.Name())
	eventID := "evt_race_" + uuid.NewString()

	mockSvc := &webhookTestStripeService{
		eventID:      eventID,
		processSleep: 100 * time.Millisecond, // widen the race window
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))

	// Release both goroutines at the same instant.
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			w := httptest.NewRecorder()
			router.ServeHTTP(w, buildWebhookRequest(t, payload))
			results[idx] = w.Code
		}(i)
	}
	close(start)
	wg.Wait()

	// Both responses should be 200 OK (one processed, one "already reserved"/"already processed").
	assert.Equal(t, http.StatusOK, results[0], "first goroutine should get 200")
	assert.Equal(t, http.StatusOK, results[1], "second goroutine should get 200")

	// Critical invariant: ProcessWebhook must run exactly once.
	// WITHOUT THE FIX: both goroutines pass isEventProcessed(SELECT) before
	// either runs markEventProcessed(INSERT), so ProcessWebhook runs twice.
	assert.Equal(t, int32(1), mockSvc.processCalls.Load(),
		"RACE: ProcessWebhook must be called exactly once for a given event_id "+
			"even under concurrent delivery. Current code uses a check-then-act "+
			"pattern (SELECT ... then INSERT) which allows two pods to both pass "+
			"the check and both run the handler. Fix: reserve the event with an "+
			"atomic INSERT ... ON CONFLICT DO NOTHING before processing.")

	// And exactly one row in webhook_events for this event_id.
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"exactly one webhook_events row should exist for the event_id")
}

// TestWebhook_ReservedThenProcessingFails_AllowsRetry locks in the retry
// behavior under the reserve-then-process flow.
//
// First delivery: ProcessWebhook fails -> 500 response, NO permanent
// reservation (so Stripe can retry). Second delivery: ProcessWebhook succeeds
// -> 200, one webhook_events row.
//
// Under the planned fix, the controller will INSERT the reservation first,
// then DELETE it if processing fails. The test asserts the externally
// observable behavior: two ProcessWebhook calls (one failed + one successful
// retry), one row at the end, no duplicate side effects.
func TestWebhook_ReservedThenProcessingFails_AllowsRetry(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_retry_" + uuid.NewString()

	processErr := errors.New("transient downstream error")
	mockSvc := &webhookTestStripeService{
		eventID:     eventID,
		processErrs: []error{processErr, nil}, // fail first, succeed second
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))

	// First delivery: expect 500.
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, buildWebhookRequest(t, payload))
	assert.Equal(t, http.StatusInternalServerError, w1.Code,
		"first delivery (handler failure) should return 500 so Stripe retries")

	// After a failed delivery there must be NO permanent reservation, else
	// the retry would be silently dropped as "already processed".
	assert.Equal(t, int64(0), countWebhookEvents(t, db, eventID),
		"after processing failure, no webhook_events row should persist "+
			"(otherwise Stripe's retry would be silently swallowed as duplicate)")

	// Second delivery (Stripe retry): expect 200 and a successful reprocess.
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, buildWebhookRequest(t, payload))
	assert.Equal(t, http.StatusOK, w2.Code, "retry should succeed with 200")

	// ProcessWebhook must have been called twice total (once failed, once ok).
	assert.Equal(t, int32(2), mockSvc.processCalls.Load(),
		"ProcessWebhook must be called twice: once on initial failure, once on retry")

	// Exactly one row at the end (the successful retry's reservation).
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"after successful retry, exactly one webhook_events row should exist")
}

// TestWebhook_DuplicateEventID_AfterSuccess_ReturnsIdempotentOK is a
// regression guard for the non-concurrent duplicate case. This already works
// today (the SELECT-before-INSERT check catches sequential duplicates) — the
// test ensures the fix does not regress this behavior.
func TestWebhook_DuplicateEventID_AfterSuccess_ReturnsIdempotentOK(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_seq_dup_" + uuid.NewString()

	mockSvc := &webhookTestStripeService{
		eventID: eventID,
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))

	// First delivery: processes successfully.
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, buildWebhookRequest(t, payload))
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second delivery (sequential, same event_id): 200 OK, handler NOT called.
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, buildWebhookRequest(t, payload))
	assert.Equal(t, http.StatusOK, w2.Code)

	// Handler called exactly once.
	assert.Equal(t, int32(1), mockSvc.processCalls.Load(),
		"sequential duplicate delivery must NOT re-invoke ProcessWebhook")

	// Exactly one row.
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID))
}

// TestHandleSubscriptionCreated_DuplicateStripeSubID_IsIdempotent asserts
// handler-level idempotency for `customer.subscription.created`.
//
// Scenario: Stripe delivers two events with the same stripe_subscription_id
// but different event_ids (which does happen — e.g. after a reservation is
// cleaned up following an unrelated transient failure, or when an upstream
// replay tool re-emits the underlying subscription event). The handler must
// detect the existing UserSubscription row by StripeSubscriptionID and no-op
// (or update in place), NOT return an error from a failed INSERT.
//
// Today: `handleSubscriptionCreated` (stripeService.go:651) blindly calls
// repository.CreateUserSubscription which hits the partial unique index
// `idx_user_stripe_sub_not_null` on UserSubscription.StripeSubscriptionID and
// returns an error. That error propagates to ProcessWebhook and the controller
// returns 500 -> Stripe retries -> same error forever.
//
// This test drives the handler end-to-end through the real controller with
// valid Stripe webhook signatures. The first delivery should succeed and
// create one UserSubscription. The second delivery (different event_id, same
// stripe_subscription_id) MUST return 200 and still result in exactly one
// UserSubscription row.
func TestHandleSubscriptionCreated_DuplicateStripeSubID_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)
	webhookSecret := "whsec_test_" + uuid.NewString()

	router := newRouterWithRealService(t, db, webhookSecret)

	// Seed a subscription plan so the handler's plan lookup succeeds.
	stripePriceID := "price_idempotency_" + uuid.NewString()
	plan := &models.SubscriptionPlan{
		Name:            "Test Idempotency Plan",
		Description:     "test plan for webhook idempotency",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		StripePriceID:   &stripePriceID,
		IsActive:        true,
	}
	require.NoError(t, db.Create(plan).Error)

	stripeSubID := "sub_" + uuid.NewString()

	buildEventPayload := func(eventID string) []byte {
		now := time.Now().Unix()
		end := time.Now().Add(30 * 24 * time.Hour).Unix()
		return []byte(fmt.Sprintf(`{
			"id": %q,
			"object": "event",
			"api_version": %q,
			"type": "customer.subscription.created",
			"created": %d,
			"data": {
				"object": {
					"id": %q,
					"object": "subscription",
					"customer": {"id": "cus_idem", "object": "customer"},
					"status": "active",
					"cancel_at_period_end": false,
					"metadata": {"user_id": "user_idem"},
					"items": {
						"object": "list",
						"data": [{
							"id": "si_idem",
							"object": "subscription_item",
							"current_period_start": %d,
							"current_period_end": %d,
							"price": {"id": %q, "object": "price", "currency": "eur", "unit_amount": 1999}
						}]
					}
				}
			}
		}`, eventID, stripe.APIVersion, now, stripeSubID, now, end, stripePriceID))
	}

	// First delivery: should succeed.
	event1 := "evt_first_" + uuid.NewString()
	payload1 := buildEventPayload(event1)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, buildSignedWebhookRequest(t, payload1, webhookSecret))
	require.Equal(t, http.StatusOK, w1.Code,
		"first subscription-created delivery should succeed (body: %s)", w1.Body.String())

	// Second delivery: DIFFERENT event_id, SAME stripe_subscription_id.
	// The handler must detect the existing row and no-op, not hit the unique
	// index.
	event2 := "evt_second_" + uuid.NewString()
	payload2 := buildEventPayload(event2)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, buildSignedWebhookRequest(t, payload2, webhookSecret))

	assert.Equal(t, http.StatusOK, w2.Code,
		"IDEMPOTENCY: second delivery with same stripe_subscription_id must "+
			"return 200. Currently the handler calls CreateUserSubscription "+
			"unconditionally, which violates the partial unique index "+
			"idx_user_stripe_sub_not_null and surfaces as 500 to Stripe. "+
			"Fix: check for an existing row by StripeSubscriptionID before "+
			"Create (no-op or update in place). Response body: %s", w2.Body.String())

	// And exactly one UserSubscription row for this stripe_subscription_id.
	var count int64
	require.NoError(t, db.Model(&models.UserSubscription{}).
		Where("stripe_subscription_id = ?", stripeSubID).
		Count(&count).Error)
	assert.Equal(t, int64(1), count,
		"exactly one UserSubscription row should exist after two deliveries "+
			"with the same stripe_subscription_id")
}

// -----------------------------------------------------------------------------
// Reservation-status tests (issue #261)
// -----------------------------------------------------------------------------
//
// These tests drive the fix for the "stuck reservation" bug: if the cleanup
// DELETE after a failed ProcessWebhook itself fails, the row stays as
// reserved forever and Stripe's retries are silently swallowed.
//
// The fix introduces a `Status` column on WebhookEvent with values:
//   - "reserved"  : a worker claimed the event and is processing it
//   - "processed" : ProcessWebhook ran to success
//   - "failed"    : ProcessWebhook returned an error; row is re-reservable
//
// reserveEvent semantics (post-fix):
//   - no row              -> INSERT(status=reserved), reserved=true
//   - row status=reserved -> reserved=false (another pod owns it)
//   - row status=processed-> reserved=false (idempotent skip)
//   - row status=failed   -> UPDATE(status=reserved), reserved=true (re-claim)
//
// The on-failure path UPDATEs status=failed (no DELETE), so a transient DB
// glitch on cleanup can no longer wedge the row in `reserved` forever.
//
// While the model field is missing these tests fail to COMPILE — that is
// the strongest red signal we can give the GREEN-phase implementer.

// fetchWebhookEventStatus reads the status column for an event_id directly
// via SQL so the test doesn't depend on which Go field name the eventual
// model uses (Status / State / etc.). Returns "" if no row exists.
func fetchWebhookEventStatus(t *testing.T, db *gorm.DB, eventID string) string {
	t.Helper()
	var status string
	row := db.Raw("SELECT status FROM webhook_events WHERE event_id = ?", eventID).Row()
	if err := row.Scan(&status); err != nil {
		// No row, or status column missing — caller asserts on the empty value.
		return ""
	}
	return status
}

// seedWebhookEvent inserts a webhook_events row with the given status.
// Used to set up "row already exists" scenarios.
func seedWebhookEvent(t *testing.T, db *gorm.DB, eventID, eventType, status string) {
	t.Helper()
	now := time.Now()
	row := &models.WebhookEvent{
		EventID:     eventID,
		EventType:   eventType,
		ProcessedAt: now,
		ExpiresAt:   now.Add(24 * time.Hour),
		Payload:     "{}",
		Status:      status, // RED: field doesn't exist yet on WebhookEvent
	}
	require.NoError(t, db.Create(row).Error)
}

// TestWebhook_FailedReservation_NextDeliveryReprocesses verifies that a row
// in status=failed is re-reservable. Stripe's retry must re-enter
// ProcessWebhook and, on success, the row must end as status=processed.
//
// Today: a failed run hard-DELETEs the row, so a "failed" row never exists.
// The bug is when the DELETE itself fails: the row sits in status=reserved
// (or, with the old schema, just exists) and reserveEvent's INSERT ON
// CONFLICT DO NOTHING returns 0 rows affected -> Stripe gets 200 -> event
// is silently lost.
//
// Post-fix: the row is left in status=failed on processing failure;
// reserveEvent recognizes status=failed and UPDATEs it back to reserved
// to allow retry.
func TestWebhook_FailedReservation_NextDeliveryReprocesses(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_failed_retry_" + uuid.NewString()

	// Seed a row already in status=failed (simulates a previous delivery
	// where ProcessWebhook returned an error and the controller marked the
	// row as failed instead of deleting it).
	seedWebhookEvent(t, db, eventID, "customer.subscription.created", "failed")

	// Sanity: row is present with status=failed.
	require.Equal(t, "failed", fetchWebhookEventStatus(t, db, eventID),
		"precondition: seeded row must be in status=failed")

	// Working ProcessWebhook (no error this time — Stripe's retry succeeds).
	mockSvc := &webhookTestStripeService{
		eventID: eventID,
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildWebhookRequest(t, payload))

	// Critical: a failed row must NOT block retries.
	assert.Equal(t, http.StatusOK, w.Code,
		"RETRY: a row in status=failed must be treated as re-reservable. "+
			"Currently reserveEvent uses INSERT ON CONFLICT DO NOTHING, which "+
			"returns RowsAffected=0 for any existing row regardless of status, "+
			"so a stuck failed row would surface as 200 'already processed' and "+
			"the event would be silently lost. Fix: detect status=failed and "+
			"UPDATE it back to status=reserved before processing.")

	assert.Equal(t, int32(1), mockSvc.processCalls.Load(),
		"ProcessWebhook must run exactly once (retry actually re-enters the handler)")

	// After a successful retry, the row must be marked processed.
	assert.Equal(t, "processed", fetchWebhookEventStatus(t, db, eventID),
		"after a successful retry, the existing row must be UPDATEd to status=processed")

	// Still exactly one row for this event_id (UPDATE, not INSERT).
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"reserveEvent must UPDATE the existing failed row, not INSERT a new one")
}

// TestWebhook_ProcessedReservation_NotReprocessed asserts that a row already
// in status=processed short-circuits to 200 OK without re-running the handler.
// This is the standard Stripe-replays-the-same-event idempotency case under
// the new schema.
func TestWebhook_ProcessedReservation_NotReprocessed(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_already_processed_" + uuid.NewString()

	// Row exists, status=processed (we already handled this event before).
	seedWebhookEvent(t, db, eventID, "customer.subscription.created", "processed")

	mockSvc := &webhookTestStripeService{
		eventID: eventID,
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildWebhookRequest(t, payload))

	assert.Equal(t, http.StatusOK, w.Code,
		"a duplicate delivery for an already-processed event must return 200")

	// Critical: the handler must NOT run again — that's the whole point of
	// reservation-status idempotency.
	assert.Equal(t, int32(0), mockSvc.processCalls.Load(),
		"IDEMPOTENCY: ProcessWebhook must NOT run when a status=processed row "+
			"already exists for this event_id. Today, INSERT ON CONFLICT DO "+
			"NOTHING already short-circuits this case (the test passes today "+
			"under the existing flow); this assertion locks in the behavior "+
			"under the new status-aware reserveEvent.")

	// Row must remain unchanged (status=processed, count=1).
	assert.Equal(t, "processed", fetchWebhookEventStatus(t, db, eventID),
		"status must remain 'processed' (no transition allowed from terminal state)")
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"exactly one row should still exist (no duplicate insert)")
}

// TestWebhook_ReservedRowExists_NotReprocessed covers the concurrent case
// where another pod is currently processing the event (its row is in
// status=reserved). The losing delivery must short-circuit, NOT call the
// handler, and leave the row alone so the active pod can finish.
func TestWebhook_ReservedRowExists_NotReprocessed(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_reserved_other_pod_" + uuid.NewString()

	// Another pod has reserved this event and is mid-processing.
	seedWebhookEvent(t, db, eventID, "customer.subscription.created", "reserved")

	mockSvc := &webhookTestStripeService{
		eventID: eventID,
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildWebhookRequest(t, payload))

	// 200 OK so Stripe doesn't keep retrying while the other pod processes.
	assert.Equal(t, http.StatusOK, w.Code,
		"when another pod is already processing the event (status=reserved), "+
			"return 200 to short-circuit Stripe — let the active pod finish")

	assert.Equal(t, int32(0), mockSvc.processCalls.Load(),
		"CONCURRENCY: ProcessWebhook must NOT run when the row is already "+
			"status=reserved (another pod owns processing). Running again "+
			"would defeat the whole point of atomic reservation.")

	// Row must remain unchanged so the active pod can transition it itself.
	assert.Equal(t, "reserved", fetchWebhookEventStatus(t, db, eventID),
		"a non-owning delivery must not mutate the reservation")
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"exactly one row should still exist (no duplicate insert)")
}

// TestWebhook_ProcessFailure_MarksRowFailed_DoesNotDelete is the core
// regression guard for issue #261: a processing failure must leave a row
// in status=failed, NOT delete the row. Hard-deleting is the bug — if the
// delete itself silently fails (transient DB error), the row stays as
// status=reserved and subsequent deliveries are silently swallowed.
func TestWebhook_ProcessFailure_MarksRowFailed_DoesNotDelete(t *testing.T) {
	db := freshTestDB(t)
	eventID := "evt_failure_marks_failed_" + uuid.NewString()

	processErr := errors.New("simulated downstream failure")
	mockSvc := &webhookTestStripeService{
		eventID:    eventID,
		processErr: processErr,
	}
	router := newRouterWithMockService(db, mockSvc)

	payload := []byte(fmt.Sprintf(`{"id":%q,"type":"customer.subscription.created"}`, eventID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, buildWebhookRequest(t, payload))

	// Failure must surface as 5xx so Stripe knows to retry.
	assert.GreaterOrEqual(t, w.Code, 500,
		"a ProcessWebhook failure must surface as 5xx so Stripe retries")
	assert.Less(t, w.Code, 600,
		"a ProcessWebhook failure must surface as 5xx so Stripe retries")

	// Critical invariant: the row MUST exist in status=failed (NOT deleted).
	// This is the heart of the fix: if releaseReservation's DELETE itself
	// fails today, the row is wedged forever as 'reserved'. The new code
	// uses an UPDATE to status=failed, which is recoverable on next retry.
	assert.Equal(t, int64(1), countWebhookEvents(t, db, eventID),
		"FIX: after ProcessWebhook fails, the reservation row MUST persist "+
			"(in status=failed). Today the controller hard-DELETEs the row "+
			"in releaseReservation; if that DELETE itself fails (transient "+
			"DB glitch), the row stays as 'reserved' and Stripe's retries "+
			"are silently swallowed. Fix: replace DELETE with UPDATE status='failed'.")

	assert.Equal(t, "failed", fetchWebhookEventStatus(t, db, eventID),
		"row must end in status='failed' so the next Stripe retry can re-reserve it")

	assert.Equal(t, int32(1), mockSvc.processCalls.Load(),
		"ProcessWebhook must have been called exactly once (we don't retry inline)")
}
