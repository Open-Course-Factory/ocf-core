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
	// PreviewProspectiveTiers prices a ladder that has not been saved yet, so an
	// admin can judge brackets before committing them.
	PreviewProspectiveTiers(input dto.ProspectivePricingInput) (*dto.ProspectivePricingOutput, error)
}

type pricingService struct {
	planRepository repositories.SubscriptionPlanRepository
}

func NewPricingService(db *gorm.DB) PricingService {
	return &pricingService{
		planRepository: repositories.NewSubscriptionPlanRepository(db),
	}
}

// GraduatedCost is the single implementation of graduated tier pricing: each
// bracket is filled in turn and the costs stack, so a customer buying 20 units
// under 1-10 @ 9.00 / 11+ @ 7.00 pays 10x9.00 + 10x7.00, not 20x7.00.
//
// It was previously written twice in this file — CalculatePricingPreview and
// GetTotalCost each walked the tiers — and is now needed a third time for
// prospective ladders. Three copies of the rule that decides what customers pay
// is exactly the shape that drifts.
//
// Returns the total in cents and the per-bracket breakdown. A non-positive
// quantity or an empty ladder yields (0, nil): callers with no tiers fall back
// to flat pricing rather than treating zero as a price.
func GraduatedCost(tiers []models.PricingTier, quantity int) (int64, []dto.TierCost) {
	if quantity <= 0 || len(tiers) == 0 {
		return 0, nil
	}

	remaining := quantity
	var total int64
	var breakdown []dto.TierCost

	for _, tier := range tiers {
		if remaining <= 0 {
			break
		}

		tierStart := tier.MinQuantity
		tierEnd := tier.MaxQuantity
		if tierEnd == 0 {
			// Open-ended bracket: absorbs everything still unpriced.
			tierEnd = remaining + tierStart - 1
		}

		capacity := tierEnd - tierStart + 1
		if capacity <= 0 {
			// Malformed bracket (max below min); skip rather than price negatively.
			continue
		}

		take := min(remaining, capacity)
		subtotal := int64(take) * tier.UnitAmount
		total += subtotal

		label := fmt.Sprintf("%d-%d", tierStart, tierStart+take-1)
		if tier.MaxQuantity == 0 {
			label = fmt.Sprintf("%d+", tierStart)
		}
		breakdown = append(breakdown, dto.TierCost{
			Range:     label,
			Quantity:  take,
			UnitPrice: tier.UnitAmount,
			Subtotal:  subtotal,
		})

		remaining -= take
	}

	return total, breakdown
}

// tiersFromDTO converts a prospective ladder into the model shape the engine takes.
func tiersFromDTO(in []dto.PricingTier) []models.PricingTier {
	out := make([]models.PricingTier, 0, len(in))
	for _, t := range in {
		out = append(out, models.PricingTier{
			MinQuantity: t.MinQuantity,
			MaxQuantity: t.MaxQuantity,
			UnitAmount:  t.UnitAmount,
			Description: t.Description,
		})
	}
	return out
}

// CalculatePricingPreview calculates a detailed pricing breakdown for a SAVED plan.
func (ps *pricingService) CalculatePricingPreview(planID uuid.UUID, quantity int) (*dto.PricingBreakdown, error) {
	plan, err := ps.planRepository.GetByID(planID)
	if err != nil {
		return nil, fmt.Errorf("plan not found: %w", err)
	}

	breakdown := &dto.PricingBreakdown{
		PlanName:            plan.Name,
		TotalQuantity:       quantity,
		IndividualUnitPrice: plan.PriceAmount,
		Currency:            plan.Currency,
	}

	if !plan.UseTieredPricing || len(plan.PricingTiers) == 0 {
		total := plan.PriceAmount * int64(quantity)
		breakdown.TotalMonthlyCost = total
		breakdown.AveragePerUnit = perUnit(total, quantity)
		breakdown.Savings = 0
		breakdown.TierBreakdown = []dto.TierCost{{
			Range:     fmt.Sprintf("1-%d", quantity),
			Quantity:  quantity,
			UnitPrice: plan.PriceAmount,
			Subtotal:  total,
		}}
		return breakdown, nil
	}

	total, tiers := GraduatedCost(plan.PricingTiers, quantity)
	breakdown.TierBreakdown = tiers
	breakdown.TotalMonthlyCost = total
	breakdown.AveragePerUnit = perUnit(total, quantity)
	breakdown.Savings = plan.PriceAmount*int64(quantity) - total

	return breakdown, nil
}

// GetTotalCost is the same computation without the breakdown.
func (ps *pricingService) GetTotalCost(plan *models.SubscriptionPlan, quantity int) int64 {
	if !plan.UseTieredPricing || len(plan.PricingTiers) == 0 {
		return plan.PriceAmount * int64(quantity)
	}
	total, _ := GraduatedCost(plan.PricingTiers, quantity)
	return total
}

// PreviewProspectiveTiers prices an unsaved ladder at each requested quantity.
func (ps *pricingService) PreviewProspectiveTiers(input dto.ProspectivePricingInput) (*dto.ProspectivePricingOutput, error) {
	if len(input.Quantities) == 0 {
		return nil, fmt.Errorf("at least one quantity is required to price a ladder")
	}

	tiers := tiersFromDTO(input.Tiers)
	out := &dto.ProspectivePricingOutput{
		Currency: input.Currency,
		Points:   make([]dto.ProspectivePricingPoint, 0, len(input.Quantities)),
	}

	for _, qty := range input.Quantities {
		var total int64
		var breakdown []dto.TierCost

		if len(tiers) == 0 {
			// Untiered: the admin has cleared the brackets, so show the flat price
			// rather than zero.
			if qty > 0 {
				total = input.FlatAmount * int64(qty)
				breakdown = []dto.TierCost{{
					Range:     fmt.Sprintf("1-%d", qty),
					Quantity:  qty,
					UnitPrice: input.FlatAmount,
					Subtotal:  total,
				}}
			}
		} else {
			total, breakdown = GraduatedCost(tiers, qty)
		}

		out.Points = append(out.Points, dto.ProspectivePricingPoint{
			Quantity:      qty,
			Total:         total,
			PerUnit:       perUnit(total, qty),
			TierBreakdown: breakdown,
		})
	}

	return out, nil
}

// perUnit converts a total in cents to a per-unit price in currency units,
// guarding the zero-quantity division.
func perUnit(totalCents int64, quantity int) float64 {
	if quantity <= 0 {
		return 0
	}
	return float64(totalCents) / float64(quantity) / 100.0
}
