package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The listed-by-default rule lives in the create converter (DtoToModel), not
// in a column default: see TestSubscriptionPlanCreate_UnstatedIsCatalog_DefaultsToTrue.
// A plan written straight through GORM states is_catalog itself (#447).

func TestSubscriptionPlan_IsCatalog_SetToFalseViaUpdate(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:            "Custom Client Plan",
		Description:     "Unlisted plan for a specific client",
		PriceAmount:     4999,
		Currency:        "eur",
		BillingInterval: "month",
		IsCatalog:       true,
	}

	err := db.Create(plan).Error
	require.NoError(t, err)

	// Now set to unlisted
	err = db.Model(plan).Update("is_catalog", false).Error
	require.NoError(t, err)

	var fetched models.SubscriptionPlan
	err = db.First(&fetched, "id = ?", plan.ID).Error
	require.NoError(t, err)

	assert.False(t, fetched.IsCatalog, "IsCatalog should be false after update")
}

func TestSubscriptionPlan_IsCatalog_UpdateFromTrueToFalse(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:            "Catalog Plan To Unlist",
		Description:     "Will be changed to unlisted",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		IsCatalog:       true,
	}

	err := db.Create(plan).Error
	require.NoError(t, err)

	// Update is_catalog to false
	err = db.Model(plan).Update("is_catalog", false).Error
	require.NoError(t, err)

	var fetched models.SubscriptionPlan
	err = db.First(&fetched, "id = ?", plan.ID).Error
	require.NoError(t, err)

	assert.False(t, fetched.IsCatalog, "IsCatalog should be updated to false")
}
