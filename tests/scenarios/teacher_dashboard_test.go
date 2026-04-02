package scenarios_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

func init() {
	// Initialize Casdoor SDK with dummy values to prevent nil pointer panics
	// in fetchUserMap during tests. HTTP calls will fail gracefully.
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
}

// --- Grade calculation tests ---

func TestCalculateGrade_AllStepsCompleted(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "grade-test", Title: "Grade Test", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 2,
		Status: "completed", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// All 3 steps completed
	for i := 0; i < 3; i++ {
		now := time.Now()
		require.NoError(t, db.Create(&models.ScenarioStepProgress{
			SessionID: session.ID, StepOrder: i, Status: "completed", CompletedAt: &now,
		}).Error)
	}

	svc := services.NewTeacherDashboardService(db, nil, nil)
	grade := svc.CalculateGrade(session.ID)
	assert.InDelta(t, 100.0, grade, 0.01)
}

func TestCalculateGrade_PartialCompletion(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "grade-partial", Title: "Partial", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 4; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 1,
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// 1 of 4 steps completed
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed", CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	grade := svc.CalculateGrade(session.ID)
	assert.InDelta(t, 25.0, grade, 0.01) // 1/4 = 25%
}

func TestCalculateGrade_NoSteps(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "grade-empty", Title: "Empty", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1",
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	grade := svc.CalculateGrade(session.ID)
	assert.InDelta(t, 0.0, grade, 0.01)
}

// --- Service-level dashboard tests ---

func TestGetGroupActivity_Success(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-2", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "activity-test", Title: "Activity", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Student 1 has active session, student 2 has none
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "active", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	activity, err := svc.GetGroupActivity(groupID)
	require.NoError(t, err)
	assert.Len(t, activity, 1)
	assert.Equal(t, "student-1", activity[0].UserID)
}

func TestGetScenarioResults_Success(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "results-test", Title: "Results", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	completedAt := time.Now()
	grade := 100.0
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "completed",
		StartedAt: time.Now().Add(-time.Hour), CompletedAt: &completedAt, Grade: &grade,
	}
	require.NoError(t, db.Create(&session).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	paginated, err := svc.GetScenarioResults(groupID, scenario.ID, nil, nil)
	require.NoError(t, err)
	assert.Len(t, paginated.Items, 1)
	assert.Equal(t, int64(1), paginated.Total)
	assert.Equal(t, 0, paginated.Limit)
	assert.Equal(t, 0, paginated.Offset)
	assert.Equal(t, "student-1", paginated.Items[0].UserID)
	assert.Equal(t, "completed", paginated.Items[0].Status)
	require.NotNil(t, paginated.Items[0].Grade)
	assert.InDelta(t, 100.0, *paginated.Items[0].Grade, 0.01)
}

// --- Pagination tests ---

func TestGetScenarioResults_Paginated(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	// Create 5 students with sessions
	for i := 0; i < 5; i++ {
		uid := fmt.Sprintf("pag-student-%d", i)
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	scenario := models.Scenario{
		Name: "paginate-test", Title: "Paginate", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 5; i++ {
		uid := fmt.Sprintf("pag-student-%d", i)
		require.NoError(t, db.Create(&models.ScenarioSession{
			ScenarioID: scenario.ID, UserID: uid, Status: "completed",
			StartedAt: time.Now().Add(-time.Duration(5-i) * time.Hour),
		}).Error)
	}

	svc := services.NewTeacherDashboardService(db, nil, nil)

	// Page 1: limit=2, offset=0 → should get 2 items, total=5
	limit := 2
	offset := 0
	page1, err := svc.GetScenarioResults(groupID, scenario.ID, &limit, &offset)
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	assert.Equal(t, int64(5), page1.Total)
	assert.Equal(t, 2, page1.Limit)
	assert.Equal(t, 0, page1.Offset)

	// Page 2: limit=2, offset=2 → should get 2 items, total=5
	offset = 2
	page2, err := svc.GetScenarioResults(groupID, scenario.ID, &limit, &offset)
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.Equal(t, int64(5), page2.Total)
	assert.Equal(t, 2, page2.Limit)
	assert.Equal(t, 2, page2.Offset)

	// Page 3: limit=2, offset=4 → should get 1 item, total=5
	offset = 4
	page3, err := svc.GetScenarioResults(groupID, scenario.ID, &limit, &offset)
	require.NoError(t, err)
	assert.Len(t, page3.Items, 1)
	assert.Equal(t, int64(5), page3.Total)

	// No pagination (nil params) → all 5 results
	all, err := svc.GetScenarioResults(groupID, scenario.ID, nil, nil)
	require.NoError(t, err)
	assert.Len(t, all.Items, 5)
	assert.Equal(t, int64(5), all.Total)
	assert.Equal(t, 0, all.Limit)
	assert.Equal(t, 0, all.Offset)
}

func TestGetScenarioResults_Paginated_HTTPController(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "pag-http-group", DisplayName: "Pag HTTP", OwnerUserID: "teacher-pag", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: group.ID, UserID: "teacher-pag", Role: groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Create 3 students with sessions
	for i := 0; i < 3; i++ {
		uid := fmt.Sprintf("pag-http-student-%d", i)
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: group.ID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	scenario := models.Scenario{
		Name: "pag-http-test", Title: "Pag HTTP", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 3; i++ {
		uid := fmt.Sprintf("pag-http-student-%d", i)
		require.NoError(t, db.Create(&models.ScenarioSession{
			ScenarioID: scenario.ID, UserID: uid, Status: "completed",
			StartedAt: time.Now().Add(-time.Duration(3-i) * time.Hour),
		}).Error)
	}

	router := setupRealTeacherRouter(t, db, "teacher-pag", []string{"member"})

	// With pagination: limit=2&offset=0
	w := httptest.NewRecorder()
	url := fmt.Sprintf("/api/v1/teacher/groups/%s/scenarios/%s/results?limit=2&offset=0", group.ID.String(), scenario.ID.String())
	req, _ := http.NewRequest("GET", url, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var paginated map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &paginated))
	items := paginated["items"].([]any)
	assert.Len(t, items, 2)
	assert.Equal(t, float64(3), paginated["total"])
	assert.Equal(t, float64(2), paginated["limit"])
	assert.Equal(t, float64(0), paginated["offset"])

	// Without pagination: should return all items in paginated envelope
	w2 := httptest.NewRecorder()
	url2 := fmt.Sprintf("/api/v1/teacher/groups/%s/scenarios/%s/results", group.ID.String(), scenario.ID.String())
	req2, _ := http.NewRequest("GET", url2, nil)
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var allResults map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &allResults))
	allItems := allResults["items"].([]any)
	assert.Len(t, allItems, 3)
	assert.Equal(t, float64(3), allResults["total"])
}

func TestGetScenarioAnalytics_Success(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-2", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "analytics-test", Title: "Analytics", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Student 1 completed with grade 100
	completedAt := time.Now()
	grade1 := 100.0
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "completed",
		StartedAt: time.Now().Add(-time.Hour), CompletedAt: &completedAt, Grade: &grade1,
	}).Error)

	// Student 2 still active (no grade)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-2", Status: "active", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	analytics, err := svc.GetScenarioAnalytics(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), analytics.TotalSessions)
	assert.Equal(t, int64(1), analytics.CompletedCount)
	assert.InDelta(t, 50.0, analytics.CompletionRate, 0.01) // 1/2
	require.NotNil(t, analytics.AvgGrade)
	assert.InDelta(t, 100.0, *analytics.AvgGrade, 0.01) // only completed sessions count for grade avg
}

// --- Bulk start tests ---

func TestBulkStartScenario_Success(t *testing.T) {
	t.Skip("BulkStartScenario now requires a non-nil TerminalTrainerService mock; needs dedicated mock implementation")
}

// --- Teacher Controller HTTP tests ---

func setupTeacherRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "teacher-1")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})

	dashboardService := services.NewTeacherDashboardService(db, nil, nil)
	controller := &testTeacherController{dashboardService: dashboardService}

	teacher := api.Group("/teacher")
	teacher.GET("/groups/:groupId/activity", controller.GetGroupActivity)
	teacher.POST("/groups/:groupId/scenarios/:scenarioId/bulk-start", controller.BulkStart)

	return r
}

type testTeacherController struct {
	dashboardService *services.TeacherDashboardService
}

func (tc *testTeacherController) GetGroupActivity(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}
	activity, err := tc.dashboardService.GetGroupActivity(groupID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, activity)
}

func (tc *testTeacherController) BulkStart(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}
	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scenario ID"})
		return
	}
	result, err := tc.dashboardService.BulkStartScenario(groupID, scenarioID, "", "", 0, "")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, result)
}

func TestTeacherAPI_GetGroupActivity(t *testing.T) {
	db := setupTestDB(t)
	router := setupTeacherRouter(db)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "api-activity", Title: "API Activity", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "active", StartedAt: time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+groupID.String()+"/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Len(t, response, 1)
}

func TestTeacherAPI_BulkStart(t *testing.T) {
	t.Skip("BulkStartScenario now requires a non-nil TerminalTrainerService mock; needs dedicated mock implementation")
}

// --- Real TeacherController tests (with access control) ---

func setupRealTeacherRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	// Reset and re-register the RouteRegistry + enforcers for test isolation
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register scenario permissions (populates RouteRegistry with teacher dashboard rules)
	mockEnforcer := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mockEnforcer)

	// Register builtin enforcers with a real DB membership checker
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})

	// Add Layer 2 enforcement middleware
	api.Use(access.Layer2Enforcement())

	ctrl := scenarioController.NewTeacherController(db)
	teacher := api.Group("/teacher")
	teacher.GET("/groups/:groupId/activity", ctrl.GetGroupActivity)
	teacher.GET("/groups/:groupId/scenarios/:scenarioId/results", ctrl.GetScenarioResults)
	teacher.GET("/groups/:groupId/scenarios/:scenarioId/analytics", ctrl.GetScenarioAnalytics)
	teacher.POST("/groups/:groupId/scenarios/:scenarioId/bulk-start", ctrl.BulkStartScenario)
	teacher.POST("/groups/:groupId/scenarios/:scenarioId/reset-sessions", ctrl.ResetGroupScenarioSessions)

	return r
}

func TestTeacherController_AccessDenied_NonTeacher(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "deny-group", DisplayName: "Deny Group", OwnerUserID: "teacher-owner", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	// Random student (not group owner/admin, not platform admin)
	router := setupRealTeacherRouter(t, db, "random-student", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTeacherController_PlatformAdminAccess(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "admin-group", DisplayName: "Admin Group", OwnerUserID: "owner-1", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	// Platform admin bypasses Layer 2 group role check (admin bypass in GroupRole enforcer).
	router := setupRealTeacherRouter(t, db, "platform-admin", []string{"admin"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherController_GroupOwnerAccess(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "owner-group", DisplayName: "Owner Group", OwnerUserID: "teacher-own", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID: group.ID, UserID: "teacher-own", Role: groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Group owner with non-admin platform role can access their own group
	router := setupRealTeacherRouter(t, db, "teacher-own", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherController_BulkStart_ViaController(t *testing.T) {
	t.Skip("BulkStartScenario now requires terminal service with API connectivity; needs integration test environment")
}

// --- Grade via VerifyCurrentStep integration test ---

func TestGradeCalculation_ViaVerifyCurrentStep(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "grade-verify", Title: "Grade Verify", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", VerifyScript: "#!/bin/bash\ntrue",
		}).Error)
	}

	terminalID := "term-grade-verify"
	verifySvc := &mockVerificationService{passed: true, output: "OK"}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-gv", scenario.ID, terminalID)
	require.NoError(t, err)

	// Complete all 3 steps
	for i := 0; i < 3; i++ {
		result, err := sessionSvc.VerifyCurrentStep(session.ID)
		require.NoError(t, err)
		assert.True(t, result.Passed)
	}

	// Verify grade was computed
	var updated models.ScenarioSession
	db.First(&updated, "id = ?", session.ID)
	assert.Equal(t, "completed", updated.Status)
	require.NotNil(t, updated.Grade)
	assert.InDelta(t, 100.0, *updated.Grade, 0.01)
}

// --- ResetGroupScenarioSessions service tests ---

func TestResetGroupScenarioSessions_ResetsActiveSessions(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	for _, uid := range []string{"reset-s1", "reset-s2"} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	scenario := models.Scenario{
		Name: "reset-test", Title: "Reset Test", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create active sessions for both students
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-s1", Status: "active", StartedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-s2", Status: "in_progress", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	count, err := svc.ResetGroupScenarioSessions(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify both sessions are now abandoned
	var sessions []models.ScenarioSession
	db.Where("scenario_id = ?", scenario.ID).Find(&sessions)
	for _, s := range sessions {
		assert.Equal(t, "abandoned", s.Status)
	}
}

func TestResetGroupScenarioSessions_SkipsCompletedAndAbandoned(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "reset-skip-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "reset-skip", Title: "Reset Skip", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	completedAt := time.Now()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-skip-s1", Status: "completed",
		StartedAt: time.Now().Add(-time.Hour), CompletedAt: &completedAt,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-skip-s1", Status: "abandoned",
		StartedAt: time.Now().Add(-2 * time.Hour),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	count, err := svc.ResetGroupScenarioSessions(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify statuses unchanged
	var sessions []models.ScenarioSession
	db.Where("scenario_id = ?", scenario.ID).Order("started_at ASC").Find(&sessions)
	require.Len(t, sessions, 2)
	assert.Equal(t, "abandoned", sessions[0].Status)
	assert.Equal(t, "completed", sessions[1].Status)
}

func TestResetGroupScenarioSessions_NoActiveSessions(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "reset-empty-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "reset-empty", Title: "Reset Empty", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	count, err := svc.ResetGroupScenarioSessions(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestResetGroupScenarioSessions_OnlyAffectsGroupMembers(t *testing.T) {
	db := setupTestDB(t)

	groupA := uuid.New()
	groupB := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupA, UserID: "reset-ga-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupB, UserID: "reset-gb-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "reset-group-iso", Title: "Reset Group Isolation", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Both have active sessions on the same scenario
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-ga-s1", Status: "active", StartedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "reset-gb-s1", Status: "active", StartedAt: time.Now(),
	}).Error)

	// Reset only group A
	svc := services.NewTeacherDashboardService(db, nil, nil)
	count, err := svc.ResetGroupScenarioSessions(groupA, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Group A session is abandoned
	var sessionA models.ScenarioSession
	db.Where("user_id = ? AND scenario_id = ?", "reset-ga-s1", scenario.ID).First(&sessionA)
	assert.Equal(t, "abandoned", sessionA.Status)

	// Group B session is still active
	var sessionB models.ScenarioSession
	db.Where("user_id = ? AND scenario_id = ?", "reset-gb-s1", scenario.ID).First(&sessionB)
	assert.Equal(t, "active", sessionB.Status)
}

// --- Soft-delete (deleted_at IS NULL) tests ---

func TestGetGroupActivity_ExcludesSoftDeletedSteps(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sd-activity-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sd-activity", Title: "Soft Delete Activity", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create 3 steps, then soft-delete one
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}
	// Soft-delete step with order=2
	var stepToDelete models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ? AND \"order\" = ?", scenario.ID, 2).First(&stepToDelete).Error)
	require.NoError(t, db.Delete(&stepToDelete).Error)

	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sd-activity-s1", Status: "active", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	activity, err := svc.GetGroupActivity(groupID)
	require.NoError(t, err)
	require.Len(t, activity, 1)
	// total_steps should be 2, not 3 (soft-deleted step excluded)
	assert.Equal(t, int64(2), activity[0].TotalSteps)
}

func TestGetScenarioResults_ExcludesSoftDeletedSteps(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sd-results-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sd-results", Title: "Soft Delete Results", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create 4 steps, then soft-delete one
	for i := 0; i < 4; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}
	var stepToDelete models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ? AND \"order\" = ?", scenario.ID, 3).First(&stepToDelete).Error)
	require.NoError(t, db.Delete(&stepToDelete).Error)

	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sd-results-s1", Status: "active", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	paginated, err := svc.GetScenarioResults(groupID, scenario.ID, nil, nil)
	require.NoError(t, err)
	require.Len(t, paginated.Items, 1)
	// total_steps should be 3, not 4
	assert.Equal(t, int64(3), paginated.Items[0].TotalSteps)
}

func TestGetSessionDetail_ExcludesSoftDeletedSteps(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sd-detail-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sd-detail", Title: "Soft Delete Detail", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create scenario assignment so the validation passes
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	// Create 3 steps
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sd-detail-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Create step progress for all 3 steps
	now := time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStepProgress{
			SessionID: session.ID, StepOrder: i, Status: "completed", CompletedAt: &now,
		}).Error)
	}

	// Soft-delete step with order=1
	var stepToDelete models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ? AND \"order\" = ?", scenario.ID, 1).First(&stepToDelete).Error)
	require.NoError(t, db.Delete(&stepToDelete).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	// Only 2 steps should appear (step 0 and step 2; step 1 was soft-deleted)
	assert.Len(t, detail.Steps, 2)
	assert.Equal(t, 0, detail.Steps[0].StepOrder)
	assert.Equal(t, 2, detail.Steps[1].StepOrder)
}

// --- Scenario assignment validation in GetSessionDetail ---

func TestGetSessionDetail_AssignedScenario_Success(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "assign-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "assign-test", Title: "Assign Test", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Assign the scenario to the group
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "assign-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, detail.SessionID)
	assert.Equal(t, scenario.ID, detail.ScenarioID)
}

func TestGetSessionDetail_UnassignedScenario_Error(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "unassign-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "unassign-test", Title: "Unassign Test", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// No ScenarioAssignment created — scenario is NOT assigned to the group

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "unassign-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	assert.Error(t, err)
	assert.Nil(t, detail)
	assert.Contains(t, err.Error(), "scenario is not assigned to this group")
}

// --- ResetGroupScenarioSessions controller tests ---

func TestTeacherController_ResetSessions_ReturnsCount(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "reset-ctrl-group", DisplayName: "Reset Controller", OwnerUserID: "teacher-reset", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	for _, uid := range []string{"rc-student-1", "rc-student-2"} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: group.ID, UserID: uid, Role: groupModels.GroupMemberRoleMember,
			JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	// Add the admin as group owner so Layer 2 GroupRole enforcement succeeds
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: group.ID, UserID: "teacher-reset", Role: groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "reset-ctrl-test", Title: "Reset Ctrl", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create active sessions
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "rc-student-1", Status: "active", StartedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "rc-student-2", Status: "active", StartedAt: time.Now(),
	}).Error)

	router := setupRealTeacherRouter(t, db, "teacher-reset", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/teacher/groups/"+group.ID.String()+"/scenarios/"+scenario.ID.String()+"/reset-sessions", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(2), response["abandoned"])
}

func TestTeacherController_ResetSessions_AccessDenied(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "reset-deny-group", DisplayName: "Reset Deny", OwnerUserID: "teacher-deny", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	scenario := models.Scenario{
		Name: "reset-deny-test", Title: "Reset Deny", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Random student (not group owner/admin, not platform admin)
	router := setupRealTeacherRouter(t, db, "random-student", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/teacher/groups/"+group.ID.String()+"/scenarios/"+scenario.ID.String()+"/reset-sessions", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
