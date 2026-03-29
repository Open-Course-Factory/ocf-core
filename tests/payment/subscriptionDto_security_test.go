// tests/payment/subscriptionDto_security_test.go
// Security regression tests for subscription DTOs.
// Prevents subscription bypass by ensuring UpdateUserSubscriptionInput
// does not expose the Status field to user-controlled input.
package payment_tests

import (
	"encoding/json"
	"reflect"
	"testing"

	"soli/formations/src/payment/dto"

	"github.com/stretchr/testify/assert"
)

// ==========================================
// Security regression — Status field must NOT be in UpdateUserSubscriptionInput
// ==========================================

// TestUpdateUserSubscriptionInput_NoStatusField verifies via reflection that
// UpdateUserSubscriptionInput does NOT contain a Status field. Exposing Status
// in the update DTO allows any authenticated user to PATCH their subscription
// status to "active" without paying, bypassing the entire payment flow.
func TestUpdateUserSubscriptionInput_NoStatusField(t *testing.T) {
	dtoType := reflect.TypeOf(dto.UpdateUserSubscriptionInput{})
	_, hasStatus := dtoType.FieldByName("Status")

	assert.False(t, hasStatus,
		"SECURITY: UpdateUserSubscriptionInput must NOT contain a Status field. "+
			"Allowing users to set their own subscription status enables payment bypass. "+
			"Status should only be set server-side by webhook handlers or admin endpoints.")
}

// TestUpdateUserSubscriptionInput_StatusJsonPayloadIgnored verifies that a JSON
// payload containing "status": "active" does NOT populate a Status value when
// unmarshalled into UpdateUserSubscriptionInput. This is a defense-in-depth check:
// even if the struct somehow has a status-like field, it must not be deserializable
// from user-controlled JSON input.
func TestUpdateUserSubscriptionInput_StatusJsonPayloadIgnored(t *testing.T) {
	payload := `{"status": "active", "cancel_at_period_end": true}`

	var input dto.UpdateUserSubscriptionInput
	err := json.Unmarshal([]byte(payload), &input)
	assert.NoError(t, err, "Unmarshalling should not fail")

	// Use reflection to check if any field named "Status" got populated
	val := reflect.ValueOf(input)
	statusField := val.FieldByName("Status")
	if statusField.IsValid() {
		assert.Empty(t, statusField.String(),
			"SECURITY: JSON payload with 'status' field must NOT populate Status in "+
				"UpdateUserSubscriptionInput. A user could PATCH their subscription to "+
				"'active' without paying. Status changes must only come from Stripe webhooks.")
	}
	// If Status field doesn't exist at all, the test passes (desired state)
}
