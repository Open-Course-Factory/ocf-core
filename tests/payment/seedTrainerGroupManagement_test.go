// tests/payment/seedTrainerGroupManagement_test.go
//
// Pins the dev/test seed belt: SetupDefaultSubscriptionPlans must seed the
// Trainer plan with the TYPED GroupManagementEnabled entitlement, so a first-run
// dev/test bulk purchase is gated correctly without waiting for the startup
// backfill (which runs BEFORE this seed in InitDevelopmentData).
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupDefaultSubscriptionPlans_TrainerPlanHasGroupManagementEntitlement(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)

	var trainer models.SubscriptionPlan
	require.NoError(t, db.Where("name = ?", "Trainer Plan").First(&trainer).Error,
		"SetupDefaultSubscriptionPlans must seed the Trainer plan")
	assert.True(t, trainer.GroupManagementEnabled,
		"the seeded Trainer plan must carry the typed GroupManagementEnabled entitlement so bulk purchase is gated on first run")
}
