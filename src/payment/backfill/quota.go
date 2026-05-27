// Package backfill provides the one-shot data migration that translated
// the legacy count-based terminal quota (AllowedMachineSizes ×
// MaxConcurrentTerminals) into a budget-based quota (MaxCPU /
// MaxMemoryMB) on each SubscriptionPlan. A reverse `Rollback` is
// provided for safety.
//
// This package is a one-shot tool retained for the cleanup-deploy era:
// once production has migrated and the legacy columns have been dropped
// by `initialization.dropOrphanSubscriptionPlanColumns`, both `Run` and
// `Rollback` become inert (the legacy column reads return zero). Kept
// around so operators can replay against historical snapshots.
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

// legacyPlanLimits is a read-only projection of the now-removed columns,
// fetched directly via SQL so the model struct can stay free of dead
// fields. Returns zeroes when the columns are absent (post-cleanup DB).
type legacyPlanLimits struct {
	MaxConcurrentTerminals int
	AllowedMachineSizes    string // JSON-encoded []string
}

// Run applies the count → budget migration. Idempotent: plans already
// carrying a non-zero MaxCPU or MaxMemoryMB are skipped.
func Run(db *gorm.DB, opts Options) (*Report, error) {
	var plans []models.SubscriptionPlan
	if err := db.Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}

	report := &Report{Total: len(plans)}

	apply := func(tx *gorm.DB) error {
		for i := range plans {
			plan := &plans[i]
			if plan.MaxCPU > 0 || plan.MaxMemoryMB > 0 {
				report.Skipped++
				report.Plans = append(report.Plans, PlanReport{
					PlanID:      plan.ID.String(),
					Name:        plan.Name,
					MaxCPU:      plan.MaxCPU,
					MaxMemoryMB: plan.MaxMemoryMB,
					Reason:      "already on budget model",
				})
				continue
			}

			cpu, mem, reason := derivePlanBudget(tx, plan)
			pr := PlanReport{
				PlanID:      plan.ID.String(),
				Name:        plan.Name,
				MaxCPU:      cpu,
				MaxMemoryMB: mem,
				Reason:      reason,
			}

			if opts.Apply {
				updates := map[string]any{
					"max_cpu":       cpu,
					"max_memory_mb": mem,
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

// Rollback reverses Run: clears MaxCPU/MaxMemoryMB. The legacy columns
// (if still present on the DB) are not touched.
func Rollback(db *gorm.DB, opts Options) (*Report, error) {
	var plans []models.SubscriptionPlan
	if err := db.Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("load plans: %w", err)
	}

	report := &Report{Total: len(plans)}

	apply := func(tx *gorm.DB) error {
		for i := range plans {
			plan := &plans[i]
			if plan.MaxCPU == 0 && plan.MaxMemoryMB == 0 {
				report.Skipped++
				report.Plans = append(report.Plans, PlanReport{
					PlanID: plan.ID.String(),
					Name:   plan.Name,
					Reason: "already cleared",
				})
				continue
			}

			pr := PlanReport{
				PlanID:      plan.ID.String(),
				Name:        plan.Name,
				MaxCPU:      0,
				MaxMemoryMB: 0,
				Reason:      "rollback to zero budget",
			}

			if opts.Apply {
				updates := map[string]any{
					"max_cpu":       0,
					"max_memory_mb": 0,
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

// derivePlanBudget translates a plan's legacy count-based limits into a
// CPU/RAM budget. Reads the legacy columns directly via SQL so it works
// before the dropOrphanSubscriptionPlanColumns migration has run; after
// the columns are dropped it returns (0, 0, …) and the plan is left at
// unlimited (which is the safe fallback during a re-run on a clean DB).
func derivePlanBudget(db *gorm.DB, plan *models.SubscriptionPlan) (cpu, mem int, reason string) {
	legacy := readLegacyLimits(db, plan.ID.String())
	if legacy.MaxConcurrentTerminals <= 0 {
		return 0, 0, fmt.Sprintf("count(%d) → budget(unlimited/zero)", legacy.MaxConcurrentTerminals)
	}

	largest := largestAllowedSize(parseAllowedSizes(legacy.AllowedMachineSizes))
	cpu = largest.CPU * legacy.MaxConcurrentTerminals
	mem = largest.MemoryMB * legacy.MaxConcurrentTerminals
	reason = fmt.Sprintf("count(%d) × max(%dc/%dMiB) → budget(%d vCPU / %d MiB)",
		legacy.MaxConcurrentTerminals, largest.CPU, largest.MemoryMB, cpu, mem)
	return cpu, mem, reason
}

// readLegacyLimits fetches the legacy columns from the DB if they still
// exist. Any error (column missing on post-cleanup DBs) is swallowed and
// returns zero limits.
func readLegacyLimits(db *gorm.DB, planID string) legacyPlanLimits {
	var out legacyPlanLimits
	_ = db.Table("subscription_plans").
		Select("max_concurrent_terminals, allowed_machine_sizes").
		Where("id = ?", planID).
		Row().
		Scan(&out.MaxConcurrentTerminals, &out.AllowedMachineSizes)
	return out
}

// parseAllowedSizes turns a JSON-encoded []string into a Go slice. The
// migration was lenient about format (some plans stored a CSV); we
// keep that lenience here by splitting on either commas or JSON list
// delimiters.
func parseAllowedSizes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(strings.Trim(part, `"`))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
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
