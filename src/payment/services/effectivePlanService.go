package services

import (
	"errors"
	"fmt"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EffectivePlanSource indicates where the user's effective plan comes from.
type EffectivePlanSource string

const (
	PlanSourcePersonal     EffectivePlanSource = "personal"
	PlanSourceOrganization EffectivePlanSource = "organization"
)

// EffectivePlanResult holds the resolved plan for a user, along with its source.
type EffectivePlanResult struct {
	Plan                     *models.SubscriptionPlan
	Source                   EffectivePlanSource
	UserSubscription         *models.UserSubscription         // non-nil if source=personal
	OrganizationSubscription *models.OrganizationSubscription // non-nil if source=organization
	IsFallback               bool                             // true when using personal subscription as fallback for a team org without its own subscription
}

// EffectivePlanService is the single source of truth for "what plan does this user have?"
//
// Resolution is org-context-aware: callers that know which organization the
// user is currently acting in MUST pass that org's ID. Only callers that
// genuinely have no org context (e.g. feature-availability gates at request
// entry, or utilities running outside any HTTP request) may pass nil, which
// resolves the user's globally highest-priority plan.
//
// Historical context: this interface previously exposed TWO resolvers —
// GetUserEffectivePlan (no-org, global highest priority) and
// GetUserEffectivePlanForOrg (org-aware). They returned DIFFERENT plans for
// the same user, so the "display" path (org-aware) and the "gate" path
// (global) silently disagreed. See MR !239 / issue #334 for the launcher-vs-
// gate mismatch this caused. The methods were merged into a single
// org-aware resolver to prevent the same SSOT drift from recurring.
type EffectivePlanService interface {
	// GetUserEffectivePlan resolves the user's effective plan.
	//
	// orgID != nil → returns THAT org's plan (or personal fallback if the org has
	// no subscription, with IsFallback=true).
	//
	// orgID == nil → returns the globally highest-priority plan across personal +
	// every org the user is in. Only callers that truly have no org context
	// should pass nil.
	GetUserEffectivePlan(userID string, orgID *uuid.UUID) (*EffectivePlanResult, error)

	// CheckEffectiveUsageLimit checks whether the user can perform the given action
	// based on their effective plan limits.
	//
	// orgID has the same semantics as GetUserEffectivePlan: pass the org when known,
	// nil only when no org context exists.
	CheckEffectiveUsageLimit(userID string, orgID *uuid.UUID, metricType string, increment int64) (*UsageLimitCheck, error)

	// CheckEffectiveUsageLimitFromResult checks usage limits using an already-resolved plan,
	// skipping the plan resolution DB round-trip. Used by CheckLimit middleware when
	// InjectEffectivePlan has already placed the result in the Gin context.
	CheckEffectiveUsageLimitFromResult(result *EffectivePlanResult, userID string, metricType string, increment int64) (*UsageLimitCheck, error)
}

type effectivePlanService struct {
	paymentRepo repositories.PaymentRepository
	orgSubRepo  repositories.OrganizationSubscriptionRepository
	db          *gorm.DB
}

// NewEffectivePlanService creates an EffectivePlanService with its own repository instances.
func NewEffectivePlanService(db *gorm.DB) EffectivePlanService {
	return &effectivePlanService{
		paymentRepo: repositories.NewPaymentRepository(db),
		orgSubRepo:  repositories.NewOrganizationSubscriptionRepository(db),
		db:          db,
	}
}

// GetUserEffectivePlan resolves which subscription plan applies to a user.
//
// orgID != nil → resolves THAT org's plan (org subscription if any, else falls
// back to the user's personal subscription with IsFallback=true). Membership
// is verified for team orgs; non-members are rejected.
//
// orgID == nil → returns the globally highest-priority plan across personal +
// every org the user is in. Reserved for callers that genuinely have no org
// context (feature-availability middleware at request entry, background-job
// helpers in featureAccess). Production gates that DO know the org context
// MUST pass it — passing nil instead silently drifts the gate away from the
// display path (see issue #334 / MR !239).
func (s *effectivePlanService) GetUserEffectivePlan(userID string, orgID *uuid.UUID) (*EffectivePlanResult, error) {
	if orgID != nil {
		return s.resolveForOrg(userID, *orgID)
	}
	return s.resolveGlobal(userID)
}

// resolveGlobal returns the globally highest-priority plan across personal +
// every org the user is in. This is the nil-orgID branch of GetUserEffectivePlan.
func (s *effectivePlanService) resolveGlobal(userID string) (*EffectivePlanResult, error) {
	var personalSub *models.UserSubscription
	var personalPlan *models.SubscriptionPlan

	// 1. Try to get the user's personal subscription
	sub, err := s.paymentRepo.GetActiveUserSubscription(userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Warn("Failed to get personal subscription for user %s: %v", userID, err)
	}
	if err == nil && sub != nil {
		personalSub = sub
		personalPlan = &sub.SubscriptionPlan
	}

	// 2. Get organization subscriptions
	orgSubs, err := s.orgSubRepo.GetUserOrganizationSubscriptions(userID)
	if err != nil {
		utils.Warn("Failed to get organization subscriptions for user %s: %v", userID, err)
	}

	// 3. Find highest-priority org plan (same logic as GetUserEffectiveFeatures)
	var bestOrgSub *models.OrganizationSubscription
	var bestOrgPlan *models.SubscriptionPlan
	highestOrgPriority := -1

	for i := range orgSubs {
		plan := orgSubs[i].SubscriptionPlan
		if plan.Priority > highestOrgPriority {
			highestOrgPriority = plan.Priority
			bestOrgSub = &orgSubs[i]
			bestOrgPlan = &orgSubs[i].SubscriptionPlan
		}
	}

	// 4. Compare personal plan priority vs best org plan priority
	hasPersonal := personalPlan != nil
	hasOrg := bestOrgPlan != nil

	if hasPersonal && hasOrg {
		if personalPlan.Priority >= bestOrgPlan.Priority {
			return &EffectivePlanResult{
				Plan:             personalPlan,
				Source:           PlanSourcePersonal,
				UserSubscription: personalSub,
			}, nil
		}
		return &EffectivePlanResult{
			Plan:                     bestOrgPlan,
			Source:                   PlanSourceOrganization,
			OrganizationSubscription: bestOrgSub,
		}, nil
	}

	if hasPersonal {
		return &EffectivePlanResult{
			Plan:             personalPlan,
			Source:           PlanSourcePersonal,
			UserSubscription: personalSub,
		}, nil
	}

	if hasOrg {
		return &EffectivePlanResult{
			Plan:                     bestOrgPlan,
			Source:                   PlanSourceOrganization,
			OrganizationSubscription: bestOrgSub,
		}, nil
	}

	// 5. No subscription found
	return nil, fmt.Errorf("no active subscription found for user %s", userID)
}

// resolveForOrg returns the plan for the user in the context of a specific
// organization. Personal orgs short-circuit to the user's personal sub; team
// orgs verify membership and either return the org's plan or fall back to the
// user's personal sub (marked IsFallback=true).
func (s *effectivePlanService) resolveForOrg(userID string, orgID uuid.UUID) (*EffectivePlanResult, error) {
	// Load the organization to check its type
	var org orgModels.Organization
	if err := s.db.First(&org, "id = ?", orgID).Error; err != nil {
		return nil, fmt.Errorf("failed to load organization %s: %w", orgID.String(), err)
	}

	if org.IsPersonalOrg() {
		// Personal org → return user's personal subscription (not assigned org plans)
		var sub models.UserSubscription
		err := s.db.Preload("SubscriptionPlan").
			// past_due included for grace consistency with resolveGlobal /
			// GetActiveUserSubscription — dunning is gated at session-creation (#371).
			Where("user_id = ? AND subscription_type = ? AND status IN ?", userID, "personal", []string{"active", "trialing", "past_due"}).
			Order("created_at DESC").
			First(&sub).Error
		if err != nil {
			return nil, fmt.Errorf("no active personal subscription for user %s: %w", userID, err)
		}
		return &EffectivePlanResult{
			Plan:             &sub.SubscriptionPlan,
			Source:           PlanSourcePersonal,
			UserSubscription: &sub,
		}, nil
	}

	// Team org → check that the user is actually a member of this org and
	// capture their role (used for role-based plan entitlements).
	var member orgModels.OrganizationMember
	err := s.db.
		Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("user %s is not a member of organization %s", userID, orgID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check org membership: %w", err)
	}

	// Role-based plan entitlement: if the org maps the member's role to a
	// specific plan, that mapping wins over the org's default subscription.
	rolePlan, err := s.orgSubRepo.GetOrganizationRolePlan(orgID, string(member.Role))
	if err == nil && rolePlan != nil {
		return &EffectivePlanResult{
			Plan:   &rolePlan.SubscriptionPlan,
			Source: PlanSourceOrganization,
		}, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to resolve role plan for organization %s: %w", orgID.String(), err)
	}

	// No role mapping for this role → fall back to the org's default subscription
	orgSub, err := s.orgSubRepo.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		// Team org has no subscription — fall back to user's personal subscription.
		// Calling resolveGlobal here, not GetUserEffectivePlan(userID, nil), to
		// keep the fallback explicit and avoid any future recursion confusion.
		result, fallbackErr := s.resolveGlobal(userID)
		if fallbackErr != nil {
			return nil, fmt.Errorf("no active subscription for organization %s and no personal fallback: %w", orgID.String(), fallbackErr)
		}
		result.IsFallback = true
		return result, nil
	}
	return &EffectivePlanResult{
		Plan:                     &orgSub.SubscriptionPlan,
		Source:                   PlanSourceOrganization,
		OrganizationSubscription: orgSub,
	}, nil
}

// CheckEffectiveUsageLimit checks whether the user can perform the given action
// based on their effective plan limits.
//
// orgID has the same semantics as GetUserEffectivePlan — pass the org context
// when known, nil only when no org context exists.
//
// Thin wrapper kept for backward compatibility with existing callers and test
// mocks. The actual quota logic lives in QuotaService — see
// src/payment/services/quotaService.go.
func (s *effectivePlanService) CheckEffectiveUsageLimit(userID string, orgID *uuid.UUID, metricType string, increment int64) (*UsageLimitCheck, error) {
	result, err := s.GetUserEffectivePlan(userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effective plan: %w", err)
	}
	return s.quotaService().CheckUserQuotaWithPlan(result, userID, metricType, increment)
}

// quotaService builds a transient QuotaService backed by this
// effectivePlanService. The two services are intentionally separate
// (QuotaService takes EffectivePlanService as a dependency) — building
// it on demand avoids a hard reference cycle while keeping the quota
// rule expressed in exactly one place.
func (s *effectivePlanService) quotaService() QuotaService {
	return NewQuotaService(s.db, s)
}

// CheckEffectiveUsageLimitFromResult checks usage limits using a pre-resolved plan result,
// avoiding the plan resolution DB round-trip. Called by CheckLimit middleware when
// InjectEffectivePlan has already resolved and stored the plan in the Gin context.
//
// Thin wrapper kept for backward compatibility — actual logic lives in QuotaService.
func (s *effectivePlanService) CheckEffectiveUsageLimitFromResult(result *EffectivePlanResult, userID string, metricType string, increment int64) (*UsageLimitCheck, error) {
	return s.quotaService().CheckUserQuotaWithPlan(result, userID, metricType, increment)
}
