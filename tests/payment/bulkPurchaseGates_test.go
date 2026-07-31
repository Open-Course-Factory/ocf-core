// tests/payment/bulkPurchaseGates_test.go
//
// Originally the RED-phase tests for the 2026-07-10 review (finding I2): the
// direct bulk-purchase path was missing gates the checkout paths enforced.
//
// #441 then split the gate in two — BulkPurchasable on the plan being sold,
// GroupManagementEnabled on the purchaser's own plan — so the two direct-path
// tests here were removed rather than repaired (see the note below where they
// stood). What remains is the bypass test: whatever the gate is, the Stripe
// checkout path must apply it too, or the direct path's gate is worthless.
//
// The shared fixtures below — bulkGatesStripeStub and installFakeCasdoor — are
// used by the replacement tests in bulkPurchaseGateSplit_test.go, so this file
// stays the home of the bulk-purchase test harness.
//
// Tests drive the REAL service with a fake Casdoor (so GetUserByUserId resolves)
// and an injected StripeService stub (so the happy path would otherwise
// complete), then assert user-observable outcomes: rejection AND no rows.
package payment_tests

import (
	"testing"
	"time"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// bulkGatesStripeStub is a full services.StripeService that lets the bulk-
// purchase happy path complete without a real Stripe backend: it returns a
// canned customer and a subscription with one item (the two calls
// PurchaseBulkLicenses makes). Every other method panics so an unexpected call
// is loud rather than silently mocked. Kept local to this file with a unique
// name so it doesn't collide with the package's other stripe fakes.
type bulkGatesStripeStub struct{}

var _ services.StripeService = (*bulkGatesStripeStub)(nil)

func (s *bulkGatesStripeStub) CreateOrGetCustomer(userID, email, name string) (string, error) {
	return "cus_bulkgates", nil
}

func (s *bulkGatesStripeStub) CreateSubscriptionWithQuantity(customerID string, plan *models.SubscriptionPlan, quantity int, paymentMethodID string) (*stripe.Subscription, error) {
	now := time.Now()
	return &stripe.Subscription{
		ID: "sub_bulkgates",
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{{
				ID:                 "si_bulkgates",
				CurrentPeriodStart: now.Unix(),
				CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour).Unix(),
			}},
		},
	}, nil
}

// CancelSubscription is the only compensation call PurchaseBulkLicenses makes
// (on DB failure). It is not expected on these paths but is a no-op rather than
// a panic so a compensating call wouldn't mask the assertion under test.
func (s *bulkGatesStripeStub) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	return nil
}

func (s *bulkGatesStripeStub) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	panic("bulkGatesStripeStub.UpdateCustomer unexpectedly called")
}
func (s *bulkGatesStripeStub) CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput, replaceSubscriptionID *uuid.UUID) (*dto.CheckoutSessionOutput, error) {
	panic("bulkGatesStripeStub.CreateCheckoutSession unexpectedly called")
}
func (s *bulkGatesStripeStub) CreateBulkCheckoutSession(userID string, input dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	panic("bulkGatesStripeStub.CreateBulkCheckoutSession unexpectedly called")
}
func (s *bulkGatesStripeStub) CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error) {
	panic("bulkGatesStripeStub.CreatePortalSession unexpectedly called")
}
func (s *bulkGatesStripeStub) CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	panic("bulkGatesStripeStub.CreateSubscriptionPlanInStripe unexpectedly called")
}
func (s *bulkGatesStripeStub) UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	panic("bulkGatesStripeStub.UpdateSubscriptionPlanInStripe unexpectedly called")
}
func (s *bulkGatesStripeStub) ArchiveSubscriptionPlanInStripe(productID string) error {
	panic("bulkGatesStripeStub.ArchiveSubscriptionPlanInStripe unexpectedly called")
}
func (s *bulkGatesStripeStub) ProcessWebhook(payload []byte, signature string) error {
	panic("bulkGatesStripeStub.ProcessWebhook unexpectedly called")
}
func (s *bulkGatesStripeStub) ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	panic("bulkGatesStripeStub.ValidateWebhookSignature unexpectedly called")
}
func (s *bulkGatesStripeStub) MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error {
	panic("bulkGatesStripeStub.MarkSubscriptionAsCancelled unexpectedly called")
}
func (s *bulkGatesStripeStub) ReactivateSubscription(subscriptionID string) error {
	panic("bulkGatesStripeStub.ReactivateSubscription unexpectedly called")
}
func (s *bulkGatesStripeStub) UpdateSubscription(subscriptionID, newPriceID, prorationBehavior string) (*stripe.Subscription, error) {
	panic("bulkGatesStripeStub.UpdateSubscription unexpectedly called")
}
func (s *bulkGatesStripeStub) SyncExistingSubscriptions() (*services.SyncSubscriptionsResult, error) {
	panic("bulkGatesStripeStub.SyncExistingSubscriptions unexpectedly called")
}
func (s *bulkGatesStripeStub) SyncUserSubscriptions(userID string) (*services.SyncSubscriptionsResult, error) {
	panic("bulkGatesStripeStub.SyncUserSubscriptions unexpectedly called")
}
func (s *bulkGatesStripeStub) SyncSubscriptionsWithMissingMetadata() (*services.SyncSubscriptionsResult, error) {
	panic("bulkGatesStripeStub.SyncSubscriptionsWithMissingMetadata unexpectedly called")
}
func (s *bulkGatesStripeStub) LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID uuid.UUID) error {
	panic("bulkGatesStripeStub.LinkSubscriptionToUser unexpectedly called")
}
func (s *bulkGatesStripeStub) SyncUserInvoices(userID string) (*services.SyncInvoicesResult, error) {
	panic("bulkGatesStripeStub.SyncUserInvoices unexpectedly called")
}
func (s *bulkGatesStripeStub) CleanupIncompleteInvoices(input dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error) {
	panic("bulkGatesStripeStub.CleanupIncompleteInvoices unexpectedly called")
}
func (s *bulkGatesStripeStub) SyncUserPaymentMethods(userID string) (*services.SyncPaymentMethodsResult, error) {
	panic("bulkGatesStripeStub.SyncUserPaymentMethods unexpectedly called")
}
func (s *bulkGatesStripeStub) AttachPaymentMethod(paymentMethodID, customerID string) error {
	panic("bulkGatesStripeStub.AttachPaymentMethod unexpectedly called")
}
func (s *bulkGatesStripeStub) DetachPaymentMethod(paymentMethodID string) error {
	panic("bulkGatesStripeStub.DetachPaymentMethod unexpectedly called")
}
func (s *bulkGatesStripeStub) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	panic("bulkGatesStripeStub.SetDefaultPaymentMethod unexpectedly called")
}
func (s *bulkGatesStripeStub) GetInvoice(invoiceID string) (*stripe.Invoice, error) {
	panic("bulkGatesStripeStub.GetInvoice unexpectedly called")
}
func (s *bulkGatesStripeStub) SendInvoice(invoiceID string) error {
	panic("bulkGatesStripeStub.SendInvoice unexpectedly called")
}
func (s *bulkGatesStripeStub) UpdateSubscriptionQuantity(subscriptionID string, subscriptionItemID string, newQuantity int) (*stripe.Subscription, error) {
	panic("bulkGatesStripeStub.UpdateSubscriptionQuantity unexpectedly called")
}
func (s *bulkGatesStripeStub) ImportPlansFromStripe() (*services.SyncPlansResult, error) {
	panic("bulkGatesStripeStub.ImportPlansFromStripe unexpectedly called")
}

func (s *bulkGatesStripeStub) SyncPlansToStripe(services.SyncToStripeOptions) (*services.StripeSyncResult, error) {
	panic("bulkGatesStripeStub.SyncPlansToStripe unexpectedly called")
}

// assertNoBulkRowsPersisted fails if any batch or license row exists — the
// user-observable contract when a bulk purchase is rejected: nothing is created.
func assertNoBulkRowsPersisted(t *testing.T) {
	t.Helper()
	var batchCount, licenseCount int64
	sharedTestDB.Model(&models.SubscriptionBatch{}).Count(&batchCount)
	sharedTestDB.Model(&models.UserSubscription{}).Count(&licenseCount)
	assert.Equal(t, int64(0), batchCount, "no subscription batch may be persisted when the purchase is rejected")
	assert.Equal(t, int64(0), licenseCount, "no licenses may be persisted when the purchase is rejected")
}

// The two direct-path gate tests that lived here — "non-catalog plan rejected"
// and "plan without group_management rejected" — were removed by #441 rather
// than repaired, because their premises stopped being true:
//
//   - IsCatalog no longer gates bulk purchase. It answers "is this listed on the
//     public pricing page?", which is a different question from "is this a seat
//     product?". A learner seat is deliberately both hidden and sellable.
//   - group_management moved to the PURCHASER's plan. Requiring it on the plan
//     being sold would have handed students group management along with their
//     seat, and let them buy seats of their own.
//
// Both tests kept passing after the split, but only because the new
// BulkPurchasable gate happened to reject their fixtures — they would have gone
// on asserting a rule the code no longer implements. Their replacements live in
// bulkPurchaseGateSplit_test.go, which pins each question separately.
//
// The checkout-path test below is kept and re-targeted, because "checkout must
// not bypass the direct path's gate" is still exactly the right thing to pin.

// TestBulkCheckout_PlanWithoutGroupManagement_Rejected closes the bypass: the
// same feature gate must also cover the Stripe checkout path
// CreateBulkCheckoutSession (stripeService.go:425+), otherwise the direct-
// purchase feature gate is trivially bypassable by buying through checkout
// instead. Driven through the REAL StripeService against a fake Stripe backend
// (captures the checkout-session POST) + fake Casdoor — mirroring
// checkoutInputHardening_test.go, which already pins the IsCatalog rejection on
// this path (NOT duplicated here — this asserts ONLY the group_management gate).
//
// RED today: an active catalog plan without group_management still creates a
// checkout session — no error, and a POST /v1/checkout/sessions is recorded.
func TestBulkCheckout_PurchaserWithoutGroupManagement_Rejected(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "bulkco@example.com", "Bulk Checkout Buyer")
	svc := services.NewStripeService(db)

	// A perfectly sellable seat product: the plan side of the gate passes, so the
	// only thing under test is the purchaser side.
	plan := seedCheckoutPlan(t, "Sellable Seat Plan", true)
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Model(plan).Update("bulk_purchasable", true).Error)

	// The buyer holds no plan granting group management.
	_, err := svc.CreateBulkCheckoutSession("user_bulkco_"+uuid.NewString(), dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	})

	assert.Error(t, err,
		"PURCHASER: bulk checkout must apply the same buyer-side gate as PurchaseBulkLicenses — "+
			"otherwise the direct path's gate is trivially bypassable by buying through Stripe "+
			"checkout instead.")
	assert.Empty(t, cap.checkoutSessionForm(),
		"no Stripe checkout session may be created for an ineligible purchaser")
}
