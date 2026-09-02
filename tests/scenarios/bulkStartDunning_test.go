// tests/scenarios/bulkStartDunning_test.go
//
// Bulk-start was the last session-creation route without a dunning gate
// (scenarios-e2e-test-plan.md §7 review, decided 2026-08-07): terminals are
// provisioned for the whole class on the TEACHER's initiative, so the
// TEACHER's past_due subscription blocks the launch. Members are typically on
// assigned/org plans that carry no personal dunning stamp — gating them
// would be theater; the payer-side actor is the caller.
package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	scenarioRoutes "soli/formations/src/scenarios/routes"
)

func setupBulkStartRouter(t *testing.T, db *gorm.DB, trainerID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", trainerID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	ctrl := scenarioRoutes.NewTeacherController(db)
	router.POST("/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start",
		ctrl.BulkStartScenario)
	return router
}

func TestBulkStartScenario_402sWhenTrainerPastDueBeyondGrace(t *testing.T) {
	db := freshTestDB(t)
	trainerID := "bulk-dunning-trainer"
	seedPastDuePlan(t, db, trainerID)

	scenario := &models.Scenario{
		Name:         "bulk-dunning-test",
		Title:        "Bulk Dunning Test",
		InstanceType: "M",
		OsType:       "deb",
		IsPublic:     true,
		CreatedByID:  trainerID,
	}
	require.NoError(t, db.Create(scenario).Error)

	router := setupBulkStartRouter(t, db, trainerID)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/teacher/groups/"+uuid.NewString()+"/scenarios/"+scenario.ID.String()+"/bulk-start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusPaymentRequired, w.Code,
		"bulk-start provisions terminals for the whole class — the trainer's "+
			"past_due sub beyond grace must block it like every other "+
			"session-creation route. Got %d. Body: %s", w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "subscription_past_due")
}

func TestBulkStartScenario_ActiveTrainerPassesGate(t *testing.T) {
	db := freshTestDB(t)
	trainerID := "bulk-active-trainer"
	// Active (not past_due) plan: the gate must NOT reject.
	seedBudgetExhaustedUser(t, db, trainerID, "bulk-active-test")

	router := setupBulkStartRouter(t, db, trainerID)

	var scenario models.Scenario
	require.NoError(t, db.First(&scenario, "name = ?", "bulk-active-test").Error)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/teacher/groups/"+uuid.NewString()+"/scenarios/"+scenario.ID.String()+"/bulk-start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The gate is what is under test, and the gate answers 402. Anything else
	// means the trainer got through it.
	//
	// Past the gate the handler resolves how the scenario's container must be
	// built, and this test runs without a tt-backend, so the resolution cannot
	// read any catalog and the request ends in 503. Asserting 200 here would
	// only be asserting that the handler never asks what it is about to build,
	// which is precisely the omission that shipped containers with no network.
	assert.NotEqual(t, http.StatusPaymentRequired, w.Code,
		"an active trainer must pass the dunning gate. Got %d. Body: %s", w.Code, w.Body.String())
	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"an unreachable backend catalog is an outage, not a scenario nobody can "+
			"run. Got %d. Body: %s", w.Code, w.Body.String())
}
