// src/payment/repositories/organizationSubscriptionRepository.go
package repositories

import (
	"soli/formations/src/payment/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionRepository interface {
	// OrganizationSubscription operations
	CreateOrganizationSubscription(subscription *models.OrganizationSubscription) error
	// CreateOrganizationSubscriptionAtomic atomically deactivates any
	// existing active/trialing subscription for the org and inserts the new
	// one inside a single transaction. Use this for any assignment path that
	// must enforce the "one active subscription per organization" invariant.
	CreateOrganizationSubscriptionAtomic(subscription *models.OrganizationSubscription) error
	GetOrganizationSubscription(id uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByOrgID(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	GetOrganizationSubscriptionByStripeID(stripeSubscriptionID string) (*models.OrganizationSubscription, error)
	GetActiveOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error)
	// GetActiveOrganizationSubscriptionByStripeCustomerID resolves the active or
	// trialing organization subscription that owns a Stripe customer. It is the
	// org-side sibling of PaymentRepository.GetActiveSubscriptionByCustomerID and
	// is the single fallback the invoice webhook handlers use when no user
	// subscription owns the customer.
	GetActiveOrganizationSubscriptionByStripeCustomerID(customerID string) (*models.OrganizationSubscription, error)
	GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error)
	GetUserOrganizationSubscriptions(userID string) ([]models.OrganizationSubscription, error)
	UpdateOrganizationSubscription(subscription *models.OrganizationSubscription) error

	// OrganizationRolePlan operations
	// GetOrganizationRolePlan fetches the role→plan entitlement mapping for a
	// given organization and member role, with its SubscriptionPlan preloaded.
	// Returns gorm.ErrRecordNotFound when no mapping exists for that role.
	GetOrganizationRolePlan(orgID uuid.UUID, role string) (*models.OrganizationRolePlan, error)
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

// CreateOrganizationSubscriptionAtomic deactivates any existing active or
// trialing subscription for the same organization, then inserts the new one,
// inside a single transaction. This enforces the "one active subscription per
// organization" invariant at the data layer.
//
// Used by every code path that activates a new subscription (admin assignment,
// trial bootstrap, Stripe webhook). The new subscription is created regardless
// of its own status — if the caller is inserting an "incomplete" subscription
// (paid plan awaiting Stripe confirmation), no prior subscription is touched.
func (r *organizationSubscriptionRepository) CreateOrganizationSubscriptionAtomic(subscription *models.OrganizationSubscription) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Only deactivate the previous active subscription when the new one
		// is being activated. Inserting an "incomplete" subscription (paid
		// plan awaiting Stripe webhook) must not cancel a currently-active
		// plan — that would leave the org without coverage.
		if subscription.Status == "active" || subscription.Status == "trialing" {
			if err := deactivatePreviousOrgSubscription(tx, subscription.OrganizationID); err != nil {
				return err
			}
		}
		return tx.Create(subscription).Error
	})
}

// deactivatePreviousOrgSubscription marks any existing active or trialing
// subscription for the given organization as cancelled. Idempotent: returns
// nil with zero affected rows when the org has no prior active subscription.
func deactivatePreviousOrgSubscription(tx *gorm.DB, orgID uuid.UUID) error {
	return tx.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ? AND status IN ?", orgID, []string{"active", "trialing"}).
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": time.Now(),
		}).Error
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

// GetActiveOrganizationSubscriptionByStripeCustomerID retrieves the active or
// trialing subscription bound to a Stripe customer. Mirrors the user-side
// GetActiveSubscriptionByCustomerID (same active/trialing status filter), and
// returns the newest match if several exist.
func (r *organizationSubscriptionRepository) GetActiveOrganizationSubscriptionByStripeCustomerID(customerID string) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("stripe_customer_id = ? AND status IN (?)", customerID, []string{"active", "trialing"}).
		Order("created_at DESC").
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetAllActiveOrganizationSubscriptions retrieves all active or trialing organization subscriptions
func (r *organizationSubscriptionRepository) GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error) {
	var subscriptions []models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("status IN (?)", []string{"active", "trialing"}).
		Order("created_at DESC").
		Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
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

// GetOrganizationRolePlan retrieves the role→plan entitlement mapping for a
// given organization and member role, with its SubscriptionPlan preloaded.
// Returns gorm.ErrRecordNotFound when no mapping exists for that (org, role).
func (r *organizationSubscriptionRepository) GetOrganizationRolePlan(orgID uuid.UUID, role string) (*models.OrganizationRolePlan, error) {
	var rolePlan models.OrganizationRolePlan
	err := r.db.Preload("SubscriptionPlan").
		Where("organization_id = ? AND role = ?", orgID, role).
		First(&rolePlan).Error
	if err != nil {
		return nil, err
	}
	return &rolePlan, nil
}

