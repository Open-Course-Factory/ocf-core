package services

import (
	"errors"
	"fmt"
	"math"

	"soli/formations/src/payment/catalog"
	paymentDto "soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuotaService is the single source of truth for "is X within quota?".
//
// Plan resolution (which subscription plan applies to a user in a given
// org context) stays in EffectivePlanService. The slot-occupancy rule
// for terminals stays in terminalTrainer/models.OccupiesSlotScope. This
// service composes those two primitives and is the ONLY place where a
// quota decision is actually computed.
//
// External consumers (effectivePlanService.CheckEffectiveUsageLimit*,
// the CheckLimit middleware, and scenario controllers) delegate to
// QuotaService. Their public surfaces are kept stable for backward
// compatibility, but the logic lives here.
type QuotaService interface {
	// CheckUserQuota resolves the user's effective plan (in the given org
	// context if non-nil) and decides whether the proposed increment keeps
	// usage within the plan limit.
	CheckUserQuota(userID string, orgID *uuid.UUID, metric string, increment int64) (*UsageLimitCheck, error)

	// CheckUserQuotaWithPlan skips plan resolution and uses a pre-resolved
	// EffectivePlanResult. Used by the CheckLimit middleware after
	// InjectEffectivePlan has placed the resolved plan in the request
	// context, avoiding a redundant DB round-trip.
	CheckUserQuotaWithPlan(plan *EffectivePlanResult, userID string, metric string, increment int64) (*UsageLimitCheck, error)

	// GetOrgQuota returns the current usage and plan limits for an
	// organization. Used by GET /organizations/:id/usage-limits.
	GetOrgQuota(orgID uuid.UUID) (*OrganizationLimits, error)

	// CheckBudget evaluates whether a session of the requested CPU/RAM cost
	// fits within the user's (or org's) effective plan.
	//
	// When MaxCPU/MaxMemoryMB on the plan are zero, the corresponding axis
	// is treated as unlimited.
	//
	// Sessions counted toward the budget follow lifecycle rule D6'
	// (supersedes D6, locked 2026-05-28), encoded in the SSOT
	// terminalModels.OccupiesSlotScope:
	//   state IN ('running','stopped') AND deleted_at IS NULL
	//   AND expires_at > NOW().
	// Every stopped session counts — the persistence_mode distinction is
	// UX-only ("a stop is a stop"). Past-expiry zombie rows are excluded:
	// a stale row whose proxy session is gone but whose state column was
	// never reset must not keep eating budget. The dashboard "Actifs"
	// counter reads through the same scope, so the gate and the dashboard
	// count cannot diverge.
	//
	// NOTE: This method is for read-time gating (composer UI, scenario
	// list). It is NOT race-safe — write-time enforcement requires a
	// transactional check inside a BeforeCreate hook (MR-CORE-5) using
	// `SELECT ... FOR UPDATE`. Two concurrent CheckBudget calls may both
	// observe enough budget for the same slice of resources.
	CheckBudget(userID string, orgID *uuid.UUID, plan *models.SubscriptionPlan, requestedCPU, requestedMemMB int) (*BudgetCheck, error)

	// ComputeRemainingBySize returns the per-size remaining count after
	// accounting for usedCPU / usedMemMB. The formula is
	//
	//   remaining(size) = floor(min((max_cpu - used_cpu)/size.cpu,
	//                                (max_mem - used_mem)/size.memory))
	//
	// An entry is returned for every canonical size in the catalog, even
	// when the count is zero. When MaxCPU/MaxMemoryMB are zero (unlimited),
	// RemainingCount reports math.MaxInt32 for every size.
	//
	// This is a pure function (no DB access) intended for endpoints that
	// already know the user's current footprint, e.g.
	// GET /terminals/session-options or the scenario list endpoint.
	ComputeRemainingBySize(plan *models.SubscriptionPlan, usedCPU, usedMemMB int) []SizeRemaining

	// RemainingBudgetFits is a one-shot helper: it queries current usage
	// for the user (org-scoped if orgID is non-nil) and answers whether
	// one container of the given size key fits in the remaining budget.
	RemainingBudgetFits(userID string, orgID *uuid.UUID, plan *models.SubscriptionPlan, sizeKey string) (bool, error)

	// GetBudgetUsage returns the user's (or org's) current CPU + RAM
	// footprint under the budget counting rule (D6). It is a thin
	// passthrough over sumActiveResources for callers that need to
	// surface usage in dashboards without re-implementing the predicate.
	GetBudgetUsage(userID string, orgID *uuid.UUID) (usedCPU, usedMemMB int, err error)

	// EnforceBudgetTx is the race-safe write-time budget gate. Unlike
	// CheckBudget (read-time, NOT race-safe — see its doc), it runs inside
	// the caller's transaction so the usage sum and the subsequent insert
	// are serialised against concurrent starts for the same scope.
	//
	// Serialisation has two layers (PostgreSQL only; SQLite serialises
	// writers on its own so both are skipped there):
	//   - A transaction-scoped advisory lock keyed on the scope
	//     (org UUID when orgID != nil, else userID). This closes the
	//     empty-budget phantom: two concurrent FIRST starts both see zero
	//     rows, so row locks alone would let both through.
	//   - A row-level FOR UPDATE on every contributing row, so existing
	//     usage cannot change between the sum and the caller's insert.
	//
	// The usage sum reuses the SAME terminalModels.OccupiesSlotScope
	// predicate as the unlocked CheckBudget path, so the locked and
	// unlocked sums can never drift apart.
	//
	// MaxCPU/MaxMemoryMB of 0 mean unlimited on that axis. The returned
	// BudgetEnforcement carries the BudgetCheck verdict plus the summed
	// UsedCPU/UsedMemMB, so callers can construct their own rejection
	// error (e.g. the hook's ErrBudgetExhausted) without re-querying.
	EnforceBudgetTx(tx *gorm.DB, userID string, orgID *uuid.UUID, plan *models.SubscriptionPlan, requestedCPU, requestedMemMB int) (*BudgetEnforcement, error)
}

// BudgetCheck reports the outcome of a CheckBudget evaluation.
//
// Reason is "" when Allowed is true. Otherwise it carries a short code
// describing the rejection cause:
//
//	"budget_cpu_exceeded"     — the request would exceed MaxCPU
//	"budget_memory_exceeded"  — the request would exceed MaxMemoryMB
type BudgetCheck struct {
	Allowed        bool
	RemainingCPU   int
	RemainingMemMB int
	Reason         string
}

// BudgetEnforcement is the result of EnforceBudgetTx. It wraps the
// BudgetCheck verdict and additionally exposes the locked usage sum
// (UsedCPU / UsedMemMB) that produced it, so a caller can build a
// rejection error carrying the "current" footprint without re-querying.
type BudgetEnforcement struct {
	BudgetCheck
	UsedCPU   int
	UsedMemMB int
}

// SizeRemaining is re-exported from payment/dto so existing callers
// (tests, other services) keep working without an import churn. The
// canonical definition lives in payment/dto/sizeRemaining.go — see
// that file for the rationale.
type SizeRemaining = paymentDto.SizeRemaining

type quotaService struct {
	db                   *gorm.DB
	effectivePlanService EffectivePlanService
	paymentRepo          repositories.PaymentRepository
	orgSubRepo           repositories.OrganizationSubscriptionRepository
}

// NewQuotaService creates a QuotaService. The EffectivePlanService is
// injected (not constructed internally) to keep dependencies explicit
// and to avoid an import cycle if effectivePlanService ever needs to
// reference quotaService.
func NewQuotaService(db *gorm.DB, eps EffectivePlanService) QuotaService {
	return &quotaService{
		db:                   db,
		effectivePlanService: eps,
		paymentRepo:          repositories.NewPaymentRepository(db),
		orgSubRepo:           repositories.NewOrganizationSubscriptionRepository(db),
	}
}

// CheckUserQuota resolves the effective plan then delegates to
// CheckUserQuotaWithPlan. Keeping the two-step shape lets the middleware
// skip resolution when it already has a plan in context.
func (s *quotaService) CheckUserQuota(userID string, orgID *uuid.UUID, metric string, increment int64) (*UsageLimitCheck, error) {
	result, err := s.effectivePlanService.GetUserEffectivePlan(userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effective plan: %w", err)
	}
	return s.CheckUserQuotaWithPlan(result, userID, metric, increment)
}

// CheckUserQuotaWithPlan is the actual decision function. Every quota
// check in the codebase eventually flows through this.
//
// Slot counts are scoped to the same org context the plan was resolved in:
// when the plan came from an organization subscription, the count is filtered
// to that org so that two orgs with separate caps cannot share a single
// global counter. When the plan is personal (or org context was nil), the
// count is global to the user.
func (s *quotaService) CheckUserQuotaWithPlan(plan *EffectivePlanResult, userID string, metric string, increment int64) (*UsageLimitCheck, error) {
	if plan == nil || plan.Plan == nil {
		return nil, fmt.Errorf("cannot check quota without a resolved plan")
	}

	limit := limitForMetric(plan.Plan, metric)

	// Derive the org scope from the resolved plan so the slot count matches
	// the limit being checked. Plans sourced from an organization carry the
	// OrganizationSubscription; personal plans leave orgID nil (global count).
	var orgID *uuid.UUID
	if plan.Source == PlanSourceOrganization && plan.OrganizationSubscription != nil {
		id := plan.OrganizationSubscription.OrganizationID
		orgID = &id
	}

	currentUsage, err := s.currentUsage(userID, orgID, metric)
	if err != nil {
		return nil, err
	}

	allowed := limit == -1 || (currentUsage+increment) <= limit

	var remaining int64
	if limit == -1 {
		remaining = -1
	} else {
		remaining = limit - currentUsage
		if remaining < 0 {
			remaining = 0
		}
	}

	message := ""
	if !allowed {
		message = fmt.Sprintf("Usage limit exceeded for %s. Current: %d, Limit: %d", metric, currentUsage, limit)
	}

	return &UsageLimitCheck{
		Allowed:        allowed,
		CurrentUsage:   currentUsage,
		Limit:          limit,
		RemainingUsage: remaining,
		Message:        message,
		UserID:         userID,
		MetricType:     metric,
		Source:         plan.Source,
	}, nil
}

// GetOrgQuota returns the active subscription's plan limits along with
// the live occupied-slot count for the org.
func (s *quotaService) GetOrgQuota(orgID uuid.UUID) (*OrganizationLimits, error) {
	subscription, err := s.orgSubRepo.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	plan := subscription.SubscriptionPlan

	// Count occupied slots via SSOT helper (active + stopped, not expired).
	// See terminalModels.OccupiesSlotScope for the canonical rule.
	currentTerminals, _ := terminalModels.CountOrgOccupiedSlots(s.db, orgID)

	var currentCourses int64
	s.db.Table("courses").
		Joins("JOIN organization_members ON organization_members.user_id = courses.owner_user_id").
		Where("organization_members.organization_id = ? AND courses.deleted_at IS NULL", orgID).
		Count(&currentCourses)

	return &OrganizationLimits{
		OrganizationID:   orgID,
		MaxCourses:       plan.MaxCourses,
		CurrentTerminals: int(currentTerminals),
		CurrentCourses:   int(currentCourses),
	}, nil
}

// currentUsage reads the persisted usage_metrics row for a user/metric.
// Returns 0 when no row exists (not an error — a first-time user has
// no metrics yet).
//
// Terminal capacity is NOT routed through here: the CPU/RAM budget engine
// (CheckBudget on SubscriptionPlan.MaxCPU / MaxMemoryMB, fed by
// sumActiveResources*) is the sole authoritative quota gate for terminals.
// The metric dispatcher only serves the remaining numeric metrics
// (courses_created, concurrent_users, ...).
func (s *quotaService) currentUsage(userID string, orgID *uuid.UUID, metric string) (int64, error) {
	_ = orgID // org-scoped usage rows do not exist for the remaining metrics.
	return s.storedUsage(userID, metric), nil
}

// storedUsage reads the persisted usage_metrics row for a user/metric.
// Returns 0 when no row exists (not an error — a first-time user has
// no metrics yet).
func (s *quotaService) storedUsage(userID, metric string) int64 {
	m, err := s.paymentRepo.GetUserUsageMetrics(userID, metric)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			utils.Warn("Failed to get usage metrics for user %s, metric %s: %v", userID, metric, err)
		}
		return 0
	}
	return m.CurrentValue
}

// limitForMetric extracts the per-metric limit from a plan. -1 means
// unlimited. Centralized here so future metric types can be added in
// one place. Terminal-related caps are NOT here — they live on the
// budget engine (MaxCPU/MaxMemoryMB).
func limitForMetric(plan *models.SubscriptionPlan, metric string) int64 {
	switch metric {
	case "courses_created":
		return int64(plan.MaxCourses)
	case "concurrent_users":
		return int64(plan.MaxConcurrentUsers)
	default:
		return -1
	}
}

// --- Budget quota methods -----------------------------------------------
//
// These methods power the CPU/RAM budget quota engine.
//
// Resource counters use the denormalised size_cpu / size_memory_mb
// columns on the Terminal table. The TerminalBudgetHook (and the
// composed-session path) populates these columns on create so new
// sessions never need a catalog fallback.

// budgetUnlimited is the sentinel returned for the "remaining" axis on
// an unlimited (zero-cap) plan. Chosen as math.MaxInt32 so JSON callers
// can recognise it without special-casing.
const budgetUnlimited = math.MaxInt32

// CheckBudget — see interface doc.
func (s *quotaService) CheckBudget(
	userID string,
	orgID *uuid.UUID,
	plan *models.SubscriptionPlan,
	requestedCPU, requestedMemMB int,
) (*BudgetCheck, error) {
	if plan == nil {
		return nil, fmt.Errorf("CheckBudget: plan is nil")
	}

	usedCPU, usedMemMB, err := s.sumActiveResources(userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("CheckBudget: sum active resources: %w", err)
	}

	return evaluateBudget(plan, usedCPU, usedMemMB, requestedCPU, requestedMemMB), nil
}

// cpuRemainingForReport produces the "remaining" CPU value used in
// rejection responses (clamped to >=0 and respecting the unlimited
// sentinel for zero-cap plans).
func cpuRemainingForReport(plan *models.SubscriptionPlan, usedCPU int) int {
	if plan.MaxCPU <= 0 {
		return budgetUnlimited
	}
	r := plan.MaxCPU - usedCPU
	if r < 0 {
		return 0
	}
	return r
}

// memRemainingForReport mirrors cpuRemainingForReport for the memory axis.
func memRemainingForReport(plan *models.SubscriptionPlan, usedMemMB int) int {
	if plan.MaxMemoryMB <= 0 {
		return budgetUnlimited
	}
	r := plan.MaxMemoryMB - usedMemMB
	if r < 0 {
		return 0
	}
	return r
}

// ComputeRemainingBySize — see interface doc.
func (s *quotaService) ComputeRemainingBySize(
	plan *models.SubscriptionPlan,
	usedCPU, usedMemMB int,
) []SizeRemaining {
	canonicalKeys := catalog.CanonicalSizeKeys()
	out := make([]SizeRemaining, 0, len(canonicalKeys))

	cpuUnlimited := plan == nil || plan.MaxCPU <= 0
	memUnlimited := plan == nil || plan.MaxMemoryMB <= 0

	remCPU := 0
	if !cpuUnlimited {
		remCPU = plan.MaxCPU - usedCPU
		if remCPU < 0 {
			remCPU = 0
		}
	}
	remMem := 0
	if !memUnlimited {
		remMem = plan.MaxMemoryMB - usedMemMB
		if remMem < 0 {
			remMem = 0
		}
	}

	for _, key := range canonicalKeys {
		size, ok := catalog.LookupSize(key)
		if !ok {
			continue
		}
		count := 0
		switch {
		case cpuUnlimited && memUnlimited:
			count = budgetUnlimited
		case cpuUnlimited:
			if size.MemoryMB > 0 {
				count = remMem / size.MemoryMB
			} else {
				count = budgetUnlimited
			}
		case memUnlimited:
			if size.CPU > 0 {
				count = remCPU / size.CPU
			} else {
				count = budgetUnlimited
			}
		default:
			byCPU := budgetUnlimited
			if size.CPU > 0 {
				byCPU = remCPU / size.CPU
			}
			byMem := budgetUnlimited
			if size.MemoryMB > 0 {
				byMem = remMem / size.MemoryMB
			}
			if byCPU < byMem {
				count = byCPU
			} else {
				count = byMem
			}
		}
		out = append(out, SizeRemaining{
			Key:            key,
			CPU:            size.CPU,
			MemoryMB:       size.MemoryMB,
			RemainingCount: count,
		})
	}
	return out
}

// RemainingBudgetFits — see interface doc.
func (s *quotaService) RemainingBudgetFits(
	userID string,
	orgID *uuid.UUID,
	plan *models.SubscriptionPlan,
	sizeKey string,
) (bool, error) {
	if plan == nil {
		return false, fmt.Errorf("RemainingBudgetFits: plan is nil")
	}
	size, ok := catalog.LookupSize(sizeKey)
	if !ok {
		return false, fmt.Errorf("RemainingBudgetFits: unknown size %q", sizeKey)
	}

	check, err := s.CheckBudget(userID, orgID, plan, size.CPU, size.MemoryMB)
	if err != nil {
		return false, err
	}
	return check.Allowed, nil
}

// GetBudgetUsage — see interface doc.
func (s *quotaService) GetBudgetUsage(userID string, orgID *uuid.UUID) (int, int, error) {
	return s.sumActiveResources(userID, orgID)
}

// sumActiveResources returns the total CPU + RAM footprint of terminals
// counted against the budget for this user (or org).
//
// The counting predicate is the SSOT terminalModels.OccupiesSlotScope
// (state IN ('running','stopped') AND deleted_at IS NULL AND
// expires_at > NOW()). See that scope's doc for the rationale (D6',
// supersedes D6, locked 2026-05-28): a stop is a stop, every stopped
// session reserves capacity until sync confirms tt-backend has reaped
// the container. The persistence_mode distinction is UX-only, not
// budget logic.
//
// This is the same scope used to derive remaining capacity for size
// catalogs and to feed the dashboard session list, so the budget gate
// and any downstream "remaining" surface cannot drift apart.
//
// orgID:
//   - nil    → personal scope (terminals where user_id matches).
//   - non-nil → org scope, summed across ALL members of the org via the
//     organization_members join (mirrors CountOrgOccupiedSlots).
//
// Returns (0, 0, nil) when no rows match — not an error.
func (s *quotaService) sumActiveResources(userID string, orgID *uuid.UUID) (int, int, error) {
	if orgID != nil {
		return s.sumActiveResourcesForOrg(*orgID)
	}
	return s.sumActiveResourcesForUser(userID)
}

func (s *quotaService) sumActiveResourcesForUser(userID string) (int, int, error) {
	var row struct {
		CPU int64
		Mem int64
	}
	q := s.db.Table("terminals").
		Scopes(terminalModels.OccupiesSlotScope).
		Select("COALESCE(SUM(terminals.size_cpu), 0) AS cpu, COALESCE(SUM(terminals.size_memory_mb), 0) AS mem").
		Where("terminals.user_id = ?", userID)
	if err := q.Scan(&row).Error; err != nil {
		utils.Error("sumActiveResourcesForUser failed for %s: %v", userID, err)
		return 0, 0, err
	}
	return int(row.CPU), int(row.Mem), nil
}

// EnforceBudgetTx — see interface doc.
func (s *quotaService) EnforceBudgetTx(
	tx *gorm.DB,
	userID string,
	orgID *uuid.UUID,
	plan *models.SubscriptionPlan,
	requestedCPU, requestedMemMB int,
) (*BudgetEnforcement, error) {
	if plan == nil {
		return nil, fmt.Errorf("EnforceBudgetTx: plan is nil")
	}

	// (1) Serialise the scope before reading. On PostgreSQL the advisory
	//     lock closes the empty-budget phantom (zero contributing rows ⇒
	//     no rows to FOR UPDATE); it auto-releases at commit/rollback.
	//     SQLite serialises writers itself, so this is skipped there.
	if err := acquireScopeAdvisoryLock(tx, userID, orgID); err != nil {
		return nil, fmt.Errorf("EnforceBudgetTx: advisory lock: %w", err)
	}

	// (2) Sum the LOCKED contributing rows through the same scope the
	//     unlocked CheckBudget path uses.
	usedCPU, usedMemMB, err := lockAndSumActiveResources(tx, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("EnforceBudgetTx: lock+sum: %w", err)
	}

	check := evaluateBudget(plan, usedCPU, usedMemMB, requestedCPU, requestedMemMB)
	return &BudgetEnforcement{
		BudgetCheck: *check,
		UsedCPU:     usedCPU,
		UsedMemMB:   usedMemMB,
	}, nil
}

// evaluateBudget is the pure axis-comparison shared by CheckBudget (read
// path) and EnforceBudgetTx (write path): given an already-computed usage
// sum, it produces the BudgetCheck verdict. Keeping it pure means the
// locked and unlocked gates apply identical thresholds.
func evaluateBudget(plan *models.SubscriptionPlan, usedCPU, usedMemMB, requestedCPU, requestedMemMB int) *BudgetCheck {
	cpuUnlimited := plan.MaxCPU <= 0
	remainingCPU := budgetUnlimited
	if !cpuUnlimited {
		remainingCPU = plan.MaxCPU - usedCPU - requestedCPU
	}

	memUnlimited := plan.MaxMemoryMB <= 0
	remainingMem := budgetUnlimited
	if !memUnlimited {
		remainingMem = plan.MaxMemoryMB - usedMemMB - requestedMemMB
	}

	switch {
	case !cpuUnlimited && remainingCPU < 0:
		left := plan.MaxCPU - usedCPU
		if left < 0 {
			left = 0
		}
		return &BudgetCheck{
			Allowed:        false,
			RemainingCPU:   left,
			RemainingMemMB: memRemainingForReport(plan, usedMemMB),
			Reason:         "budget_cpu_exceeded",
		}
	case !memUnlimited && remainingMem < 0:
		left := plan.MaxMemoryMB - usedMemMB
		if left < 0 {
			left = 0
		}
		return &BudgetCheck{
			Allowed:        false,
			RemainingCPU:   cpuRemainingForReport(plan, usedCPU),
			RemainingMemMB: left,
			Reason:         "budget_memory_exceeded",
		}
	}

	return &BudgetCheck{
		Allowed:        true,
		RemainingCPU:   remainingCPU,
		RemainingMemMB: remainingMem,
	}
}

// acquireScopeAdvisoryLock takes a transaction-scoped PostgreSQL advisory
// lock keyed on the budget scope ("terminal_budget:<orgUUID|userID>") so
// the whole scope serialises regardless of how many rows currently exist.
// This is what actually closes the empty-budget phantom that a row-level
// FOR UPDATE cannot (there are no rows to lock on the first start). It is
// a no-op on dialects without advisory locks (SQLite), which serialise
// writers anyway. The lock releases automatically at commit/rollback.
func acquireScopeAdvisoryLock(tx *gorm.DB, userID string, orgID *uuid.UUID) error {
	if !supportsRowLock(tx) {
		return nil
	}
	scopeKey := userID
	if orgID != nil {
		scopeKey = orgID.String()
	}
	return tx.Exec(
		"SELECT pg_advisory_xact_lock(hashtext(?))",
		"terminal_budget:"+scopeKey,
	).Error
}

// lockAndSumActiveResources sums the CPU + RAM footprint of the rows that
// contribute to the budget for a user (or org), composing the SSOT
// terminalModels.OccupiesSlotScope predicate and additionally taking a
// row lock (SELECT ... FOR UPDATE) on each contributing row. On
// PostgreSQL/MySQL this serialises concurrent starts for the same scope
// against existing usage; on SQLite the locking clause is skipped but the
// sum is still correct.
//
// It composes the exact same scope as the unlocked sumActiveResources*
// helpers (org variant adds the organization_members join) so the locked
// and unlocked sums can never read different predicates. Returns
// (0, 0, nil) when no rows match.
//
// The rows are SELECTed individually and summed in Go rather than via a
// SQL SUM(): PostgreSQL forbids FOR UPDATE alongside an aggregate, and the
// per-scope row count is bounded by the plan budget so the payload stays
// small. The lock is scoped to the terminals table so the org variant's
// organization_members join does not lock membership rows.
func lockAndSumActiveResources(tx *gorm.DB, userID string, orgID *uuid.UUID) (int, int, error) {
	var rows []struct {
		SizeCPU      int
		SizeMemoryMB int
	}
	q := tx.Table("terminals").
		Scopes(terminalModels.OccupiesSlotScope).
		Select("terminals.size_cpu AS size_cpu, terminals.size_memory_mb AS size_memory_mb")
	if orgID != nil {
		q = q.Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
			Where("organization_members.organization_id = ? AND organization_members.deleted_at IS NULL", *orgID)
	} else {
		q = q.Where("terminals.user_id = ?", userID)
	}
	if supportsRowLock(tx) {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "terminals"}})
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
// SELECT ... FOR UPDATE (and PostgreSQL advisory locks). We attach the
// locking clause and advisory lock only for those dialects so unit tests
// on SQLite keep working without syntax errors.
func supportsRowLock(tx *gorm.DB) bool {
	if tx == nil || tx.Dialector == nil {
		return false
	}
	name := tx.Dialector.Name()
	return name == "postgres" || name == "mysql"
}

func (s *quotaService) sumActiveResourcesForOrg(orgID uuid.UUID) (int, int, error) {
	var row struct {
		CPU int64
		Mem int64
	}
	// Mirror CountOrgOccupiedSlots: join through organization_members so
	// we count terminals owned by any active member of the org. The
	// terminals.* predicates (soft-delete, expiry, state) live in
	// OccupiesSlotScope; the join-table filter
	// (organization_members.deleted_at IS NULL) stays inline because
	// the scope only knows about the terminals table.
	q := s.db.Table("terminals").
		Scopes(terminalModels.OccupiesSlotScope).
		Select("COALESCE(SUM(terminals.size_cpu), 0) AS cpu, COALESCE(SUM(terminals.size_memory_mb), 0) AS mem").
		Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
		Where("organization_members.organization_id = ? AND organization_members.deleted_at IS NULL", orgID)
	if err := q.Scan(&row).Error; err != nil {
		utils.Error("sumActiveResourcesForOrg failed for %s: %v", orgID, err)
		return 0, 0, err
	}
	return int(row.CPU), int(row.Mem), nil
}
