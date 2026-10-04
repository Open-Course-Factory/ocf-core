package scenarioController

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/dto"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetReadOnlySteps godoc
// @Summary Read a scenario's steps without editing rights
// @Description Every step as the editor reads it — scripts, hints and hint rows, quiz questions with answers, flag settings, effects, translations — and the setup script, for a user who may see the scenario but not edit it. Gated on CanSeeScenario and TeachesAnywhere, which every user with a personal organisation passes: any registered user reads a public scenario in full, as they could by duplicating it (accepted 2026-10-04 — public scenarios are public). Other organisations' private scenarios stay 404. The flag secret is never sent.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {object} dto.ReadOnlyStepsOutput
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/steps/read-only [get]
// @Security BearerAuth
func (sc *scenarioController) GetReadOnlySteps(ctx *gin.Context) {
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
			errors.Respond(ctx, http.StatusForbidden, "Only authors may read a scenario's steps")
			return
		}
	}

	steps, err := sc.readOnlySteps(scenario.ID, nil)
	if err != nil {
		slog.Error("failed to read scenario steps", "scenario_id", scenario.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to read the steps")
		return
	}
	ctx.JSON(http.StatusOK, dto.ReadOnlyStepsOutput{SetupScript: scenario.SetupScript, Steps: steps})
}

// CopySteps godoc
// @Summary Copy steps from other scenarios into this one
// @Description Copies the named steps — content, scripts, hints, quiz questions, translations — into the scenario, in the order given, starting at position (omitted: appended), and answers with the new steps. The caller must manage the target and be allowed to copy every source scenario into the target's organisation.
// @Tags scenarios
// @Accept json
// @Produce json
// @Param id path string true "Target scenario ID"
// @Param body body dto.CopyStepsInput true "Steps to copy and where"
// @Success 201 {object} dto.ReadOnlyStepsOutput
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

	copied := make([]uuid.UUID, len(copies))
	for i := range copies {
		copied[i] = copies[i].ID
	}
	steps, err := sc.readOnlySteps(target.ID, copied)
	if err != nil {
		slog.Error("failed to read copied steps", "target_id", target.ID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to read the copied steps")
		return
	}
	ctx.JSON(http.StatusCreated, dto.ReadOnlyStepsOutput{Steps: steps})
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

// readOnlySteps converts a scenario's steps — all of them, or only stepIDs —
// with the editor's converters, adding each step's hint rows and translations.
func (sc *scenarioController) readOnlySteps(scenarioID uuid.UUID, stepIDs []uuid.UUID) ([]dto.ReadOnlyStepOutput, error) {
	query := sc.db.
		Preload("Hints", func(db *gorm.DB) *gorm.DB { return db.Order("level ASC") }).
		Preload("Questions").
		Where("scenario_id = ?", scenarioID)
	if stepIDs != nil {
		query = query.Where("id IN ?", stepIDs)
	}
	var steps []models.ScenarioStep
	if err := query.Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	ids := make([]uuid.UUID, len(steps))
	for i := range steps {
		ids[i] = steps[i].ID
	}
	var translations []models.ScenarioStepTranslation
	if len(ids) > 0 {
		if err := sc.db.Where("step_id IN ?", ids).Order("locale ASC").Find(&translations).Error; err != nil {
			return nil, fmt.Errorf("load step translations: %w", err)
		}
	}
	translationsByStep := map[uuid.UUID][]dto.ScenarioStepTranslationOutput{}
	for i := range translations {
		translationsByStep[translations[i].StepID] = append(translationsByStep[translations[i].StepID],
			scenarioRegistration.ScenarioStepTranslationToOutput(&translations[i]))
	}

	out := make([]dto.ReadOnlyStepOutput, len(steps))
	for i := range steps {
		hints := make([]dto.ScenarioStepHintOutput, len(steps[i].Hints))
		for h := range steps[i].Hints {
			hints[h] = scenarioRegistration.ScenarioStepHintToOutput(&steps[i].Hints[h])
		}
		out[i] = dto.ReadOnlyStepOutput{
			ScenarioStepOutput: scenarioRegistration.ScenarioStepToOutput(&steps[i]),
			Hints:              hints,
			Translations:       translationsByStep[steps[i].ID],
		}
	}
	return out, nil
}
