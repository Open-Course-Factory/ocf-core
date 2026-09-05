package services

import (
	"fmt"

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
// enough for an operator to fix the answer. The checks are not a second set of
// rules: size costs come from the same catalog the budget engine divides by, so
// the report and the gate cannot drift into disagreeing.
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
func CheckAllPlanHealth(db *gorm.DB) ([]PlanHealth, error) {
	var plans []models.SubscriptionPlan
	if err := db.Unscoped().Order("name ASC").Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("cannot read the plans: %w", err)
	}

	report := []PlanHealth{}
	for i := range plans {
		health, err := checkPlanHealth(db, &plans[i])
		if err != nil {
			// One unreadable plan must not hide the faults in every other one.
			// A page that fails whole is a page nobody opens twice.
			return nil, err
		}
		if len(health.Findings) > 0 {
			report = append(report, health)
		}
	}
	return report, nil
}

func checkPlanHealth(db *gorm.DB, plan *models.SubscriptionPlan) (PlanHealth, error) {
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
		subscribers, err := countLiveSubscribers(db, plan.ID.String())
		if err != nil {
			return health, err
		}
		if subscribers == 0 {
			// Retired cleanly. Nothing below is worth saying about a plan
			// nobody holds.
			return health, nil
		}
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthDanglingReference,
			Severity: PlanHealthBlocking,
			Detail:   fmt.Sprintf("%d live subscription(s) still reference this deleted plan", subscribers),
		})
	}

	if plan.MaxCPU <= 0 || plan.MaxMemoryMB <= 0 {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthZeroBudget,
			Severity: PlanHealthBlocking,
			Detail:   zeroBudgetDetail(plan),
		})
		// Everything below divides by a budget. Asking those questions of a
		// plan that has none would report faults that are really this one
		// restated.
		return health, nil
	}

	// A budget too small for even the cheapest catalog size. Not a zero budget
	// — the numbers are positive and the plan looks configured — but it cannot
	// launch a single terminal, which is the same outcome reached quietly.
	if !affordsAnySize(plan) {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthAffordsNoSize,
			Severity: PlanHealthBlocking,
			Detail: fmt.Sprintf(
				"%d mCPU / %d MB is below the smallest catalog size",
				plan.MaxCPU, plan.MaxMemoryMB),
		})
		// The imbalance advisory divides by an affordance this plan does not
		// have; reporting it too would restate this finding in weaker terms.
		return health, nil
	}

	// A plan on the shelf that Stripe cannot charge for. Free plans are exempt:
	// nothing is ever charged, so no price is needed.
	if plan.IsCatalog && plan.PriceAmount > 0 && isBlank(plan.StripePriceID) {
		health.Findings = append(health.Findings, PlanHealthFinding{
			Code:     PlanHealthCatalogWithoutPrice,
			Severity: PlanHealthWarning,
			Detail:   "offered in the catalogue but carries no Stripe price",
		})
	}

	if finding, found := axisImbalanceFinding(plan); found {
		health.Findings = append(health.Findings, finding)
	}

	return health, nil
}

// isBlank treats a nil pointer and an empty string alike: both mean the price
// was never set, and only one of them is a distinction the reader cares about.
func isBlank(s *string) bool { return s == nil || *s == "" }

func zeroBudgetDetail(plan *models.SubscriptionPlan) string {
	switch {
	case plan.MaxCPU <= 0 && plan.MaxMemoryMB <= 0:
		return "neither a CPU nor a memory budget"
	case plan.MaxCPU <= 0:
		return "no CPU budget"
	default:
		return "no memory budget"
	}
}

// affordsAnySize reports whether the plan can pay for one whole session of at
// least one catalog size, reading the same costs the budget engine divides by.
func affordsAnySize(plan *models.SubscriptionPlan) bool {
	for _, key := range catalog.CanonicalSizeKeys() {
		size, ok := catalog.LookupSize(key)
		if !ok || size.CPU <= 0 || size.MemoryMB <= 0 {
			continue
		}
		if plan.MaxCPU/size.CPU >= 1 && plan.MaxMemoryMB/size.MemoryMB >= 1 {
			return true
		}
	}
	return false
}

// axisImbalanceFinding reports a plan whose two budgets afford materially
// different numbers of sessions, so the smaller axis silently decides how many
// terminals the plan really delivers.
//
// It reads size costs from the same catalog the budget engine divides by, so
// the advisory cannot describe an affordance the gate would not grant.
//
// It measures at ONE reference size — the smallest the plan can afford a whole
// session of — rather than scanning the catalogue. Scanning reports noise: xl
// costs the same CPU as l but twice the memory, so its cost ratio is an outlier
// and every plan looks memory-bound when measured against it. The smallest
// affordable size is both the most sensitive detector of a genuine ratio
// mismatch and the one that yields the largest, clearest counts.
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

		smaller, larger := byCPU, byMem
		binding := "CPU"
		if byMem < byCPU {
			smaller, larger = byMem, byCPU
			binding = "memory"
		}
		if larger < smaller*axisImbalanceRatio {
			return PlanHealthFinding{}, false
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

// countLiveSubscribers counts the subscriptions that still entitle someone
// through this plan, personal and organizational alike. Cancelled and expired
// rows are excluded: they no longer entitle anyone, so a plan they point at is
// retired rather than dangling.
func countLiveSubscribers(db *gorm.DB, planID string) (int64, error) {
	live := []string{"active", "trialing", "past_due"}

	var personal int64
	if err := db.Model(&models.UserSubscription{}).
		Where("subscription_plan_id = ? AND status IN ?", planID, live).
		Count(&personal).Error; err != nil {
		return 0, fmt.Errorf("cannot count personal subscriptions: %w", err)
	}

	var org int64
	if err := db.Model(&models.OrganizationSubscription{}).
		Where("subscription_plan_id = ? AND status IN ?", planID, live).
		Count(&org).Error; err != nil {
		return 0, fmt.Errorf("cannot count organization subscriptions: %w", err)
	}

	return personal + org, nil
}
