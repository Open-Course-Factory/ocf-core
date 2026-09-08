package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedPlan inserts one SubscriptionPlan and returns it. Every non-zero field of
// overrides is kept; ID, Currency and BillingInterval are filled when empty and
// the plan is always active.
//
// Zero-valued overrides: PriceAmount, Priority and every bool stay whatever the
// caller wrote, since nothing here fills them. The one flag a caller cannot
// clear through overrides is IsActive (gorm default:true — GORM omits a false
// bool on Create, so the DB default wins): deactivate afterwards with
// db.Model(plan).Update("is_active", false). IsCatalog carries no default tag,
// so false is written as-is.
func seedPlan(t testing.TB, db *gorm.DB, overrides models.SubscriptionPlan) *models.SubscriptionPlan {
	t.Helper()
	plan := overrides
	if plan.ID == uuid.Nil {
		plan.BaseModel = entityManagementModels.BaseModel{ID: uuid.New()}
	}
	if plan.Currency == "" {
		plan.Currency = "eur"
	}
	if plan.BillingInterval == "" {
		plan.BillingInterval = "month"
	}
	plan.IsActive = true
	require.NoError(t, db.Create(&plan).Error)
	return &plan
}
