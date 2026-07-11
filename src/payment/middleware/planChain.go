package middleware

import (
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PlanChain assembles the plan-gating middlewares declared by a PlanRequirement
// into one canonical order, so the previously hand-wired call sites collapse to
// a single builder (the SSOT for plan gating). An empty requirement yields an
// empty chain.
//
// Ordering invariant: the org context must be injected BEFORE plan resolution,
// because InjectEffectivePlan reads org_context_id to let an org-sourced plan
// satisfy RequirePlan. RAM headroom is checked last, once the plan (and the
// requested size, read from the body) are both known.
func PlanChain(db *gorm.DB, req entityManagementInterfaces.PlanRequirement, ts terminalServices.TerminalTrainerService) []gin.HandlerFunc {
	var chain []gin.HandlerFunc

	if req.OrgContext {
		chain = append(chain, InjectOrgContext())
	}
	if req.RequirePlan {
		chain = append(chain, InjectEffectivePlan(services.NewEffectivePlanService(db), db), RequirePlan())
	}
	if req.CheckHostRAM {
		// Fail fast: a CheckHostRAM requirement with a nil TerminalTrainerService
		// is a startup misconfiguration. Panicking here prevents mounting a route
		// whose RAM gate would silently no-op.
		if ts == nil {
			panic("payment.PlanChain: CheckHostRAM requires a non-nil TerminalTrainerService")
		}
		chain = append(chain, CheckRAMAvailability(ts))
	}

	return chain
}

// WithPlanChain assembles a plan-gated route's full gin handler slice — the
// leading middlewares, the plan chain built from req, then the trailing
// handler(s) — as one slice callers spread as gin's sole variadic argument.
//
// It exists because Go forbids mixing individual handler arguments with a ...
// spread in a single call, so a plan chain that must sit BETWEEN leading
// middleware and the handler cannot be spread inline into gin's Handle. Routing
// every plan-gated site through this one seam keeps the wiring order identical
// across call sites.
func WithPlanChain(db *gorm.DB, req entityManagementInterfaces.PlanRequirement, ts terminalServices.TerminalTrainerService, leading []gin.HandlerFunc, tail ...gin.HandlerFunc) []gin.HandlerFunc {
	chain := PlanChain(db, req, ts)
	out := make([]gin.HandlerFunc, 0, len(leading)+len(chain)+len(tail))
	out = append(out, leading...)
	out = append(out, chain...)
	out = append(out, tail...)
	return out
}
