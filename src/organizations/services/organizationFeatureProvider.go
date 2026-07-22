package services

import (
	"fmt"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	paymentRepo "soli/formations/src/payment/repositories"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationFeatureProvider implements FeatureProvider for organizations
// It fetches features from organization subscriptions
type OrganizationFeatureProvider struct {
	db *gorm.DB
}

// NewOrganizationFeatureProvider creates a new feature provider for organizations
func NewOrganizationFeatureProvider(db *gorm.DB) entityManagementInterfaces.FeatureProvider {
	return &OrganizationFeatureProvider{
		db: db,
	}
}

// GetFeatures retrieves features for an organization from its subscription
func (p *OrganizationFeatureProvider) GetFeatures(entityID string) ([]string, bool, error) {
	orgID, err := uuid.Parse(entityID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid organization ID: %w", err)
	}

	// Create subscription repository
	subscriptionRepo := paymentRepo.NewOrganizationSubscriptionRepository(p.db)

	// Fetch organization subscription
	subscription, err := subscriptionRepo.GetActiveOrganizationSubscription(orgID)
	if err != nil {
		// No active subscription - organization is on free tier or no plan
		utils.Debug("No active subscription for organization %s: %v", orgID, err)
		return []string{}, false, nil
	}

	// Project the plan's TYPED capability fields into the canonical entitlement
	// set (single source of truth), rather than returning the legacy free-form
	// plan.Features array.
	plan := subscription.SubscriptionPlan
	features := paymentServices.DerivePlanEntitlements(&plan)
	hasSubscription := true

	// Additional validation: check subscription status
	if subscription.Status != "active" && subscription.Status != "trialing" {
		hasSubscription = false
		features = []string{}
		utils.Debug("Organization %s has subscription but status is %s", orgID, subscription.Status)
	}

	return features, hasSubscription, nil
}
