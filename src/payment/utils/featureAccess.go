// src/payment/utils/featureAccess.go
package utils

import (
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// GetUserEffectiveFeatures returns the highest-tier features from all user's organizations
// This is a helper function that delegates to OrganizationSubscriptionService
func GetUserEffectiveFeatures(db *gorm.DB, userID string) (*models.SubscriptionPlan, error) {
	service := services.NewOrganizationSubscriptionService(db)
	features, err := service.GetUserEffectiveFeatures(userID)
	if err != nil {
		return nil, err
	}

	// Return the highest priority plan
	return features.HighestPlan, nil
}

// CanUserAccessFeature checks if user can access a feature through any organization
// This is a helper function that delegates to OrganizationSubscriptionService
func CanUserAccessFeature(db *gorm.DB, userID string, feature string) (bool, error) {
	service := services.NewOrganizationSubscriptionService(db)
	return service.CanUserAccessFeature(userID, feature)
}

// GetUserOrganizationWithFeature returns the organization that provides a specific feature
// If multiple organizations provide the feature, returns the one with highest priority plan
// This is a helper function that delegates to OrganizationSubscriptionService
func GetUserOrganizationWithFeature(db *gorm.DB, userID string, feature string) (*organizationModels.Organization, error) {
	service := services.NewOrganizationSubscriptionService(db)
	return service.GetUserOrganizationWithFeature(userID, feature)
}

// GetUserEffectiveLimits returns the maximum usage limits from all user's organizations
// Returns the highest limits across all organization subscriptions
func GetUserEffectiveLimits(db *gorm.DB, userID string) (*EffectiveLimits, error) {
	service := services.NewOrganizationSubscriptionService(db)
	features, err := service.GetUserEffectiveFeatures(userID)
	if err != nil {
		return nil, err
	}

	return &EffectiveLimits{
		MaxConcurrentTerminals: features.MaxConcurrentTerminals,
		MaxCourses:             features.MaxCourses,
		MaxLabSessions:         features.MaxLabSessions,
	}, nil
}

// EffectiveLimits represents the aggregate limits for a user across all organizations
type EffectiveLimits struct {
	MaxConcurrentTerminals int
	MaxCourses             int
	MaxLabSessions         int
}
