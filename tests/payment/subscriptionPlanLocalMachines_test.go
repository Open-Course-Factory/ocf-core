// tests/payment/subscriptionPlanLocalMachines_test.go
//
// A plan says whether its holders may run scenarios on a machine in their own
// browser tab: off (the default), allowed (scenarios set to Local), or the
// default for scenarios set to By account. The value is a closed set: a typo
// must be refused at the door, not read later as "off" or worse.
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityServices "soli/formations/src/entityManagement/services"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPlanValidation_LocalMachinesIsAClosedSet(t *testing.T) {
	for _, value := range []string{"", models.LocalMachinesOff, models.LocalMachinesAllowed, models.LocalMachinesByAccountDefault} {
		require.NoError(t, execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
			Name: "Plan", MaxCPU: 1000, MaxMemoryMB: 1024, LocalMachines: value,
		}), "local_machines %q must be accepted on create", value)
		require.NoError(t, execPlanValidation(hooks.BeforeUpdate, map[string]any{"local_machines": &value}),
			"local_machines %q must be accepted on update", value)
	}

	require.Error(t, execPlanValidation(hooks.BeforeCreate, &models.SubscriptionPlan{
		Name: "Plan", MaxCPU: 1000, MaxMemoryMB: 1024, LocalMachines: "yes",
	}), "an unknown local_machines value must be refused on create")
	unknown := "always"
	require.Error(t, execPlanValidation(hooks.BeforeUpdate, map[string]any{"local_machines": &unknown}),
		"an unknown local_machines value must be refused on update")
	require.NoError(t, execPlanValidation(hooks.BeforeUpdate, map[string]any{"name": "Renamed"}),
		"a PATCH without local_machines is a partial update")
}

func TestSubscriptionPlan_LocalMachinesDefaultsToOffAndIsCreated(t *testing.T) {
	_ = freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)

	svc := entityServices.NewGenericService(sharedTestDB, nil)
	for name, value := range map[string]string{"Local Plan": models.LocalMachinesAllowed, "Plain Plan": ""} {
		_, err := svc.CreateEntityWithUser(dto.CreateSubscriptionPlanInput{
			Name: name, PriceAmount: 1000, Currency: "eur", BillingInterval: "month",
			MaxCPU: 1000, MaxMemoryMB: 1024, LocalMachines: value,
		}, "SubscriptionPlan", "admin-1")
		require.NoError(t, err)
	}

	var local, plain models.SubscriptionPlan
	require.NoError(t, sharedTestDB.First(&local, "name = ?", "Local Plan").Error)
	require.NoError(t, sharedTestDB.First(&plain, "name = ?", "Plain Plan").Error)
	assert.Equal(t, models.LocalMachinesAllowed, local.LocalMachines)
	assert.Equal(t, models.LocalMachinesOff, plain.LocalMachines, "a plan that says nothing has local machines off")
}

func TestSubscriptionPlan_LocalMachinesPatchedEndToEnd(t *testing.T) {
	db := freshTestDB(t)
	registerSubscriptionPlanForScoping(t)
	withPaymentHooksRegistered(t)
	planID := seedEditablePlan(t, db)

	patch := func(body string) *httptest.ResponseRecorder {
		r := subscriptionPlanEditRouter(db, "admin-1", []string{"administrator"})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/subscription-plans/"+planID.String(), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := patch(`{"local_machines":"by_account_default"}`)
	require.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())
	var reloaded models.SubscriptionPlan
	require.NoError(t, db.First(&reloaded, "id = ?", planID).Error)
	assert.Equal(t, models.LocalMachinesByAccountDefault, reloaded.LocalMachines)

	w = patch(`{"local_machines":"everywhere"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	require.NoError(t, db.First(&reloaded, "id = ?", planID).Error)
	assert.Equal(t, models.LocalMachinesByAccountDefault, reloaded.LocalMachines, "a refused value must not be stored")
}
