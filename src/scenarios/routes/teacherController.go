package scenarioController

import (
	groupModels "soli/formations/src/groups/models"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/scenarios/dto"
	scenarioModels "soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// TeacherController handles teacher-facing dashboard endpoints
type TeacherController struct {
	db                  *gorm.DB
	dashboardService    *services.TeacherDashboardService
	sessionService      *services.ScenarioSessionService
	terminalService     ttServices.TerminalTrainerService
	provisioningService *services.ScenarioProvisioningService
}

// NewTeacherController creates a new teacher controller
func NewTeacherController(db *gorm.DB) *TeacherController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	terminalService := ttServices.NewTerminalTrainerService(db)
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	return &TeacherController{
		db:                  db,
		dashboardService:    services.NewTeacherDashboardService(db, terminalService, sessionService),
		sessionService:      sessionService,
		terminalService:     terminalService,
		provisioningService: services.NewScenarioProvisioningService(db, terminalService),
	}
}

// GetMyGroups godoc
// @Summary List the classes the caller teaches
// @Description Returns one dashboard row per class-group the authenticated user owns or
// @Description manages: member count, live terminal sessions, and each active scenario
// @Description assignment with its progress. Self-scoped — the groups are derived from the
// @Description authenticated caller, never from a client-supplied user id, and platform
// @Description administrators get their OWN classes like anyone else.
// @Tags scenario-teacher
// @Produce json
// @Success 200 {array} services.TeacherGroupSummary
// @Failure 500 {object} map[string]string
// @Router /teacher/groups [get]
// @Security BearerAuth
func (tc *TeacherController) GetMyGroups(c *gin.Context) {
	summaries, err := tc.dashboardService.GetManagedGroupsOverview(c.GetString("userId"))
	if err != nil {
		slog.Error("failed to get managed groups overview", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get managed groups"})
		return
	}

	c.JSON(http.StatusOK, summaries)
}

// GetGroupLiveProgress godoc
// @Summary Get the merged per-learner live view of a class
// @Description Returns one row per ACTIVE group member, joining supervision presence
// @Description (a live terminal session visible in the group's organization), scenario
// @Description position on each active assignment (current step, its title, elapsed time,
// @Description status, grade) and assignment results (hints used). A member who has done
// @Description nothing is still listed, as not_started with connected=false.
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Success 200 {array} services.LearnerLiveProgress
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/live-progress [get]
// @Security BearerAuth
func (tc *TeacherController) GetGroupLiveProgress(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	rows, err := tc.dashboardService.GetGroupLiveProgress(groupID)
	if err != nil {
		slog.Error("failed to get group live progress", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group live progress"})
		return
	}

	c.JSON(http.StatusOK, rows)
}

// GetParticipationReport godoc
// @Summary Get a class's participation report
// @Description What each learner did over a period: first and last recorded activity per day, commands and scenario progress. It reports recorded activity, not hours attended.
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param from query string true "First day, YYYY-MM-DD"
// @Param to query string true "Last day included, YYYY-MM-DD"
// @Param tz query string false "IANA timezone that days are cut in (default UTC)"
// @Param days query string false "Course days to report, YYYY-MM-DD separated by commas (default: every day with activity)"
// @Success 200 {object} services.ParticipationReport
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/participation-report [get]
// @Security BearerAuth
func (tc *TeacherController) GetParticipationReport(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}
	location, err := time.LoadLocation(c.DefaultQuery("tz", "UTC"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timezone"})
		return
	}
	from, to, err := parseReportPeriod(c.Query("from"), c.Query("to"), location)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	courseDays, err := parseCourseDays(c.Query("days"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scope, err := services.NewClassPeriodScope(tc.db, groupID, from, to, courseDays)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "class not found"})
		return
	}
	if err != nil {
		slog.Error("failed to load participation scope", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build the report"})
		return
	}
	report, err := services.BuildParticipationReport(tc.db, scope, ttServices.NewCommandActivityReader(), location)
	if err != nil {
		slog.Error("failed to build participation report", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build the report"})
		return
	}
	c.JSON(http.StatusOK, report)
}

// parseReportPeriod turns two inclusive days into a half-open period.
func parseReportPeriod(fromDay, toDay string, location *time.Location) (from, to time.Time, err error) {
	from, err = time.ParseInLocation(time.DateOnly, fromDay, location)
	if err != nil {
		return from, to, errors.New("from must be a date, YYYY-MM-DD")
	}
	lastDay, err := time.ParseInLocation(time.DateOnly, toDay, location)
	if err != nil {
		return from, to, errors.New("to must be a date, YYYY-MM-DD")
	}
	if lastDay.Before(from) {
		return from, to, errors.New("to must not be before from")
	}
	if !lastDay.Before(from.AddDate(1, 0, 0)) {
		return from, to, errors.New("the period must not exceed one year")
	}
	return from, lastDay.AddDate(0, 0, 1), nil
}

// parseCourseDays reads a comma-separated list of days, sorted and without duplicates.
func parseCourseDays(param string) ([]string, error) {
	if param == "" {
		return nil, nil
	}
	days := strings.Split(param, ",")
	if err := groupModels.ValidateCourseDays(days); err != nil {
		return nil, err
	}
	slices.Sort(days)
	return slices.Compact(days), nil
}

// GetGroupAssignmentsProgress godoc
// @Summary Get group assignments progress
// @Description Returns one progress summary per scenario assigned to a group, aggregating active members' non-preview sessions (total, completed, average grade)
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Success 200 {array} services.AssignmentProgressItem
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/assignments-progress [get]
// @Security BearerAuth
func (tc *TeacherController) GetGroupAssignmentsProgress(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	results, err := tc.dashboardService.GetGroupAssignmentsProgress(groupID)
	if err != nil {
		slog.Error("failed to get group assignments progress", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group assignments progress"})
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

// SkipToStep godoc
// @Summary Skip a learner's run to a later step
// @Description Moves an active session of the group forward to the given step: the steps in
// @Description between are marked skipped (not graded as completed) and only the target step's
// @Description setup runs. Group managers (and platform admins) only.
// @Tags scenario-teacher
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param sessionId path string true "Session ID (UUID)"
// @Param body body dto.SkipToStepInput true "Target step"
// @Success 200 {object} dto.SkipToStepResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/sessions/{sessionId}/skip-to-step [post]
// @Security BearerAuth
func (tc *TeacherController) SkipToStep(c *gin.Context) {
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
	var input dto.SkipToStepInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Layer 2 proves the caller manages :groupId; this proves the session is
	// one of that group's (member + assigned scenario), the same IDOR check as
	// the detail view.
	if _, err := tc.dashboardService.GetSessionDetail(groupID, sessionID); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "session not found") || strings.Contains(errStr, "does not belong") || strings.Contains(errStr, "scenario is not assigned") {
			c.JSON(http.StatusBadRequest, gin.H{"error": errStr})
			return
		}
		slog.Error("failed to check session before skip", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}

	result, err := tc.sessionService.SkipToStep(sessionID, input.StepOrder)
	switch {
	case errors.Is(err, services.ErrSessionNotActive), errors.Is(err, services.ErrSkipRaced):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	case err != nil:
		slog.Warn("failed to skip to step", "session_id", sessionID, "step_order", input.StepOrder, "err", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSessionDetailsBulkRequest is the body of POST /teacher/groups/:groupId/sessions/details.
type GetSessionDetailsBulkRequest struct {
	SessionIDs []uuid.UUID `json:"session_ids" binding:"required"`
}

// GetSessionDetailsBulk godoc
// @Summary Get session details (bulk)
// @Description Returns details for multiple sessions in one request. Same per-session
// @Description auth checks as the single endpoint. Used by CSV export to avoid N+1.
// @Tags scenario-teacher
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param body body GetSessionDetailsBulkRequest true "Session IDs to fetch"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /teacher/groups/{groupId}/sessions/details [post]
// @Security BearerAuth
func (tc *TeacherController) GetSessionDetailsBulk(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	var req GetSessionDetailsBulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	details, err := tc.dashboardService.GetSessionDetails(groupID, req.SessionIDs)
	if err != nil {
		// Map known errors to appropriate status codes. The service preserves
		// GetSessionDetail's verbatim phrasing for "session not found" and
		// "session does not belong to this group" — both are 4xx.
		errStr := err.Error()
		if strings.Contains(errStr, "too many session IDs") {
			c.JSON(http.StatusBadRequest, gin.H{"error": errStr})
			return
		}
		if strings.Contains(errStr, "session not found") || strings.Contains(errStr, "does not belong") || strings.Contains(errStr, "scenario is not assigned") {
			c.JSON(http.StatusBadRequest, gin.H{"error": errStr})
			return
		}
		slog.Error("failed to get bulk session details", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": details})
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
			services.ErrSessionNotInGroup,
			services.ErrScenarioNotAssignedToGroup:
			// Don't leak existence — return 404 for all three
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
	// Deprecated: ignored. The distribution comes from the scenario's own
	// declaration, resolved the same way a single learner's launch resolves it.
	// Kept so existing clients keep parsing; ocf-front's picker can go.
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

	// Loaded once, and before the gate: the organization the class's runs live
	// in decides which plan the gate reads and which servers apply.
	scenario, err := tc.provisioningService.ScenarioForProvisioning(scenarioID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scenario not found"})
		return
	}
	// A class that cannot be read leaves the scenario's organization: the
	// service reports a missing class itself, after the gate has had its say.
	var class groupModels.ClassGroup
	tc.db.Select("organization_id").First(&class, "id = ?", groupID)
	runOrgID := services.ClassRunOrganization(class.OrganizationID, &scenario)

	// Dunning gate (MANDATORY on every new session-creation route): bulk-start
	// provisions terminals for the whole class on the TEACHER's initiative, so
	// the TEACHER's past_due subscription blocks the launch — members are
	// typically on assigned/org plans with no personal dunning stamp, and the
	// payer-side actor here is the caller (decided 2026-08-07). The plan is
	// resolved in the class's run organization, mirroring the per-member
	// resolution inside BulkStartScenario; a failed resolution does not block
	// (the service reports its own errors).
	planResult, planErr := paymentServices.NewEffectivePlanService(tc.db).
		GetUserEffectivePlan(trainerID, runOrgID)
	if planErr == nil && paymentMiddleware.GatePastDueBeyondGraceForResult(c, planResult) {
		return
	}

	// What to build is the scenario's answer, not the caller's. Deliberately
	// after the gate: a trainer who owes money is refused for that reason, not
	// told the terminal service is unavailable.
	//
	// req.InstanceType is not read: a scenario naming its images has stated a
	// requirement, and substituting one its author never approved is what the
	// resolution refuses. Which host to build on is the teacher's call:
	// req.Backend, else the one the class's assignment names, else automatic.
	//
	// A class's host is checked against the organization's servers, not the
	// trainer's own plan (nil): each learner's start is then checked against
	// theirs.
	backend := req.Backend
	if backend == "" {
		if backend, err = tc.provisioningService.ClassAssignmentBackend(groupID, scenarioID); err != nil {
			slog.Error("failed to read the class assignment's backend", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read the class assignment"})
			return
		}
	}
	provisioning, resolveErr := tc.provisioningService.Resolve(scenario, runOrgID, nil, backend)
	if resolveErr != nil {
		respondProvisioningFailure(c, scenario.Name, resolveErr)
		return
	}

	result, err := tc.dashboardService.BulkStartScenario(groupID, scenarioID, provisioning, req.SessionDurationMinutes, trainerID)
	if err != nil {
		if errors.Is(err, scenarioModels.ErrScenarioArchived) || errors.Is(err, groupModels.ErrClassArchived) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
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

	count, skipped, err := tc.dashboardService.ResetGroupScenarioSessions(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to reset sessions", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"abandoned": count, "skipped_users": skipped})
}

// PublishScenarioResults godoc
// @Summary Release a class's exam grades
// @Description Learners of the class see their grades on this scenario's exam quizzes from now on.
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Success 200 {object} map[string]any "results_published_at"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/results/publish [post]
// @Security BearerAuth
func (tc *TeacherController) PublishScenarioResults(c *gin.Context) {
	groupID, scenarioID, ok := parseGroupScenarioParams(c)
	if !ok {
		return
	}
	publishedAt, err := tc.dashboardService.PublishScenarioResults(groupID, scenarioID)
	respondResultsPublication(c, publishedAt, err)
}

// UnpublishScenarioResults godoc
// @Summary Withhold a class's exam grades again
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param scenarioId path string true "Scenario ID (UUID)"
// @Success 200 {object} map[string]any "results_published_at: null"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /teacher/groups/{groupId}/scenarios/{scenarioId}/results/publish [delete]
// @Security BearerAuth
func (tc *TeacherController) UnpublishScenarioResults(c *gin.Context) {
	groupID, scenarioID, ok := parseGroupScenarioParams(c)
	if !ok {
		return
	}
	respondResultsPublication(c, nil, tc.dashboardService.UnpublishScenarioResults(groupID, scenarioID))
}

func parseGroupScenarioParams(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return uuid.Nil, uuid.Nil, false
	}
	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return uuid.Nil, uuid.Nil, false
	}
	return groupID, scenarioID, true
}

func respondResultsPublication(c *gin.Context, publishedAt *time.Time, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"results_published_at": publishedAt})
		return
	}
	if errors.Is(err, services.ErrScenarioNotAssignedToGroup) {
		c.JSON(http.StatusNotFound, gin.H{"error": "scenario is not assigned to this group"})
		return
	}
	slog.Error("failed to change results publication", "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change results publication"})
}

// AllowRetake godoc
// @Summary Allow a learner one more exam attempt
// @Description Releases the learner of this class session from their submitted exam so they can start a new run. Their previous runs and grades are kept.
// @Tags scenario-teacher
// @Produce json
// @Param groupId path string true "Group ID (UUID)"
// @Param sessionId path string true "Session ID (UUID)"
// @Success 200 {object} map[string]any "retake_allowed_at"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string "reason: exam_not_locked"
// @Router /teacher/groups/{groupId}/sessions/{sessionId}/allow-retake [post]
// @Security BearerAuth
func (tc *TeacherController) AllowRetake(c *gin.Context) {
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

	retakeAllowedAt, err := tc.dashboardService.AllowRetake(groupID, sessionID)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"retake_allowed_at": retakeAllowedAt})
		return
	}
	if errors.Is(err, services.ErrSessionNotInGroup) {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if errors.Is(err, services.ErrExamNotLocked) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "reason": "exam_not_locked"})
		return
	}
	slog.Error("failed to allow a retake", "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to allow a retake"})
}
