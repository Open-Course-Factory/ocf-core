package scenarioController

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"soli/formations/src/auth/errors"
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
	AbandonSession(ctx *gin.Context)
	GetSessionByTerminal(ctx *gin.Context)
	GetSessionInfo(ctx *gin.Context)
	GetMySessions(ctx *gin.Context)
}

type scenarioController struct {
	db              *gorm.DB
	sessionService  *services.ScenarioSessionService
	importerService *services.ScenarioImporterService
}

// NewScenarioController creates a new scenario controller with its service dependencies
func NewScenarioController(db *gorm.DB) ScenarioController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	importerService := services.NewScenarioImporterService(db)

	return &scenarioController{
		db:              db,
		sessionService:  sessionService,
		importerService: importerService,
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

	// Check admin role
	userRoles, exists := ctx.Get("userRoles")
	if !exists {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied",
		})
		return
	}
	isAdmin := false
	if roles, ok := userRoles.([]string); ok {
		for _, role := range roles {
			if role == "admin" || role == "administrator" {
				isAdmin = true
				break
			}
		}
	}
	if !isAdmin {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	userID := ctx.GetString("userId")

	name := utils.GenerateSlug(input.Title)

	// Check if a scenario with this name already exists (upsert)
	var existing models.Scenario
	isUpdate := false
	if err := sc.db.Where("name = ?", name).First(&existing).Error; err == nil {
		isUpdate = true
	}

	var flagSecret string
	if input.FlagsEnabled {
		if isUpdate && existing.FlagSecret != "" {
			// Keep existing flag secret on update so active sessions remain valid
			flagSecret = existing.FlagSecret
		} else {
			secretBytes := make([]byte, 32)
			if _, err := crypto_rand.Read(secretBytes); err != nil {
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to generate flag secret",
				})
				return
			}
			flagSecret = hex.EncodeToString(secretBytes)
		}
	}

	// Build new steps
	newSteps := make([]models.ScenarioStep, len(input.Steps))
	for i, s := range input.Steps {
		newSteps[i] = models.ScenarioStep{
			Order:            i,
			Title:            s.Title,
			TextContent:      s.TextContent,
			HintContent:      s.HintContent,
			VerifyScript:     s.VerifyScript,
			BackgroundScript: s.BackgroundScript,
			ForegroundScript: s.ForegroundScript,
			HasFlag:          s.HasFlag,
			FlagPath:         s.FlagPath,
		}
	}

	var scenario models.Scenario
	if isUpdate {
		// Update existing scenario in a transaction
		err := sc.db.Transaction(func(tx *gorm.DB) error {
			// Update scenario fields
			if err := tx.Model(&existing).Updates(map[string]any{
				"title":          input.Title,
				"description":    input.Description,
				"difficulty":     input.Difficulty,
				"estimated_time": input.EstimatedTime,
				"instance_type":  input.InstanceType,
				"os_type":        input.OsType,
				"flags_enabled":  input.FlagsEnabled,
				"flag_secret":    flagSecret,
				"gsh_enabled":    input.GshEnabled,
				"crash_traps":    input.CrashTraps,
				"intro_text":     input.IntroText,
				"finish_text":    input.FinishText,
			}).Error; err != nil {
				return fmt.Errorf("failed to update scenario: %w", err)
			}

			// Delete old steps
			if err := tx.Where("scenario_id = ?", existing.ID).Delete(&models.ScenarioStep{}).Error; err != nil {
				return fmt.Errorf("failed to delete old steps: %w", err)
			}

			// Create new steps
			for i := range newSteps {
				newSteps[i].ScenarioID = existing.ID
				if err := tx.Create(&newSteps[i]).Error; err != nil {
					return fmt.Errorf("failed to create step: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			slog.Error("failed to update scenario", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to update scenario",
			})
			return
		}

		// Reload with steps
		if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).First(&scenario, "id = ?", existing.ID).Error; err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to reload scenario",
			})
			return
		}
	} else {
		// Create new scenario
		scenario = models.Scenario{
			Name:          name,
			Title:         input.Title,
			Description:   input.Description,
			Difficulty:    input.Difficulty,
			EstimatedTime: input.EstimatedTime,
			InstanceType:  input.InstanceType,
			OsType:        input.OsType,
			SourceType:    "seed",
			FlagsEnabled:  input.FlagsEnabled,
			FlagSecret:    flagSecret,
			GshEnabled:    input.GshEnabled,
			CrashTraps:    input.CrashTraps,
			IntroText:     input.IntroText,
			FinishText:    input.FinishText,
			CreatedByID:   userID,
		}
		scenario.Steps = newSteps

		if err := sc.db.Create(&scenario).Error; err != nil {
			slog.Error("failed to create scenario", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to create scenario",
			})
			return
		}
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}

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
	ctx.JSON(statusCode, output)
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
	// Check admin role (same pattern as SeedScenario)
	userRoles, exists := ctx.Get("userRoles")
	if !exists {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied",
		})
		return
	}
	isAdmin := false
	if roles, ok := userRoles.([]string); ok {
		for _, role := range roles {
			if role == "admin" || role == "administrator" {
				isAdmin = true
				break
			}
		}
	}
	if !isAdmin {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
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

	// Build output (same pattern as SeedScenario)
	output := dto.ScenarioOutput{
		ID:             loaded.ID,
		Name:           loaded.Name,
		Title:          loaded.Title,
		Description:    loaded.Description,
		Difficulty:     loaded.Difficulty,
		EstimatedTime:  loaded.EstimatedTime,
		InstanceType:   loaded.InstanceType,
		OsType:         loaded.OsType,
		SourceType:     loaded.SourceType,
		FlagsEnabled:   loaded.FlagsEnabled,
		GshEnabled:     loaded.GshEnabled,
		CrashTraps:     loaded.CrashTraps,
		IntroText:      loaded.IntroText,
		FinishText:     loaded.FinishText,
		CreatedByID:    loaded.CreatedByID,
		OrganizationID: loaded.OrganizationID,
		CreatedAt:      loaded.CreatedAt,
		UpdatedAt:      loaded.UpdatedAt,
	}
	if len(loaded.Steps) > 0 {
		steps := make([]dto.ScenarioStepOutput, 0, len(loaded.Steps))
		for _, step := range loaded.Steps {
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
	ctx.JSON(http.StatusOK, output)
}

