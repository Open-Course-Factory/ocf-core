// tests/payment/seatPricingCoherence_test.go
//
// #446: the seat offer's invariants belong to the monthly plan and the day pack
// TOGETHER, so neither plan can validate itself.
//
// The one that matters most is the crossover. It is not automatic: monthly
// per-seat price falls with volume, so a flat pack rate produces a DIFFERENT
// crossover day at each seat count — the required rate intervals are literally
// disjoint (5 seats needs 1.50-1.80, 15 seats needs 0.93-1.87 against the agreed
// ladders). Holding the crossover steady means the pack needs its own degression
// tracking the monthly one, which is a relationship an admin editing one ladder
// cannot see.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agreedMonthly() []dto.PricingTier {
	return []dto.PricingTier{
		{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
		{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 700},
		{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 550},
	}
}

func agreedPack() []dto.PricingTier {
	return []dto.PricingTier{
		{MinQuantity: 1, MaxQuantity: 30, UnitAmount: 165},
		{MinQuantity: 31, MaxQuantity: 60, UnitAmount: 125},
		{MinQuantity: 61, MaxQuantity: 0, UnitAmount: 105},
	}
}

func agreedInput() dto.SeatPricingCheckInput {
	return dto.SeatPricingCheckInput{
		MonthlyTiers:         agreedMonthly(),
		PackTiers:            agreedPack(),
		IndividualPlanAmount: 1200, // Solo
		SeatCounts:           []int{1, 3, 5, 10, 15, 20, 25, 30},
		WorkingWeekDays:      5,
	}
}

// TestSeatPricingCoherence_AgreedLaddersPass is the regression guard on the
// offer itself: the ladders signed off in #442 satisfy every invariant, and the
// crossover lands on day 6 at every seat count.
func TestSeatPricingCoherence_AgreedLaddersPass(t *testing.T) {
	out, err := services.NewSeatPricingChecker().Check(agreedInput())
	require.NoError(t, err)

	assert.True(t, out.OK, "the agreed ladders must pass: %+v", out.Violations)
	assert.Empty(t, out.Violations)

	require.Len(t, out.Points, 8)
	for _, p := range out.Points {
		assert.Equal(t, 6, p.CrossoverDay,
			"a working week must stay cheaper at %d seats, tipping over on day 6", p.Seats)
		assert.Less(t, p.WeekTotal, p.MonthlyTotal,
			"the 5-day week must cost less than the month at %d seats", p.Seats)
	}

	// Spot-check the numbers the offer was agreed on.
	assert.Equal(t, int64(4500), out.Points[2].MonthlyTotal, "5 seats = 45.00/month")
	assert.InDelta(t, 9.00, out.Points[2].MonthlyPerSeat, 0.005)
	assert.Equal(t, int64(8000), out.Points[3].MonthlyTotal, "10 seats = 80.00/month")
	assert.InDelta(t, 8.00, out.Points[3].MonthlyPerSeat, 0.005)
}

// TestSeatPricingCoherence_DetectsBrokenCrossover is the failure this endpoint
// exists to catch: an admin edits the monthly ladder alone and a week silently
// stops being cheaper than a month.
func TestSeatPricingCoherence_DetectsBrokenCrossover(t *testing.T) {
	in := agreedInput()
	// Halve the monthly price without touching the pack.
	in.MonthlyTiers = []dto.PricingTier{{MinQuantity: 1, MaxQuantity: 0, UnitAmount: 450}}

	out, err := services.NewSeatPricingChecker().Check(in)
	require.NoError(t, err)

	assert.False(t, out.OK, "a month cheaper than a week must not pass")
	codes := violationCodes(out.Violations)
	assert.Contains(t, codes, services.ViolationWeekNotCheaper,
		"the broken crossover must be reported, got %v", codes)
}

// TestSeatPricingCoherence_DetectsSeatAboveIndividualPlan pins Tom's rule that a
// learner seat always undercuts the individual plan — otherwise a trainer would
// be better off telling learners to buy Solo themselves.
func TestSeatPricingCoherence_DetectsSeatAboveIndividualPlan(t *testing.T) {
	in := agreedInput()
	in.MonthlyTiers = []dto.PricingTier{{MinQuantity: 1, MaxQuantity: 0, UnitAmount: 1500}}

	out, err := services.NewSeatPricingChecker().Check(in)
	require.NoError(t, err)

	assert.False(t, out.OK)
	assert.Contains(t, violationCodes(out.Violations), services.ViolationSeatAboveIndividual,
		"a seat at 15.00 against Solo at 12.00 must be reported")
}

// TestSeatPricingCoherence_DetectsDeadFirstBracket catches the mistake we made
// ourselves while modelling: a first bracket wider than every order in range, so
// no customer ever experiences the degression that was carefully designed.
func TestSeatPricingCoherence_DetectsDeadFirstBracket(t *testing.T) {
	in := agreedInput()
	in.SeatCounts = []int{1, 3, 5, 10}
	// First bracket covers 1-50: nobody in range ever leaves it.
	in.MonthlyTiers = []dto.PricingTier{
		{MinQuantity: 1, MaxQuantity: 50, UnitAmount: 900},
		{MinQuantity: 51, MaxQuantity: 0, UnitAmount: 550},
	}

	out, err := services.NewSeatPricingChecker().Check(in)
	require.NoError(t, err)

	assert.Contains(t, violationCodes(out.Violations), services.ViolationDeadFirstBracket,
		"a ladder whose discount nobody in range can reach must be flagged")
}

// TestSeatPricingCoherence_SingleTierIsNotADeadBracket guards against a false
// positive: a deliberately flat ladder has no degression to be unreachable.
func TestSeatPricingCoherence_SingleTierIsNotADeadBracket(t *testing.T) {
	in := agreedInput()
	in.MonthlyTiers = []dto.PricingTier{{MinQuantity: 1, MaxQuantity: 0, UnitAmount: 900}}

	out, err := services.NewSeatPricingChecker().Check(in)
	require.NoError(t, err)

	assert.NotContains(t, violationCodes(out.Violations), services.ViolationDeadFirstBracket,
		"a single-bracket ladder is flat on purpose, not broken")
}

// TestSeatPricingCoherence_RejectsEmptySeatCounts — an empty request would return
// an empty report, which reads as "everything is fine".
func TestSeatPricingCoherence_RejectsEmptySeatCounts(t *testing.T) {
	in := agreedInput()
	in.SeatCounts = nil

	_, err := services.NewSeatPricingChecker().Check(in)
	assert.Error(t, err, "nothing to check must be an explicit error, not a silent pass")
}

func violationCodes(vs []dto.SeatPricingViolation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Code)
	}
	return out
}
