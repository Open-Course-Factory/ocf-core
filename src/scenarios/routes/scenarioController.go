package scenarioController

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioController defines the handler methods for scenario-related endpoints
type ScenarioController interface {
	ImportScenario(ctx *gin.Context)
	SeedScenario(ctx *gin.Context)
	StartScenario(ctx *gin.Context)
	GetCurrentStep(ctx *gin.Context)
	VerifyStep(ctx *gin.Context)
	SubmitFlag(ctx *gin.Context)
	AbandonSession(ctx *gin.Context)
	GetSessionByTerminal(ctx *gin.Context)
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

// ImportScenario handles POST /scenarios/import
// Currently returns 501 Not Implemented as git cloning is not yet built.
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

// StartScenario handles POST /scenario-sessions/start
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

	session, err := sc.sessionService.StartScenario(userID, scenarioID, input.TerminalSessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"id":                  session.ID,
		"scenario_id":         session.ScenarioID,
		"user_id":             session.UserID,
		"terminal_session_id": session.TerminalSessionID,
		"current_step":        session.CurrentStep,
		"status":              session.Status,
		"started_at":          session.StartedAt,
	})
}

// GetCurrentStep handles GET /scenario-sessions/:id/current-step
func (sc *scenarioController) GetCurrentStep(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	step, err := sc.sessionService.GetCurrentStep(session.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, step)
}

// VerifyStep handles POST /scenario-sessions/:id/verify
func (sc *scenarioController) VerifyStep(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	result, err := sc.sessionService.VerifyCurrentStep(session.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// SubmitFlag handles POST /scenario-sessions/:id/submit-flag
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
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// AbandonSession handles POST /scenario-sessions/:id/abandon
func (sc *scenarioController) AbandonSession(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	err = sc.sessionService.AbandonSession(session.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Session abandoned"})
}

// GetSessionByTerminal handles GET /scenario-sessions/by-terminal/:terminalId
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

	ctx.JSON(http.StatusOK, gin.H{
		"id":                  session.ID,
		"scenario_id":         session.ScenarioID,
		"user_id":             session.UserID,
		"terminal_session_id": session.TerminalSessionID,
		"current_step":        session.CurrentStep,
		"status":              session.Status,
		"started_at":          session.StartedAt,
	})
}

// SeedScenario handles POST /scenarios/seed
// Creates a scenario with all its steps from a single JSON payload (admin/testing).
func (sc *scenarioController) SeedScenario(ctx *gin.Context) {
	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	name := generateSlug(input.Title)

	var flagSecret string
	if input.FlagsEnabled {
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

	scenario := models.Scenario{
		Name:          name,
		Title:         input.Title,
		Description:   input.Description,
		Difficulty:    input.Difficulty,
		EstimatedTime: input.EstimatedTime,
		InstanceType:  input.InstanceType,
		SourceType:    "seed",
		FlagsEnabled:  input.FlagsEnabled,
		FlagSecret:    flagSecret,
		GshEnabled:    input.GshEnabled,
		CrashTraps:    input.CrashTraps,
		IntroText:     input.IntroText,
		FinishText:    input.FinishText,
		CreatedByID:   userID,
	}

	steps := make([]models.ScenarioStep, len(input.Steps))
	for i, s := range input.Steps {
		steps[i] = models.ScenarioStep{
			Order:            i,
			Title:            s.Title,
			TextContent:      s.TextContent,
			HintContent:      s.HintContent,
			VerifyScript:     s.VerifyScript,
			BackgroundScript: s.BackgroundScript,
			ForegroundScript: s.ForegroundScript,
			HasFlag:          s.HasFlag,
		}
	}
	scenario.Steps = steps

	if err := sc.db.Create(&scenario).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, scenario)
}

// generateSlug creates a URL-friendly name from a title
func generateSlug(title string) string {
	slug := ""
	for _, c := range title {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			slug += string(c)
		} else if c >= 'A' && c <= 'Z' {
			slug += string(c - 'A' + 'a')
		} else if c == ' ' || c == '_' {
			slug += "-"
		}
	}
	// Remove consecutive and trailing dashes
	result := ""
	prev := byte(0)
	for i := 0; i < len(slug); i++ {
		if slug[i] == '-' && prev == '-' {
			continue
		}
		result += string(slug[i])
		prev = slug[i]
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return result
}
