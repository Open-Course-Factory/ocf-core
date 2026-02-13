// src/payment/repositories/organizationSubscriptionRepository.go
package repositories

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionRepository interface {
	// OrganizationSubscription operations
	CreateOrganizationSubscription(subscription *models.OrganizationSubscription) error
	GetOrganizationSubscription(id uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByOrgID(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByStripeID(stripeSubscriptionID string) (*models.OrganizationSubscription, error)
	GetActiveOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	GetUserOrganizationSubscriptions(userID string) ([]models.OrganizationSubscription, error)
	UpdateOrganizationSubscription(subscription *models.OrganizationSubscription) error
}

type organizationSubscriptionRepository struct {
	db *gorm.DB
}

func NewOrganizationSubscriptionRepository(db *gorm.DB) OrganizationSubscriptionRepository {
	return &organizationSubscriptionRepository{
		db: db,
	}
}

// CreateOrganizationSubscription creates a new organization subscription
func (r *organizationSubscriptionRepository) CreateOrganizationSubscription(subscription *models.OrganizationSubscription) error {
	return r.db.Create(subscription).Error
}

// GetOrganizationSubscription retrieves a subscription by ID
func (r *organizationSubscriptionRepository) GetOrganizationSubscription(id uuid.UUID) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").Where("id = ?", id).First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetOrganizationSubscriptionByOrgID retrieves the subscription for an organization
func (r *organizationSubscriptionRepository) GetOrganizationSubscriptionByOrgID(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetOrganizationSubscriptionByStripeID retrieves a subscription by Stripe subscription ID
func (r *organizationSubscriptionRepository) GetOrganizationSubscriptionByStripeID(stripeSubscriptionID string) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("stripe_subscription_id = ?", stripeSubscriptionID).
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetActiveOrganizationSubscription retrieves the active subscription for an organization
func (r *organizationSubscriptionRepository) GetActiveOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("organization_id = ? AND status IN (?)", orgID, []string{"active", "trialing"}).
		Order("created_at DESC").
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetUserOrganizationSubscriptions retrieves all organization subscriptions for a user
// Returns subscriptions from all organizations the user is a member of
func (r *organizationSubscriptionRepository) GetUserOrganizationSubscriptions(userID string) ([]models.OrganizationSubscription, error) {
	var subscriptions []models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Joins("JOIN organization_members ON organization_members.organization_id = organization_subscriptions.organization_id").
		Where("organization_members.user_id = ? AND organization_members.is_active = ? AND organization_subscriptions.status IN (?)",
			userID, true, []string{"active", "trialing"}).
		Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

// UpdateOrganizationSubscription updates an organization subscription
func (r *organizationSubscriptionRepository) UpdateOrganizationSubscription(subscription *models.OrganizationSubscription) error {
	return r.db.Save(subscription).Error
}

