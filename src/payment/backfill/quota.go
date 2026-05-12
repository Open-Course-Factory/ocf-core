// Package backfill provides one-off data migrations for payment plans.
//
// The current migration translates the legacy count-based terminal quota
// (AllowedMachineSizes × MaxConcurrentTerminals) into a budget-based quota
// (MaxCPU / MaxMemoryMB) on each SubscriptionPlan, then flips QuotaModel
// to "budget". A reverse `Rollback` is provided for safety.
//
// The migration is intentionally additive: existing AllowedMachineSizes
// and MaxConcurrentTerminals are left untouched and will continue to be
// honoured by all current readers. The new MaxCPU/MaxMemoryMB columns are
// only consumed once subsequent MRs wire them through the middleware.
package backfill

import (
	"fmt"
	"strings"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// MachineSize describes one entry in the resource catalog used to derive
// budget caps from the legacy AllowedMachineSizes field.
//
// MIRROR OF tt-backend/backend/db.go `dbSeedSizes` — keep in sync. If a
// new size is added on the terminal backend it must be mirrored here so
// the backfill produces accurate budgets.
type MachineSize struct {
	CPU      int
	MemoryMB int
}

// sizeCatalog is keyed by both the canonical uppercase code and the
// lowercase variant that some existing plans store.
var sizeCatalog = map[string]MachineSize{
	"XS": {CPU: 1, MemoryMB: 256},
	"xs": {CPU: 1, MemoryMB: 256},
	"S":  {CPU: 1, MemoryMB: 512},
	"s":  {CPU: 1, MemoryMB: 512},
	"M":  {CPU: 2, MemoryMB: 1024},
	"m":  {CPU: 2, MemoryMB: 1024},
	"L":  {CPU: 4, MemoryMB: 2048},
	"l":  {CPU: 4, MemoryMB: 2048},
	"XL": {CPU: 4, MemoryMB: 4096},
	"xl": {CPU: 4, MemoryMB: 4096},
}

// xlSize is the implicit cap when AllowedMachineSizes is empty or
// contains "all" — matches the catalog's largest entry.
var xlSize = MachineSize{CPU: 4, MemoryMB: 4096}

// Options controls a backfill / rollback run.
type Options struct {
	// Apply commits the changes. When false (the default), the migration
	// only reports what it would do without writing anything.
	Apply bool
}

// PlanReport is the per-plan delta the migration emits to stdout.
type PlanReport struct {
	PlanID      string
	Name        string
	BeforeModel string
	AfterModel  string
	MaxCPU      int
	MaxMemoryMB int
	Reason      string
}

// Report aggregates the outcome of a single Run / Rollback invocation.
type Report struct {
	Total       int
	Updated     int
	WouldUpdate int
	Skipped     int
	Plans       []PlanReport
}

// Run applies the count → budget migration. It is idempotent: plans
// already on the budget model are skipped.
func Run(db *gorm.DB, opts Options) (*Report, error) {
	var plans []models.SubscriptionPlan
	if err := db.Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}

	report := &Report{Total: len(plans)}

	apply := func(tx *gorm.DB) error {
		for i := range plans {
			plan := &plans[i]
			if plan.QuotaModel == "budget" {
				report.Skipped++
				report.Plans = append(report.Plans, PlanReport{
					PlanID:      plan.ID.String(),
					Name:        plan.Name,
					BeforeModel: plan.QuotaModel,
					AfterModel:  plan.QuotaModel,
					MaxCPU:      plan.MaxCPU,
					MaxMemoryMB: plan.MaxMemoryMB,
					Reason:      "already on budget model",
				})
				continue
			}

			cpu, mem, reason := derivePlanBudget(plan)
			pr := PlanReport{
				PlanID:      plan.ID.String(),
				Name:        plan.Name,
				BeforeModel: plan.QuotaModel,
				AfterModel:  "budget",
				MaxCPU:      cpu,
				MaxMemoryMB: mem,
				Reason:      reason,
			}

			if opts.Apply {
				updates := map[string]any{
					"max_cpu":       cpu,
					"max_memory_mb": mem,
					"quota_model":   "budget",
				}
				if err := tx.Model(plan).Updates(updates).Error; err != nil {
					return fmt.Errorf("update plan %s: %w", plan.ID, err)
				}
				report.Updated++
			} else {
				report.WouldUpdate++
			}
			report.Plans = append(report.Plans, pr)
		}
		return nil
	}

	if opts.Apply {
		if err := db.Transaction(apply); err != nil {
			return nil, err
		}
	} else {
		// Dry-run: still wrap in a transaction so we can roll it back
		// even though we never call apply mutators. Cheaper: just call
		// the closure with the same db handle — we never write.
		if err := apply(db); err != nil {
			return nil, err
		}
	}

	return report, nil
}

// Rollback reverses Run: clears MaxCPU/MaxMemoryMB and flips QuotaModel
// back to "count". AllowedMachineSizes is preserved.
func Rollback(db *gorm.DB, opts Options) (*Report, error) {
	var plans []models.SubscriptionPlan
	if err := db.Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}

	report := &Report{Total: len(plans)}

	apply := func(tx *gorm.DB) error {
		for i := range plans {
			plan := &plans[i]
			if plan.QuotaModel == "count" && plan.MaxCPU == 0 && plan.MaxMemoryMB == 0 {
				report.Skipped++
				report.Plans = append(report.Plans, PlanReport{
					PlanID:      plan.ID.String(),
					Name:        plan.Name,
					BeforeModel: plan.QuotaModel,
					AfterModel:  plan.QuotaModel,
					Reason:      "already on count model",
				})
				continue
			}

			pr := PlanReport{
				PlanID:      plan.ID.String(),
				Name:        plan.Name,
				BeforeModel: plan.QuotaModel,
				AfterModel:  "count",
				MaxCPU:      0,
				MaxMemoryMB: 0,
				Reason:      "rollback to count model",
			}

			if opts.Apply {
				updates := map[string]any{
					"max_cpu":       0,
					"max_memory_mb": 0,
					"quota_model":   "count",
				}
				if err := tx.Model(plan).Updates(updates).Error; err != nil {
					return fmt.Errorf("rollback plan %s: %w", plan.ID, err)
				}
				report.Updated++
			} else {
				report.WouldUpdate++
			}
			report.Plans = append(report.Plans, pr)
		}
		return nil
	}

	if opts.Apply {
		if err := db.Transaction(apply); err != nil {
			return nil, err
		}
	} else {
		if err := apply(db); err != nil {
			return nil, err
		}
	}

	return report, nil
}

// derivePlanBudget translates the legacy count-based limits into a
// budget. Returns (cpu, mem, reason) where reason is a human-readable
// summary suitable for the migration log line.
func derivePlanBudget(plan *models.SubscriptionPlan) (cpu, mem int, reason string) {
	// Unlimited terminals → unlimited budget. Same for explicit zero
	// (no terminals allowed) — both map to "0" sentinel.
	if plan.MaxConcurrentTerminals <= 0 {
		return 0, 0, fmt.Sprintf("count(%d) → budget(unlimited/zero)", plan.MaxConcurrentTerminals)
	}

	largest := largestAllowedSize(plan.AllowedMachineSizes)
	cpu = largest.CPU * plan.MaxConcurrentTerminals
	mem = largest.MemoryMB * plan.MaxConcurrentTerminals
	reason = fmt.Sprintf("count(%d) × max(%dc/%dMiB) → budget(%d vCPU / %d MiB)",
		plan.MaxConcurrentTerminals, largest.CPU, largest.MemoryMB, cpu, mem)
	return cpu, mem, reason
}

// largestAllowedSize returns the worst-case size implied by an
// AllowedMachineSizes value. Empty list or any "all" entry means the
// plan was unrestricted, so we fall back to XL.
func largestAllowedSize(allowed []string) MachineSize {
	if len(allowed) == 0 {
		return xlSize
	}

	var largest MachineSize
	matched := false
	for _, raw := range allowed {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if strings.EqualFold(code, "all") {
			return xlSize
		}
		size, ok := sizeCatalog[code]
		if !ok {
			// Unknown size: be conservative and assume XL — this keeps
			// the migration safe for any custom plan that snuck in a
			// non-catalog code.
			return xlSize
		}
		matched = true
		if size.CPU > largest.CPU {
			largest.CPU = size.CPU
		}
		if size.MemoryMB > largest.MemoryMB {
			largest.MemoryMB = size.MemoryMB
		}
	}
	if !matched {
		return xlSize
	}
	return largest
}
