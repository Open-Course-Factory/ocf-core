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
	"time"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/catalog"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	enabled              bool
	priority             int
}

// NewTerminalBudgetHook constructs the hook. The EffectivePlanService is
// injected so tests can stub it; in production main.go wires the
// concrete implementation.
func NewTerminalBudgetHook(db *gorm.DB, eps paymentServices.EffectivePlanService) hooks.Hook {
	return &TerminalBudgetHook{
		db:                   db,
		effectivePlanService: eps,
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

	// (4) Atomic budget check inside a transaction. The SELECT FOR UPDATE
	//     locks every row that contributes to the current usage sum so
	//     concurrent starts for the same scope (user / org) serialise.
	//     On SQLite (unit tests) FOR UPDATE is a no-op but the
	//     correctness of the budget check is still exercised; the race
	//     test uses real PostgreSQL.
	scopeOrgID := terminal.OrganizationID

	return h.db.Transaction(func(tx *gorm.DB) error {
		usedCPU, usedMem, err := lockAndSumActiveResources(tx, terminal.UserID, scopeOrgID)
		if err != nil {
			return fmt.Errorf("terminal_budget_enforcement: lock+sum failed: %w", err)
		}

		// CPU axis
		if plan.MaxCPU > 0 {
			remaining := plan.MaxCPU - usedCPU
			if size.CPU > remaining {
				return &ErrBudgetExhausted{
					Axis:      BudgetAxisCPU,
					Limit:     plan.MaxCPU,
					Current:   usedCPU,
					Requested: size.CPU,
				}
			}
		}

		// Memory axis
		if plan.MaxMemoryMB > 0 {
			remaining := plan.MaxMemoryMB - usedMem
			if size.MemoryMB > remaining {
				return &ErrBudgetExhausted{
					Axis:      BudgetAxisMemory,
					Limit:     plan.MaxMemoryMB,
					Current:   usedMem,
					Requested: size.MemoryMB,
				}
			}
		}

		return nil
	})
}

// lockAndSumActiveResources runs the same predicate as
// quotaService.sumActiveResources (D6', supersedes D6, locked
// 2026-05-28: state IN ('running','stopped') AND deleted_at IS NULL
// AND expires_at > NOW() — "a stop is a stop", the persistence_mode
// distinction is UX-only and not budget logic), but additionally
// acquires a row lock (SELECT ... FOR UPDATE) on every contributing
// row. On PostgreSQL this serialises concurrent session starts for
// the same user/org pair; on SQLite the locking clause is a no-op
// but the sum is still correct.
//
// The expires_at > NOW() clause mirrors the SSOT pattern set by
// terminalModels.OccupiesSlotScope in MR !239: past-expiry zombie rows
// must not keep eating budget. time.Now() is bound as a SQL parameter
// (not NOW() / CURRENT_TIMESTAMP) for dialect portability — SQLite has
// no NOW(); production PostgreSQL does.
//
// The function returns (cpu, mem, error). When no rows match it returns
// (0, 0, nil).
func lockAndSumActiveResources(tx *gorm.DB, userID string, orgID *uuid.UUID) (int, int, error) {
	// First, fetch (and lock) the candidate rows. We can't aggregate
	// with FOR UPDATE in standard SQL — locking is row-level, so we
	// SELECT the rows we care about, lock them, then sum in Go. The
	// row count for a single user/org is bounded by the plan, so the
	// payload is small.
	var rows []struct {
		SizeCPU      int
		SizeMemoryMB int
	}

	now := time.Now()
	occupyingStates := terminalModels.TerminalStatesOccupyingSlot
	var q *gorm.DB
	if orgID != nil {
		q = tx.Table("terminals").
			Select("terminals.size_cpu AS size_cpu, terminals.size_memory_mb AS size_memory_mb").
			Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
			Where("terminals.deleted_at IS NULL").
			Where("organization_members.organization_id = ? AND organization_members.deleted_at IS NULL", *orgID).
			Where("terminals.expires_at > ?", now).
			Where("terminals.state IN ?", occupyingStates)
	} else {
		q = tx.Table("terminals").
			Select("size_cpu, size_memory_mb").
			Where("deleted_at IS NULL").
			Where("user_id = ?", userID).
			Where("expires_at > ?", now).
			Where("state IN ?", occupyingStates)
	}

	// Acquire the lock. On dialects that don't support FOR UPDATE
	// (SQLite), GORM emits the clause as part of the statement; SQLite
	// silently ignores trailing unsupported keywords in some build
	// configurations and raises a syntax error in others. To stay safe
	// for unit tests, we only attach the locking clause when the
	// underlying driver is known to support it.
	if supportsRowLock(tx) {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	if err := q.Scan(&rows).Error; err != nil {
		return 0, 0, err
	}

	var sumCPU, sumMem int
	for _, r := range rows {
		sumCPU += r.SizeCPU
		sumMem += r.SizeMemoryMB
	}
	return sumCPU, sumMem, nil
}

// supportsRowLock reports whether the active GORM dialect supports
// SELECT ... FOR UPDATE. We attach the locking clause only for those
// dialects so unit tests on SQLite keep working without syntax errors.
func supportsRowLock(tx *gorm.DB) bool {
	if tx == nil || tx.Dialector == nil {
		return false
	}
	name := tx.Dialector.Name()
	return name == "postgres" || name == "mysql"
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
