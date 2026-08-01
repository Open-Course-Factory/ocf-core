package services

// #455: a learner-day pack was priced as learners × days and delivered as that
// many month-long, individually-assignable seats. These tests pin the three
// numbers apart — who it covers, what is charged, when it stops — because
// collapsing them into one "quantity" is what caused the defect.

import (
	"testing"
	"time"

	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func packPlan(unit string) *models.SubscriptionPlan {
	return &models.SubscriptionPlan{Name: "test-seat", SeatUnit: unit}
}

// The case from the issue: ten learners for three days.
func TestResolvePackTerms_LearnerDayChargesUnitsButDeliversLearners(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	terms, err := ResolvePackTerms(packPlan(models.SeatUnitLearnerDay), 10, 3, now)

	require.NoError(t, err)
	assert.Equal(t, 10, terms.Licences,
		"ten learners get ten licences — not thirty, which is what the pre-multiplied quantity produced")
	assert.Equal(t, 30, terms.BillingUnits,
		"the pack is priced per learner-day, so thirty units are charged")
	require.NotNil(t, terms.ExpiresAt)
	assert.Equal(t, now.AddDate(0, 0, 3), *terms.ExpiresAt,
		"a three-day pack stops after three days")
}

// A monthly seat is unchanged: one licence per seat, no deadline of its own.
func TestResolvePackTerms_SeatMonthIsOneUnitPerSeatAndNeverExpires(t *testing.T) {
	terms, err := ResolvePackTerms(packPlan(models.SeatUnitSeatMonth), 7, 0, time.Now())

	require.NoError(t, err)
	assert.Equal(t, 7, terms.Licences)
	assert.Equal(t, 7, terms.BillingUnits)
	assert.Nil(t, terms.ExpiresAt,
		"a monthly seat ends with its Stripe subscription, not with a pack deadline")
}

// An unset SeatUnit means seat_month, resolved through EffectiveSeatUnit.
func TestResolvePackTerms_UnsetSeatUnitBehavesAsSeatMonth(t *testing.T) {
	terms, err := ResolvePackTerms(packPlan(""), 4, 0, time.Now())

	require.NoError(t, err)
	assert.Equal(t, 4, terms.BillingUnits)
	assert.Nil(t, terms.ExpiresAt)
}

func TestResolvePackTerms_Refusals(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name      string
		unit      string
		learners  int
		days      int
		wantError string
	}{
		{
			name:      "a pack without a duration is refused rather than lasting forever",
			unit:      models.SeatUnitLearnerDay,
			learners:  10,
			days:      0,
			wantError: "duration in days is required",
		},
		{
			name:      "a monthly seat with a duration is refused rather than silently ignoring it",
			unit:      models.SeatUnitSeatMonth,
			learners:  10,
			days:      5,
			wantError: "takes no duration",
		},
		{
			name:      "a pack longer than the ceiling is refused",
			unit:      models.SeatUnitLearnerDay,
			learners:  1,
			days:      maxPackDurationDays + 1,
			wantError: "cannot run longer",
		},
		{
			name:      "zero learners buys nothing",
			unit:      models.SeatUnitLearnerDay,
			learners:  0,
			days:      3,
			wantError: "at least one learner",
		},
		{
			name:      "negative learners buys nothing",
			unit:      models.SeatUnitSeatMonth,
			learners:  -5,
			days:      0,
			wantError: "at least one learner",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolvePackTerms(packPlan(tc.unit), tc.learners, tc.days, now)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantError)
		})
	}
}

func TestResolvePackTerms_NilPlanIsRefused(t *testing.T) {
	_, err := ResolvePackTerms(nil, 1, 0, time.Now())
	assert.Error(t, err, "a nil plan must not resolve to a free unlimited pack")
}

// The pack must be worth less than the seats it replaces, which is only true if
// the licence count is the learner count. A one-day pack for one learner is one
// licence and one billing unit — the smallest sane purchase.
func TestResolvePackTerms_SmallestPackIsCoherent(t *testing.T) {
	terms, err := ResolvePackTerms(packPlan(models.SeatUnitLearnerDay), 1, 1, time.Now())

	require.NoError(t, err)
	assert.Equal(t, 1, terms.Licences)
	assert.Equal(t, 1, terms.BillingUnits)
	require.NotNil(t, terms.ExpiresAt)
}
