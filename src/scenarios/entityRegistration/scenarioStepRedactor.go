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

// scenarioStepRedactor strips sensitive step fields and embedded question
// content from a ScenarioStepOutput DTO when the requesting user is NOT
// authorized to manage the parent scenario (per scenarioHooks.CanManageScenario,
// with admin bypass).
//
// Sensitive step fields (issue #293):
//   - HintContent — may include flag-revealing hints
//   - FlagPath, FlagLevel — CTF flag metadata
//   - VerifyScriptID, BackgroundScriptID, ForegroundScriptID — script UUIDs
//     enable enumeration of solution scripts
//   - TextFileID, HintFileID — project-file UUIDs
//   - Questions slice — contains CorrectAnswer + Explanation
//
// Note: ScenarioStep has DefaultIncludes=["Questions"], so direct
// GET /scenario-steps/:id always preloads Questions. Stripping the slice
// here covers both the default case and any `?include=Questions` request.
func scenarioStepRedactor(c *gin.Context, dtoPtr any, db *gorm.DB) error {
	wrapper, ok := dtoPtr.(*any)
	if !ok {
		return nil
	}
	output, ok := (*wrapper).(dto.ScenarioStepOutput)
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
		stripScenarioStepDto(&output)
		*wrapper = output
		return nil
	}

	if db == nil {
		stripScenarioStepDto(&output)
		*wrapper = output
		return nil
	}

	// Look up the parent scenario via the step's ScenarioID. The DTO does
	// not carry CreatedByID / OrganizationID, so we must hit the DB.
	var scenario models.Scenario
	if err := db.Where("id = ?", output.ScenarioID).First(&scenario).Error; err != nil {
		// Parent scenario missing — fail closed.
		stripScenarioStepDto(&output)
		*wrapper = output
		return nil
	}

	groupSvc := groupServices.NewGroupService(db)
	allowed, err := scenarioHooks.CanManageScenario(db, groupSvc, &scenario, userID)
	if err != nil {
		return fmt.Errorf("scenarioStepRedactor: check manage permission: %w", err)
	}
	if allowed {
		return nil
	}

	stripScenarioStepDto(&output)
	*wrapper = output
	return nil
}

// stripScenarioStepDto zeros the sensitive fields on a ScenarioStepOutput in
// place. JSON `omitempty` drops empty strings, zero ints, nil pointers, and
// nil slices, so the redacted fields disappear entirely from the response.
//
// Note: although models.ScenarioStep tags VerifyScript/BackgroundScript/
// ForegroundScript with `json:"-"`, the response is marshalled from
// dto.ScenarioStepOutput, which redeclares those fields with
// `json:"verify_script,omitempty"` (etc.) and the converter in
// scenarioStepRegistration.go explicitly copies the raw script bodies into
// the DTO. The DTO's tag wins at marshal time, so we MUST zero the bodies
// here — otherwise the verify-script (i.e. the grading logic / answer key)
// leaks to any authenticated learner.
func stripScenarioStepDto(out *dto.ScenarioStepOutput) {
	out.HintContent = ""
	out.VerifyScript = ""
	out.BackgroundScript = ""
	out.ForegroundScript = ""
	out.FlagPath = ""
	out.FlagLevel = 0
	out.VerifyScriptID = nil
	out.BackgroundScriptID = nil
	out.ForegroundScriptID = nil
	out.TextFileID = nil
	out.HintFileID = nil
	// Drop the Questions slice entirely so embedded CorrectAnswer +
	// Explanation never reach a non-manager.
	out.Questions = nil
}
