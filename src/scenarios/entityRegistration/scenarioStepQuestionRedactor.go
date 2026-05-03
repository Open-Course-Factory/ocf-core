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

// scenarioStepQuestionRedactor strips CorrectAnswer + Explanation from a
// ScenarioStepQuestionOutput DTO when the requesting user is NOT authorized
// to manage the parent scenario.
//
// Authorization is transitive — same chain as the question authorization
// hook (scenarioHooks.ScenarioStepQuestionAuthorizationHook):
//
//   question.StepID → ScenarioStep.ScenarioID → Scenario → CanManageScenario
//
// The DTO has no parent-scenario field, so two DB lookups are required.
func scenarioStepQuestionRedactor(c *gin.Context, dtoPtr any, db *gorm.DB) error {
	wrapper, ok := dtoPtr.(*any)
	if !ok {
		return nil
	}
	output, ok := (*wrapper).(dto.ScenarioStepQuestionOutput)
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
		stripScenarioStepQuestionDto(&output)
		*wrapper = output
		return nil
	}

	if db == nil {
		stripScenarioStepQuestionDto(&output)
		*wrapper = output
		return nil
	}

	// Hop 1: question.StepID → ScenarioStep
	var step models.ScenarioStep
	if err := db.Where("id = ?", output.StepID).First(&step).Error; err != nil {
		stripScenarioStepQuestionDto(&output)
		*wrapper = output
		return nil
	}

	// Hop 2: step.ScenarioID → Scenario
	var scenario models.Scenario
	if err := db.Where("id = ?", step.ScenarioID).First(&scenario).Error; err != nil {
		stripScenarioStepQuestionDto(&output)
		*wrapper = output
		return nil
	}

	groupSvc := groupServices.NewGroupService(db)
	allowed, err := scenarioHooks.CanManageScenario(db, groupSvc, &scenario, userID)
	if err != nil {
		return fmt.Errorf("scenarioStepQuestionRedactor: check manage permission: %w", err)
	}
	if allowed {
		return nil
	}

	stripScenarioStepQuestionDto(&output)
	*wrapper = output
	return nil
}

// stripScenarioStepQuestionDto zeros CorrectAnswer + Explanation in place.
// JSON `omitempty` drops the empty strings entirely from the response.
// QuestionText, Options, Order, Points stay visible — non-managers can see
// the quiz, just not the answer key.
func stripScenarioStepQuestionDto(out *dto.ScenarioStepQuestionOutput) {
	out.CorrectAnswer = ""
	out.Explanation = ""
}
