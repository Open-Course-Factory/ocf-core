package scenarioController

import (
	stderrors "errors"
	"log/slog"
	"net/http"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetStepOutline godoc
// @Summary Outline a scenario's steps for an author
// @Description What each step of a scenario is (title, type, text, translations, hint and question counts) without how it is graded: no script, hint, flag path or answer. For authors deciding which steps to copy; a learner is refused, to whom the outline is a walkthrough.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {array} dto.ScenarioStepOutline
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/step-outline [get]
// @Security BearerAuth
func (sc *scenarioController) GetStepOutline(ctx *gin.Context) {
	scenario, ok := sc.loadVisibleScenario(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	if !access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		teaches, err := scenarioHooks.TeachesAnywhere(sc.db, ctx.GetString("userId"))
		if err != nil {
			slog.Error("failed to check whether the user teaches", "err", err)
			errors.Respond(ctx, http.StatusInternalServerError, "Internal error")
			return
		}
		if !teaches {
			errors.Respond(ctx, http.StatusForbidden, "Only authors may outline a scenario's steps")
			return
		}
	}

	outline, err := services.StepOutline(sc.db, scenario.ID)
	if err != nil {
		slog.Error("failed to outline scenario steps", "scenario_id", scenario.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to outline the steps")
		return
	}
	ctx.JSON(http.StatusOK, outline)
}

// CopySteps godoc
// @Summary Copy steps from other scenarios into this one
// @Description Copies the named steps — content, scripts, hints, quiz questions, translations — into the scenario, in the order given, starting at position (omitted: appended). The copy is made server-side; the response outlines the new steps without their scripts or answers. The caller must manage the target and be allowed to copy every source scenario into the target's organisation.
// @Tags scenarios
// @Accept json
// @Produce json
// @Param id path string true "Target scenario ID"
// @Param body body dto.CopyStepsInput true "Steps to copy and where"
// @Success 201 {array} dto.ScenarioStepOutline
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/steps/copy [post]
// @Security BearerAuth
func (sc *scenarioController) CopySteps(ctx *gin.Context) {
	target := sc.loadManageableScenario(ctx)
	if target == nil {
		return
	}
	var input dto.CopyStepsInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		errors.Respond(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !sc.mayCopyStepsInto(ctx, input.SourceStepIDs, target) {
		return
	}

	copies, err := sc.duplicateService.CopySteps(target.ID, input.SourceStepIDs, input.Position)
	switch {
	case stderrors.Is(err, services.ErrNoStepsToCopy), stderrors.Is(err, services.ErrStepPositionOutOfRange):
		errors.Respond(ctx, http.StatusBadRequest, err.Error())
		return
	case stderrors.Is(err, services.ErrSourceStepNotFound):
		errors.Respond(ctx, http.StatusNotFound, "Step not found")
		return
	case err != nil:
		slog.Error("failed to copy steps", "target_id", target.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to copy the steps")
		return
	}

	outline, err := services.StepOutline(sc.db, target.ID)
	if err != nil {
		slog.Error("failed to outline copied steps", "target_id", target.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to outline the copied steps")
		return
	}
	ctx.JSON(http.StatusCreated, outlineOf(outline, copies))
}

// mayCopyStepsInto applies CanCopyScenarioInto — the duplicate rule — to the
// scenario of every source step, platform admins excepted. A step the caller
// may not copy is reported as absent, like a scenario on the duplicate routes.
// It answers the request itself when it returns false.
func (sc *scenarioController) mayCopyStepsInto(ctx *gin.Context, stepIDs []uuid.UUID, target *models.Scenario) bool {
	scenarioIDs, err := services.SourceScenarioIDsOfSteps(sc.db, stepIDs)
	if stderrors.Is(err, services.ErrSourceStepNotFound) {
		errors.Respond(ctx, http.StatusNotFound, "Step not found")
		return false
	}
	if err != nil {
		slog.Error("failed to load source steps", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to copy the steps")
		return false
	}
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return true
	}
	var sources []models.Scenario
	if err := sc.db.Where("id IN ?", scenarioIDs).Find(&sources).Error; err != nil {
		slog.Error("failed to load source scenarios", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to copy the steps")
		return false
	}
	for i := range sources {
		allowed, err := scenarioHooks.CanCopyScenarioInto(sc.db, sc.groupService, &sources[i], target.OrganizationID, ctx.GetString("userId"))
		if err != nil {
			slog.Error("failed to check whether the steps may be copied", "err", err)
			errors.Respond(ctx, http.StatusInternalServerError, "Failed to copy the steps")
			return false
		}
		if !allowed {
			errors.Respond(ctx, http.StatusNotFound, "Step not found")
			return false
		}
	}
	return true
}

// loadVisibleScenario loads the scenario named by idParam if the caller may
// see it (CanSeeScenario, platform admins excepted), and reports an unseen one
// as absent. It answers the request itself when it returns false.
func (sc *scenarioController) loadVisibleScenario(ctx *gin.Context, idParam string) (*models.Scenario, bool) {
	scenarioID, err := uuid.Parse(idParam)
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid scenario ID")
		return nil, false
	}
	var scenario models.Scenario
	if err := sc.db.First(&scenario, "id = ?", scenarioID).Error; err != nil {
		errors.Respond(ctx, http.StatusNotFound, "Scenario not found")
		return nil, false
	}
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return &scenario, true
	}
	visible, err := scenarioHooks.CanSeeScenario(sc.db, sc.groupService, &scenario, ctx.GetString("userId"))
	if err != nil {
		slog.Error("failed to check scenario visibility", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Internal error")
		return nil, false
	}
	if !visible {
		errors.Respond(ctx, http.StatusNotFound, "Scenario not found")
		return nil, false
	}
	return &scenario, true
}

// outlineOf keeps the outline entries of the given steps.
func outlineOf(outline []dto.ScenarioStepOutline, steps []models.ScenarioStep) []dto.ScenarioStepOutline {
	wanted := make(map[uuid.UUID]bool, len(steps))
	for _, step := range steps {
		wanted[step.ID] = true
	}
	kept := make([]dto.ScenarioStepOutline, 0, len(steps))
	for _, entry := range outline {
		if wanted[entry.ID] {
			kept = append(kept, entry)
		}
	}
	return kept
}
