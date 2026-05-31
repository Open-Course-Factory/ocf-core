package scenarioController

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ScenarioProgressController defines the handler methods for learner-facing
// scenario progress tracking (steps, verification, flags, quizzes, hints,
// abandon).
type ScenarioProgressController interface {
	GetCurrentStep(ctx *gin.Context)
	GetStepByOrder(ctx *gin.Context)
	VerifyStep(ctx *gin.Context)
	SubmitFlag(ctx *gin.Context)
	SubmitQuiz(ctx *gin.Context)
	RevealHint(ctx *gin.Context)
	AbandonSession(ctx *gin.Context)
	GetSessionFlags(ctx *gin.Context)
}

type scenarioProgressController struct {
	scenarioControllerBase
	sessionService  *services.ScenarioSessionService
	terminalService terminalServices.TerminalTrainerService
}

// NewScenarioProgressController creates a controller for scenario progress
// tracking with its service dependencies.
func NewScenarioProgressController(db *gorm.DB) ScenarioProgressController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	terminalService := terminalServices.NewTerminalTrainerService(db)

	// Wire terminal stop callback so the session service can stop terminals on setup failure
	sessionService.SetTerminalStopFunc(func(terminalSessionID string) error {
		return terminalService.StopSession(terminalSessionID)
	})

	return &scenarioProgressController{
		scenarioControllerBase: scenarioControllerBase{db: db},
		sessionService:         sessionService,
		terminalService:        terminalService,
	}
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
func (pc *scenarioProgressController) GetCurrentStep(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	step, err := pc.sessionService.GetCurrentStep(session.ID)
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
func (pc *scenarioProgressController) GetStepByOrder(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
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

	step, err := pc.sessionService.GetStepByOrder(session.ID, stepOrder)
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
func (pc *scenarioProgressController) VerifyStep(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	result, err := pc.sessionService.VerifyCurrentStep(session.ID)
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
func (pc *scenarioProgressController) SubmitFlag(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
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

	result, err := pc.sessionService.SubmitFlag(session.ID, input.Flag)
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

// SubmitQuiz godoc
// @Summary Submit quiz answers
// @Description Submit quiz answers for the current step. Each answer is scored against the canonical correct_answer; the response includes per-question correctness and an aggregate score.
// @Tags scenario-sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body dto.SubmitQuizInput true "Quiz answers"
// @Success 200 {object} dto.SubmitQuizResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 422 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenario-sessions/{id}/submit-quiz [post]
// @Security BearerAuth
func (pc *scenarioProgressController) SubmitQuiz(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	var input dto.SubmitQuizInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	result, err := pc.sessionService.SubmitQuiz(session.ID, input)
	if err != nil {
		// Service rejects with a domain error for invalid step type / unknown
		// question IDs / empty answers — surface as 422 so the frontend can
		// distinguish from a 500.
		slog.Warn("failed to submit quiz", "err", err)
		ctx.JSON(http.StatusUnprocessableEntity, &errors.APIError{
			ErrorCode:    http.StatusUnprocessableEntity,
			ErrorMessage: err.Error(),
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
func (pc *scenarioProgressController) RevealHint(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
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

	result, err := pc.sessionService.RevealHint(session.ID, stepOrder, level)
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
func (pc *scenarioProgressController) AbandonSession(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	err = pc.sessionService.AbandonSession(session.ID)
	if err != nil {
		slog.Error("failed to abandon session", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to abandon session",
		})
		return
	}

	// Stop the linked terminal session (best-effort, don't block the abandon)
	if session.TerminalSessionID != nil && *session.TerminalSessionID != "" {
		if stopErr := pc.terminalService.StopSession(*session.TerminalSessionID); stopErr != nil {
			slog.Warn("failed to stop terminal session on abandon", "terminal_session_id", *session.TerminalSessionID, "err", stopErr)
		}
	}

	ctx.JSON(http.StatusOK, dto.MessageResponse{Message: "Session abandoned"})
}

// GetSessionFlags returns all validated (correct) flags for a session.
func (pc *scenarioProgressController) GetSessionFlags(ctx *gin.Context) {
	session, err := pc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	var flags []models.ScenarioFlag
	pc.db.Where("session_id = ? AND is_correct = ?", session.ID, true).
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
