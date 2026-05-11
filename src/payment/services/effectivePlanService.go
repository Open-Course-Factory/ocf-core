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
type EffectivePlanService interface {
	GetUserEffectivePlan(userID string) (*EffectivePlanResult, error)
	GetUserEffectivePlanForOrg(userID string, orgID *uuid.UUID) (*EffectivePlanResult, error)
	CheckEffectiveUsageLimit(userID string, metricType string, increment int64) (*UsageLimitCheck, error)
	CheckEffectiveUsageLimitForOrg(userID string, orgID *uuid.UUID, metricType string, increment int64) (*UsageLimitCheck, error)
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

// GetUserEffectivePlan resolves which subscription plan applies to a user by comparing
// personal and organization subscriptions and returning the highest-priority one.
func (s *effectivePlanService) GetUserEffectivePlan(userID string) (*EffectivePlanResult, error) {
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

// CheckEffectiveUsageLimit checks whether the user can perform the given action
// based on their effective plan limits.
//
// Thin wrapper kept for backward compatibility with existing callers and test
// mocks. The actual quota logic lives in QuotaService — see
// src/payment/services/quotaService.go.
func (s *effectivePlanService) CheckEffectiveUsageLimit(userID string, metricType string, increment int64) (*UsageLimitCheck, error) {
	result, err := s.GetUserEffectivePlan(userID)
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

// GetUserEffectivePlanForOrg resolves the effective plan for a user in the context of a
// specific organization. If orgID is nil, falls back to the global resolution.
func (s *effectivePlanService) GetUserEffectivePlanForOrg(userID string, orgID *uuid.UUID) (*EffectivePlanResult, error) {
	// If no org context, fall back to global resolution (backward compat)
	if orgID == nil {
		return s.GetUserEffectivePlan(userID)
	}

	// Load the organization to check its type
	var org orgModels.Organization
	if err := s.db.First(&org, "id = ?", *orgID).Error; err != nil {
		return nil, fmt.Errorf("failed to load organization %s: %w", orgID.String(), err)
	}

	if org.IsPersonalOrg() {
		// Personal org → return user's personal subscription (not assigned org plans)
		var sub models.UserSubscription
		err := s.db.Preload("SubscriptionPlan").
			Where("user_id = ? AND subscription_type = ? AND status IN ?", userID, "personal", []string{"active", "trialing"}).
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

	// Team org → check that the user is actually a member of this org
	var memberCount int64
	if err := s.db.Model(&orgModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND is_active = ?", *orgID, userID, true).
		Count(&memberCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check org membership: %w", err)
	}
	if memberCount == 0 {
		return nil, fmt.Errorf("user %s is not a member of organization %s", userID, orgID.String())
	}

	// Return that org's subscription
	orgSub, err := s.orgSubRepo.GetActiveOrganizationSubscription(*orgID)
	if err != nil {
		// Team org has no subscription — fall back to user's personal subscription
		result, fallbackErr := s.GetUserEffectivePlan(userID)
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

// CheckEffectiveUsageLimitForOrg checks usage limits in the context of a specific org.
// If orgID is nil, falls back to the global resolution.
//
// Thin wrapper kept for backward compatibility — actual logic lives in QuotaService.
func (s *effectivePlanService) CheckEffectiveUsageLimitForOrg(userID string, orgID *uuid.UUID, metricType string, increment int64) (*UsageLimitCheck, error) {
	result, err := s.GetUserEffectivePlanForOrg(userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effective plan for org context: %w", err)
	}
	return s.quotaService().CheckUserQuotaWithPlan(result, userID, metricType, increment)
}

// CheckEffectiveUsageLimitFromResult checks usage limits using a pre-resolved plan result,
// avoiding the plan resolution DB round-trip. Called by CheckLimit middleware when
// InjectEffectivePlan has already resolved and stored the plan in the Gin context.
//
// Thin wrapper kept for backward compatibility — actual logic lives in QuotaService.
func (s *effectivePlanService) CheckEffectiveUsageLimitFromResult(result *EffectivePlanResult, userID string, metricType string, increment int64) (*UsageLimitCheck, error) {
	return s.quotaService().CheckUserQuotaWithPlan(result, userID, metricType, increment)
}
