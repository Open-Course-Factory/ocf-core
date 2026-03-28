package services

import (
	"errors"
	"fmt"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/utils"

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
}

// EffectivePlanService is the single source of truth for "what plan does this user have?"
type EffectivePlanService interface {
	GetUserEffectivePlan(userID string) (*EffectivePlanResult, error)
	CheckEffectiveUsageLimit(userID string, metricType string, increment int64) (*UsageLimitCheck, error)
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
func (s *effectivePlanService) CheckEffectiveUsageLimit(userID string, metricType string, increment int64) (*UsageLimitCheck, error) {
	// 1. Get the effective plan
	result, err := s.GetUserEffectivePlan(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effective plan: %w", err)
	}

	plan := result.Plan

	// 2. Determine the limit from the plan
	var limit int64
	switch metricType {
	case "concurrent_terminals":
		limit = int64(plan.MaxConcurrentTerminals)
	case "courses_created":
		limit = int64(plan.MaxCourses)
	default:
		limit = -1 // unlimited
	}

	// 3. Get current usage
	var currentUsage int64
	if metricType == "concurrent_terminals" {
		// Real-time count from the terminals table
		countErr := s.db.Table("terminals").
			Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active").
			Count(&currentUsage).Error
		if countErr != nil {
			return nil, fmt.Errorf("failed to count active terminals: %w", countErr)
		}
	} else {
		// Check from usage_metrics table for the current period
		metrics, metricsErr := s.paymentRepo.GetUserUsageMetrics(userID, metricType)
		if metricsErr != nil {
			if !errors.Is(metricsErr, gorm.ErrRecordNotFound) {
				utils.Warn("Failed to get usage metrics for user %s, metric %s: %v", userID, metricType, metricsErr)
			}
			currentUsage = 0
		} else {
			currentUsage = metrics.CurrentValue
		}
	}

	// 4. Calculate and return
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
		message = fmt.Sprintf("Usage limit exceeded for %s. Current: %d, Limit: %d", metricType, currentUsage, limit)
	}

	return &UsageLimitCheck{
		Allowed:        allowed,
		CurrentUsage:   currentUsage,
		Limit:          limit,
		RemainingUsage: remaining,
		Message:        message,
		UserID:         userID,
		MetricType:     metricType,
	}, nil
}
