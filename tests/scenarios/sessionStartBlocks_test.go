// tests/scenarios/sessionStartBlocks_test.go
//
// Session start blocks (#579) on the scenario paths: a class block removes the
// class's assignments from what the learner may launch or sees as available,
// a class or organisation block makes a teacher's bulk start skip the learner,
// and a block refusing the start reaches the learner as 403
// session_start_blocked.
package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	testTools "soli/formations/tests/testTools"
)

// classWithAssignedScenario creates a class with userID as an active member and
// a private scenario assigned to that class only.
func classWithAssignedScenario(t *testing.T, db *gorm.DB, userID, name string) (groupID uuid.UUID, scenario models.Scenario) {
	t.Helper()
	scenario = models.Scenario{Name: name, Title: name, InstanceType: "M", OsType: "deb", CreatedByID: "teacher-" + name}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "content"}).Error)

	group := groupModels.ClassGroup{Name: name + "-class", DisplayName: name, OwnerUserID: "teacher-" + name, MaxMembers: 50}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: group.ID, UserID: userID, Role: "member", IsActive: true, JoinedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &group.ID, Scope: "group", CreatedByID: "teacher-" + name, IsActive: true,
	}).Error)
	return group.ID, scenario
}

func availableScenarioIDs(t *testing.T, db *gorm.DB, userID string) map[string]bool {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/available", nil)
	setupMemberRouter(db, userID).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	ids := map[string]bool{}
	for _, s := range response {
		if id, ok := s["id"].(string); ok {
			ids[id] = true
		}
	}
	return ids
}

func TestGetAvailableScenarios_ClassBlock_HidesThatClassAssignmentsOnly(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-available-" + uuid.NewString()
	blockedClass, blockedScenario := classWithAssignedScenario(t, db, userID, "blk-available-a")
	_, otherScenario := classWithAssignedScenario(t, db, userID, "blk-available-b")
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "group", &blockedClass, "late"))

	ids := availableScenarioIDs(t, db, userID)

	assert.False(t, ids[blockedScenario.ID.String()], "the blocked class's assignment no longer reaches the learner")
	assert.True(t, ids[otherScenario.ID.String()], "another class's assignment still does")
}

// setupMemberLaunchRouter wires the launch route with the production
// middleware chain and a plain member, so the assignment check runs.
func setupMemberLaunchRouter(t *testing.T, db *gorm.DB, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	ctrl := scenarioController.NewScenarioLaunchController(db)
	router.POST("/api/v1/scenario-sessions/launch",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(paymentServices.NewEffectivePlanService(db)),
		paymentMiddleware.RequirePlan(),
		ctrl.LaunchScenario,
	)
	return router
}

func TestLaunchScenario_ClassBlock_NoAccessThroughThatClass(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-launch-class-" + uuid.NewString()
	seedActivePlan(t, db, userID)
	classID, scenario := classWithAssignedScenario(t, db, userID, "blk-launch-class")
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "group", &classID, "late"))

	w := postLaunch(setupMemberLaunchRouter(t, db, userID), scenario.ID.String())

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "No access to this scenario",
		"the class's assignment no longer gives access to its scenario")
}

func TestLaunchScenario_ClassBlockElsewhere_AccessKept(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-launch-other-" + uuid.NewString()
	seedActivePlan(t, db, userID)
	_, scenario := classWithAssignedScenario(t, db, userID, "blk-launch-other")
	otherClass := uuid.New()
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "group", &otherClass, "late"))

	w := postLaunch(setupMemberLaunchRouter(t, db, userID), scenario.ID.String())

	assert.NotContains(t, w.Body.String(), "No access to this scenario",
		"a block in another class says nothing about this one")
}

func TestLaunchScenario_GlobalBlock_403SessionStartBlocked(t *testing.T) {
	db := freshTestDB(t)
	userID := "blk-launch-global-" + uuid.NewString()
	seedActivePlan(t, db, userID)
	seedPersistenceUserKey(t, db, userID)
	scenario := &models.Scenario{
		Name: "blk-launch-global", Title: "Blocked launch",
		InstanceType: "M", OsType: "deb", IsPublic: true, CreatedByID: userID,
	}
	require.NoError(t, db.Create(scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "content"}).Error)
	require.NoError(t, testTools.SeedSessionBlock(db, userID, "global", nil, "fraud"))

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)

	w := postLaunch(setupLaunchRouterWithProdMiddleware(t, db, userID), scenario.ID.String())

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "session_start_blocked", payload["error_message"])
	assert.Equal(t, "global", payload["scope"])
	assert.Equal(t, "fraud", payload["reason"])
	var runs int64
	db.Model(&models.ScenarioSession{}).Where("user_id = ?", userID).Count(&runs)
	assert.Zero(t, runs, "no run is left behind by a refused launch")
}

// ---------------------------------------------------------------------------
// Teacher bulk start — skip and report blocked learners
// ---------------------------------------------------------------------------

func bulkStartWithBlock(t *testing.T, name, blockScope string) (*services.BulkStartResult, *capturingTTService) {
	t.Helper()
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, name, map[string][]string{
		"member": {name + "-ok", name + "-ko"},
	})
	var group groupModels.ClassGroup
	require.NoError(t, sharedTestDB.First(&group, "id = ?", groupID).Error)
	scopeID := &groupID
	switch blockScope {
	case "org":
		require.NotNil(t, group.OrganizationID)
		scopeID = group.OrganizationID
	case "global":
		scopeID = nil
	}
	require.NoError(t, testTools.SeedSessionBlock(sharedTestDB, name+"-ko", blockScope, scopeID, "late"))

	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{
		Distribution: "debian", Size: "XS",
	}, 0, "trainer")
	require.NoError(t, err)
	return result, ttMock
}

func assertBlockedLearnerSkipped(t *testing.T, name string, result *services.BulkStartResult, ttMock *capturingTTService) {
	t.Helper()
	assert.Equal(t, 1, result.Created, "the other learner still starts")
	assert.Contains(t, result.SkippedUsers, services.SkippedLearner{UserID: name + "-ko", Reason: "session_start_blocked"},
		"the blocked learner is reported with the reason")
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, []string{name + "-ok"}, ttMock.capturedUserIDs, "no container for the blocked learner")
}

func TestBulkStartScenario_ClassBlockedLearner_SkippedWithReason(t *testing.T) {
	result, ttMock := bulkStartWithBlock(t, "blk-bulk-class", "group")
	assertBlockedLearnerSkipped(t, "blk-bulk-class", result, ttMock)
}

func TestBulkStartScenario_OrgBlockedLearner_SkippedWithReason(t *testing.T) {
	result, ttMock := bulkStartWithBlock(t, "blk-bulk-org", "org")
	assertBlockedLearnerSkipped(t, "blk-bulk-org", result, ttMock)
}

func TestBulkStartScenario_GloballyBlockedLearner_SkippedWithReason(t *testing.T) {
	result, ttMock := bulkStartWithBlock(t, "blk-bulk-global", "global")
	assertBlockedLearnerSkipped(t, "blk-bulk-global", result, ttMock)
}
