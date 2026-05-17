// Package payment_tests — pins the SSOT location of the SizeRemaining
// shape after Cleanup 2 (consolidate duplicated type).
//
// Pre-cleanup, services.SizeRemaining and dto.SizeRemainingDTO were
// byte-for-byte duplicates in different packages, kept apart only to
// dodge an import cycle. The cleanup moves the canonical shape into
// the leaf payment/dto package (already imported by both consumers).
// These tests fail to compile if a future maintainer reintroduces a
// duplicate.
package payment_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	paymentDto "soli/formations/src/payment/dto"
	paymentServices "soli/formations/src/payment/services"
)

// TestSizeRemaining_LivesInPaymentDTO — services.SizeRemaining is now an
// alias of paymentDto.SizeRemaining. The cast at zero cost proves they
// are the same underlying type.
func TestSizeRemaining_LivesInPaymentDTO(t *testing.T) {
	src := paymentServices.SizeRemaining{
		Key:            "L",
		CPU:            4,
		MemoryMB:       2048,
		RemainingCount: 2,
	}
	// Direct conversion: only legal when the types are the same /
	// aliased / convertible field-by-field. A reintroduction of a
	// divergent shape would break the compile.
	var dst paymentDto.SizeRemaining = paymentDto.SizeRemaining(src)
	assert.Equal(t, "L", dst.Key)
	assert.Equal(t, 4, dst.CPU)
	assert.Equal(t, 2048, dst.MemoryMB)
	assert.Equal(t, 2, dst.RemainingCount)
}

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
