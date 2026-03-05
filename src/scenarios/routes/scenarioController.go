package scenarioController

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioController defines the handler methods for scenario-related endpoints
type ScenarioController interface {
	ImportScenario(ctx *gin.Context)
	StartScenario(ctx *gin.Context)
	GetCurrentStep(ctx *gin.Context)
	VerifyStep(ctx *gin.Context)
	SubmitFlag(ctx *gin.Context)
	AbandonSession(ctx *gin.Context)
}

type scenarioController struct {
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
		sessionService:  sessionService,
		importerService: importerService,
	}
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

// StartScenario handles POST /scenarios/:id/start
func (sc *scenarioController) StartScenario(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	session, err := sc.sessionService.StartScenario(userID, scenarioID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, session)
}

// GetCurrentStep handles GET /scenario-sessions/:id/current-step
func (sc *scenarioController) GetCurrentStep(ctx *gin.Context) {
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid session ID",
		})
		return
	}

	step, err := sc.sessionService.GetCurrentStep(sessionID)
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
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid session ID",
		})
		return
	}

	result, err := sc.sessionService.VerifyCurrentStep(sessionID)
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
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid session ID",
		})
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

	result, err := sc.sessionService.SubmitFlag(sessionID, input.Flag)
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
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid session ID",
		})
		return
	}

	err = sc.sessionService.AbandonSession(sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Session abandoned"})
}
