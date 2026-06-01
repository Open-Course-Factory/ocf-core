package scenarioController

import (
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// scenarioControllerBase holds dependencies and helpers shared by the
// scenario controllers (the main scenarioController and the focused
// scenarioProgressController). Both controllers embed it so they can reach
// helpers such as getSessionIfOwned without duplication.
type scenarioControllerBase struct {
	db *gorm.DB
}

// getSessionIfOwned loads a session by ID and checks that the authenticated user owns it.
func (b *scenarioControllerBase) getSessionIfOwned(ctx *gin.Context) (*models.ScenarioSession, error) {
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
	if err := b.db.First(&session, "id = ?", sessionID).Error; err != nil {
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

// hasAdminRole checks if the context has admin/administrator role without writing a response.
func (b *scenarioControllerBase) hasAdminRole(ctx *gin.Context) bool {
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

// buildScenarioOutput converts a Scenario model to a ScenarioOutput DTO
func (b *scenarioControllerBase) buildScenarioOutput(scenario *models.Scenario) dto.ScenarioOutput {
	output := dto.ScenarioOutput{
		ID:               scenario.ID,
		Name:             scenario.Name,
		Title:            scenario.Title,
		Description:      scenario.Description,
		Difficulty:       scenario.Difficulty,
		EstimatedTime:    scenario.EstimatedTime,
		InstanceType:     scenario.InstanceType,
		OsType:           scenario.OsType,
		SourceType:       scenario.SourceType,
		FlagsEnabled:     scenario.FlagsEnabled,
		AllowedFlagPaths: scenario.AllowedFlagPaths,
		GshEnabled:       scenario.GshEnabled,
		CrashTraps:       scenario.CrashTraps,
		IntroText:        scenario.IntroText,
		FinishText:       scenario.FinishText,
		CreatedByID:      scenario.CreatedByID,
		OrganizationID:   scenario.OrganizationID,
		CreatedAt:        scenario.CreatedAt,
		UpdatedAt:        scenario.UpdatedAt,
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
