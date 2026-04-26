// tests/payment/paymentDeletionHelper_test.go
//
// Covers the concrete paymentDeletionHelper.CancelAllActiveSubscriptionsForUser
// implementation (GORM query + iteration + stop-on-first-error semantics).
// The 7 userDeletion_test.go cases mock PaymentDeletionHelper at the auth
// boundary, so without this file the SQL filter and loop behavior have no
// direct coverage. See MR !176 / issue #259.
//
// NOTE: uses a local minimal stub of StripeService rather than
// SharedMockStripeService, because the shared mock only implements a subset
// of the StripeService interface and cannot be passed as one. We only need
// CancelSubscription here — every other method panics if ever called,
// which is exactly the safety we want.
package payment_tests

import (
	"errors"
	"sync"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// pdhStrPtr returns a pointer to the given string (local helper so the file
// stays self-contained; no shared helper of this name exists in tests/payment/).
func pdhStrPtr(s string) *string { return &s }

// stubStripeForPDH implements services.StripeService. Only CancelSubscription
// is wired; every other method panics on call. The test sets a cancel handler
// that records invocations and chooses what to return.
type stubStripeForPDH struct {
	mu          sync.Mutex
	cancelCalls []stubCancelCall
	// cancelFn chooses the error returned for a given subscriptionID.
	// If nil, all cancellations succeed.
	cancelFn func(subscriptionID string, cancelAtPeriodEnd bool) error
}

type stubCancelCall struct {
	subscriptionID    string
	cancelAtPeriodEnd bool
}

func (s *stubStripeForPDH) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	s.mu.Lock()
	s.cancelCalls = append(s.cancelCalls, stubCancelCall{subscriptionID, cancelAtPeriodEnd})
	s.mu.Unlock()
	if s.cancelFn == nil {
		return nil
	}
	return s.cancelFn(subscriptionID, cancelAtPeriodEnd)
}

// Unused methods — panic loudly if ever invoked (they are not part of the
// contract we are testing, so a call would indicate a test regression).
func (s *stubStripeForPDH) CreateOrGetCustomer(string, string, string) (string, error) {
	panic("stubStripeForPDH: CreateOrGetCustomer not stubbed")
}
func (s *stubStripeForPDH) UpdateCustomer(string, *stripe.CustomerParams) error {
	panic("stubStripeForPDH: UpdateCustomer not stubbed")
}
func (s *stubStripeForPDH) CreateCheckoutSession(string, dto.CreateCheckoutSessionInput, *uuid.UUID) (*dto.CheckoutSessionOutput, error) {
	panic("stubStripeForPDH: CreateCheckoutSession not stubbed")
}
func (s *stubStripeForPDH) CreateBulkCheckoutSession(string, dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	panic("stubStripeForPDH: CreateBulkCheckoutSession not stubbed")
}
func (s *stubStripeForPDH) CreatePortalSession(string, dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error) {
	panic("stubStripeForPDH: CreatePortalSession not stubbed")
}
func (s *stubStripeForPDH) CreateSubscriptionPlanInStripe(*models.SubscriptionPlan) error {
	panic("stubStripeForPDH: CreateSubscriptionPlanInStripe not stubbed")
}
func (s *stubStripeForPDH) UpdateSubscriptionPlanInStripe(*models.SubscriptionPlan) error {
	panic("stubStripeForPDH: UpdateSubscriptionPlanInStripe not stubbed")
}
func (s *stubStripeForPDH) ArchiveSubscriptionPlanInStripe(string) error {
	panic("stubStripeForPDH: ArchiveSubscriptionPlanInStripe not stubbed")
}
func (s *stubStripeForPDH) ProcessWebhook([]byte, string) error {
	panic("stubStripeForPDH: ProcessWebhook not stubbed")
}
func (s *stubStripeForPDH) ValidateWebhookSignature([]byte, string) (*stripe.Event, error) {
	panic("stubStripeForPDH: ValidateWebhookSignature not stubbed")
}
func (s *stubStripeForPDH) MarkSubscriptionAsCancelled(*models.UserSubscription) error {
	panic("stubStripeForPDH: MarkSubscriptionAsCancelled not stubbed")
}
func (s *stubStripeForPDH) ReactivateSubscription(string) error {
	panic("stubStripeForPDH: ReactivateSubscription not stubbed")
}
func (s *stubStripeForPDH) UpdateSubscription(string, string, string) (*stripe.Subscription, error) {
	panic("stubStripeForPDH: UpdateSubscription not stubbed")
}
func (s *stubStripeForPDH) SyncExistingSubscriptions() (*services.SyncSubscriptionsResult, error) {
	panic("stubStripeForPDH: SyncExistingSubscriptions not stubbed")
}
func (s *stubStripeForPDH) SyncUserSubscriptions(string) (*services.SyncSubscriptionsResult, error) {
	panic("stubStripeForPDH: SyncUserSubscriptions not stubbed")
}
func (s *stubStripeForPDH) SyncSubscriptionsWithMissingMetadata() (*services.SyncSubscriptionsResult, error) {
	panic("stubStripeForPDH: SyncSubscriptionsWithMissingMetadata not stubbed")
}
func (s *stubStripeForPDH) LinkSubscriptionToUser(string, string, uuid.UUID) error {
	panic("stubStripeForPDH: LinkSubscriptionToUser not stubbed")
}
func (s *stubStripeForPDH) SyncUserInvoices(string) (*services.SyncInvoicesResult, error) {
	panic("stubStripeForPDH: SyncUserInvoices not stubbed")
}
func (s *stubStripeForPDH) CleanupIncompleteInvoices(dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error) {
	panic("stubStripeForPDH: CleanupIncompleteInvoices not stubbed")
}
func (s *stubStripeForPDH) SyncUserPaymentMethods(string) (*services.SyncPaymentMethodsResult, error) {
	panic("stubStripeForPDH: SyncUserPaymentMethods not stubbed")
}
func (s *stubStripeForPDH) AttachPaymentMethod(string, string) error {
	panic("stubStripeForPDH: AttachPaymentMethod not stubbed")
}
func (s *stubStripeForPDH) DetachPaymentMethod(string) error {
	panic("stubStripeForPDH: DetachPaymentMethod not stubbed")
}
func (s *stubStripeForPDH) SetDefaultPaymentMethod(string, string) error {
	panic("stubStripeForPDH: SetDefaultPaymentMethod not stubbed")
}
func (s *stubStripeForPDH) GetInvoice(string) (*stripe.Invoice, error) {
	panic("stubStripeForPDH: GetInvoice not stubbed")
}
func (s *stubStripeForPDH) SendInvoice(string) error {
	panic("stubStripeForPDH: SendInvoice not stubbed")
}
func (s *stubStripeForPDH) CreateSubscriptionWithQuantity(string, *models.SubscriptionPlan, int, string) (*stripe.Subscription, error) {
	panic("stubStripeForPDH: CreateSubscriptionWithQuantity not stubbed")
}
func (s *stubStripeForPDH) UpdateSubscriptionQuantity(string, string, int) (*stripe.Subscription, error) {
	panic("stubStripeForPDH: UpdateSubscriptionQuantity not stubbed")
}
func (s *stubStripeForPDH) ImportPlansFromStripe() (*services.SyncPlansResult, error) {
	panic("stubStripeForPDH: ImportPlansFromStripe not stubbed")
}

// Compile-time assertion that the stub satisfies StripeService.
var _ services.StripeService = (*stubStripeForPDH)(nil)

// TestPaymentDeletionHelper_CancelAllActiveSubscriptionsForUser_FiltersAndIterates
// exercises the three behaviors the interface docstring commits to:
//  1. Only subscriptions in status active/trialing/past_due with a non-empty
//     StripeSubscriptionID are cancelled — all other rows are skipped.
//  2. When multiple subscriptions qualify, every one of them is cancelled
//     (the loop iterates, it does not stop after the first).
//  3. If Stripe returns an error on one subscription, CancelAllActive…
//     stops immediately and returns that error (so the caller can abort
//     the Casdoor delete). Remaining subscriptions are NOT attempted.
func TestPaymentDeletionHelper_CancelAllActiveSubscriptionsForUser_FiltersAndIterates(t *testing.T) {
	t.Run("filters out non-eligible statuses and empty stripe IDs", func(t *testing.T) {
		db := freshTestDB(t)
		userID := "user-pdh-filter"
		planID := uuid.New()

		require.NoError(t, db.Create(&models.SubscriptionPlan{
			BaseModel: entityManagementModels.BaseModel{ID: planID},
			Name:      "test-plan",
		}).Error)

		now := time.Now()
		// Eligible rows: active, trialing, past_due, each with a stripe ID.
		eligible := []models.UserSubscription{
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "active",
				StripeSubscriptionID: pdhStrPtr("sub_active_1"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "trialing",
				StripeSubscriptionID: pdhStrPtr("sub_trialing_1"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "past_due",
				StripeSubscriptionID: pdhStrPtr("sub_pastdue_1"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
		}
		for i := range eligible {
			require.NoError(t, db.Create(&eligible[i]).Error)
		}

		// Ineligible rows that MUST be skipped:
		// - cancelled status
		// - incomplete status
		// - active but nil stripe ID
		// - different user
		ineligible := []models.UserSubscription{
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "cancelled",
				StripeSubscriptionID: pdhStrPtr("sub_cancelled_1"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "incomplete",
				StripeSubscriptionID: pdhStrPtr("sub_incomplete_1"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "active",
				StripeSubscriptionID: nil, // no stripe linkage — must skip
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               "other-user",
				SubscriptionPlanID:   planID,
				Status:               "active",
				StripeSubscriptionID: pdhStrPtr("sub_other_user"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
		}
		for i := range ineligible {
			require.NoError(t, db.Create(&ineligible[i]).Error)
		}

		stub := &stubStripeForPDH{}
		helper := services.NewPaymentDeletionHelperWithDeps(db, stub)
		err := helper.CancelAllActiveSubscriptionsForUser(userID)
		require.NoError(t, err)

		// Exactly the three eligible stripe IDs were cancelled, with
		// cancelAtPeriodEnd=false (immediate cancellation).
		require.Len(t, stub.cancelCalls, 3)
		gotIDs := map[string]bool{}
		for _, c := range stub.cancelCalls {
			gotIDs[c.subscriptionID] = true
			assert.False(t, c.cancelAtPeriodEnd,
				"cancelAtPeriodEnd must be false for account deletion")
		}
		assert.True(t, gotIDs["sub_active_1"])
		assert.True(t, gotIDs["sub_trialing_1"])
		assert.True(t, gotIDs["sub_pastdue_1"])
		// Explicit negative assertions.
		assert.False(t, gotIDs["sub_cancelled_1"], "cancelled subs must not be re-cancelled")
		assert.False(t, gotIDs["sub_incomplete_1"], "incomplete subs must not be cancelled")
		assert.False(t, gotIDs["sub_other_user"], "other users' subs must not be touched")
	})

	t.Run("stops on first Stripe error and returns it", func(t *testing.T) {
		db := freshTestDB(t)
		userID := "user-pdh-stop"
		planID := uuid.New()

		require.NoError(t, db.Create(&models.SubscriptionPlan{
			BaseModel: entityManagementModels.BaseModel{ID: planID},
			Name:      "test-plan",
		}).Error)

		now := time.Now()
		// Two eligible subs. Stripe fails on whichever comes back first —
		// iteration must abort before attempting the second.
		subs := []models.UserSubscription{
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "active",
				StripeSubscriptionID: pdhStrPtr("sub_first"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
			{
				BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
				UserID:               userID,
				SubscriptionPlanID:   planID,
				Status:               "active",
				StripeSubscriptionID: pdhStrPtr("sub_second"),
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     now.Add(30 * 24 * time.Hour),
			},
		}
		for i := range subs {
			require.NoError(t, db.Create(&subs[i]).Error)
		}

		stripeErr := errors.New("stripe API unavailable")
		stub := &stubStripeForPDH{
			cancelFn: func(string, bool) error { return stripeErr },
		}

		helper := services.NewPaymentDeletionHelperWithDeps(db, stub)
		err := helper.CancelAllActiveSubscriptionsForUser(userID)

		require.Error(t, err)
		assert.ErrorIs(t, err, stripeErr,
			"wrapped Stripe error must be retrievable via errors.Is")
		// Exactly ONE Stripe call — iteration stopped after the first failure.
		require.Len(t, stub.cancelCalls, 1)
	})

	t.Run("no-op when user has zero eligible subscriptions", func(t *testing.T) {
		db := freshTestDB(t)
		userID := "user-pdh-empty"
		planID := uuid.New()

		require.NoError(t, db.Create(&models.SubscriptionPlan{
			BaseModel: entityManagementModels.BaseModel{ID: planID},
			Name:      "test-plan",
		}).Error)

		// Only an ineligible row (cancelled status with a stripe ID).
		require.NoError(t, db.Create(&models.UserSubscription{
			BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
			UserID:               userID,
			SubscriptionPlanID:   planID,
			Status:               "cancelled",
			StripeSubscriptionID: pdhStrPtr("sub_old"),
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
		}).Error)

		stub := &stubStripeForPDH{}
		helper := services.NewPaymentDeletionHelperWithDeps(db, stub)
		err := helper.CancelAllActiveSubscriptionsForUser(userID)

		require.NoError(t, err)
		require.Empty(t, stub.cancelCalls,
			"Stripe must not be called when the user has no eligible subscriptions")
	})
}
