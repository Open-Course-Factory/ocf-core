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

	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

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
