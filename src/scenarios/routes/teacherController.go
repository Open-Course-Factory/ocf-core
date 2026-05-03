package scenarioController

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/services"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// TeacherController handles teacher-facing dashboard endpoints
type TeacherController struct {
	dashboardService *services.TeacherDashboardService
	sessionService   *services.ScenarioSessionService
	terminalService  ttServices.TerminalTrainerService
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
	}
}

// GetGroupActivity godoc
// @Summary Get group activity
// @Description Returns all active scenario sessions for members of a group
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Success 200 {array} services.GroupActivityItem
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/activity [get]
// @Security BearerAuth
func (tc *TeacherController) GetGroupActivity(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
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

// GetScenarioResults godoc
// @Summary Get scenario results
// @Description Returns sessions for a specific scenario within a group, with optional pagination
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Param limit query int false "Max results per page (0 or absent = all)"
// @Param offset query int false "Number of results to skip"
// @Success 200 {object} services.PaginatedScenarioResults
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/results [get]
// @Security BearerAuth
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

	// Parse optional pagination params
	var limit, offset *int
	if limitStr := c.Query("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
		limit = &v
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset parameter"})
			return
		}
		offset = &v
	}

	results, err := tc.dashboardService.GetScenarioResults(groupID, scenarioID, limit, offset)
	if err != nil {
		slog.Error("failed to get scenario results", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario results"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetScenarioAnalytics godoc
// @Summary Get scenario analytics
// @Description Returns aggregate analytics (completion rate, avg grade, avg time) for a scenario within a group
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Success 200 {object} services.ScenarioAnalytics
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/analytics [get]
// @Security BearerAuth
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

	analytics, err := tc.dashboardService.GetScenarioAnalytics(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to get scenario analytics", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario analytics"})
		return
	}

	c.JSON(http.StatusOK, analytics)
}

// GetSessionDetail godoc
// @Summary Get session detail
// @Description Returns step-by-step progress for a specific session within a group
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param sessionId path string true "Session ID (UUID)"
// @Success 200 {object} services.SessionDetailResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/sessions/{sessionId}/detail [get]
// @Security BearerAuth
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

	detail, err := tc.dashboardService.GetSessionDetail(groupID, sessionID)
	if err != nil {
		slog.Error("failed to get session detail", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session detail"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// GetSessionCommands godoc
// @Summary Get session terminal commands
// @Description Returns the command history for the terminal session attached to a scenario session.
// @Description Group managers (and platform admins) only — Layer 2 enforces GroupRole(manager) on :groupId.
// @Description Proxies to tt-backend's admin bulk history endpoint with the OCF admin API key.
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param sessionId path string true "Scenario session ID (UUID)"
// @Param limit query int false "Max results per page (default 50, max 1000)"
// @Param offset query int false "Number of results to skip"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/sessions/{sessionId}/commands [get]
// @Security BearerAuth
func (tc *TeacherController) GetSessionCommands(c *gin.Context) {
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

	// Parse optional pagination params
	limit := 0
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
		limit = v
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset parameter"})
			return
		}
		offset = v
	}

	body, contentType, err := tc.dashboardService.GetSessionCommands(groupID, sessionID, limit, offset)
	if err != nil {
		switch err {
		case services.ErrSessionNotFound,
			services.ErrSessionNotInGroup:
			// Don't leak existence — return 404 for both
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		case services.ErrSessionHasNoTerminal:
			c.JSON(http.StatusNotFound, gin.H{"error": "session has no terminal yet"})
			return
		}
		slog.Error("failed to get session commands", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session commands"})
		return
	}

	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(http.StatusOK, contentType, body)
}

// BulkStartRequest is the request body for bulk starting a scenario
type BulkStartRequest struct {
	InstanceType           string `json:"instance_type"`
	Backend                string `json:"backend,omitempty"`
	Hostname               string `json:"hostname,omitempty"`
	SessionDurationMinutes int    `json:"session_duration_minutes,omitempty"`
}

// BulkStartScenario godoc
// @Summary Bulk start a scenario for a group
// @Description Starts a scenario for all active group members who don't already have an active session, creating terminal sessions and auto-provisioning keys if needed
// @Tags scenario-teacher
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Param body body BulkStartRequest false "Optional start parameters"
// @Success 200 {object} services.BulkStartResult
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/bulk-start [post]
// @Security BearerAuth
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

	var req BulkStartRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}

	trainerID := c.GetString("userId")
	result, err := tc.dashboardService.BulkStartScenario(groupID, scenarioID, req.InstanceType, req.Backend, req.SessionDurationMinutes, trainerID)
	if err != nil {
		slog.Error("failed to bulk start scenario", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ResetGroupScenarioSessions godoc
// @Summary Reset group scenario sessions
// @Description Abandons all active sessions for a group+scenario combination, used to clean up orphaned sessions
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Success 200 {object} map[string]int "abandoned count"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/reset-sessions [post]
// @Security BearerAuth
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

	count, err := tc.dashboardService.ResetGroupScenarioSessions(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to reset sessions", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"abandoned": count})
}
