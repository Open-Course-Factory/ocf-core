// src/payment/services/couponErrorMapper.go
package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v82"
)

// couponRelatedStripeErrors maps Stripe error codes to user-friendly messages
// for coupon/discount-related errors.
var couponRelatedStripeErrors = map[stripe.ErrorCode]string{
	stripe.ErrorCodeCouponExpired:   "This coupon has expired",
	stripe.ErrorCodeResourceMissing: "Invalid coupon code",
}

// MapStripeCouponError converts a Stripe coupon-related error into a
// user-friendly error message. Returns nil if err is nil. Returns the
// original error unchanged if it is not coupon-related.
func MapStripeCouponError(err error) error {
	if err == nil {
		return nil
	}

	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) {
		// Not a Stripe error at all — return generic fallback
		return fmt.Errorf("Unable to apply coupon. Please try again.")
	}

	// Check for known coupon error codes
	if msg, ok := couponRelatedStripeErrors[stripeErr.Code]; ok {
		return fmt.Errorf("%s", msg)
	}

	// Check if the error message mentions coupon/promotion even if code is
	// not in our map (e.g., "promotion_code_inactive" or future codes).
	lowerMsg := strings.ToLower(stripeErr.Msg)
	if strings.Contains(lowerMsg, "coupon") || strings.Contains(lowerMsg, "promotion") {
		return fmt.Errorf("Unable to apply coupon. Please try again.")
	}

	// Not coupon-related — return the original Stripe error unchanged
	return stripeErr
}
