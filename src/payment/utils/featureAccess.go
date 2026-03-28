// src/payment/utils/featureAccess.go
package utils

import (
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// GetUserEffectiveFeatures returns the effective plan for the user (org or personal, whichever is highest).
// This is a helper function that delegates to EffectivePlanService.
func GetUserEffectiveFeatures(db *gorm.DB, userID string) (*models.SubscriptionPlan, error) {
	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)
	if err != nil {
		return nil, err
	}
	return result.Plan, nil
}

// CanUserAccessFeature checks if user can access a feature via their effective plan
// (unified resolution: org subscription with personal fallback).
func CanUserAccessFeature(db *gorm.DB, userID string, feature string) (bool, error) {
	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)
	if err != nil {
		return false, nil // no plan → no access (not an error for callers)
	}
	for _, f := range result.Plan.Features {
		if f == feature {
			return true, nil
		}
	}
	return false, nil
}

// GetUserOrganizationWithFeature returns the organization that provides a specific feature.
// If multiple organizations provide the feature, returns the one with highest priority plan.
// This still delegates to OrganizationSubscriptionService because it needs org-specific resolution.
func GetUserOrganizationWithFeature(db *gorm.DB, userID string, feature string) (*organizationModels.Organization, error) {
	service := services.NewOrganizationSubscriptionService(db)
	return service.GetUserOrganizationWithFeature(userID, feature)
}

// GetUserEffectiveLimits returns the usage limits from the user's effective plan
// (unified resolution: org subscription with personal fallback).
func GetUserEffectiveLimits(db *gorm.DB, userID string) (*EffectiveLimits, error) {
	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)
	if err != nil {
		return nil, err
	}

	return &EffectiveLimits{
		MaxConcurrentTerminals: result.Plan.MaxConcurrentTerminals,
		MaxCourses:             result.Plan.MaxCourses,
	}, nil
}

// EffectiveLimits represents the aggregate limits for a user across all organizations
type EffectiveLimits struct {
	MaxConcurrentTerminals int
	MaxCourses             int
}
