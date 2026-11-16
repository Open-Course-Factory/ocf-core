// tests/scenarios/scenarioSessionGenericCreate_test.go
//
// A scenario run is created by the launch and bulk-start paths only: they run
// the access, archive and terminal checks. The generic entity route is
// read-only for members, and abandoning a run only releases a terminal owned
// by the run's own user.
package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioController "soli/formations/src/scenarios/routes"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

func TestScenarioSession_Layer1_MemberIsReadOnly(t *testing.T) {
	svc := ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenarioSession(svc)
	roles := svc.GetAllEntityRoles()["ScenarioSession"]

	member := roles.Roles[string(authModels.Member)]
	assert.Contains(t, member, http.MethodGet, "members still read their own runs")
	assert.NotContains(t, member, http.MethodPost,
		"a member must not create a run through the generic route: it skips every launch check")

	// The policy the registration writes, through ocf-core's own Casbin model.
	m, err := model.NewModelFromFile("../../src/configuration/keymatch_model.conf")
	require.NoError(t, err)
	e, err := casbin.NewEnforcer(m)
	require.NoError(t, err)
	recorder := mocks.NewMockEnforcer()
	recorder.AddPolicyFunc = func(params ...any) (bool, error) { return e.AddPolicy(params...) }
	svc.SetDefaultEntityAccesses("ScenarioSession", roles, recorder)

	allowed, err := e.Enforce(string(authModels.Member), "/api/v1/scenario-sessions", http.MethodPost)
	require.NoError(t, err)
	assert.False(t, allowed, "Casbin must refuse POST /scenario-sessions to a member")
	allowed, err = e.Enforce(string(authModels.Member), "/api/v1/scenario-sessions", http.MethodGet)
	require.NoError(t, err)
	assert.True(t, allowed, "control: GET stays allowed")
}

// A run pointing at someone else's terminal must not delete it on abandon.
func TestAbandonSession_TerminalOwnedByAnotherUser_IsNotDeleted(t *testing.T) {
	srv, rec := newPersistenceTTBackend(t)
	configureTTServerForPersistence(t, srv.URL)
	db := freshTestDB(t)
	scenario := seedSessionScenario(t, db)

	// The other user's terminal, and the caller's run bound to it.
	seedBulkRun(t, db, scenario.ID, "terminal-owner", "completed", "term-of-other-user", true)
	runID := seedBulkRun(t, db, scenario.ID, "run-owner", "active", "term-of-other-user", false)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "run-owner")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	pc := scenarioController.NewScenarioProgressControllerWithTerminalService(db, terminalServices.NewTerminalTrainerService(db))
	api.POST("/scenario-sessions/:id/abandon", pc.AbandonSession)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+runID.String()+"/abandon", nil))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, "abandoned", sessionStatus(t, db, runID), "control: the caller's own run is abandoned")

	rec.mu.Lock()
	deleted := append([]string(nil), rec.deleted...)
	rec.mu.Unlock()
	assert.Empty(t, deleted, "another user's terminal must not be deleted on tt-backend")
	assert.Equal(t, terminalModels.StateRunning, terminalState(t, db, "term-of-other-user"),
		"another user's terminal must stay running")
}
