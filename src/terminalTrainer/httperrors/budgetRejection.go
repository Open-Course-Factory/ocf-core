// Package httperrors holds the HTTP renderings of terminal-trainer service
// errors that more than one routes package must emit identically. It is a
// leaf: services stay HTTP-free, and controllers depend on this instead of
// re-encoding the shapes.
package httperrors

import (
	stderrors "errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	services "soli/formations/src/terminalTrainer/services"
)

// WriteBudgetRejection writes the canonical structured 403 for a
// *BudgetRejection error and reports whether it handled the error. Every
// session-creating route (terminal create AND scenario launch/preview) must
// funnel its StartComposedSession error through this before falling back to a
// generic status, so budget exhaustion always reaches the frontend with
// source=budget + a coarse reason instead of a raw 500.
func WriteBudgetRejection(ctx *gin.Context, err error, userID string) bool {
	var budgetErr *services.BudgetRejection
	if !stderrors.As(err, &budgetErr) {
		return false
	}

	// Collapse the CPU- and RAM-axis granular reasons into the single
	// "budget_exhausted" code: the API surface speaks in size-count language,
	// and leaking the axis invites users to game one axis at the expense of
	// the other. The granular reason is logged server-side for debugging.
	publicReason := coarseBudgetReason(budgetErr.Reason)
	if publicReason != budgetErr.Reason {
		slog.Debug("collapsing budget axis reason for HTTP response",
			"internal_reason", budgetErr.Reason,
			"public_reason", publicReason,
			"user_id", userID)
	}
	ctx.JSON(http.StatusForbidden, gin.H{
		"error_code":    http.StatusForbidden,
		"error_message": budgetErr.Error(),
		"source":        "budget",
		"reason":        publicReason,
		"remaining": gin.H{
			"cpu":       budgetErr.RemainingCPU,
			"memory_mb": budgetErr.RemainingMemoryMB,
		},
	})
	return true
}

// coarseBudgetReason collapses the granular CPU- and RAM-axis reasons emitted
// by QuotaService into a single customer-facing code.
func coarseBudgetReason(internal string) string {
	switch internal {
	case "budget_cpu_exceeded", "budget_memory_exceeded":
		return "budget_exhausted"
	default:
		return internal
	}
}
