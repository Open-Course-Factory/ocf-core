// src/payment/services/organizationSubscriptionService.go
package services

import (
	"fmt"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/utils"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionService interface {
	// Subscription management
	GetOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByID(id uuid.UUID) (*models.OrganizationSubscription, error)
	CreateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID, ownerUserID string, quantity int, isAdminAssigned bool) (*models.OrganizationSubscription, error)
	UpdateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID) (*models.OrganizationSubscription, error)
	CancelOrganizationSubscription(orgID uuid.UUID, cancelAtPeriodEnd bool) error

	// Admin bulk access
	GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error)

	// Feature access (for members)
	GetOrganizationFeatures(orgID uuid.UUID) (*models.SubscriptionPlan, error)
	CanOrganizationAccessFeature(orgID uuid.UUID, feature string) (bool, error)
	GetOrganizationUsageLimits(orgID uuid.UUID) (*OrganizationLimits, error)

	// User-level feature aggregation
	GetUserEffectiveFeatures(userID string) (*UserEffectiveFeatures, error)
	CanUserAccessFeature(userID string, feature string) (bool, error)
	GetUserOrganizationWithFeature(userID string, feature string) (*organizationModels.Organization, error)
}

// Business types for organization limits
type OrganizationLimits struct {
	OrganizationID         uuid.UUID
	MaxConcurrentTerminals int
	MaxCourses             int
	CurrentTerminals       int
	CurrentCourses         int
}

type UserEffectiveFeatures struct {
	HighestPlan            *models.SubscriptionPlan
	AllFeatures            []string
	MaxConcurrentTerminals int
	MaxCourses             int
	Organizations          []OrganizationFeatureInfo
}

type OrganizationFeatureInfo struct {
	OrganizationID   uuid.UUID
	OrganizationName string
	SubscriptionPlan models.SubscriptionPlan
	IsOwner          bool
	IsManager        bool
}

type organizationSubscriptionService struct {
	repository  repositories.OrganizationSubscriptionRepository
	paymentRepo repositories.PaymentRepository
	db          *gorm.DB
}

func NewOrganizationSubscriptionService(db *gorm.DB) OrganizationSubscriptionService {
	return &organizationSubscriptionService{
		repository:  repositories.NewOrganizationSubscriptionRepository(db),
		paymentRepo: repositories.NewPaymentRepository(db),
		db:          db,
	}
}

// GetOrganizationSubscription retrieves the active subscription for an organization
func (oss *organizationSubscriptionService) GetOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
	return oss.repository.GetActiveOrganizationSubscription(orgID)
}

// GetOrganizationSubscriptionByID retrieves a subscription by its ID
func (oss *organizationSubscriptionService) GetOrganizationSubscriptionByID(id uuid.UUID) (*models.OrganizationSubscription, error) {
	return oss.repository.GetOrganizationSubscription(id)
}

// GetAllActiveOrganizationSubscriptions retrieves all active or trialing organization subscriptions
func (oss *organizationSubscriptionService) GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error) {
	return oss.repository.GetAllActiveOrganizationSubscriptions()
}

// CreateOrganizationSubscription creates a new organization subscription
// For free plans (PriceAmount == 0), creates an active subscription
// For paid plans, creates an incomplete subscription that will be activated by Stripe webhook
// When isAdminAssigned is true, paid plans are activated immediately (no Stripe flow)
func (oss *organizationSubscriptionService) CreateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID, ownerUserID string, quantity int, isAdminAssigned bool) (*models.OrganizationSubscription, error) {
	// Verify the organization exists
	var org organizationModels.Organization
	if err := oss.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	// Get the plan to check if it's free
	var plan models.SubscriptionPlan
	if err := oss.db.Where("id = ?", planID).First(&plan).Error; err != nil {
		return nil, fmt.Errorf("invalid plan ID: %w", err)
	}

	now := time.Now()

	// Default to 1 seat if quantity is not provided or invalid
	if quantity <= 0 {
		quantity = 1
	}

	subscription := &models.OrganizationSubscription{
		OrganizationID:     orgID,
		SubscriptionPlanID: planID,
		Quantity:           quantity,
	}

	// FREE PLAN or ADMIN-ASSIGNED: Activate immediately without Stripe
	if plan.PriceAmount == 0 || isAdminAssigned {
		subscription.Status = "active"
		subscription.CurrentPeriodStart = now
		subscription.CurrentPeriodEnd = now.AddDate(1, 0, 0)

		if isAdminAssigned {
			utils.Info("Creating admin-assigned organization subscription for org %s (plan: %s)", orgID, plan.Name)
		} else {
			utils.Info("Creating free organization subscription for org %s (plan: %s)", orgID, plan.Name)
		}
	} else {
		// PAID PLAN: Will be activated by Stripe webhook
		subscription.Status = "incomplete"
		utils.Debug("Creating incomplete organization subscription for org %s (will be activated by Stripe)", orgID)
	}

	err := oss.repository.CreateOrganizationSubscription(subscription)
	if err != nil {
		return nil, err
	}

	// Update Organization.SubscriptionPlanID
	err = oss.db.Model(&org).Update("subscription_plan_id", planID).Error
	if err != nil {
		utils.Warn("Failed to update organization subscription_plan_id: %v", err)
	}

	return oss.GetOrganizationSubscriptionByID(subscription.ID)
}

// UpdateOrganizationSubscription updates an organization's subscription plan
func (oss *organizationSubscriptionService) UpdateOrganizationSubscription(orgID uuid.UUID, planID uuid.UUID) (*models.OrganizationSubscription, error) {
	// Get the organization's active subscription
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	// Get the new plan to verify it exists
	var newPlan models.SubscriptionPlan
	if err := oss.db.Where("id = ?", planID).First(&newPlan).Error; err != nil {
		return nil, fmt.Errorf("invalid plan ID: %w", err)
	}

	// Update subscription plan ID
	subscription.SubscriptionPlanID = planID
	subscription.SubscriptionPlan = newPlan

	err = oss.repository.UpdateOrganizationSubscription(subscription)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	// Update Organization.SubscriptionPlanID
	err = oss.db.Model(&organizationModels.Organization{}).
		Where("id = ?", orgID).
		Update("subscription_plan_id", planID).Error
	if err != nil {
		utils.Warn("Failed to update organization subscription_plan_id: %v", err)
	}

	return oss.repository.GetOrganizationSubscription(subscription.ID)
}

// CancelOrganizationSubscription cancels an organization's subscription
func (oss *organizationSubscriptionService) CancelOrganizationSubscription(orgID uuid.UUID, cancelAtPeriodEnd bool) error {
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return fmt.Errorf("no active subscription found for organization: %w", err)
	}

	if cancelAtPeriodEnd {
		subscription.CancelAtPeriodEnd = true
		utils.Info("Organization subscription %s will be cancelled at period end", subscription.ID)
	} else {
		subscription.Status = "cancelled"
		now := time.Now()
		subscription.CancelledAt = &now
		utils.Info("Organization subscription %s cancelled immediately", subscription.ID)
	}

	return oss.repository.UpdateOrganizationSubscription(subscription)
}

// GetOrganizationFeatures returns the subscription plan features for an organization
func (oss *organizationSubscriptionService) GetOrganizationFeatures(orgID uuid.UUID) (*models.SubscriptionPlan, error) {
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	return &subscription.SubscriptionPlan, nil
}

// CanOrganizationAccessFeature checks if an organization has access to a specific feature
func (oss *organizationSubscriptionService) CanOrganizationAccessFeature(orgID uuid.UUID, feature string) (bool, error) {
	plan, err := oss.GetOrganizationFeatures(orgID)
	if err != nil {
		return false, err
	}

	// Check if feature is in the plan's features list
	for _, f := range plan.Features {
		if f == feature {
			return true, nil
		}
	}

	return false, nil
}

// GetOrganizationUsageLimits returns the current usage and limits for an organization
func (oss *organizationSubscriptionService) GetOrganizationUsageLimits(orgID uuid.UUID) (*OrganizationLimits, error) {
	subscription, err := oss.repository.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for organization: %w", err)
	}

	plan := subscription.SubscriptionPlan

	// Count current usage
	var currentTerminals int64
	oss.db.Table("terminals").
		Joins("JOIN organization_members ON organization_members.user_id = terminals.user_id").
		Where("organization_members.organization_id = ? AND terminals.status = ? AND terminals.deleted_at IS NULL",
			orgID, "active").
		Count(&currentTerminals)

	var currentCourses int64
	oss.db.Table("courses").
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

// GetUserEffectiveFeatures returns the highest-tier features from all user's organizations
func (oss *organizationSubscriptionService) GetUserEffectiveFeatures(userID string) (*UserEffectiveFeatures, error) {
	// Get all organization subscriptions for the user
	subscriptions, err := oss.repository.GetUserOrganizationSubscriptions(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user organization subscriptions: %w", err)
	}

	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("user has no organization subscriptions")
	}

	// Aggregate features
	features := &UserEffectiveFeatures{
		AllFeatures:            make([]string, 0),
		MaxConcurrentTerminals: 0,
		MaxCourses:             0,
		Organizations:          make([]OrganizationFeatureInfo, 0),
	}

	featureSet := make(map[string]bool)
	var highestPriority int = -1

	// Get organization details
	for _, sub := range subscriptions {
		var org organizationModels.Organization
		if err := oss.db.Where("id = ?", sub.OrganizationID).First(&org).Error; err != nil {
			utils.Warn("Failed to get organization %s: %v", sub.OrganizationID, err)
			continue
		}

		// Get member info
		var member organizationModels.OrganizationMember
		if err := oss.db.Where("organization_id = ? AND user_id = ?", sub.OrganizationID, userID).First(&member).Error; err != nil {
			utils.Warn("Failed to get member info for org %s: %v", sub.OrganizationID, err)
			continue
		}

		plan := sub.SubscriptionPlan

		// Track highest priority plan
		if plan.Priority > highestPriority {
			highestPriority = plan.Priority
			features.HighestPlan = &plan
		}

		// Aggregate features
		for _, feature := range plan.Features {
			featureSet[feature] = true
		}

		// Take maximum limits (-1 means unlimited, which is always the maximum)
		if features.MaxConcurrentTerminals != -1 {
			if plan.MaxConcurrentTerminals == -1 || plan.MaxConcurrentTerminals > features.MaxConcurrentTerminals {
				features.MaxConcurrentTerminals = plan.MaxConcurrentTerminals
			}
		}
		if features.MaxCourses != -1 {
			if plan.MaxCourses == -1 || plan.MaxCourses > features.MaxCourses {
				features.MaxCourses = plan.MaxCourses
			}
		}

		// Add organization info
		features.Organizations = append(features.Organizations, OrganizationFeatureInfo{
			OrganizationID:   org.ID,
			OrganizationName: org.DisplayName,
			SubscriptionPlan: plan,
			IsOwner:          member.IsOwner(),
			IsManager:        member.IsManager(),
		})
	}

	// Convert feature set to slice
	for feature := range featureSet {
		features.AllFeatures = append(features.AllFeatures, feature)
	}

	return features, nil
}

// CanUserAccessFeature checks if user can access a feature through any organization
func (oss *organizationSubscriptionService) CanUserAccessFeature(userID string, feature string) (bool, error) {
	features, err := oss.GetUserEffectiveFeatures(userID)
	if err != nil {
		return false, err
	}

	for _, f := range features.AllFeatures {
		if f == feature {
			return true, nil
		}
	}

	return false, nil
}

// GetUserOrganizationWithFeature returns the organization that provides a specific feature
// If multiple organizations provide the feature, returns the one with highest priority plan
func (oss *organizationSubscriptionService) GetUserOrganizationWithFeature(userID string, feature string) (*organizationModels.Organization, error) {
	subscriptions, err := oss.repository.GetUserOrganizationSubscriptions(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user organization subscriptions: %w", err)
	}

	var bestOrg *organizationModels.Organization
	highestPriority := -1

	for _, sub := range subscriptions {
		plan := sub.SubscriptionPlan

		// Check if this plan has the feature
		hasFeature := false
		for _, f := range plan.Features {
			if f == feature {
				hasFeature = true
				break
			}
		}

		if hasFeature && plan.Priority > highestPriority {
			var org organizationModels.Organization
			if err := oss.db.Where("id = ?", sub.OrganizationID).First(&org).Error; err == nil {
				bestOrg = &org
				highestPriority = plan.Priority
			}
		}
	}

	if bestOrg == nil {
		return nil, fmt.Errorf("no organization provides feature: %s", feature)
	}

	return bestOrg, nil
}
