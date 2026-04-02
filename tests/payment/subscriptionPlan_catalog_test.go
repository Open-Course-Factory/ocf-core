package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPlan_IsCatalog_DefaultsToTrue(t *testing.T) {
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		Name:            "Default Catalog Plan",
		Description:     "Plan without explicit is_catalog value",
		PriceAmount:     999,
		Currency:        "eur",
		BillingInterval: "month",
	}

	err := db.Create(plan).Error
	require.NoError(t, err)

	var fetched models.SubscriptionPlan
	err = db.First(&fetched, "id = ?", plan.ID).Error
	require.NoError(t, err)

	assert.True(t, fetched.IsCatalog, "IsCatalog should default to true when not specified")
}

func TestSubscriptionPlan_IsCatalog_SetToFalseViaUpdate(t *testing.T) {
	db := freshTestDB(t)

	// Create as catalog plan first (default true), then set to unlisted.
	// This matches the real-world workflow: plans are created via the admin UI
	// (which goes through DtoToModel), then can be toggled to unlisted.
	// Note: GORM's db.Create() skips zero-value bools when the field has
	// gorm:"default:true", so setting IsCatalog:false at creation time
	// requires db.Select("*").Create() — same limitation as IsActive.
	plan := &models.SubscriptionPlan{
		Name:            "Custom Client Plan",
		Description:     "Unlisted plan for a specific client",
		PriceAmount:     4999,
		Currency:        "eur",
		BillingInterval: "month",
	}

	err := db.Create(plan).Error
	require.NoError(t, err)
	assert.True(t, plan.IsCatalog, "IsCatalog should default to true on creation")

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
