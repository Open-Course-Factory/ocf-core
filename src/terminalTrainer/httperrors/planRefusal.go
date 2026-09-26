package httperrors

import "strings"

// IsPlanRefusal reports whether a StartComposedSession error is the plan
// refusing the machine asked for — a size, a feature or persistence it does
// not cover — rather than a failure. Every session-creating route answers it
// with a 403, not a 500.
//
// tt-backend and the composer say so only in the error text, so this is the
// one place that reads it.
func IsPlanRefusal(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "plan_limit") || strings.Contains(msg, "plan_disabled") || strings.Contains(msg, "not allowed")
}
