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
	// The handler passes &entityDto (interface holding ScenarioOutput).
	// Unwrap to the concrete value.
	wrapper, ok := dtoPtr.(*any)
	if !ok {
		// Defensive: if the contract changes, do not panic — just leave
		// the DTO untouched. This is a redaction layer, not validation.
		return nil
	}
	output, ok := (*wrapper).(dto.ScenarioOutput)
	if !ok {
		return nil
	}

	// Admin always sees full content.
	roles := readRoles(c)
	if access.IsAdmin(roles) {
		return nil
	}

	userID := c.GetString("userId")
	if userID == "" {
		// No identified user — strip defensively.
		stripScenarioDto(&output)
		*wrapper = output
		return nil
	}

	if db == nil {
		// Without DB we cannot run the manage check — strip defensively.
		stripScenarioDto(&output)
		*wrapper = output
		return nil
	}

	// Reload a thin Scenario model for the manage check (the DTO already
	// has the scope fields, but CanManageScenario takes *models.Scenario).
	scenario := &models.Scenario{}
	scenario.ID = output.ID
	scenario.CreatedByID = output.CreatedByID
	scenario.OrganizationID = output.OrganizationID

	groupSvc := groupServices.NewGroupService(db)
	allowed, err := scenarioHooks.CanManageScenario(db, groupSvc, scenario, userID)
	if err != nil {
		return fmt.Errorf("scenarioRedactor: check manage permission: %w", err)
	}
	if allowed {
		return nil
	}

	stripScenarioDto(&output)
	*wrapper = output
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
