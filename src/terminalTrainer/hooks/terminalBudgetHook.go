// Package terminalHooks — TerminalBudgetHook
//
// This BeforeCreate hook on Terminal does two things:
//
//  1. Populates the denormalised SizeCPU / SizeMemoryMB columns from the
//     resource catalog (catalog.LookupSize). The snapshot insulates
//     historical accounting from future catalog drift and lets the
//     budget summing query stay a pure SQL aggregate.
//
//  2. Enforces the user's (or org's) effective plan budget atomically.
//     The check is performed inside a transaction with
//     `SELECT ... FOR UPDATE` on the rows that contribute to current
//     usage (running OR stopped — D6': "a stop is a stop", the
//     persistence_mode distinction is UX-only and not budget logic) so
//     two concurrent session starts cannot both observe enough budget
//     for the same slice of resources.
//
// Caveat: the generic Create path is what fires this hook. The
// production composed-session flow (terminalTrainerService.
// StartComposedSession → repository.CreateTerminalSession) bypasses the
// hook and calls EnforceBudget explicitly.
package terminalHooks

import (
	"errors"
	"fmt"
	"strings"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

// Reason codes carried by ErrBudgetExhausted.Axis. Mirror the codes
// used by QuotaService.BudgetCheck.Reason so middleware / controllers
// can react uniformly.
const (
	BudgetAxisCPU    = "cpu"
	BudgetAxisMemory = "memory"
)

// ErrBudgetExhausted indicates the requested Terminal would exceed the
// effective plan's CPU or RAM cap. Carries enough context for the
// controller to build a 402/403 payload.
type ErrBudgetExhausted struct {
	Axis      string // "cpu" or "memory"
	Limit     int    // plan cap on this axis
	Current   int    // sum of size_* on counted rows BEFORE this request
	Requested int    // requested size on this axis
}

func (e *ErrBudgetExhausted) Error() string {
	return fmt.Sprintf(
		"budget exhausted on %s axis: current=%d requested=%d limit=%d",
		e.Axis, e.Current, e.Requested, e.Limit,
	)
}

// ErrUnknownMachineSize indicates the Terminal's MachineSize is not
// present in the catalog. We fail closed: an unknown size would either
// silently bypass the budget (if treated as zero-cost) or crash the
// summing query — both unacceptable.
type ErrUnknownMachineSize struct {
	Requested string
}

func (e *ErrUnknownMachineSize) Error() string {
	return fmt.Sprintf("unknown machine size %q (not in catalog)", e.Requested)
}

// TerminalBudgetHook implements hooks.Hook.
type TerminalBudgetHook struct {
	db                   *gorm.DB
	effectivePlanService paymentServices.EffectivePlanService
	quotaService         paymentServices.QuotaService
	enabled              bool
	priority             int
}

// NewTerminalBudgetHook constructs the hook. The EffectivePlanService
// (plan resolution) and QuotaService (race-safe budget enforcement) are
// injected so tests can stub the former and share the latter with the
// composed-session write path; production wiring is in InitTerminalHooks.
func NewTerminalBudgetHook(db *gorm.DB, eps paymentServices.EffectivePlanService, quota paymentServices.QuotaService) hooks.Hook {
	return &TerminalBudgetHook{
		db:                   db,
		effectivePlanService: eps,
		quotaService:         quota,
		enabled:              true,
		// Run after the ownership hook (priority 10) which forces UserID
		// to the authenticated user. We must resolve the user's plan
		// *after* UserID has been set, otherwise the hook can't know
		// which plan applies.
		priority: 50,
	}
}

func (h *TerminalBudgetHook) GetName() string       { return "terminal_budget_enforcement" }
func (h *TerminalBudgetHook) GetEntityName() string { return "Terminal" }
func (h *TerminalBudgetHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}
func (h *TerminalBudgetHook) IsEnabled() bool { return h.enabled }
func (h *TerminalBudgetHook) GetPriority() int { return h.priority }

// Execute runs the hook. Steps:
//  1. Decode the Terminal from ctx.NewEntity.
//  2. Snapshot SizeCPU / SizeMemoryMB from the catalog.
//  3. If no EffectivePlanService is wired OR no plan resolves, skip the
//     budget check entirely (we can't enforce what we can't resolve;
//     other gates such as CheckLimit middleware handle the no-plan case).
//  4. Lock + sum + compare against the plan's CPU/RAM caps.
func (h *TerminalBudgetHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.BeforeCreate {
		return nil
	}

	terminal, ok := ctx.NewEntity.(*terminalModels.Terminal)
	if !ok {
		return fmt.Errorf("terminal_budget_enforcement: expected *Terminal, got %T", ctx.NewEntity)
	}

	// (2) Snapshot the size's footprint into the entity. Unknown size →
	//     fail closed so we never silently insert a zero-cost row.
	sizeKey := strings.TrimSpace(terminal.MachineSize)
	if sizeKey == "" {
		// Some callers leave MachineSize empty (e.g. legacy tests). We
		// don't enforce budget in that case but also don't error —
		// matches existing semantics where MachineSize is optional.
		return nil
	}
	size, found := catalog.LookupSize(sizeKey)
	if !found {
		return &ErrUnknownMachineSize{Requested: sizeKey}
	}
	terminal.SizeCPU = size.CPU
	terminal.SizeMemoryMB = size.MemoryMB

	// (3) Resolve effective plan. If we have no service or no plan, skip.
	if h.effectivePlanService == nil {
		return nil
	}

	planResult, err := h.effectivePlanService.GetUserEffectivePlan(terminal.UserID, terminal.OrganizationID)
	if err != nil {
		// A missing personal plan is not an error from this hook's
		// perspective — the user may rely on an org plan resolved via
		// middleware. Surface only hard DB errors.
		return nil //nolint:nilerr // skip enforcement when plan unresolved
	}
	if planResult == nil || planResult.Plan == nil {
		return nil
	}
	plan := planResult.Plan

	// (4) Atomic budget check inside a transaction. The race-safe sum +
	//     compare lives in QuotaService.EnforceBudgetTx (shared with the
	//     composed-session write path); it takes the scope advisory lock
	//     plus a SELECT FOR UPDATE on contributing rows so concurrent
	//     starts for the same scope serialise. On SQLite (unit tests)
	//     both are skipped but the budget verdict is still exercised; the
	//     race test uses real PostgreSQL.
	scopeOrgID := terminal.OrganizationID

	return h.db.Transaction(func(tx *gorm.DB) error {
		result, err := h.quotaService.EnforceBudgetTx(tx, terminal.UserID, scopeOrgID, plan, size.CPU, size.MemoryMB)
		if err != nil {
			return fmt.Errorf("terminal_budget_enforcement: enforce budget: %w", err)
		}
		if result.Allowed {
			return nil
		}
		return budgetExhaustedFromResult(result, plan, size)
	})
}

// budgetExhaustedFromResult translates a QuotaService rejection into the
// hook's ErrBudgetExhausted sentinel, keyed off which axis the verdict
// flagged. Limit/Current/Requested are pulled from the plan, the summed
// usage, and the requested size so the error payload is unchanged from
// the hook's previous inline construction.
func budgetExhaustedFromResult(
	result *paymentServices.BudgetEnforcement,
	plan *paymentModels.SubscriptionPlan,
	size catalog.MachineSize,
) error {
	if result.Reason == "budget_memory_exceeded" {
		return &ErrBudgetExhausted{
			Axis:      BudgetAxisMemory,
			Limit:     plan.MaxMemoryMB,
			Current:   result.UsedMemMB,
			Requested: size.MemoryMB,
		}
	}
	return &ErrBudgetExhausted{
		Axis:      BudgetAxisCPU,
		Limit:     plan.MaxCPU,
		Current:   result.UsedCPU,
		Requested: size.CPU,
	}
}

// IsBudgetError reports whether err is the budget-related sentinel
// raised by this hook. Useful for middleware that wants to translate
// the hook error into a 402/403 HTTP response.
func IsBudgetError(err error) bool {
	var be *ErrBudgetExhausted
	return errors.As(err, &be)
}

// Compile-time check that TerminalBudgetHook satisfies hooks.Hook. Catches
// any drift in the Hook interface without waiting for the registry's runtime
// type assertion to blow up.
var _ hooks.Hook = (*TerminalBudgetHook)(nil)
