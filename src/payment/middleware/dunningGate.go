package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/utils"
)

// GatePastDueBeyondGrace rejects a NEW session-creation request when the
// caller's effective personal subscription is past_due and its grace window
// (PastDueGraceDays) has elapsed.
//
// STRUCTURAL CONTRACT: every NEW session-creation route MUST call this helper
// (and return on true) — terminal creation/resume/bulk AND scenario
// launch/preview. Any new path that provisions or resumes a terminal must add
// the same call, or a past_due customer slips past the dunning gate. It
// previously lived as a private method on the terminal controller, which is
// exactly how the scenario launch path shipped without it.
//
// It reads the EffectivePlanResult injected by InjectEffectivePlan, so it must
// run on routes that carry that middleware. It writes a 402 with the stable
// error code `subscription_past_due` (mirroring the structured BudgetRejection
// response shape) and returns true when it rejected — callers must return
// immediately.
//
// Only a personal past_due subscription is gated (org-sourced plans have no
// per-user dunning stamp here). Legacy past_due rows with a NULL PastDueSince —
// which entered past_due before this shipped — are treated as within grace so
// they are never locked out instantly (#371); a subsequent failed invoice will
// stamp them and start the clock.
func GatePastDueBeyondGrace(ctx *gin.Context) bool {
	val, exists := ctx.Get("effective_plan_result")
	if !exists {
		return false
	}
	result, ok := val.(*paymentServices.EffectivePlanResult)
	if !ok {
		return false
	}
	return GatePastDueBeyondGraceForResult(ctx, result)
}

// GatePastDueBeyondGraceForResult is the middleware-free core of the gate,
// for handlers that resolve their EffectivePlanResult manually (scenario
// preview) instead of through InjectEffectivePlan.
func GatePastDueBeyondGraceForResult(ctx *gin.Context, result *paymentServices.EffectivePlanResult) bool {
	if result == nil || result.UserSubscription == nil {
		return false
	}
	sub := result.UserSubscription
	if sub.Status != "past_due" || sub.PastDueSince == nil {
		return false
	}
	grace := time.Duration(paymentServices.PastDueGraceDays()) * 24 * time.Hour
	if time.Since(*sub.PastDueSince) <= grace {
		return false // still within the grace window
	}

	utils.Warn("🚫 Blocking new session for user %s: subscription past_due since %s (grace %d days elapsed)",
		ctx.GetString("userId"), sub.PastDueSince.Format(time.RFC3339), paymentServices.PastDueGraceDays())
	ctx.JSON(http.StatusPaymentRequired, gin.H{
		"error_code":    "subscription_past_due",
		"error_message": "Your subscription payment is overdue. Please update your payment method to start new sessions.",
		"source":        "dunning",
	})
	ctx.Abort()
	return true
}
