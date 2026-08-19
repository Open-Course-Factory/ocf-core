package services

// The consolidation in #454 fixed six dropped fields. This test exists so the
// seventh cannot happen: it walks the DTO by reflection rather than listing
// today's fields, so a field added to SubscriptionPlanOutput and forgotten in the
// converter fails here instead of silently reaching an API response as a zero
// value.
//
// That distinction matters because zero is not neutral on this DTO — MaxCPU and
// MaxMemoryMB of 0 mean "unlimited", so a dropped budget field reads as a
// permission rather than as missing data.

import (
	"reflect"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fullyPopulatedPlan returns a plan with every capability field set to a
// non-zero, non-default value, so that any field the converter fails to carry
// shows up as a zero value on the output.
func fullyPopulatedPlan() *models.SubscriptionPlan {
	productID := "prod_test"
	priceID := "price_test"
	return &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{
			ID: uuid.New(),
			Model: gorm.Model{
				CreatedAt: time.Now().Add(-time.Hour),
				UpdatedAt: time.Now(),
			},
		},
		Name:            "Formateur",
		Description:     "plan under test",
		Priority:        20,
		StripeProductID: &productID,
		StripePriceID:   &priceID,
		PriceAmount:     3900,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		IsCatalog:       true,
		RequiredRole:    "trainer",

		GroupManagementEnabled:    true,
		BulkPurchasable:           true,
		SeatUnit:                  models.SeatUnitLearnerDay,
		IsDefaultFree:             true,
		NetworkAccessEnabled:      true,
		DataPersistenceEnabled:    true,
		SessionSupervisionEnabled: true,

		MaxSessionDurationMinutes:   240,
		DataPersistenceGB:           50,
		CommandHistoryRetentionDays: 30,

		DefaultBackend:  "backend-a",
		AllowedBackends: []string{"backend-a", "backend-b"},

		MaxCPU:      16000,
		MaxMemoryMB: 16384,

		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 3900, Description: "small"},
		},
	}
}

func TestSubscriptionPlanToOutput_CarriesEveryDTOField(t *testing.T) {
	out := SubscriptionPlanToOutput(fullyPopulatedPlan())

	v := reflect.ValueOf(out)
	typ := v.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		value := v.Field(i)

		assert.Falsef(t, value.IsZero(),
			"SubscriptionPlanOutput.%s was left at its zero value by the converter. "+
				"Every field must be carried: a dropped field does not read as missing "+
				"data downstream, it reads as a default — and for the budget fields the "+
				"default means UNLIMITED.", field.Name)
	}
}

// The six fields that were actually dropped by the two removed producers, pinned
// by name. The reflection test above would catch them, but naming them keeps the
// regression legible to whoever reads this after the next drift.
func TestSubscriptionPlanToOutput_CarriesThePreviouslyDroppedFields(t *testing.T) {
	plan := fullyPopulatedPlan()
	out := SubscriptionPlanToOutput(plan)

	assert.True(t, out.GroupManagementEnabled, "group management must survive conversion")
	assert.True(t, out.BulkPurchasable, "bulk purchasability must survive conversion")
	assert.Equal(t, models.SeatUnitLearnerDay, out.SeatUnit, "seat unit must survive conversion")
	assert.True(t, out.SessionSupervisionEnabled, "session supervision must survive conversion")
	assert.Equal(t, 16000, out.MaxCPU, "a dropped MaxCPU would report the plan as unlimited")
	assert.Equal(t, 16384, out.MaxMemoryMB, "a dropped MaxMemoryMB would report the plan as unlimited")
	assert.True(t, out.IsCatalog, "catalog visibility must survive conversion")
}

func TestSubscriptionPlanToOutput_NilPlanYieldsZeroValue(t *testing.T) {
	out := SubscriptionPlanToOutput(nil)
	assert.Equal(t, uuid.Nil, out.ID, "a nil plan must convert to an empty DTO, not panic")
}

// A zero-cap plan is a real state meaning "unlimited", so it must round-trip as 0
// rather than being confused with an absent field.
func TestSubscriptionPlanToOutput_ZeroBudgetIsPreservedNotInvented(t *testing.T) {
	plan := fullyPopulatedPlan()
	plan.MaxCPU = 0
	plan.MaxMemoryMB = 0

	out := SubscriptionPlanToOutput(plan)

	require.Equal(t, 0, out.MaxCPU)
	require.Equal(t, 0, out.MaxMemoryMB)
}
