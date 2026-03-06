package scenarioController

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/services"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// TeacherController handles teacher-facing dashboard endpoints
type TeacherController struct {
	dashboardService *services.TeacherDashboardService
	sessionService   *services.ScenarioSessionService
	terminalService  ttServices.TerminalTrainerService
	db               *gorm.DB
}

// NewTeacherController creates a new teacher controller
func NewTeacherController(db *gorm.DB) *TeacherController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	terminalService := ttServices.NewTerminalTrainerService(db)
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	return &TeacherController{
		dashboardService: services.NewTeacherDashboardService(db, terminalService, sessionService),
		sessionService:   sessionService,
		terminalService:  terminalService,
		db:               db,
	}
}

// validateTeacherAccess checks that the current user is an admin or a group owner/admin
func (tc *TeacherController) validateTeacherAccess(c *gin.Context, groupID uuid.UUID) bool {
	userID := c.GetString("userId")
	userRoles, _ := c.Get("userRoles")

	// Platform admins have access
	if roles, ok := userRoles.([]string); ok {
		for _, role := range roles {
			if role == "admin" || role == "administrator" {
				return true
			}
		}
	}

	// Check group-level ownership/admin
	var member groupModels.GroupMember
	err := tc.db.Where("group_id = ? AND user_id = ? AND is_active = true AND role IN ?",
		groupID, userID, []string{"owner", "admin"}).First(&member).Error
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized as group teacher"})
		return false
	}
	return true
}

// GetGroupActivity returns active sessions for all members of a group
func (tc *TeacherController) GetGroupActivity(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	results, err := tc.dashboardService.GetGroupActivity(groupID)
	if err != nil {
		slog.Error("failed to get group activity", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group activity"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetScenarioResults returns all sessions for a specific scenario within a group
func (tc *TeacherController) GetScenarioResults(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	results, err := tc.dashboardService.GetScenarioResults(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to get scenario results", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario results"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetScenarioAnalytics returns aggregate analytics for a scenario within a group
func (tc *TeacherController) GetScenarioAnalytics(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	analytics, err := tc.dashboardService.GetScenarioAnalytics(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to get scenario analytics", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario analytics"})
		return
	}

	c.JSON(http.StatusOK, analytics)
}

// GetSessionDetail returns step-by-step progress for a specific session within a group
func (tc *TeacherController) GetSessionDetail(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	sessionID, err := uuid.Parse(c.Param("sessionId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	detail, err := tc.dashboardService.GetSessionDetail(groupID, sessionID)
	if err != nil {
		slog.Error("failed to get session detail", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session detail"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// BulkStartRequest is the request body for bulk starting a scenario
type BulkStartRequest struct {
	InstanceType           string `json:"instance_type"`
	Backend                string `json:"backend,omitempty"`
	SessionDurationMinutes int    `json:"session_duration_minutes,omitempty"`
}

// BulkStartScenario starts a scenario for all active group members who don't already have an active session.
// Also creates terminal sessions for each student, auto-provisioning keys if needed.
func (tc *TeacherController) BulkStartScenario(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	var req BulkStartRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	result, err := tc.dashboardService.BulkStartScenario(groupID, scenarioID, req.InstanceType, req.Backend, req.SessionDurationMinutes)
	if err != nil {
		slog.Error("failed to bulk start scenario", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ResetGroupScenarioSessions abandons all active sessions for a group+scenario.
// Used to clean up orphaned sessions (e.g., created before students had terminal keys).
func (tc *TeacherController) ResetGroupScenarioSessions(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	count, err := tc.dashboardService.ResetGroupScenarioSessions(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to reset sessions", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"abandoned": count})
}
