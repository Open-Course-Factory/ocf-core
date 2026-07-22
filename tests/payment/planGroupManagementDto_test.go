// tests/payment/planGroupManagementDto_test.go
//
// RED tests for the new typed entitlement field
// SubscriptionPlan.GroupManagementEnabled — it must round-trip through the
// create converter (write path) and the generic GET output (read path), exactly
// like the DefaultBackend / AllowedBackends / SessionSupervisionEnabled fields.
//
// The reflection sweep in subscriptionPlanConverterRoundTrip_test.go
// automatically flags the output side once the model field exists (the field
// arrives zero because ModelToDto does not copy it yet). These explicit tests
// make the create-converter loss and the output loss legible on their own.
//
// Reuses getSubscriptionPlanOps / registerSubscriptionPlanForScoping /
// subscriptionPlanScopingRouter / getPlanByIdAsAdmin from the same package.
package payment_tests

import (
	"encoding/json"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// (a) write path: the create converter must copy GroupManagementEnabled.
func TestSubscriptionPlanConverter_DtoToModel_PreservesGroupManagementEnabled(t *testing.T) {
	ops := getSubscriptionPlanOps(t)

	in := dto.CreateSubscriptionPlanInput{
		Name:                   "Create Group Mgmt Plan",
		PriceAmount:            1000,
		Currency:               "eur",
		BillingInterval:        "month",
		GroupManagementEnabled: true,
	}
	out, err := ops.ConvertDtoToModel(in)
	require.NoError(t, err)
	model, ok := out.(*models.SubscriptionPlan)
	require.True(t, ok, "converter must return *SubscriptionPlan, got %T", out)

	assert.True(t, model.GroupManagementEnabled,
		"DtoToModel must copy GroupManagementEnabled from the create DTO so the entitlement is settable through the API")
}

// (a) read path: GroupManagementEnabled set in the DB must reach the API output.
func TestSubscriptionPlanOutput_GroupManagementEnabled_RoundTrips(t *testing.T) {
	registerSubscriptionPlanForScoping(t)
	db := freshTestDB(t)

	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Group Mgmt Plan",
		PriceAmount:            1000,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		IsCatalog:              true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(plan).Error)

	r := subscriptionPlanScopingRouter(db, "admin-1", []string{"administrator"})
	out := getPlanByIdAsAdmin(t, r, plan.ID.String())

	raw, present := out["group_management_enabled"]
	require.True(t, present,
		"group_management_enabled must be part of the plan output contract")
	var got bool
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.True(t, got,
		"GroupManagementEnabled=true in the DB must be observable in the API output")
}
