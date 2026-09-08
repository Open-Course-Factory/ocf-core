package services

import (
	"fmt"
	"slices"
	"strings"

	"soli/formations/src/payment/catalog"
	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// Plan health: what a subscription plan promises, measured against what it can
// actually deliver.
//
// Every check here asks a question the platform already answers at the moment
// it matters — the budget engine when a session starts, the plan resolver when
// a subscription is read, Stripe when someone tries to pay — and asks it early
// enough for an operator to fix the answer. The answers come from the owners
// of those rules: MissingBudgetAxes for a zero budget, danglingReferencesTo
// (ScopeEntitling, role mappings included) for a deleted plan still held, and
// QuotaService.ComputeRemainingBySize for what a budget affords. The size
// table is shared; the per-axis division in axisImbalanceFinding is the one
// arithmetic this file owns.
//
// The faults share a shape with the scenario ones: nothing errors, nothing
// logs, and the plan looks correct in the admin form. A zero budget reads as a
// perfectly ordinary row until a class cannot start.

// Severities. Blocking means the plan cannot deliver something it is sold as
// delivering; warning means it can, but not through the path a customer takes.
// Advisory means nothing is wrong — it is a number worth knowing.
const (
	PlanHealthBlocking = "blocking"
	PlanHealthWarning  = "warning"
	PlanHealthAdvisory = "advisory"
)

// Finding codes. Stable strings, because the front end writes the sentence from
// the code and a reworded one must not silently become untranslated.
//
// Every code here must have a sentence in ocf-front
// src/components/Pages/Admin/PlanHealth.vue and be listed in ocf-front
// tests/components/PlanHealth-findingCodes.test.ts.
const (
	PlanHealthZeroBudget          = "zero_budget"
	PlanHealthDanglingReference   = "dangling_plan_reference"
	PlanHealthCatalogWithoutPrice = "catalog_without_price"
	PlanHealthAffordsNoSize       = "affords_no_size"
	PlanHealthAxisImbalance       = "axis_imbalance"
)

// axisImbalanceRatio is how far the two budget axes must diverge before the
// advisory is worth an operator's attention.
//
// 2 is not arbitrary: it is the shape every imbalanced plan in production
// actually has — CPU affording exactly half what RAM allows — and it is what
// made "Formateur affords 6 terminals" surprising to someone reading 6 GiB of
// RAM. A lower threshold would report rounding.
const axisImbalanceRatio = 2

// PlanHealthFinding is one thing worth saying about one plan.
type PlanHealthFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	// Detail carries the numbers a reader needs to act — which axis, how many
	// sessions, how many subscribers — in the platform's language rather than
	// the reader's. The front end writes the sentence from Code; this fills in
	// what it cannot know.
	Detail string `json:"detail,omitempty"`
}

// PlanHealth is the report for one plan.
type PlanHealth struct {
	PlanID      string              `json:"plan_id"`
	Name        string              `json:"name"`
	IsActive    bool                `json:"is_active"`
	IsCatalog   bool                `json:"is_catalog"`
	IsDeleted   bool                `json:"is_deleted"`
	MaxCPU      int                 `json:"max_cpu"`
	MaxMemoryMB int                 `json:"max_memory_mb"`
	Findings    []PlanHealthFinding `json:"findings"`
}

// CheckAllPlanHealth reports every plan with something worth saying about it,
// deleted plans included — a retired plan still entitles whoever is subscribed
// to it, which is exactly the fault worth catching.
//
// Plans with nothing wrong are absent rather than listed as healthy: the report
// is a list of things to fix, and a page that has to filter out its own good
// news reads as noise.
//
// The only error is failing to read the plans at all, and then the page fails
// whole: a partial report would present the plans it did read as the complete
// list of faults, which is the silence this page exists to break. The per-plan
// checks do not error; a count that fails is logged and reads as zero, like the
// startup report it shares its query with.
func CheckAllPlanHealth(db *gorm.DB, quota QuotaService) ([]PlanHealth, error) {
	var plans []models.SubscriptionPlan
	if err := db.Unscoped().Order("name ASC").Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("cannot read the plans: %w", err)
	}

	report := []PlanHealth{}
	for i := range plans {
		health := checkPlanHealth(db, quota, &plans[i])
		if len(health.Findings) > 0 {
			report = append(report, health)
		}
	}
	return report, nil
}

func checkPlanHealth(db *gorm.DB, quota QuotaService, plan *models.SubscriptionPlan) PlanHealth {
	health := PlanHealth{
		PlanID:      plan.ID.String(),
		Name:        plan.Name,
		IsActive:    plan.IsActive,
		IsCatalog:   plan.IsCatalog,
		IsDeleted:   plan.DeletedAt.Valid,
		MaxCPU:      plan.MaxCPU,
		MaxMemoryMB: plan.MaxMemoryMB,
		Findings:    []PlanHealthFinding{},
	}

	// A retired plan is only a fault while someone is still entitled by it.
	// Checking this first means a deleted plan's remaining findings describe a
	// row that actually matters.
	if plan.DeletedAt.Valid {
		holders := danglingReferencesTo(db, plan.ID).Total()
		if holders == 0 {
			// Retired cleanly. Nothing below is worth saying about a plan
			// nobody holds.
			return health
		}
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthDanglingReference,
			Severity: PlanHealthBlocking,
			Detail:   fmt.Sprintf("%d live subscription(s) or role mapping(s) still reference this deleted plan", holders),
		})
	}

	if axes := plan.MissingBudgetAxes(); len(axes) > 0 {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthZeroBudget,
			Severity: PlanHealthBlocking,
			Detail:   "no budget on " + strings.Join(axes, " and "),
		})
		// Everything below divides by a budget. Asking those questions of a
		// plan that has none would report faults that are really this one
		// restated.
		return health
	}

	// A budget too small for even the cheapest catalog size. Not a zero budget
	// — the numbers are positive and the plan looks configured — but it cannot
	// launch a single terminal, which is the same outcome reached quietly.
	if !affordsAnySize(quota, plan) {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthAffordsNoSize,
			Severity: PlanHealthBlocking,
			Detail: fmt.Sprintf(
				"%d mCPU / %d MB is below the smallest catalog size",
				plan.MaxCPU, plan.MaxMemoryMB),
		})
		// The imbalance advisory divides by an affordance this plan does not
		// have; reporting it too would restate this finding in weaker terms.
		return health
	}

	// A plan on the shelf that Stripe cannot charge for. Free plans are exempt:
	// nothing is ever charged, so no price is needed.
	if plan.IsCatalog && !plan.IsFree() && isBlank(plan.StripePriceID) {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthCatalogWithoutPrice,
			Severity: PlanHealthWarning,
			Detail:   "offered in the catalogue but carries no Stripe price",
		})
	}

	if finding, found := axisImbalanceFinding(plan); found {
		health.Findings = append(health.Findings, finding)
	}

	return health
}

// isBlank treats a nil pointer and an empty string alike: both mean the price
// was never set, and only one of them is a distinction the reader cares about.
func isBlank(s *string) bool { return s == nil || *s == "" }

// affordsAnySize reports whether the plan can pay for one whole session of at
// least one catalog size, asking the budget engine itself with nothing in use.
func affordsAnySize(quota QuotaService, plan *models.SubscriptionPlan) bool {
	return slices.ContainsFunc(quota.ComputeRemainingBySize(plan, 0, 0), func(s SizeRemaining) bool { return s.RemainingCount >= 1 })
}

// axisImbalanceFinding reports a plan whose two budgets afford materially
// different numbers of sessions, so the smaller axis silently decides how many
// terminals the plan really delivers.
//
// It reads size costs from the same catalog the budget engine divides by, so
// the advisory cannot describe an affordance the gate would not grant. The
// per-axis division is its own, deliberately: the engine collapses the two
// counts to their minimum, and the gap between them is the whole point here.
//
// It measures at ONE reference size — the smallest the plan can afford a whole
// session of — rather than scanning the catalogue. Scanning reports noise: xl
// costs the same CPU as l but twice the memory, so its cost ratio is an outlier
// and every plan looks memory-bound when measured against it. The smallest
// affordable size is both the most sensitive detector of a genuine ratio
// mismatch and the one that yields the largest, clearest counts. For the
// current catalogue the loop always lands on xs: xs, s, m and l share one
// CPU-to-memory ratio and only xl differs, so any plan that affords a larger
// size affords xs too.
func axisImbalanceFinding(plan *models.SubscriptionPlan) (PlanHealthFinding, bool) {
	for _, key := range catalog.CanonicalSizeKeys() {
		size, ok := catalog.LookupSize(key)
		if !ok || size.CPU <= 0 || size.MemoryMB <= 0 {
			continue
		}

		byCPU := plan.MaxCPU / size.CPU
		byMem := plan.MaxMemoryMB / size.MemoryMB

		// Not affordable at this size; try the next one up in cost. A plan that
		// affords no size at all has no affordance to describe — and is a zero
		// budget in all but name, which is reported separately.
		if byCPU < 1 || byMem < 1 {
			continue
		}

		if max(byCPU, byMem) < min(byCPU, byMem)*axisImbalanceRatio {
			return PlanHealthFinding{}, false
		}
		binding := "CPU"
		if byMem < byCPU {
			binding = "memory"
		}

		return PlanHealthFinding{
			Code:     PlanHealthAxisImbalance,
			Severity: PlanHealthAdvisory,
			Detail: fmt.Sprintf(
				"at size %s: %d session(s) by CPU, %d by memory — %s binds",
				key, byCPU, byMem, binding),
		}, true
	}

	return PlanHealthFinding{}, false
}
