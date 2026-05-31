package scenarioController

import (
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
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
