// src/payment/services/pricingService.go
package services

import (
	"fmt"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PricingService interface {
	CalculatePricingPreview(planID uuid.UUID, quantity int) (*dto.PricingBreakdown, error)
	GetTotalCost(plan *models.SubscriptionPlan, quantity int) int64
}

type pricingService struct {
	planRepository repositories.SubscriptionPlanRepository
}

func NewPricingService(db *gorm.DB) PricingService {
	return &pricingService{
		planRepository: repositories.NewSubscriptionPlanRepository(db),
	}
}

// CalculatePricingPreview calculates detailed pricing breakdown for a given plan and quantity
func (ps *pricingService) CalculatePricingPreview(planID uuid.UUID, quantity int) (*dto.PricingBreakdown, error) {
	// Get the subscription plan
	plan, err := ps.planRepository.GetByID(planID)
	if err != nil {
		return nil, fmt.Errorf("plan not found: %w", err)
	}

	breakdown := &dto.PricingBreakdown{
		PlanName:      plan.Name,
		TotalQuantity: quantity,
		Currency:      plan.Currency,
	}

	// If tiered pricing is not enabled, use simple flat pricing
	if !plan.UseTieredPricing || len(plan.PricingTiers) == 0 {
		totalCost := plan.PriceAmount * int64(quantity)
		breakdown.TotalMonthlyCost = totalCost
		breakdown.AveragePerUnit = float64(plan.PriceAmount) / 100.0
		breakdown.Savings = 0

		// Add single tier for display
		breakdown.TierBreakdown = []dto.TierCost{
			{
				Range:     fmt.Sprintf("1-%d", quantity),
				Quantity:  quantity,
				UnitPrice: plan.PriceAmount,
				Subtotal:  totalCost,
			},
		}

		return breakdown, nil
	}

	// Calculate tiered pricing
	remaining := quantity
	totalCost := int64(0)

	for _, tier := range plan.PricingTiers {
		if remaining <= 0 {
			break
		}

		// Determine how many licenses fall in this tier
		tierStart := tier.MinQuantity
		tierEnd := tier.MaxQuantity
		if tierEnd == 0 {
			tierEnd = remaining + tierStart - 1 // Unlimited tier, take all remaining
		}

		// Calculate how many licenses are in this tier range
		tierCapacity := tierEnd - tierStart + 1
		tierQty := min(remaining, tierCapacity)

		// Calculate cost for this tier
		tierCost := int64(tierQty) * tier.UnitAmount
		totalCost += tierCost

		// Add to breakdown
		rangeStr := fmt.Sprintf("%d-%d", tierStart, tierStart+tierQty-1)
		if tier.MaxQuantity == 0 {
			rangeStr = fmt.Sprintf("%d+", tierStart)
		}

		breakdown.TierBreakdown = append(breakdown.TierBreakdown, dto.TierCost{
			Range:     rangeStr,
			Quantity:  tierQty,
			UnitPrice: tier.UnitAmount,
			Subtotal:  tierCost,
		})

		remaining -= tierQty
	}

	breakdown.TotalMonthlyCost = totalCost
	breakdown.AveragePerUnit = float64(totalCost) / float64(quantity) / 100.0

	// Calculate savings vs individual (flat) pricing
	individualCost := plan.PriceAmount * int64(quantity)
	breakdown.Savings = individualCost - totalCost

	return breakdown, nil
}

// GetTotalCost calculates the total cost for a given plan and quantity (no breakdown)
func (ps *pricingService) GetTotalCost(plan *models.SubscriptionPlan, quantity int) int64 {
	if !plan.UseTieredPricing || len(plan.PricingTiers) == 0 {
		return plan.PriceAmount * int64(quantity)
	}

	remaining := quantity
	totalCost := int64(0)

	for _, tier := range plan.PricingTiers {
		if remaining <= 0 {
			break
		}

		tierStart := tier.MinQuantity
		tierEnd := tier.MaxQuantity
		if tierEnd == 0 {
			tierEnd = remaining + tierStart - 1
		}

		tierCapacity := tierEnd - tierStart + 1
		tierQty := min(remaining, tierCapacity)
		totalCost += int64(tierQty) * tier.UnitAmount

		remaining -= tierQty
	}

	return totalCost
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
