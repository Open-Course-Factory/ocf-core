package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The card of a lab that would run in the learner's browser carries the
// machine's memory, so the page can warn, or refuse a computer too small,
// before launch. A lab that would run in a container carries none.
func TestGetAvailableScenarios_CardCarriesTheLocalMachineMemory(t *testing.T) {
	cards := func(t *testing.T, db *gorm.DB, userID string) []map[string]any {
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
