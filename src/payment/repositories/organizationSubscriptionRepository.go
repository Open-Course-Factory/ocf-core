// src/payment/repositories/organizationSubscriptionRepository.go
package repositories

import (
	"soli/formations/src/payment/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionRepository struct {
	db *gorm.DB
}

func NewOrganizationSubscriptionRepository(db *gorm.DB) *OrganizationSubscriptionRepository {
	return &OrganizationSubscriptionRepository{
		db: db,
	}
}

// CreateOrganizationSubscription creates a new organization subscription
func (r *OrganizationSubscriptionRepository) CreateOrganizationSubscription(subscription *models.OrganizationSubscription) error {
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
func (r *OrganizationSubscriptionRepository) CreateOrganizationSubscriptionAtomic(subscription *models.OrganizationSubscription) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Only deactivate the previous active subscription when the new one
		// is being activated. Inserting an "incomplete" subscription (paid
		// plan awaiting Stripe webhook) must not cancel a currently-active
		// plan — that would leave the org without coverage.
		if subscription.Status == "active" {
			if err := deactivatePreviousOrgSubscription(tx, subscription.OrganizationID); err != nil {
				return err
			}
		}
		return tx.Create(subscription).Error
	})
}

// deactivatePreviousOrgSubscription marks any existing active subscription for
// the given organization as cancelled. Idempotent: returns nil with zero
// affected rows when the org has no prior active subscription.
func deactivatePreviousOrgSubscription(tx *gorm.DB, orgID uuid.UUID) error {
	return tx.Model(&models.OrganizationSubscription{}).
		// Deliberately NOT the entitling predicate: this upholds the same
		// "one active subscription per organization" invariant as the partial
		// UNIQUE INDEX in models/organizationSubscription.go, whose WHERE clause
		// states exactly this status. Widening it here would cancel rows the
		// index never constrained. Change the two together.
		Where("organization_id = ? AND status = ?", orgID, "active").
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": time.Now(),
		}).Error
}

// GetOrganizationSubscription retrieves a subscription by ID
func (r *OrganizationSubscriptionRepository) GetOrganizationSubscription(id uuid.UUID) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").Where("id = ?", id).First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetOrganizationSubscriptionByOrgID retrieves the subscription for an organization
func (r *OrganizationSubscriptionRepository) GetOrganizationSubscriptionByOrgID(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
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
func (r *OrganizationSubscriptionRepository) GetOrganizationSubscriptionByStripeID(stripeSubscriptionID string) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("stripe_subscription_id = ?", stripeSubscriptionID).
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetActiveOrganizationSubscription retrieves the entitling subscription for an
// organization. Entitling, so an org in dunning still resolves its plan — the
// same grace behaviour the user side has always had.
func (r *OrganizationSubscriptionRepository) GetActiveOrganizationSubscription(orgID uuid.UUID) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Scopes(models.ScopeEntitling).
		Where("organization_id = ?", orgID).
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
func (r *OrganizationSubscriptionRepository) GetActiveOrganizationSubscriptionByStripeCustomerID(customerID string) (*models.OrganizationSubscription, error) {
	var subscription models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Scopes(models.ScopeBillable).
		Where("stripe_customer_id = ?", customerID).
		Order("created_at DESC").
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetAllActiveOrganizationSubscriptions retrieves all active or trialing organization subscriptions
func (r *OrganizationSubscriptionRepository) GetAllActiveOrganizationSubscriptions() ([]models.OrganizationSubscription, error) {
	var subscriptions []models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		// Billable: this is an operational listing of cleanly-paid org
		// subscriptions, not a gate deciding anyone's access.
		Scopes(models.ScopeBillable).
		Order("created_at DESC").
		Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

// GetUserOrganizationSubscriptions retrieves all organization subscriptions for a user
// Returns subscriptions from all organizations the user is a member of
func (r *OrganizationSubscriptionRepository) GetUserOrganizationSubscriptions(userID string) ([]models.OrganizationSubscription, error) {
	var subscriptions []models.OrganizationSubscription
	err := r.db.Preload("SubscriptionPlan").
		Joins("JOIN organization_members ON organization_members.organization_id = organization_subscriptions.organization_id").
		// Entitling: this feeds effectivePlanService.resolveGlobal and
		// GetUserEffectiveFeatures, both of which decide what the user may do.
		// Qualified column, so the canonical set is used rather than the scope.
		Where("organization_members.user_id = ? AND organization_members.is_active = ? AND organization_subscriptions.status IN (?)",
			userID, true, models.EntitlingStatuses()).
		Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

// UpdateOrganizationSubscription updates an organization subscription
func (r *OrganizationSubscriptionRepository) UpdateOrganizationSubscription(subscription *models.OrganizationSubscription) error {
	return r.db.Save(subscription).Error
}

// GetOrganizationRolePlan retrieves the role→plan entitlement mapping for a
// given organization and member role, with its SubscriptionPlan preloaded.
// Returns gorm.ErrRecordNotFound when no mapping exists for that (org, role).
func (r *OrganizationSubscriptionRepository) GetOrganizationRolePlan(orgID uuid.UUID, role string) (*models.OrganizationRolePlan, error) {
	var rolePlan models.OrganizationRolePlan
	err := r.db.Preload("SubscriptionPlan").
		Where("organization_id = ? AND role = ?", orgID, role).
		First(&rolePlan).Error
	if err != nil {
		return nil, err
	}
	return &rolePlan, nil
}

// GetOrganizationRolePlans retrieves every role→plan entitlement mapping for a
// given organization, each with its SubscriptionPlan preloaded, ordered by role
// for a stable listing.
func (r *OrganizationSubscriptionRepository) GetOrganizationRolePlans(orgID uuid.UUID) ([]models.OrganizationRolePlan, error) {
	var rolePlans []models.OrganizationRolePlan
	err := r.db.Preload("SubscriptionPlan").
		Where("organization_id = ?", orgID).
		Order("role ASC").
		Find(&rolePlans).Error
	if err != nil {
		return nil, err
	}
	return rolePlans, nil
}
