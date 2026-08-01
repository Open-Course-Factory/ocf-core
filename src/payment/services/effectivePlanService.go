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

	// ScopeOrganizationID answers "what pool does this plan draw on?" — the single
	// input to quota scoping. Non-nil means the plan belongs to that organization
	// and its CPU/RAM budget is shared across the organization's members; nil means
	// the plan is the user's own and the budget is counted for that user alone.
	//
	// It exists because callers were deriving the scope from OrganizationSubscription,
	// which the role-plan branch leaves nil — so role-plans silently fell back to
	// global counting. And because the budget hook derived it from the REQUEST's
	// organization_id instead, which made omitting that parameter turn a shared org
	// pool into a per-member copy of it (#457).
	//
	// The two cases it encodes:
	//   - a school / OF owns the plan   → shared pool across its members
	//   - a trainer owns the plan, and his organization owns nothing; his learners
	//     hold their own assigned seats → each counted individually
	ScopeOrganizationID *uuid.UUID
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

	// CanRunClassrooms is the single owner of "may this user run classrooms?" —
	// create class groups, convert an organization to a team, buy seats for
	// learners. It lives here because the hard part of the question is plan
	// resolution, which this service already owns.
	//
	// orgID has the same semantics as GetUserEffectivePlan: pass the org when the
	// caller is acting inside one, nil only when there is genuinely no org context.
	//
	// Every gate on classroom capability MUST call this rather than reading
	// GroupManagementEnabled off a plan it resolved itself. Five call sites did the
	// latter and returned three different answers for the same user (#453).
	CanRunClassrooms(userID string, orgID *uuid.UUID) ClassroomEntitlement

	// CanPurchaseSeats reports whether this user may buy licences for other people.
	//
	// It applies the same rule as CanRunClassrooms but over a narrower resolution:
	// ONLY the plan the user holds themselves, bought or assigned as a seat. A plan
	// inherited from an organization does not travel — a teacher does not buy seats
	// on the strength of their school's subscription, because the school's
	// subscription is what decides for the school (#461).
	//
	// Distinct from CanRunClassrooms(userID, nil): that answers "does any plan this
	// user benefits from grant classrooms", which is the right question for a
	// capability flag and the wrong one for spending.
	CanPurchaseSeats(userID string) ClassroomEntitlement
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
			ScopeOrganizationID:      &bestOrgSub.OrganizationID,
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
			ScopeOrganizationID:      &bestOrgSub.OrganizationID,
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
			Scopes(models.ScopeEntitling).
			Where("user_id = ? AND subscription_type = ?", userID, "personal").
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
			// A role-plan is still the organization's plan, so it draws on the
			// organization's pool. This branch carries no OrganizationSubscription,
			// which is precisely why the scope needs its own field: callers reading
			// the subscription saw nil and silently counted globally.
			ScopeOrganizationID: &orgID,
		}, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to resolve role plan for organization %s: %w", orgID.String(), err)
	}

	// No role mapping for this role → fall back to the org's default subscription
	orgSub, err := s.orgSubRepo.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		// Team org has no subscription — fall back to the plan this user holds
		// THEMSELVES: bought personally, or assigned to them as a seat.
		//
		// Deliberately not resolveGlobal, which also considers plans inherited
		// through membership of OTHER organizations. That let any member of a
		// school create their own team organization — they are its owner, so every
		// role check passes — and re-host the school's plan inside it, consuming
		// the school's contract in a workspace the school cannot see (#461).
		//
		// A plan you hold follows you; a plan you merely benefit from somewhere
		// else does not.
		sub, fallbackErr := s.paymentRepo.GetActiveUserSubscription(userID)
		if fallbackErr != nil || sub == nil {
			return nil, fmt.Errorf("no active subscription for organization %s and no personal fallback: %w", orgID.String(), fallbackErr)
		}
		return &EffectivePlanResult{
			Plan:             &sub.SubscriptionPlan,
			Source:           PlanSourcePersonal,
			UserSubscription: sub,
			IsFallback:       true,
			// ScopeOrganizationID stays nil: a personally-held plan is a personal
			// budget, counted for this user alone, even inside an organization.
		}, nil
	}
	return &EffectivePlanResult{
		Plan:                     &orgSub.SubscriptionPlan,
		Source:                   PlanSourceOrganization,
		OrganizationSubscription: orgSub,
		ScopeOrganizationID:      &orgID,
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
