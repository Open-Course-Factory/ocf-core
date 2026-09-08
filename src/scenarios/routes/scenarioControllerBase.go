package scenarioController

import (
	"fmt"
	"net/http"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	groupServices "soli/formations/src/groups/services"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// scenarioControllerBase holds dependencies and helpers shared by the four
// scenario controllers (main, management, launch, progress). They embed it so
// they can reach helpers such as getSessionIfOwned and canManageScenarioByID
// without duplication.
type scenarioControllerBase struct {
	db           *gorm.DB
	groupService groupServices.GroupService
}

func newScenarioControllerBase(db *gorm.DB) scenarioControllerBase {
	return scenarioControllerBase{db: db, groupService: groupServices.NewGroupService(db)}
}

// canManageScenarioByID loads the scenario and answers whether the caller may
// manage it: a platform admin always may, anyone else through
// CanManageScenario (creator, org manager, manager of an assigned group). It
// is the one rule behind PATCH, DELETE, archive and every export, so a handler
// that must refuse on its own (defense in depth behind Layer 2) calls this
// rather than restating a narrower check. A missing scenario comes back as
// gorm.ErrRecordNotFound.
func (b *scenarioControllerBase) canManageScenarioByID(ctx *gin.Context, scenarioID uuid.UUID) (*models.Scenario, bool, error) {
	var scenario models.Scenario
	if err := b.db.Where("id = ?", scenarioID).First(&scenario).Error; err != nil {
		return nil, false, err
	}

	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if access.IsAdmin(roles) {
		return &scenario, true, nil
	}

	allowed, err := scenarioHooks.CanManageScenario(b.db, b.groupService, &scenario, ctx.GetString("userId"))
	if err != nil {
		return nil, false, err
	}
	return &scenario, allowed, nil
}

// getSessionIfOwned loads a session by ID and checks that the authenticated user owns it.
func (b *scenarioControllerBase) getSessionIfOwned(ctx *gin.Context) (*models.ScenarioSession, error) {
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid session ID")
		return nil, err
	}

	userID := ctx.GetString("userId")

	var session models.ScenarioSession
	if err := b.db.First(&session, "id = ?", sessionID).Error; err != nil {
		errors.Respond(ctx, http.StatusNotFound, "Session not found")
		return nil, err
	}

	if session.UserID != userID {
		errors.Respond(ctx, http.StatusForbidden, "You do not own this session")
		return nil, fmt.Errorf("forbidden")
	}

	return &session, nil
}

// rejectIfArchived answers the request and reports true when the scenario has
// been retired. Archiving stops new runs only — sessions already in flight are
// left to finish, so this belongs on the launch entry points and not on the
// session-progress routes.
func (b *scenarioControllerBase) rejectIfArchived(ctx *gin.Context, scenario *models.Scenario) bool {
	if !scenario.IsArchived() {
		return false
	}
	errors.Respond(ctx, http.StatusConflict, models.ErrScenarioArchived.Error())
	return true
}
