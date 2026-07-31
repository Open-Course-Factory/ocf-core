package services

import (
	"fmt"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// Violation codes. Machine-readable so the admin UI can translate them; the
// Detail string on each violation is an English fallback, not a display string.
const (
	// ViolationWeekNotCheaper: a full working week costs at least as much as a
	// month, so the day pack has no reason to exist at that seat count.
	ViolationWeekNotCheaper = "week_not_cheaper"
	// ViolationSeatAboveIndividual: a learner seat costs more than the individual
	// plan, so the trainer would be better off telling learners to buy it directly.
	ViolationSeatAboveIndividual = "seat_above_individual"
	// ViolationDeadFirstBracket: every order in the checked range fits inside the
	// first bracket, so the degression below it is unreachable.
	ViolationDeadFirstBracket = "dead_first_bracket"
)

const (
	defaultWorkingWeekDays = 5
	defaultMaxDaysToProbe  = 31
)

// SeatPricingChecker validates the seat offer's cross-plan invariants.
//
// These cannot be checked by either plan alone. The crossover in particular is
// a relationship: monthly per-seat price falls with volume, so a flat pack rate
// yields a different crossover day at every seat count — the required rate
// intervals are disjoint. Holding it steady means the pack needs its own
// degression tracking the monthly one, which is invisible to an admin editing
// one ladder in isolation.
type SeatPricingChecker interface {
	Check(in dto.SeatPricingCheckInput) (*dto.SeatPricingCheckOutput, error)
}

type seatPricingChecker struct{}

func NewSeatPricingChecker() SeatPricingChecker { return &seatPricingChecker{} }

// ladderCost prices a quantity under a ladder, falling back to the flat rate
// when no brackets are defined.
func ladderCost(tiers []models.PricingTier, flat int64, qty int) (int64, []dto.TierCost) {
	if qty <= 0 {
		return 0, nil
	}
	if len(tiers) == 0 {
		return flat * int64(qty), nil
	}
	return GraduatedCost(tiers, qty)
}

func (c *seatPricingChecker) Check(in dto.SeatPricingCheckInput) (*dto.SeatPricingCheckOutput, error) {
	if len(in.SeatCounts) == 0 {
		return nil, fmt.Errorf("at least one seat count is required to check the seat pricing")
	}

	week := in.WorkingWeekDays
	if week <= 0 {
		week = defaultWorkingWeekDays
	}
	maxDays := in.MaxDaysToProbe
	if maxDays <= 0 {
		maxDays = defaultMaxDaysToProbe
	}

	monthly := tiersFromDTO(in.MonthlyTiers)
	pack := tiersFromDTO(in.PackTiers)

	out := &dto.SeatPricingCheckOutput{
		Points:     make([]dto.SeatPricingCheckPoint, 0, len(in.SeatCounts)),
		Violations: []dto.SeatPricingViolation{},
	}

	everyOrderInFirstBracket := true

	for _, seats := range in.SeatCounts {
		monthTotal, monthBreakdown := ladderCost(monthly, in.MonthlyFlat, seats)
		weekTotal, _ := ladderCost(pack, in.PackFlat, seats*week)

		if len(monthBreakdown) > 1 {
			everyOrderInFirstBracket = false
		}

		out.Points = append(out.Points, dto.SeatPricingCheckPoint{
			Seats:          seats,
			MonthlyTotal:   monthTotal,
			MonthlyPerSeat: perUnit(monthTotal, seats),
			WeekTotal:      weekTotal,
			WeekPerSeat:    perUnit(weekTotal, seats),
			CrossoverDay:   crossoverDay(pack, in.PackFlat, seats, monthTotal, maxDays),
		})

		if weekTotal >= monthTotal {
			out.Violations = append(out.Violations, dto.SeatPricingViolation{
				Code:  ViolationWeekNotCheaper,
				Seats: seats,
				Detail: fmt.Sprintf(
					"at %d seats a %d-day week costs %d cents but a month costs %d — the day pack is never worth buying",
					seats, week, weekTotal, monthTotal),
			})
		}

		if in.IndividualPlanAmount > 0 && seats > 0 {
			perSeatCents := monthTotal / int64(seats)
			if perSeatCents >= in.IndividualPlanAmount {
				out.Violations = append(out.Violations, dto.SeatPricingViolation{
					Code:  ViolationSeatAboveIndividual,
					Seats: seats,
					Detail: fmt.Sprintf(
						"at %d seats a seat costs %d cents per month, at or above the individual plan at %d",
						seats, perSeatCents, in.IndividualPlanAmount),
				})
			}
		}
	}

	// A single-bracket ladder is flat on purpose, so there is no unreachable
	// degression to report — only a multi-bracket ladder can have a dead one.
	if len(monthly) > 1 && everyOrderInFirstBracket {
		out.Violations = append(out.Violations, dto.SeatPricingViolation{
			Code: ViolationDeadFirstBracket,
			Detail: fmt.Sprintf(
				"every checked order fits inside the first bracket (up to %d), so no customer in range reaches the degression below it",
				monthly[0].MaxQuantity),
		})
	}

	out.OK = len(out.Violations) == 0
	return out, nil
}

// crossoverDay returns the first day count at which the month costs no more than
// the pack, or 0 if the pack stays cheaper throughout the probed range.
func crossoverDay(pack []models.PricingTier, packFlat int64, seats int, monthTotal int64, maxDays int) int {
	for d := 1; d <= maxDays; d++ {
		total, _ := ladderCost(pack, packFlat, seats*d)
		if total >= monthTotal {
			return d
		}
	}
	return 0
}
