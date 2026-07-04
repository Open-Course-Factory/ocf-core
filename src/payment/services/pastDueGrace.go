package services

import (
	"os"
	"strconv"
)

// defaultPastDueGraceDays is the dunning grace window used when
// PAYMENT_PAST_DUE_GRACE_DAYS is unset or invalid.
const defaultPastDueGraceDays = 7

// PastDueGraceDays returns the dunning grace window, in days, read from the
// PAYMENT_PAST_DUE_GRACE_DAYS environment variable (default 7). A past_due
// subscription keeps full access until this many days after it went past_due;
// beyond the window, new session-creation paths are gated with a
// 402 subscription_past_due. Kept as a tiny env-reading seam so tests and the
// gate share one source of truth.
func PastDueGraceDays() int {
	if v := os.Getenv("PAYMENT_PAST_DUE_GRACE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultPastDueGraceDays
}
