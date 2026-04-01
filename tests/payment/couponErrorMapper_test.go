// tests/payment/couponErrorMapper_test.go
package payment_tests

import (
	"fmt"
	"testing"

	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v82"
)

func TestMapStripeCouponError_InvalidCode_ReturnsFriendlyError(t *testing.T) {
	// Simulate Stripe "resource_missing" error for an invalid coupon code
	stripeErr := &stripe.Error{
		Code: stripe.ErrorCodeResourceMissing,
		Msg:  "No such coupon: 'INVALID123'",
		Type: stripe.ErrorTypeInvalidRequest,
	}

	result := services.MapStripeCouponError(stripeErr)

	assert.Equal(t, "Invalid coupon code", result.Error())
}

func TestMapStripeCouponError_ExpiredCode_ReturnsFriendlyError(t *testing.T) {
	// Simulate Stripe "coupon_expired" error
	stripeErr := &stripe.Error{
		Code: stripe.ErrorCodeCouponExpired,
		Msg:  "This coupon has expired",
		Type: stripe.ErrorTypeInvalidRequest,
	}

	result := services.MapStripeCouponError(stripeErr)

	assert.Equal(t, "This coupon has expired", result.Error())
}

func TestMapStripeCouponError_ValidCode_ReturnsNil(t *testing.T) {
	// When there is no error, the mapper should return nil
	result := services.MapStripeCouponError(nil)

	assert.Nil(t, result)
}

func TestMapStripeCouponError_NonStripeError_ReturnsFallback(t *testing.T) {
	// A non-Stripe error should get the generic fallback message
	genericErr := fmt.Errorf("network timeout")

	result := services.MapStripeCouponError(genericErr)

	assert.Equal(t, "Unable to apply coupon. Please try again.", result.Error())
}

func TestMapStripeCouponError_UnrelatedStripeError_ReturnsOriginal(t *testing.T) {
	// A Stripe error not related to coupons should pass through unchanged
	stripeErr := &stripe.Error{
		Code: stripe.ErrorCodeCardDeclined,
		Msg:  "Your card was declined",
		Type: stripe.ErrorTypeCard,
	}

	result := services.MapStripeCouponError(stripeErr)

	// Unrelated Stripe errors pass through as-is
	assert.Equal(t, stripeErr, result)
}
