// src/payment/services/userDeletionHelper.go
//
// PaymentDeletionHelper encapsulates the payment-side cleanup performed when
// a user account is deleted. It is consumed by src/auth/services/userService
// via a narrow interface so the deletion orchestration can be unit-tested
// without pulling the full payment service graph into userService.
//
// Invoices are intentionally LEFT UNTOUCHED: French commercial law
// (Art. L. 123-22 Code de commerce) mandates a 10-year retention on
// billing/accounting records. The legal basis for that retention is
// legitimate interest (GDPR Art. 6.1.f), and pseudonymization is therefore
// limited to identifying fields in BillingAddress and PaymentMethod rows.
package services

import (
	"errors"
	"fmt"
	"net/http"

	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"github.com/stripe/stripe-go/v85"
	stripeCustomer "github.com/stripe/stripe-go/v85/customer"
	"gorm.io/gorm"
)

// Placeholder values for pseudonymized fields. CardLast4 is varchar(4) in
// PostgreSQL so it gets a shorter sentinel than the longer textual fields.
const (
	pseudonymizedTextPlaceholder = "[deleted]"
	pseudonymizedLast4           = "****"
)

// PaymentDeletionHelper is the narrow contract userService depends on to
// cascade the payment-side cleanup on user deletion.
type PaymentDeletionHelper interface {
	// CancelAllActiveSubscriptionsForUser cancels every "active-ish" Stripe
	// subscription owned by the user (statuses active, trialing, past_due
	// with a non-empty StripeSubscriptionID). Returns the first error
	// encountered — callers rely on "stop on first error" semantics: if this
	// returns non-nil, the caller MUST abort the deletion so Casdoor is not
	// dropped while Stripe is still billing.
	CancelAllActiveSubscriptionsForUser(userID string) error

	// PseudonymizeBillingDataForUser replaces PII in BillingAddress and
	// PaymentMethod rows with a neutral placeholder. Invoices are left
	// untouched (10y legal retention). Best-effort: callers log and continue
	// on failure, because the security-critical work (Stripe cancel) has
	// already happened by the time this runs.
	PseudonymizeBillingDataForUser(userID string) error
}

// paymentDeletionHelper is the production implementation.
type paymentDeletionHelper struct {
	db            *gorm.DB
	stripeService StripeService
}

// NewPaymentDeletionHelper returns a PaymentDeletionHelper wired to the real
// Stripe service and the provided DB handle.
func NewPaymentDeletionHelper(db *gorm.DB) PaymentDeletionHelper {
	return &paymentDeletionHelper{
		db:            db,
		stripeService: NewStripeService(db),
	}
}

// NewPaymentDeletionHelperWithDeps creates a PaymentDeletionHelper with
// injectable dependencies (used by tests that stub Stripe).
func NewPaymentDeletionHelperWithDeps(db *gorm.DB, stripeService StripeService) PaymentDeletionHelper {
	return &paymentDeletionHelper{
		db:            db,
		stripeService: stripeService,
	}
}

// CancelAllActiveSubscriptionsForUser — see interface docstring.
func (h *paymentDeletionHelper) CancelAllActiveSubscriptionsForUser(userID string) error {
	if h.db == nil {
		return errors.New("paymentDeletionHelper: db is nil")
	}

	activeStatuses := []string{"active", "trialing", "past_due"}

	var subs []models.UserSubscription
	err := h.db.
		Where("user_id = ? AND status IN ? AND stripe_subscription_id IS NOT NULL AND stripe_subscription_id <> ''",
			userID, activeStatuses).
		Find(&subs).Error
	if err != nil {
		return fmt.Errorf("failed to load active subscriptions for user %s: %w", userID, err)
	}

	if len(subs) == 0 {
		return nil
	}

	for _, sub := range subs {
		if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
			continue
		}
		if err := h.stripeService.CancelSubscription(*sub.StripeSubscriptionID, false); err != nil {
			return fmt.Errorf("failed to cancel Stripe subscription %s for user %s: %w",
				*sub.StripeSubscriptionID, userID, err)
		}
	}

	return nil
}

// DeleteStripeCustomersForUser erases the Stripe Customer object(s) the user
// OWNS, satisfying the RGPD / GDPR Art. 17 right to erasure (the existing flow
// only cancelled subscriptions and pseudonymized local PII, leaving the user's
// identifying data on Stripe indefinitely).
//
// Scope — only customers the user OWNS. An assigned bulk-license row has
// user_id = the assignee but stripe_customer_id = the PURCHASER's customer, and
// purchaser_user_id = that purchaser; deleting it when the assignee is erased
// would wrongly destroy the purchaser's Stripe Customer. Self-purchased rows
// have purchaser_user_id NULL (see UserSubscription: "null = self-purchase") or
// equal to the user, so `purchaser_user_id IS NULL OR purchaser_user_id =
// user_id` is exactly "the customer id belongs to this user". organization_
// subscriptions customers are org-owned and out of scope.
//
// Fail-closed (mirrors CancelAllActiveSubscriptionsForUser): a hard Stripe error
// aborts and returns, so the caller does not drop the Casdoor account while the
// customer survives. Idempotent: a 404 / resource_missing (already deleted)
// counts as success. Deleting a customer also cancels its remaining
// subscriptions server-side, so callers run this AFTER the cancel step.
//
// This method is intentionally NOT part of the PaymentDeletionHelper interface:
// the orchestration invokes it via a capability assertion, so helper stand-ins
// that predate it (test mocks) are unaffected.
func (h *paymentDeletionHelper) DeleteStripeCustomersForUser(userID string) error {
	if h.db == nil {
		return errors.New("paymentDeletionHelper: db is nil")
	}

	var customerIDs []string
	err := h.db.Model(&models.UserSubscription{}).
		Where("user_id = ? AND stripe_customer_id IS NOT NULL AND stripe_customer_id <> '' "+
			"AND (purchaser_user_id IS NULL OR purchaser_user_id = user_id)", userID).
		Distinct().
		Pluck("stripe_customer_id", &customerIDs).Error
	if err != nil {
		return fmt.Errorf("failed to load Stripe customer ids for user %s: %w", userID, err)
	}

	for _, customerID := range customerIDs {
		if _, delErr := stripeCustomer.Del(customerID, nil); delErr != nil {
			if isStripeResourceMissing(delErr) {
				// Already gone on Stripe's side — idempotent success.
				utils.Info("Stripe customer %s already absent for user %s (treating as erased)", customerID, userID)
				continue
			}
			return fmt.Errorf("failed to delete Stripe customer %s for user %s: %w", customerID, userID, delErr)
		}
		utils.Info("Deleted Stripe customer %s for user %s (RGPD erasure)", customerID, userID)
	}

	return nil
}

// isStripeResourceMissing reports whether err is a Stripe 404 / resource_missing
// (the customer was already deleted), which erasure treats as success.
func isStripeResourceMissing(err error) bool {
	var se *stripe.Error
	if errors.As(err, &se) {
		return se.HTTPStatusCode == http.StatusNotFound || se.Code == stripe.ErrorCodeResourceMissing
	}
	return false
}

// PseudonymizeBillingDataForUser — see interface docstring.
//
// Both updates run inside a single transaction: either both succeed or neither
// lands. On any error the caller logs and continues — the important security
// work (Stripe cancel) is already done by the time this runs.
func (h *paymentDeletionHelper) PseudonymizeBillingDataForUser(userID string) error {
	if h.db == nil {
		return errors.New("paymentDeletionHelper: db is nil")
	}

	return h.db.Transaction(func(tx *gorm.DB) error {
		// Pseudonymize BillingAddress rows. Country is preserved for tax/
		// audit traceability — it is not personally identifying on its own.
		if err := tx.Model(&models.BillingAddress{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{
				"line1":       pseudonymizedTextPlaceholder,
				"line2":       pseudonymizedTextPlaceholder,
				"city":        pseudonymizedTextPlaceholder,
				"state":       pseudonymizedTextPlaceholder,
				"postal_code": pseudonymizedTextPlaceholder,
			}).Error; err != nil {
			return fmt.Errorf("failed to pseudonymize billing addresses for user %s: %w", userID, err)
		}

		// Pseudonymize PaymentMethod PII. StripePaymentMethodID is preserved
		// so invoices remain traceable for the 10-year legal retention.
		if err := tx.Model(&models.PaymentMethod{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{
				"card_brand":     pseudonymizedTextPlaceholder,
				"card_last4":     pseudonymizedLast4,
				"card_exp_month": 0,
				"card_exp_year":  0,
				"is_active":      false,
			}).Error; err != nil {
			return fmt.Errorf("failed to pseudonymize payment methods for user %s: %w", userID, err)
		}

		utils.Info("Pseudonymized billing data for user %s", userID)
		return nil
	})
}
