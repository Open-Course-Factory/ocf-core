package scenarioController

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"soli/formations/src/auth/errors"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioController defines the handler methods for scenario-related endpoints
type ScenarioController interface {
	ImportScenario(ctx *gin.Context)
	SeedScenario(ctx *gin.Context)
	UploadScenario(ctx *gin.Context)
	StartScenario(ctx *gin.Context)
	GetCurrentStep(ctx *gin.Context)
	GetStepByOrder(ctx *gin.Context)
	VerifyStep(ctx *gin.Context)
	SubmitFlag(ctx *gin.Context)
	RevealHint(ctx *gin.Context)
	AbandonSession(ctx *gin.Context)
	GetSessionByTerminal(ctx *gin.Context)
	GetSessionInfo(ctx *gin.Context)
	GetSessionFlags(ctx *gin.Context)
	GetMySessions(ctx *gin.Context)
	ExportScenario(ctx *gin.Context)
	ExportScenarios(ctx *gin.Context)
	ImportJSON(ctx *gin.Context)
	GroupExportScenario(ctx *gin.Context)
	GroupImportJSON(ctx *gin.Context)
	GroupUploadScenario(ctx *gin.Context)
}

type scenarioController struct {
	db              *gorm.DB
	sessionService  *services.ScenarioSessionService
	importerService *services.ScenarioImporterService
	exportService   *services.ScenarioExportService
	seedService     *services.ScenarioSeedService
}

// NewScenarioController creates a new scenario controller with its service dependencies
func NewScenarioController(db *gorm.DB) ScenarioController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	importerService := services.NewScenarioImporterService(db)
	exportService := services.NewScenarioExportService(db)
	seedService := services.NewScenarioSeedService(db)

	return &scenarioController{
		db:              db,
		sessionService:  sessionService,
		importerService: importerService,
		exportService:   exportService,
		seedService:     seedService,
	}
}

// getSessionIfOwned loads a session by ID and checks that the authenticated user owns it.
func (sc *scenarioController) getSessionIfOwned(ctx *gin.Context) (*models.ScenarioSession, error) {
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid session ID",
		})
		return nil, err
	}

	userID := ctx.GetString("userId")

	var session models.ScenarioSession
	if err := sc.db.First(&session, "id = ?", sessionID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return nil, err
	}

	if session.UserID != userID {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You do not own this session",
		})
		return nil, fmt.Errorf("forbidden")
	}

	return &session, nil
}

// ImportScenario godoc
// @Summary Import a scenario from git
// @Description Import a KillerCoda-compatible scenario from a git repository (not yet implemented)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.ImportScenarioInput true "Import request"
// @Success 201 {object} models.Scenario
// @Failure 400 {object} errors.APIError
// @Failure 501 {object} errors.APIError
// @Router /scenarios/import [post]
// @Security BearerAuth
func (sc *scenarioController) ImportScenario(ctx *gin.Context) {
	var input dto.ImportScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusNotImplemented, &errors.APIError{
		ErrorCode:    http.StatusNotImplemented,
		ErrorMessage: "Git import not yet implemented. Use directory import via admin API.",
	})
}

// StartScenario godoc
// @Summary Start a scenario session
// @Description Start a new scenario session on a terminal for the authenticated user
// @Tags scenario-sessions
// @Accept json
// @Produce json
// @Param body body dto.StartScenarioInput true "Start request"
// @Success 201 {object} dto.ScenarioSessionOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/start [post]
// @Security BearerAuth
func (sc *scenarioController) StartScenario(ctx *gin.Context) {
	var input dto.StartScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	scenarioID, err := uuid.Parse(input.ScenarioID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	// Validate terminal session ownership
	var terminal terminalModels.Terminal
	if err := sc.db.Where("session_id = ?", input.TerminalSessionID).First(&terminal).Error; err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Terminal session not found",
		})
		return
	}
	if terminal.UserID != userID {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You do not own this terminal session",
		})
		return
	}

	// Check group-based scenario assignment access (admins bypass)
	if !sc.hasAdminRole(ctx) {
		var groupIDs []uuid.UUID
		if err := sc.db.Model(&groupModels.GroupMember{}).
			Where("user_id = ? AND is_active = true", userID).
			Pluck("group_id", &groupIDs).Error; err != nil {
			slog.Error("failed to check group membership", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to verify scenario access",
			})
			return
		}

		if len(groupIDs) == 0 {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "You must be a member of a group to start this scenario",
			})
			return
		}

		var count int64
		if err := sc.db.Model(&models.ScenarioAssignment{}).
			Where("scenario_id = ? AND group_id IN ? AND scope = ? AND is_active = true AND (deadline IS NULL OR deadline > ?)",
				scenarioID, groupIDs, "group", time.Now()).
			Count(&count).Error; err != nil {
			slog.Error("failed to check group scenario assignment", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to verify scenario access",
			})
			return
		}

		if count == 0 {
			// Also check organization-scoped assignments
			var orgIDs []uuid.UUID
			if err := sc.db.Model(&orgModels.OrganizationMember{}).
				Where("user_id = ? AND is_active = true", userID).
				Pluck("organization_id", &orgIDs).Error; err != nil {
				slog.Error("failed to check org membership", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to verify scenario access",
				})
				return
			}
			if len(orgIDs) > 0 {
				if err := sc.db.Model(&models.ScenarioAssignment{}).
					Where("scenario_id = ? AND organization_id IN ? AND scope = ? AND is_active = true AND (deadline IS NULL OR deadline > ?)",
						scenarioID, orgIDs, "org", time.Now()).
					Count(&count).Error; err != nil {
					slog.Error("failed to check org scenario assignment", "err", err)
					ctx.JSON(http.StatusInternalServerError, &errors.APIError{
						ErrorCode:    http.StatusInternalServerError,
						ErrorMessage: "Failed to verify scenario access",
					})
					return
				}
			}
		}

		if count == 0 {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Scenario is not assigned to your group or organization",
			})
			return
		}
	}

	session, err := sc.sessionService.StartScenario(userID, scenarioID, input.TerminalSessionID)
	if err != nil {
		slog.Error("failed to start scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to start scenario",
		})
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusCreated, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		StartedAt:         session.StartedAt,
	})
}

// GetCurrentStep godoc
// @Summary Get current step
// @Description Get the current step content for a scenario session
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} dto.CurrentStepResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/{id}/current-step [get]
// @Security BearerAuth
func (sc *scenarioController) GetCurrentStep(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	step, err := sc.sessionService.GetCurrentStep(session.ID)
	if err != nil {
		slog.Error("failed to get current step", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get current step",
		})
		return
	}

	ctx.JSON(http.StatusOK, step)
}

// GetStepByOrder godoc
// @Summary Get step by order
// @Description Get the content of a specific step by its order for a scenario session. Only completed or active steps can be viewed.
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param stepOrder path int true "Step order (0-based)"
// @Success 200 {object} dto.CurrentStepResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/{id}/step/{stepOrder} [get]
// @Security BearerAuth
func (sc *scenarioController) GetStepByOrder(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	stepOrder, err := strconv.Atoi(ctx.Param("stepOrder"))
	if err != nil || stepOrder < 0 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid step order",
		})
		return
	}

	step, err := sc.sessionService.GetStepByOrder(session.ID, stepOrder)
	if err != nil {
		if err.Error() == "step is locked" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Step is locked",
			})
			return
		}
		slog.Error("failed to get step by order", "err", err)
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Step not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, step)
}

// VerifyStep godoc
// @Summary Verify current step
// @Description Run the verification script for the current step
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} dto.VerifyStepResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/{id}/verify [post]
// @Security BearerAuth
func (sc *scenarioController) VerifyStep(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	result, err := sc.sessionService.VerifyCurrentStep(session.ID)
	if err != nil {
		slog.Error("failed to verify step", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to verify step",
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// SubmitFlag godoc
// @Summary Submit a flag
// @Description Submit a CTF flag answer for the current step
// @Tags scenario-sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body dto.SubmitFlagInput true "Flag submission"
// @Success 200 {object} dto.SubmitFlagResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/{id}/submit-flag [post]
// @Security BearerAuth
func (sc *scenarioController) SubmitFlag(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	var input dto.SubmitFlagInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	result, err := sc.sessionService.SubmitFlag(session.ID, input.Flag)
	if err != nil {
		slog.Error("failed to submit flag", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to submit flag",
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// RevealHint godoc
// @Summary Reveal a progressive hint
// @Description Reveal a progressive hint for a specific step in a scenario session. Hints must be revealed sequentially.
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param stepOrder path int true "Step order (0-based)"
// @Param level path int true "Hint level (1-based)"
// @Success 200 {object} dto.RevealHintResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/{id}/steps/{stepOrder}/hints/{level}/reveal [post]
// @Security BearerAuth
func (sc *scenarioController) RevealHint(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	stepOrder, err := strconv.Atoi(ctx.Param("stepOrder"))
	if err != nil || stepOrder < 0 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid step order",
		})
		return
	}

	level, err := strconv.Atoi(ctx.Param("level"))
	if err != nil || level < 1 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid hint level",
		})
		return
	}

	result, err := sc.sessionService.RevealHint(session.ID, stepOrder, level)
	if err != nil {
		slog.Error("failed to reveal hint", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// AbandonSession godoc
// @Summary Abandon a session
// @Description Abandon the scenario session and discard progress
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/{id}/abandon [post]
// @Security BearerAuth
func (sc *scenarioController) AbandonSession(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	err = sc.sessionService.AbandonSession(session.ID)
	if err != nil {
		slog.Error("failed to abandon session", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to abandon session",
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.MessageResponse{Message: "Session abandoned"})
}

// GetSessionByTerminal godoc
// @Summary Get scenario session by terminal
// @Description Find the active scenario session linked to a terminal session
// @Tags scenario-sessions
// @Produce json
// @Param terminalId path string true "Terminal session ID"
// @Success 200 {object} dto.ScenarioSessionOutput
// @Failure 400 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/by-terminal/{terminalId} [get]
// @Security BearerAuth
func (sc *scenarioController) GetSessionByTerminal(ctx *gin.Context) {
	terminalID := ctx.Param("terminalId")
	if terminalID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Terminal session ID is required",
		})
		return
	}

	var session models.ScenarioSession
	err := sc.db.Where("terminal_session_id = ? AND status = ?", terminalID, "active").First(&session).Error
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active scenario session for this terminal",
		})
		return
	}

	userID := ctx.GetString("userId")
	if session.UserID != userID {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You do not own this session",
		})
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusOK, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		StartedAt:         session.StartedAt,
	})
}

// GetSessionInfo godoc
// @Summary Get session info
// @Description Get session info for the authenticated user (ownership check)
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} dto.SessionResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/{id}/info [get]
// @Security BearerAuth
func (sc *scenarioController) GetSessionInfo(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusOK, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		Grade:             session.Grade,
		StartedAt:         session.StartedAt,
	})
}

// GetSessionFlags returns all validated (correct) flags for a session.
func (sc *scenarioController) GetSessionFlags(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	var flags []models.ScenarioFlag
	sc.db.Where("session_id = ? AND is_correct = ?", session.ID, true).
		Order("step_order asc").Find(&flags)

	type flagResponse struct {
		StepOrder   int        `json:"step_order"`
		Flag        string     `json:"flag"`
		SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	}

	result := make([]flagResponse, 0, len(flags))
	for _, f := range flags {
		if f.SubmittedFlag != nil {
			result = append(result, flagResponse{
				StepOrder:   f.StepOrder,
				Flag:        *f.SubmittedFlag,
				SubmittedAt: f.SubmittedAt,
			})
		}
	}

	ctx.JSON(http.StatusOK, result)
}

// GetMySessions godoc
// @Summary Get my scenario sessions
// @Description Get all scenario sessions for the authenticated user
// @Tags scenario-sessions
// @Produce json
// @Success 200 {array} dto.MySessionResponse
// @Failure 401 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/my [get]
// @Security BearerAuth
func (sc *scenarioController) GetMySessions(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	if userID == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "Unauthorized",
		})
		return
	}

	sessions, err := sc.sessionService.GetMySessions(userID)
	if err != nil {
		slog.Error("failed to get my sessions", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get sessions",
		})
		return
	}

	ctx.JSON(http.StatusOK, sessions)
}

// SeedScenario godoc
// @Summary Seed a scenario with steps
// @Description Create a scenario with all steps from a single JSON payload (admin/testing)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.SeedScenarioInput true "Seed request"
// @Success 201 {object} models.Scenario
// @Failure 400 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/seed [post]
// @Security BearerAuth
func (sc *scenarioController) SeedScenario(ctx *gin.Context) {
	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	if !sc.isAdmin(ctx) {
		return
	}

	userID := ctx.GetString("userId")

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, nil)
	if err != nil {
		slog.Error("failed to seed scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to seed scenario",
		})
		return
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}

	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// UploadScenario godoc
// @Summary Upload a scenario archive
// @Description Upload a .zip or .tar.gz archive containing a KillerCoda-compatible scenario directory and import it
// @Tags scenarios
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Scenario archive (.zip, .tar.gz, or .tgz)"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/upload [post]
// @Security BearerAuth
func (sc *scenarioController) UploadScenario(ctx *gin.Context) {
	if !sc.isAdmin(ctx) {
		return
	}

	userID := ctx.GetString("userId")

	// Get file from multipart form
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File is required",
		})
		return
	}

	// Validate file size (10MB max)
	if file.Size > 10*1024*1024 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File size exceeds 10MB limit",
		})
		return
	}

	// Validate extension
	filename := strings.ToLower(file.Filename)
	var ext string
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(filename, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(filename, ".zip"):
		ext = ".zip"
	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File must be .zip, .tar.gz, or .tgz",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save uploaded file",
		})
		return
	}

	// Extract archive
	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Failed to extract archive: %s", err.Error()),
		})
		return
	}

	// Find index.json
	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Archive must contain an index.json file",
		})
		return
	}

	// Import scenario
	scenario, err := sc.importerService.ImportFromDirectory(scenarioDir, userID, nil, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to import scenario: %s", err.Error()),
		})
		return
	}

	// Reload with steps
	var loaded models.Scenario
	if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reload scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(&loaded))
}

// hasAdminRole checks if the context has admin/administrator role without writing a response.
func (sc *scenarioController) hasAdminRole(ctx *gin.Context) bool {
	userRoles, _ := ctx.Get("userRoles")
	if roles, ok := userRoles.([]string); ok {
		for _, role := range roles {
			if role == "admin" || role == "administrator" {
				return true
			}
		}
	}
	return false
}

// isAdmin checks if the current user has admin/administrator role.
// Returns false and writes a 403 response if not admin.
func (sc *scenarioController) isAdmin(ctx *gin.Context) bool {
	if sc.hasAdminRole(ctx) {
		return true
	}
	ctx.JSON(http.StatusForbidden, &errors.APIError{
		ErrorCode:    http.StatusForbidden,
		ErrorMessage: "Admin access required",
	})
	return false
}

// validateTeacherAccess checks that the current user is an admin or a group owner/admin
func (sc *scenarioController) validateTeacherAccess(ctx *gin.Context, groupID uuid.UUID) bool {
	userID := ctx.GetString("userId")
	userRoles, _ := ctx.Get("userRoles")

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
	err := sc.db.Where("group_id = ? AND user_id = ? AND is_active = true AND role IN ?",
		groupID, userID, []string{"owner", "admin"}).First(&member).Error
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "not authorized"})
		return false
	}
	return true
}

// buildScenarioOutput converts a Scenario model to a ScenarioOutput DTO
func (sc *scenarioController) buildScenarioOutput(scenario *models.Scenario) dto.ScenarioOutput {
	output := dto.ScenarioOutput{
		ID:             scenario.ID,
		Name:           scenario.Name,
		Title:          scenario.Title,
		Description:    scenario.Description,
		Difficulty:     scenario.Difficulty,
		EstimatedTime:  scenario.EstimatedTime,
		InstanceType:   scenario.InstanceType,
		OsType:         scenario.OsType,
		SourceType:     scenario.SourceType,
		FlagsEnabled:   scenario.FlagsEnabled,
		GshEnabled:     scenario.GshEnabled,
		CrashTraps:     scenario.CrashTraps,
		IntroText:      scenario.IntroText,
		FinishText:     scenario.FinishText,
		CreatedByID:    scenario.CreatedByID,
		OrganizationID: scenario.OrganizationID,
		CreatedAt:      scenario.CreatedAt,
		UpdatedAt:      scenario.UpdatedAt,
	}
	if len(scenario.Steps) > 0 {
		steps := make([]dto.ScenarioStepOutput, 0, len(scenario.Steps))
		for _, step := range scenario.Steps {
			steps = append(steps, dto.ScenarioStepOutput{
				ID:          step.ID,
				ScenarioID:  step.ScenarioID,
				Order:       step.Order,
				Title:       step.Title,
				TextContent: step.TextContent,
				HintContent: step.HintContent,
				HasFlag:     step.HasFlag,
				FlagPath:    step.FlagPath,
				FlagLevel:   step.FlagLevel,
				CreatedAt:   step.CreatedAt,
				UpdatedAt:   step.UpdatedAt,
			})
		}
		output.Steps = steps
	}
	return output
}

// ExportScenario godoc
// @Summary Export a scenario
// @Description Export a scenario as JSON or KillerCoda archive
// @Tags scenarios
// @Produce json
// @Produce application/zip
// @Param id path string true "Scenario ID"
// @Param format query string false "Export format: json (default) or killerkoda"
// @Success 200 {object} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/export [get]
// @Security BearerAuth
func (sc *scenarioController) ExportScenario(ctx *gin.Context) {
	if !sc.isAdmin(ctx) {
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	format := ctx.DefaultQuery("format", "json")

	switch format {
	case "json":
		export, err := sc.exportService.ExportAsJSON(scenarioID)
		if err != nil {
			slog.Error("failed to export scenario as JSON", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.JSON(http.StatusOK, export)

	case "killerkoda":
		zipBytes, filename, err := sc.exportService.ExportAsArchive(scenarioID)
		if err != nil {
			slog.Error("failed to export scenario as archive", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		ctx.Data(http.StatusOK, "application/zip", zipBytes)

	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid format. Use 'json' or 'killerkoda'",
		})
	}
}

// ExportScenarios godoc
// @Summary Export multiple scenarios
// @Description Export multiple scenarios as JSON array
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.ExportScenariosInput true "Scenario IDs to export"
// @Success 200 {array} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Router /scenarios/export [post]
// @Security BearerAuth
func (sc *scenarioController) ExportScenarios(ctx *gin.Context) {
	if !sc.isAdmin(ctx) {
		return
	}

	var input dto.ExportScenariosInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	exports, err := sc.exportService.ExportMultipleAsJSON(input.IDs)
	if err != nil {
		slog.Error("failed to export scenarios", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to export scenarios",
		})
		return
	}

	ctx.JSON(http.StatusOK, exports)
}

// ImportJSON godoc
// @Summary Import a scenario from JSON
// @Description Create or update a scenario from a JSON payload (admin)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.SeedScenarioInput true "Scenario data"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/import-json [post]
// @Security BearerAuth
func (sc *scenarioController) ImportJSON(ctx *gin.Context) {
	if !sc.isAdmin(ctx) {
		return
	}

	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, nil)
	if err != nil {
		slog.Error("failed to import scenario from JSON", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import scenario",
		})
		return
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}
	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// GroupExportScenario godoc
// @Summary Export a group scenario
// @Description Export a scenario assigned to a group as JSON or KillerCoda archive
// @Tags scenarios
// @Produce json
// @Produce application/zip
// @Param groupId path string true "Group ID"
// @Param scenarioId path string true "Scenario ID"
// @Param format query string false "Export format: json (default) or killerkoda"
// @Success 200 {object} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/{scenarioId}/export [get]
// @Security BearerAuth
func (sc *scenarioController) GroupExportScenario(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	if !sc.validateTeacherAccess(ctx, groupID) {
		return
	}

	// Verify scenario is assigned to the group
	var assignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ? AND is_active = true",
		scenarioID, groupID).First(&assignment).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not assigned to this group",
		})
		return
	}

	format := ctx.DefaultQuery("format", "json")

	switch format {
	case "json":
		export, err := sc.exportService.ExportAsJSON(scenarioID)
		if err != nil {
			slog.Error("failed to export group scenario as JSON", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.JSON(http.StatusOK, export)

	case "killerkoda":
		zipBytes, filename, err := sc.exportService.ExportAsArchive(scenarioID)
		if err != nil {
			slog.Error("failed to export group scenario as archive", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		ctx.Data(http.StatusOK, "application/zip", zipBytes)

	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid format. Use 'json' or 'killerkoda'",
		})
	}
}

// GroupImportJSON godoc
// @Summary Import a scenario into a group from JSON
// @Description Create or update a scenario from JSON and assign it to a group
// @Tags scenarios
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID"
// @Param body body dto.SeedScenarioInput true "Scenario data"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/import-json [post]
// @Security BearerAuth
func (sc *scenarioController) GroupImportJSON(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	if !sc.validateTeacherAccess(ctx, groupID) {
		return
	}

	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	// Get the group's organization ID
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, group.OrganizationID)
	if err != nil {
		slog.Error("failed to import scenario for group", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import scenario",
		})
		return
	}

	// Auto-create ScenarioAssignment for the group (if not already assigned)
	var existingAssignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ?",
		scenario.ID, groupID).First(&existingAssignment).Error; err != nil {
		// No existing assignment, create one
		assignment := models.ScenarioAssignment{
			ScenarioID:  scenario.ID,
			GroupID:     &groupID,
			Scope:       "group",
			CreatedByID: userID,
			IsActive:    true,
		}
		if err := sc.db.Create(&assignment).Error; err != nil {
			slog.Error("failed to create scenario assignment", "err", err)
			// Don't fail the whole request, scenario was already created
		}
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}
	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// GroupUploadScenario godoc
// @Summary Upload a scenario archive for a group
// @Description Upload a .zip or .tar.gz archive containing a KillerCoda-compatible scenario and assign it to a group
// @Tags scenarios
// @Accept multipart/form-data
// @Produce json
// @Param groupId path string true "Group ID"
// @Param file formData file true "Scenario archive (.zip, .tar.gz, or .tgz)"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/upload [post]
// @Security BearerAuth
func (sc *scenarioController) GroupUploadScenario(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	if !sc.validateTeacherAccess(ctx, groupID) {
		return
	}

	userID := ctx.GetString("userId")

	// Get the group's organization ID
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	// Get file from multipart form
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File is required",
		})
		return
	}

	// Validate file size (10MB max)
	if file.Size > 10*1024*1024 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File size exceeds 10MB limit",
		})
		return
	}

	// Validate extension
	filename := strings.ToLower(file.Filename)
	var ext string
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(filename, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(filename, ".zip"):
		ext = ".zip"
	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File must be .zip, .tar.gz, or .tgz",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save uploaded file",
		})
		return
	}

	// Extract archive
	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Failed to extract archive: %s", err.Error()),
		})
		return
	}

	// Find index.json
	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Archive must contain an index.json file",
		})
		return
	}

	// Import scenario with org ID from group
	scenario, err := sc.importerService.ImportFromDirectory(scenarioDir, userID, group.OrganizationID, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to import scenario: %s", err.Error()),
		})
		return
	}

	// Auto-create ScenarioAssignment for the group (if not already assigned)
	var existingAssignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ?",
		scenario.ID, groupID).First(&existingAssignment).Error; err != nil {
		assignment := models.ScenarioAssignment{
			ScenarioID:  scenario.ID,
			GroupID:     &groupID,
			Scope:       "group",
			CreatedByID: userID,
			IsActive:    true,
		}
		if err := sc.db.Create(&assignment).Error; err != nil {
			slog.Error("failed to create scenario assignment", "err", err)
		}
	}

	// Reload with steps
	var loaded models.Scenario
	if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reload scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(&loaded))
}

