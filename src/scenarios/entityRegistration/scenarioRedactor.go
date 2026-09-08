package scenarioRegistration

import (
	"fmt"

	"soli/formations/src/auth/access"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/dto"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// scenarioRedactor strips sensitive scenario fields and step + question
// content from a ScenarioOutput DTO when the requesting user is NOT
// authorized to manage the scenario (per scenarioHooks.CanManageScenario,
// with admin bypass).
//
// Sensitive fields exposed by the leak (issue #293):
//   - Scenario.SetupScript, SetupScriptID, IntroFileID, FinishFileID
//   - Step.HintContent, FlagPath, FlagLevel
//   - Step.VerifyScriptID, BackgroundScriptID, ForegroundScriptID
//   - Step.TextFileID, HintFileID
//   - Question.CorrectAnswer, Explanation (entire Questions slice)
//
// The simplest correct redaction is to drop the Steps slice entirely — JSON
// `omitempty` then keeps the field out of the response. The scenario header
// (name, title, description, etc.) remains visible so listings still work.
//
// The redactor is invoked by the generic GET handlers (single + list) AFTER
// model→DTO conversion, so it works regardless of any `?include=Steps.Questions`
// the client sends — fixing the leak even when default preloads or explicit
// includes have already populated the steps in the model.
func scenarioRedactor(c *gin.Context, dtoPtr any, db *gorm.DB) error {
	return redactUnlessManager(c, dtoPtr, db, "scenarioRedactor", scenarioFromOutput, stripScenarioDto)
}

// scenarioFromOutput builds a thin Scenario for the manage check from the
// scope fields the DTO already carries — no DB round-trip needed.
func scenarioFromOutput(_ *gorm.DB, output *dto.ScenarioOutput) (*models.Scenario, error) {
	scenario := &models.Scenario{}
	scenario.ID = output.ID
	scenario.CreatedByID = output.CreatedByID
	scenario.OrganizationID = output.OrganizationID
	return scenario, nil
}

// redactUnlessManager is the skeleton shared by the scenario, step and
// question redactors: unwrap the handler's &entityDto (an interface holding
// T), let admins through, resolve the parent scenario, and strip the DTO in
// place unless the user can manage that scenario (scenarioHooks.CanManageScenario).
//
// Fail-closed rules: no identified user, no DB, or an unresolvable parent
// scenario all strip. A wrapper of an unexpected type is left untouched —
// this is a redaction layer, not validation. Only a failing manage check
// surfaces as an error, prefixed with name for log attribution.
func redactUnlessManager[T any](
	c *gin.Context,
	dtoPtr any,
	db *gorm.DB,
	name string,
	parentScenario func(*gorm.DB, *T) (*models.Scenario, error),
	strip func(*T),
) error {
	wrapper, ok := dtoPtr.(*any)
	if !ok {
		return nil
	}
	output, ok := (*wrapper).(T)
	if !ok {
		return nil
	}

	if access.IsAdmin(readRoles(c)) {
		return nil
	}

	stripAndStore := func() {
		strip(&output)
		*wrapper = output
	}

	userID := c.GetString("userId")
	if userID == "" || db == nil {
		stripAndStore()
		return nil
	}

	scenario, err := parentScenario(db, &output)
	if err != nil {
		stripAndStore()
		return nil
	}

	groupSvc := groupServices.NewGroupService(db)
	allowed, err := scenarioHooks.CanManageScenario(db, groupSvc, scenario, userID)
	if err != nil {
		return fmt.Errorf("%s: check manage permission: %w", name, err)
	}
	if allowed {
		return nil
	}

	stripAndStore()
	return nil
}

// stripScenarioDto clears the sensitive parts of a ScenarioOutput in place.
// Steps is the umbrella container for HintContent, FlagPath, script IDs,
// file IDs, and the entire Questions slice (with CorrectAnswer + Explanation).
// Setting it to nil makes JSON `omitempty` drop the field entirely.
//
// Top-level scenario fields that also leak setup-time secrets:
//   - SetupScript: shell script that may contain secrets / cleanup commands
//   - SetupScriptID, IntroFileID, FinishFileID: project-file UUIDs that
//     enable enumeration of internal artifacts
func stripScenarioDto(out *dto.ScenarioOutput) {
	out.Steps = nil
	out.SetupScript = ""
	out.SetupScriptID = nil
	out.IntroFileID = nil
	out.FinishFileID = nil
}

// readRoles extracts the roles slice from the gin context, tolerating
// both []string and missing-key cases.
func readRoles(c *gin.Context) []string {
	v, exists := c.Get("userRoles")
	if !exists {
		return nil
	}
	if rs, ok := v.([]string); ok {
		return rs
	}
	return nil
}
