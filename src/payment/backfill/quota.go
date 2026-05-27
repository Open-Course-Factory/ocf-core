// Package backfill provides the one-shot data migration that translates
// the legacy count-based terminal quota
// (AllowedMachineSizes × MaxConcurrentTerminals) into a budget-based
// quota (MaxCPU / MaxMemoryMB) on each SubscriptionPlan, then flips
// QuotaModel to "budget". A reverse `Rollback` is provided for safety.
//
// This package is a one-shot tool retained for the cleanup-deploy era:
// once production has migrated and the legacy columns have been dropped
// by `initialization.dropOrphanSubscriptionPlanColumns`, both `Run` and
// `Rollback` become inert (no plans to update). Kept around so operators
// can re-run safely on staging or replay against historical snapshots.
package backfill

import (
	"fmt"
	"strings"

	"soli/formations/src/payment/catalog"
	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

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
// plan was unrestricted, so we fall back to the catalog's largest entry.
func largestAllowedSize(allowed []string) catalog.MachineSize {
	if len(allowed) == 0 {
		return catalog.LargestSize
	}

	var largest catalog.MachineSize
	matched := false
	for _, raw := range allowed {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if strings.EqualFold(code, "all") {
			return catalog.LargestSize
		}
		size, ok := catalog.LookupSize(code)
		if !ok {
			// Unknown size: be conservative and assume the largest —
			// this keeps the migration safe for any custom plan that
			// snuck in a non-catalog code.
			return catalog.LargestSize
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
		return catalog.LargestSize
	}
	return largest
}
