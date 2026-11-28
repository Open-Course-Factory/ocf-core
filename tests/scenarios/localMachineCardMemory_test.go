package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/payment/catalog"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cards is the scenario catalogue as userID sees it.
func cards(t *testing.T, db *gorm.DB, userID string) []map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", userID); c.Set("userRoles", []string{"admin"}); c.Next() })
	router.GET("/api/v1/scenario-sessions/available",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(paymentServices.NewEffectivePlanService(db)),
		scenarioController.NewScenarioLaunchController(db).GetAvailableScenarios)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/available", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

// The card of a lab that would run in the learner's browser carries the
// machine's memory, so the page can warn, or refuse a computer too small,
// before launch. A lab that would run in a container carries none.
func TestGetAvailableScenarios_CardCarriesTheLocalMachineMemory(t *testing.T) {
	t.Run("in the browser", func(t *testing.T) {
		db, userID, scenario, _ := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
		require.NoError(t, db.Model(scenario).Update("instance_type", "xl").Error)
		got := cards(t, db, userID)
		require.Len(t, got, 1)
		assert.Equal(t, float64(4096), got[0]["local_machine_memory_mib"])
	})

	t.Run("in a container", func(t *testing.T) {
		db, userID, scenario, _ := localLaunchFixture(t, false, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
		require.NoError(t, db.Model(scenario).Update("instance_type", "xl").Error)
		got := cards(t, db, userID)
		require.Len(t, got, 1)
		_, present := got[0]["local_machine_memory_mib"]
		assert.False(t, present)
	})
}

// A card judges a run in the learner's browser as the launch will: no
// container host is asked, the size is the local machine's, and what the
// launch would refuse shows as blocked, with the reason.
func TestGetAvailableScenarios_CardOfALocalRunIsJudgedAsLocal(t *testing.T) {
	t.Run("no container host needed", func(t *testing.T) {
		db, userID, scenario, _ := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
		require.NoError(t, db.Model(scenario).Update("instance_type", "m").Error)
		configureTTServerForPersistence(t, "http://127.0.0.1:1") // tt-backend down

		got := cards(t, db, userID)
		require.Len(t, got, 1)
		assert.Equal(t, true, got[0]["launchable"], "%v", got[0])
		assert.Equal(t, "m", got[0]["resolved_size"])
		assert.Nil(t, got[0]["block_reason"])
	})

	t.Run("refused locally", func(t *testing.T) {
		db, userID, scenario, _ := localLaunchFixture(t, true, paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal)
		require.NoError(t, db.Model(scenario).Update("crash_traps", true).Error)

		got := cards(t, db, userID)
		require.Len(t, got, 1)
		assert.Equal(t, false, got[0]["launchable"])
		assert.Equal(t, "needs_container", got[0]["block_reason"])
	})
}

// The machine a run gets in the browser is the size the budget charges it
// for (LocalProvisioning): memory and vCPUs follow that size, an unsized
// scenario included.
func TestLocalMachine_MemoryAndCPUsFollowTheChargedSize(t *testing.T) {
	for _, instanceType := range []string{"", "unknown", "XL", "xs"} {
		scenario := models.Scenario{InstanceType: instanceType}
		size, ok := catalog.LookupSize(services.LocalProvisioning(scenario).Size)
		require.True(t, ok, instanceType)
		assert.Equal(t, max(size.MemoryMB, 1024), services.LocalMachineMemoryMiB(scenario), instanceType)
		assert.Equal(t, max((size.CPU+999)/1000, 1), services.LocalMachineCPUs(scenario), instanceType)
	}
}
