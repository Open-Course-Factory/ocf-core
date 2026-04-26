// tests/payment/stripeSubscriptionPlanHook_test.go
//
// Tests for StripeSubscriptionPlanHook.handleAfterDelete behavior.
//
// Bug under test (issue #267, MR !181):
//   When a SubscriptionPlan is deleted, the hook calls
//   UpdateSubscriptionPlanInStripe(plan), which forwards plan.IsActive=true
//   to Stripe. As a result, the Stripe product is NEVER archived after a
//   GORM soft delete.
//
// Expected behavior (after fix):
//   The hook MUST call ArchiveSubscriptionPlanInStripe(productID) on
//   AfterDelete. This method (to be added on services.StripeService) will
//   issue a `product.Update(productID, {Active: false})` call to Stripe so
//   the product is properly archived.
//
// These tests assert the new contract. They are RED until backend-dev:
//   1. Adds ArchiveSubscriptionPlanInStripe(productID string) error to the
//      services.StripeService interface and the concrete stripeService.
//   2. Updates handleAfterDelete to call it.
//
// Note on integration: we cannot unit-test the real Stripe SDK call here
// without an integration test setup. We only verify the hook calls the right
// method on the StripeService interface (via mock spy). The actual SDK call
// (product.Update with Active=false) is covered by the contract of the new
// method.
package payment_tests

import (
	"errors"
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stripe/stripe-go/v82"
)

// stringPtr returns a pointer to the given string. Used to build *string
// fields like StripeProductID.
func stringPtr(s string) *string {
	return &s
}

// ============================================================================
// fakeStripeService — focused testify mock implementing services.StripeService.
//
// We can't reuse SharedMockStripeService because its method signatures differ
// from the real interface (it uses (plan any) instead of *models.SubscriptionPlan
// and similar simplifications). The hook depends on the strongly-typed
// services.StripeService interface, so we need a mock that satisfies it.
//
// Only the methods exercised by the hook are wired through testify.On(); all
// other interface methods panic if called (none should be called by the hook).
// ============================================================================
type fakeStripeService struct {
	mock.Mock
}

// --- Methods used by StripeSubscriptionPlanHook (record calls via mock) ---

func (m *fakeStripeService) CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *fakeStripeService) UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	args := m.Called(plan)
	return args.Error(0)
}

// ArchiveSubscriptionPlanInStripe is the new method the hook MUST call on
// AfterDelete. It does not yet exist on services.StripeService — adding it is
// the backend-dev's task. While this method is missing from the interface,
// this file fails to compile (RED). Once added to the interface, the test
// will compile but assertion on .AssertCalled(...) will fail because the hook
// still calls UpdateSubscriptionPlanInStripe (still RED). When the hook is
// updated to call Archive, tests turn GREEN.
func (m *fakeStripeService) ArchiveSubscriptionPlanInStripe(productID string) error {
	args := m.Called(productID)
	return args.Error(0)
}

// --- All other methods of services.StripeService — unused, panic on call ---

func (m *fakeStripeService) CreateOrGetCustomer(userID, email, name string) (string, error) {
	panic("fakeStripeService.CreateOrGetCustomer unexpectedly called")
}
func (m *fakeStripeService) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	panic("fakeStripeService.UpdateCustomer unexpectedly called")
}
func (m *fakeStripeService) CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput, replaceSubscriptionID *uuid.UUID) (*dto.CheckoutSessionOutput, error) {
	panic("fakeStripeService.CreateCheckoutSession unexpectedly called")
}
func (m *fakeStripeService) CreateBulkCheckoutSession(userID string, input dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	panic("fakeStripeService.CreateBulkCheckoutSession unexpectedly called")
}
func (m *fakeStripeService) CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error) {
	panic("fakeStripeService.CreatePortalSession unexpectedly called")
}
func (m *fakeStripeService) ProcessWebhook(payload []byte, signature string) error {
	panic("fakeStripeService.ProcessWebhook unexpectedly called")
}
func (m *fakeStripeService) ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	panic("fakeStripeService.ValidateWebhookSignature unexpectedly called")
}
func (m *fakeStripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	panic("fakeStripeService.CancelSubscription unexpectedly called")
}
func (m *fakeStripeService) MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error {
	panic("fakeStripeService.MarkSubscriptionAsCancelled unexpectedly called")
}
func (m *fakeStripeService) ReactivateSubscription(subscriptionID string) error {
	panic("fakeStripeService.ReactivateSubscription unexpectedly called")
}
func (m *fakeStripeService) UpdateSubscription(subscriptionID, newPriceID, prorationBehavior string) (*stripe.Subscription, error) {
	panic("fakeStripeService.UpdateSubscription unexpectedly called")
}
func (m *fakeStripeService) SyncExistingSubscriptions() (*services.SyncSubscriptionsResult, error) {
	panic("fakeStripeService.SyncExistingSubscriptions unexpectedly called")
}
func (m *fakeStripeService) SyncUserSubscriptions(userID string) (*services.SyncSubscriptionsResult, error) {
	panic("fakeStripeService.SyncUserSubscriptions unexpectedly called")
}
func (m *fakeStripeService) SyncSubscriptionsWithMissingMetadata() (*services.SyncSubscriptionsResult, error) {
	panic("fakeStripeService.SyncSubscriptionsWithMissingMetadata unexpectedly called")
}
func (m *fakeStripeService) LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID uuid.UUID) error {
	panic("fakeStripeService.LinkSubscriptionToUser unexpectedly called")
}
func (m *fakeStripeService) SyncUserInvoices(userID string) (*services.SyncInvoicesResult, error) {
	panic("fakeStripeService.SyncUserInvoices unexpectedly called")
}
func (m *fakeStripeService) CleanupIncompleteInvoices(input dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error) {
	panic("fakeStripeService.CleanupIncompleteInvoices unexpectedly called")
}
func (m *fakeStripeService) SyncUserPaymentMethods(userID string) (*services.SyncPaymentMethodsResult, error) {
	panic("fakeStripeService.SyncUserPaymentMethods unexpectedly called")
}
func (m *fakeStripeService) AttachPaymentMethod(paymentMethodID, customerID string) error {
	panic("fakeStripeService.AttachPaymentMethod unexpectedly called")
}
func (m *fakeStripeService) DetachPaymentMethod(paymentMethodID string) error {
	panic("fakeStripeService.DetachPaymentMethod unexpectedly called")
}
func (m *fakeStripeService) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	panic("fakeStripeService.SetDefaultPaymentMethod unexpectedly called")
}
func (m *fakeStripeService) GetInvoice(invoiceID string) (*stripe.Invoice, error) {
	panic("fakeStripeService.GetInvoice unexpectedly called")
}
func (m *fakeStripeService) SendInvoice(invoiceID string) error {
	panic("fakeStripeService.SendInvoice unexpectedly called")
}
func (m *fakeStripeService) CreateSubscriptionWithQuantity(customerID string, plan *models.SubscriptionPlan, quantity int, paymentMethodID string) (*stripe.Subscription, error) {
	panic("fakeStripeService.CreateSubscriptionWithQuantity unexpectedly called")
}
func (m *fakeStripeService) UpdateSubscriptionQuantity(subscriptionID string, subscriptionItemID string, newQuantity int) (*stripe.Subscription, error) {
	panic("fakeStripeService.UpdateSubscriptionQuantity unexpectedly called")
}
func (m *fakeStripeService) ImportPlansFromStripe() (*services.SyncPlansResult, error) {
	panic("fakeStripeService.ImportPlansFromStripe unexpectedly called")
}

// Compile-time assertion that fakeStripeService satisfies services.StripeService.
var _ services.StripeService = (*fakeStripeService)(nil)

// ============================================================================
// AfterDelete tests
// ============================================================================

// TestStripeSubscriptionPlanHook_AfterDelete_CallsArchive_WhenStripeProductIDPresent
// is the primary regression test for issue #267. It asserts that on
// AfterDelete, the hook calls ArchiveSubscriptionPlanInStripe(productID) —
// NOT UpdateSubscriptionPlanInStripe(plan).
func TestStripeSubscriptionPlanHook_AfterDelete_CallsArchive_WhenStripeProductIDPresent(t *testing.T) {
	stripeMock := &fakeStripeService{}
	stripeMock.On("ArchiveSubscriptionPlanInStripe", "prod_test_123").Return(nil)
	// We also tolerate UpdateSubscriptionPlanInStripe so testify doesn't panic
	// on the buggy code path — the AssertNotCalled below is the actual check.
	stripeMock.On("UpdateSubscriptionPlanInStripe", mock.Anything).Return(nil)

	hook := paymentHooks.NewStripeSubscriptionPlanHookWithService(stripeMock)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Test Plan",
		StripeProductID: stringPtr("prod_test_123"),
		IsActive:        true, // soft-deleted plans still have IsActive=true; that's the bug
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.AfterDelete,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "hook should not return an error")

	stripeMock.AssertCalled(t, "ArchiveSubscriptionPlanInStripe", "prod_test_123")
	stripeMock.AssertNotCalled(t, "UpdateSubscriptionPlanInStripe", mock.Anything)
}

// TestStripeSubscriptionPlanHook_AfterDelete_SkipsArchive_WhenStripeProductIDIsNil
// asserts the hook skips Stripe archival entirely for plans that were never
// synced to Stripe (e.g. free plans with PriceAmount=0).
func TestStripeSubscriptionPlanHook_AfterDelete_SkipsArchive_WhenStripeProductIDIsNil(t *testing.T) {
	stripeMock := &fakeStripeService{}
	// Tolerate any call to avoid testify panic — AssertNotCalled below is the check.
	stripeMock.On("ArchiveSubscriptionPlanInStripe", mock.Anything).Return(nil)
	stripeMock.On("UpdateSubscriptionPlanInStripe", mock.Anything).Return(nil)

	hook := paymentHooks.NewStripeSubscriptionPlanHookWithService(stripeMock)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Free Plan",
		StripeProductID: nil, // never synced to Stripe
		IsActive:        true,
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.AfterDelete,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)

	stripeMock.AssertNotCalled(t, "ArchiveSubscriptionPlanInStripe", mock.Anything)
	stripeMock.AssertNotCalled(t, "UpdateSubscriptionPlanInStripe", mock.Anything)
}

// TestStripeSubscriptionPlanHook_AfterDelete_DoesNotPropagateArchiveError
// asserts that when ArchiveSubscriptionPlanInStripe returns an error, the
// hook logs but does not propagate it (matches the existing pattern at line
// 130 of stripeSubscriptionPlanHook.go: errors are logged and the hook
// returns nil). This prevents Stripe outages from blocking plan deletion.
func TestStripeSubscriptionPlanHook_AfterDelete_DoesNotPropagateArchiveError(t *testing.T) {
	stripeMock := &fakeStripeService{}
	stripeMock.On("ArchiveSubscriptionPlanInStripe", "prod_test_999").
		Return(errors.New("stripe API down"))
	// Tolerate the buggy call so the test produces a clean failure instead of a panic.
	stripeMock.On("UpdateSubscriptionPlanInStripe", mock.Anything).Return(nil)

	hook := paymentHooks.NewStripeSubscriptionPlanHookWithService(stripeMock)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Test Plan",
		StripeProductID: stringPtr("prod_test_999"),
		IsActive:        true,
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.AfterDelete,
		NewEntity:  plan,
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Stripe API errors must NOT propagate from AfterDelete hook")

	stripeMock.AssertCalled(t, "ArchiveSubscriptionPlanInStripe", "prod_test_999")
}

// TestStripeSubscriptionPlanHook_AfterDelete_NeverCallsUpdate
// is an explicit regression guard against the original bug: AfterDelete must
// NEVER call UpdateSubscriptionPlanInStripe. UpdateSubscriptionPlanInStripe
// forwards plan.IsActive (still true after a soft delete) to Stripe, which
// keeps the product active — exactly the behavior we're fixing.
func TestStripeSubscriptionPlanHook_AfterDelete_NeverCallsUpdate(t *testing.T) {
	stripeMock := &fakeStripeService{}
	stripeMock.On("ArchiveSubscriptionPlanInStripe", mock.Anything).Return(nil)
	// Tolerate the buggy call so the test produces a clean assertion failure.
	stripeMock.On("UpdateSubscriptionPlanInStripe", mock.Anything).Return(nil)

	hook := paymentHooks.NewStripeSubscriptionPlanHookWithService(stripeMock)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Test Plan",
		StripeProductID: stringPtr("prod_test_456"),
		IsActive:        true,
	}

	ctx := &hooks.HookContext{
		EntityName: "SubscriptionPlan",
		HookType:   hooks.AfterDelete,
		NewEntity:  plan,
	}

	_ = hook.Execute(ctx)

	stripeMock.AssertNotCalled(t, "UpdateSubscriptionPlanInStripe", mock.Anything)
}
