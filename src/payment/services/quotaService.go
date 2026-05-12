package services

import (
	"errors"
	"fmt"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

	// GetUserUsage returns the live current usage for a metric. For
	// concurrent_terminals this is a live count via the SSOT slot helper;
	// for other metrics it falls back to the stored usage_metrics row.
	GetUserUsage(userID string, orgID *uuid.UUID, metric string) (int64, error)
}

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
	result, err := s.effectivePlanService.GetUserEffectivePlanForOrg(userID, orgID)
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
		OrganizationID:         orgID,
		MaxConcurrentTerminals: plan.MaxConcurrentTerminals,
		MaxCourses:             plan.MaxCourses,
		CurrentTerminals:       int(currentTerminals),
		CurrentCourses:         int(currentCourses),
	}, nil
}

// GetUserUsage returns the live usage for a metric. The dispatcher
// lives in currentUsage so that new metric types can be added in one
// place.
func (s *quotaService) GetUserUsage(userID string, orgID *uuid.UUID, metric string) (int64, error) {
	return s.currentUsage(userID, orgID, metric)
}

// currentUsage centralizes the per-metric strategy for reading the
// current value. concurrent_terminals comes from a live count via the
// SSOT slot scope; other metrics fall back to the stored usage_metrics
// row (which is what the legacy paths did).
//
// For concurrent_terminals we fail closed: if the live count fails
// (e.g. a transient DB error), the error is surfaced to the caller so
// the request returns 5xx rather than silently reporting near-zero
// usage and granting unlimited terminals. Post-Phase D the materialized
// counter is no longer kept in sync, so the previous fallback would
// have effectively bypassed the cap.
func (s *quotaService) currentUsage(userID string, orgID *uuid.UUID, metric string) (int64, error) {
	switch metric {
	case "concurrent_terminals":
		count, err := terminalModels.CountUserOccupiedSlots(s.db, userID, orgID)
		if err != nil {
			utils.Error("Live terminal count failed for user %s (org=%v): %v", userID, orgID, err)
			return 0, fmt.Errorf("failed to count user terminal slots: %w", err)
		}
		return count, nil
	default:
		return s.storedUsage(userID, metric), nil
	}
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
// one place.
func limitForMetric(plan *models.SubscriptionPlan, metric string) int64 {
	switch metric {
	case "concurrent_terminals":
		return int64(plan.MaxConcurrentTerminals)
	case "courses_created":
		return int64(plan.MaxCourses)
	case "concurrent_users":
		return int64(plan.MaxConcurrentUsers)
	default:
		return -1
	}
}
