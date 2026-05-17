// Tests the exported paymentServices.SizeKeyInAllowlist helper that
// replaces the previously-unexported sizeKeyInAllowlist plus its
// verbatim duplicate in terminalTrainer/hooks/terminalBudgetHook.go.
//
// Behavioural parity matters because the same D8 allowlist rule is now
// applied in three places: the budget gate inside the BeforeCreate hook,
// the read-time RemainingBudgetFitsWithReason call, and (potentially)
// any future composer-side preview. A drift between them would let a
// user pass one gate but trip another.
package payment_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	paymentServices "soli/formations/src/payment/services"
)

func TestSizeKeyInAllowlist_EmptyAllowlist_AllowsAnything(t *testing.T) {
	assert.True(t, paymentServices.SizeKeyInAllowlist(nil, "L"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{}, "L"))
}

func TestSizeKeyInAllowlist_AllSentinel_AllowsAnything(t *testing.T) {
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"all"}, "L"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"all"}, "xl"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"ALL"}, "M"))
}

func TestSizeKeyInAllowlist_CaseInsensitive(t *testing.T) {
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"L"}, "l"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"l"}, "L"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"  L  "}, "l"))
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"L"}, "  l  "))
}

func TestSizeKeyInAllowlist_NotInAllowlist_Rejects(t *testing.T) {
	assert.False(t, paymentServices.SizeKeyInAllowlist([]string{"XS", "S"}, "L"))
	assert.False(t, paymentServices.SizeKeyInAllowlist([]string{"M"}, "XL"))
}

func TestSizeKeyInAllowlist_BlankEntriesIgnored(t *testing.T) {
	// Blank entries must not be confused for the empty-allowlist sentinel.
	assert.True(t, paymentServices.SizeKeyInAllowlist([]string{"", " ", "L"}, "L"))
	assert.False(t, paymentServices.SizeKeyInAllowlist([]string{"", " "}, "L"))
}
