package services

import (
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// SubscriptionPlanToOutput is the ONLY producer of dto.SubscriptionPlanOutput.
//
// There were three, with three different field sets, and the differences were not
// visible at any call site (#454):
//
//   - the entity registration built the complete DTO;
//   - conversionService dropped GroupManagementEnabled, BulkPurchasable, SeatUnit,
//     SessionSupervisionEnabled, MaxCPU and MaxMemoryMB;
//   - convertSubscriptionPlanToOutput dropped those five capability fields too,
//     plus IsCatalog.
//
// The omissions were not neutral, because the zero value carries meaning here:
// MaxCPU / MaxMemoryMB of 0 means UNLIMITED (see models.SubscriptionPlan), so two
// of the three producers reported an unlimited CPU/RAM budget for every plan they
// converted, while also reporting group management as absent on plans that grant
// it. Wrong in the permissive direction on quota, restrictive on features.
//
// Anything that needs this DTO MUST come through here. A DTO with more than one
// builder drifts; this one already had, silently, across six fields.
func SubscriptionPlanToOutput(plan *models.SubscriptionPlan) dto.SubscriptionPlanOutput {
	if plan == nil {
		return dto.SubscriptionPlanOutput{}
	}

	pricingTiers := make([]dto.PricingTier, len(plan.PricingTiers))
	for i, tier := range plan.PricingTiers {
		pricingTiers[i] = dto.PricingTier{
			MinQuantity: tier.MinQuantity,
			MaxQuantity: tier.MaxQuantity,
			UnitAmount:  tier.UnitAmount,
			Description: tier.Description,
		}
	}

	return dto.SubscriptionPlanOutput{
		ID:              plan.ID,
		Name:            plan.Name,
		Description:     plan.Description,
		Priority:        plan.Priority,
		StripeProductID: plan.StripeProductID,
		StripePriceID:   plan.StripePriceID,
		PriceAmount:     plan.PriceAmount,
		Currency:        plan.Currency,
		BillingInterval: plan.BillingInterval,
		TaxBehavior:     plan.TaxBehavior,
		Features:        derivePlanEntitlements(plan),
		IsActive:        plan.IsActive,
		IsCatalog:       plan.IsCatalog,
		RequiredRole:    plan.RequiredRole,
		CreatedAt:       plan.CreatedAt,
		UpdatedAt:       plan.UpdatedAt,

		// Capability flags
		GroupManagementEnabled:    plan.GroupManagementEnabled,
		BulkPurchasable:           plan.BulkPurchasable,
		IsDefaultFree:             plan.IsDefaultFree,
		SeatUnit:                  plan.SeatUnit,
		NetworkAccessEnabled:      plan.NetworkAccessEnabled,
		PortExposureEnabled:       plan.PortExposureEnabled,
		DataPersistenceEnabled:    plan.DataPersistenceEnabled,
		SessionSupervisionEnabled: plan.SessionSupervisionEnabled,

		// Terminal-specific limits
		MaxSessionDurationMinutes:   plan.MaxSessionDurationMinutes,
		DataPersistenceGB:           plan.DataPersistenceGB,
		CommandHistoryRetentionDays: plan.CommandHistoryRetentionDays,

		// Backend routing
		DefaultBackend:  plan.DefaultBackend,
		AllowedBackends: plan.AllowedBackends,

		// Budget-based quota
		MaxCPU:      plan.MaxCPU,
		MaxMemoryMB: plan.MaxMemoryMB,

		// Tiered pricing
		UseTieredPricing: plan.UseTieredPricing,
		PricingTiers:     pricingTiers,
	}
}
