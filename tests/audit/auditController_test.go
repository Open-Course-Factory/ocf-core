package audit_tests

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

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/audit/controllers"
	"soli/formations/src/audit/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupControllerTest(t *testing.T) (controllers.AuditController, services.AuditService) {
	t.Helper()
	db := freshTestDB(t)
	ctrl := controllers.NewAuditController(db)
	svc := services.NewAuditService(db)
	return ctrl, svc
}

// --- GetAuditLogs ---

func TestAuditController_GetAuditLogs_DefaultPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	// Seed some logs
	for i := 0; i < 3; i++ {
		err := svc.Log(auditModels.AuditLogCreate{
			EventType: auditModels.AuditEventLogin,
			Severity:  auditModels.AuditSeverityInfo,
			Action:    "login",
			Status:    "success",
		})
		require.NoError(t, err)
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, float64(3), resp["total"])
	assert.Equal(t, float64(50), resp["limit"])  // default limit
	assert.Equal(t, float64(0), resp["offset"])   // default offset

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 3)
}

func TestAuditController_GetAuditLogs_WithEventTypeFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "login",
		Status:    "success",
	})
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogout,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "logout",
		Status:    "success",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?event_type=auth.login", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}

func TestAuditController_GetAuditLogs_WithLimitAndOffset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	for i := 0; i < 5; i++ {
		svc.Log(auditModels.AuditLogCreate{
			EventType: auditModels.AuditEventLogin,
			Severity:  auditModels.AuditSeverityInfo,
			Action:    "login",
			Status:    "success",
		})
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?limit=2&offset=1", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(5), resp["total"])
	assert.Equal(t, float64(2), resp["limit"])
	assert.Equal(t, float64(1), resp["offset"])

	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestAuditController_GetAuditLogs_InvalidActorID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?actor_id=not-a-uuid", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error_message"], "Invalid actor_id")
}

func TestAuditController_GetAuditLogs_InvalidTargetID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?target_id=invalid", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditController_GetAuditLogs_InvalidOrganizationID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?organization_id=bad", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditController_GetAuditLogs_InvalidStartDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?start_date=not-a-date", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error_message"], "Invalid start_date")
}

func TestAuditController_GetAuditLogs_InvalidEndDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?end_date=2024-13-99", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditController_GetAuditLogs_InvalidLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	// Limit = 0 (invalid)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?limit=0", nil)

	ctrl.GetAuditLogs(ctx)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Limit > 1000 (invalid)
	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?limit=2000", nil)

	ctrl.GetAuditLogs(ctx)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Limit = not a number
	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?limit=abc", nil)

	ctrl.GetAuditLogs(ctx)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditController_GetAuditLogs_InvalidOffset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?offset=-1", nil)

	ctrl.GetAuditLogs(ctx)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditController_GetAuditLogs_ValidDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "login",
		Status:    "success",
	})

	// Use UTC to avoid timezone offset with '+' sign that gets mangled in URL query parsing
	start := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	end := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest("GET", "/audit/logs", nil)
	q := req.URL.Query()
	q.Set("start_date", start)
	q.Set("end_date", end)
	req.URL.RawQuery = q.Encode()
	ctx.Request = req

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Note: SQLite stores timestamps as text, so date range filtering may not match
	// entries exactly as PostgreSQL would. We verify the endpoint returns a valid
	// response and doesn't error — the date filtering logic is validated at the
	// service level with direct DB queries in TestAuditService_GetAuditLogs_FilterByDateRange.
	assert.Contains(t, resp, "total")
	assert.Contains(t, resp, "data")
}

// --- GetUserAuditLogs ---

func TestAuditController_GetUserAuditLogs_Valid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	userID := uuid.New()
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		ActorID:   &userID,
		Action:    "login",
		Status:    "success",
	})
	// Log from another user
	otherID := uuid.New()
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		ActorID:   &otherID,
		Action:    "login",
		Status:    "success",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/users/"+userID.String()+"/logs", nil)
	ctx.Params = gin.Params{{Key: "user_id", Value: userID.String()}}

	ctrl.GetUserAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}

func TestAuditController_GetUserAuditLogs_InvalidUserID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/users/not-a-uuid/logs", nil)
	ctx.Params = gin.Params{{Key: "user_id", Value: "not-a-uuid"}}

	ctrl.GetUserAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error_message"], "Invalid user_id")
}

func TestAuditController_GetUserAuditLogs_WithPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	userID := uuid.New()
	for i := 0; i < 5; i++ {
		svc.Log(auditModels.AuditLogCreate{
			EventType: auditModels.AuditEventLogin,
			Severity:  auditModels.AuditSeverityInfo,
			ActorID:   &userID,
			Action:    "login",
			Status:    "success",
		})
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/users/"+userID.String()+"/logs?limit=2&offset=1", nil)
	ctx.Params = gin.Params{{Key: "user_id", Value: userID.String()}}

	ctrl.GetUserAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(5), resp["total"])
	assert.Equal(t, float64(2), resp["limit"])
	assert.Equal(t, float64(1), resp["offset"])
}

func TestAuditController_GetUserAuditLogs_InvalidLimitFallsToDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	userID := uuid.New()
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/users/"+userID.String()+"/logs?limit=-1", nil)
	ctx.Params = gin.Params{{Key: "user_id", Value: userID.String()}}

	ctrl.GetUserAuditLogs(ctx)

	// The controller silently defaults to 50 for invalid limit in user logs
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(50), resp["limit"])
}

// --- GetOrganizationAuditLogs ---

func TestAuditController_GetOrganizationAuditLogs_Valid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	orgID := uuid.New()
	svc.Log(auditModels.AuditLogCreate{
		EventType:      auditModels.AuditEventMemberAdded,
		Severity:       auditModels.AuditSeverityInfo,
		OrganizationID: &orgID,
		Action:         "member added",
		Status:         "success",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/organizations/"+orgID.String()+"/logs", nil)
	ctx.Params = gin.Params{{Key: "organization_id", Value: orgID.String()}}

	ctrl.GetOrganizationAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}

func TestAuditController_GetOrganizationAuditLogs_InvalidOrgID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, _ := setupControllerTest(t)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/organizations/invalid/logs", nil)
	ctx.Params = gin.Params{{Key: "organization_id", Value: "invalid"}}

	ctrl.GetOrganizationAuditLogs(ctx)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Contains(t, resp["error_message"], "Invalid organization_id")
}

func TestAuditController_GetOrganizationAuditLogs_WithPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	orgID := uuid.New()
	for i := 0; i < 4; i++ {
		svc.Log(auditModels.AuditLogCreate{
			EventType:      auditModels.AuditEventMemberAdded,
			Severity:       auditModels.AuditSeverityInfo,
			OrganizationID: &orgID,
			Action:         "member added",
			Status:         "success",
		})
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/organizations/"+orgID.String()+"/logs?limit=2", nil)
	ctx.Params = gin.Params{{Key: "organization_id", Value: orgID.String()}}

	ctrl.GetOrganizationAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(4), resp["total"])
	assert.Equal(t, float64(2), resp["limit"])

	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestAuditController_GetAuditLogs_WithSeverityFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "login",
		Status:    "success",
	})
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventAccessDenied,
		Severity:  auditModels.AuditSeverityCritical,
		Action:    "access denied",
		Status:    "detected",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?severity=critical", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}

func TestAuditController_GetAuditLogs_WithStatusFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "login",
		Status:    "success",
	})
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLoginFailed,
		Severity:  auditModels.AuditSeverityWarning,
		Action:    "login failed",
		Status:    "failed",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?status=failed", nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}

func TestAuditController_GetAuditLogs_ValidActorID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	ctrl, svc := setupControllerTest(t)

	actorID := uuid.New()
	svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		ActorID:   &actorID,
		Action:    "login",
		Status:    "success",
	})

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request, _ = http.NewRequest("GET", "/audit/logs?actor_id="+actorID.String(), nil)

	ctrl.GetAuditLogs(ctx)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["total"])
}
