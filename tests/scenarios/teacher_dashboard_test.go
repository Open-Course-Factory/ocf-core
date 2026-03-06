package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

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

	svc := services.NewTeacherDashboardService(db)
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

	svc := services.NewTeacherDashboardService(db)
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

	svc := services.NewTeacherDashboardService(db)
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

	svc := services.NewTeacherDashboardService(db)
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

	svc := services.NewTeacherDashboardService(db)
	results, err := svc.GetScenarioResults(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "student-1", results[0].UserID)
	assert.Equal(t, "completed", results[0].Status)
	require.NotNil(t, results[0].Grade)
	assert.InDelta(t, 100.0, *results[0].Grade, 0.01)
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

	svc := services.NewTeacherDashboardService(db)
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
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-2", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "bulk-start", Title: "Bulk Start", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	// Student 1 already has active session
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "active", StartedAt: time.Now(),
	}).Error)

	svc := services.NewTeacherDashboardService(db)
	result, err := svc.BulkStartScenario(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)  // only student-2
	assert.Equal(t, 1, result.Skipped)  // student-1 already active

	// Verify student-2 now has a session
	var count int64
	db.Model(&models.ScenarioSession{}).Where("user_id = ? AND scenario_id = ?", "student-2", scenario.ID).Count(&count)
	assert.Equal(t, int64(1), count)
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

	dashboardService := services.NewTeacherDashboardService(db)
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
	result, err := tc.dashboardService.BulkStartScenario(groupID, scenarioID)
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
	db := setupTestDB(t)
	router := setupTeacherRouter(db)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "student-2", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "api-bulk", Title: "API Bulk", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	// Student 1 already has active session
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "active", StartedAt: time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/teacher/groups/"+groupID.String()+"/scenarios/"+scenario.ID.String()+"/bulk-start", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(1), response["created"])
	assert.Equal(t, float64(1), response["skipped"])
}

// --- Real TeacherController tests (with access control) ---

func setupRealTeacherRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})

	ctrl := scenarioController.NewTeacherController(db)
	teacher := api.Group("/teacher")
	teacher.GET("/groups/:groupId/activity", ctrl.GetGroupActivity)
	teacher.GET("/groups/:groupId/scenarios/:scenarioId/results", ctrl.GetScenarioResults)
	teacher.GET("/groups/:groupId/scenarios/:scenarioId/analytics", ctrl.GetScenarioAnalytics)
	teacher.POST("/groups/:groupId/scenarios/:scenarioId/bulk-start", ctrl.BulkStartScenario)

	return r
}

func TestTeacherController_AccessDenied_NonTeacher(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "deny-group", DisplayName: "Deny Group", OwnerUserID: "teacher-owner", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	// Random student (not group owner/admin, not platform admin)
	router := setupRealTeacherRouter(db, "random-student", []string{"member"})

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

	// Platform admin can access any group
	router := setupRealTeacherRouter(db, "platform-admin", []string{"admin"})

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
	router := setupRealTeacherRouter(db, "teacher-own", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups/"+group.ID.String()+"/activity", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTeacherController_BulkStart_ViaController(t *testing.T) {
	db := setupTestDB(t)

	group := groupModels.ClassGroup{
		Name: "bulk-ctrl-group", DisplayName: "Bulk Controller", OwnerUserID: "teacher-b", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	for _, uid := range []string{"bs-student-1", "bs-student-2", "bs-student-3"} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: group.ID, UserID: uid, Role: groupModels.GroupMemberRoleMember,
			JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	scenario := models.Scenario{
		Name: "bulk-ctrl-test", Title: "Bulk Ctrl", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	// Pre-create an active session for student-1
	termID := "existing-t"
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "bs-student-1", TerminalSessionID: &termID,
		Status: "active", StartedAt: time.Now(),
	}).Error)

	router := setupRealTeacherRouter(db, "platform-admin", []string{"admin"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/teacher/groups/"+group.ID.String()+"/scenarios/"+scenario.ID.String()+"/bulk-start", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(2), response["started"])
	assert.Equal(t, float64(1), response["skipped"])
	assert.Equal(t, float64(0), response["failed"])

	// Verify 3 total sessions
	var count int64
	db.Model(&models.ScenarioSession{}).Where("scenario_id = ?", scenario.ID).Count(&count)
	assert.Equal(t, int64(3), count)
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
