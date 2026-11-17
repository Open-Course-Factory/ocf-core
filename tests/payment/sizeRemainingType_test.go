// Package payment_tests — pins the JSON shape of SizeRemaining, whose
// canonical definition lives in the leaf payment/dto package.
package payment_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	paymentDto "soli/formations/src/payment/dto"
)

// TestSizeRemaining_JSONShape — the public JSON shape must remain
// stable; this is what the frontend renders. A field rename here is a
// breaking change for the org terminal usage endpoint and the scenario
// list endpoint.
func TestSizeRemaining_JSONShape(t *testing.T) {
	v := paymentDto.SizeRemaining{Key: "M", CPU: 2, MemoryMB: 1024, RemainingCount: 3}
	out, err := json.Marshal(v)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"key":"M","cpu":2,"memory_mb":1024,"remaining_count":3}`, string(out))
}
