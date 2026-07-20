// tests/payment/bulkPurchaseGates_test.go
//
// RED-phase tests for the 2026-07-10 review, finding I2: the direct bulk-
// purchase path bulkLicenseService.PurchaseBulkLicenses is missing two gates
// that BOTH checkout paths already enforce.
//
// SSOT context — the canonical predicates these tests pin already exist on the
// checkout paths and are exercised by checkoutInputHardening_test.go:
//   1. IsCatalog gate. CreateCheckoutSession (stripeService.go ~:273) and
//      CreateBulkCheckoutSession (~:428) both reject an active-but-non-catalog
//      plan with "subscription plan is not available for purchase".
//      PurchaseBulkLicenses (bulkLicenseService.go:72) only checks IsActive, so
//      an active bespoke/admin plan (IsCatalog=false) can be bulk-purchased by
//      any Member who knows its UUID.
//   2. group_management feature gate. The handler godoc
//      (bulkLicenseController.go:97) documents « Requires group_management
//      feature in user's plan », but PurchaseBulkLicenses never checks it.
//
// Both gates sit BEFORE the Casdoor/Stripe calls, so the fix rejects the
// purchase before any external side effect. These tests drive the REAL
// PurchaseBulkLicenses with a fake Casdoor (so GetUserByUserId resolves) and an
// injected full StripeService stub (so the happy path would otherwise complete),
// then assert the purchase is rejected AND no batch/license rows are persisted.
//
// RED today: with no gate, the purchase SUCCEEDS — it returns a batch and
// persists rows — so require.Error and the zero-row assertions fail. That is the
// precise red signal: "purchase succeeded but should have been rejected".
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
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

// TestBulkPurchase_NonCatalogPlan_Rejected isolates gate 1: an ACTIVE plan with
// IsCatalog=false must not be bulk-purchasable, mirroring the checkout paths.
// The plan is given the group_management feature so gate 2 would pass — this
// test fails ONLY on the missing IsCatalog gate.
func TestBulkPurchase_NonCatalogPlan_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "noncatalog@example.com", "Non Catalog Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: planID},
		Name:      "Bespoke Admin Plan",
		Currency:  "eur",
		IsActive:  true,
		IsCatalog:              false, // the bug: active but not catalog
		GroupManagementEnabled: true,  // isolate gate 1 (the group-management gate passes)
	}
	require.NoError(t, db.Create(plan).Error)
	// GORM skips the zero-value bool on a gorm:"default:true" field, so is_catalog
	// persists TRUE at Create; force it false via a follow-up Update (the pattern
	// subscriptionPlan_catalog_test.go / checkoutInputHardening_test.go use).
	require.NoError(t, db.Model(plan).Update("is_catalog", false).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses("buyer-noncatalog", dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           3,
	})

	require.Error(t, err,
		"CATALOG: an active but non-catalog plan (IsCatalog=false) must be rejected on the "+
			"direct bulk-purchase path, exactly as CreateCheckoutSession/CreateBulkCheckoutSession "+
			"reject it. Today PurchaseBulkLicenses only checks IsActive, so a bespoke/admin plan "+
			"is bulk-purchasable by any Member who knows its UUID.")
	assert.Nil(t, batch, "no batch may be returned for a rejected non-catalog plan")
	assert.Nil(t, licenses, "no licenses may be returned for a rejected non-catalog plan")
	assertNoBulkRowsPersisted(t)
}

// TestBulkPurchase_PlanWithoutGroupManagement_Rejected isolates gate 2: an
// active CATALOG plan that lacks the group_management feature must not be bulk-
// purchasable — the handler godoc says « Requires group_management feature »,
// but no such check exists. IsCatalog=true so gate 1 passes and this test fails
// ONLY on the missing feature gate.
func TestBulkPurchase_PlanWithoutGroupManagement_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "nogroupmgmt@example.com", "No Group Mgmt Buyer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: planID},
		Name:      "Catalog Plan Without Group Management",
		Currency:  "eur",
		IsActive:  true,
		IsCatalog: true, // gate 1 passes
		// GroupManagementEnabled defaults false → lacks the group-management gate
	}
	require.NoError(t, db.Create(plan).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses("buyer-nogroupmgmt", dto.BulkPurchaseInput{
		SubscriptionPlanID: planID,
		Quantity:           2,
	})

	require.Error(t, err,
		"FEATURE: a plan lacking the group_management feature must be rejected on the bulk-"+
			"purchase path — the handler godoc documents « Requires group_management feature », "+
			"but PurchaseBulkLicenses never enforces it.")
	assert.Nil(t, batch, "no batch may be returned when the plan lacks group_management")
	assert.Nil(t, licenses, "no licenses may be returned when the plan lacks group_management")
	assertNoBulkRowsPersisted(t)
}

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
func TestBulkCheckout_PlanWithoutGroupManagement_Rejected(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "bulkco@example.com", "Bulk Checkout Buyer")
	svc := services.NewStripeService(db)

	// seedCheckoutPlan builds an active plan with a Stripe price and IsCatalog=true
	// but no Features, so it lacks group_management — gate 1 passes, gate 2 must fire.
	plan := seedCheckoutPlan(t, "Catalog Bulk Plan Without Group Management", true)
	require.NoError(t, db.Create(plan).Error)

	_, err := svc.CreateBulkCheckoutSession("user_bulkco_"+uuid.NewString(), dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           5,
		SuccessURL:         "https://app.test/success",
		CancelURL:          "https://app.test/cancel",
	})

	assert.Error(t, err,
		"FEATURE: bulk checkout must also reject a plan lacking group_management — otherwise "+
			"the direct-purchase feature gate (PurchaseBulkLicenses) is bypassable via the Stripe "+
			"checkout path.")
	assert.Empty(t, cap.checkoutSessionForm(),
		"no Stripe checkout session may be created for a plan that lacks group_management")
}
