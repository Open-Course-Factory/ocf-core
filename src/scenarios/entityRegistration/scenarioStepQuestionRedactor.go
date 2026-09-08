package scenarioRegistration

import (
	"soli/formations/src/scenarios/dto"
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
	return redactUnlessManager(c, dtoPtr, db, "scenarioStepQuestionRedactor", scenarioOfQuestionOutput, stripScenarioStepQuestionDto)
}

// scenarioOfQuestionOutput resolves question.StepID → ScenarioStep.ScenarioID → Scenario.
func scenarioOfQuestionOutput(db *gorm.DB, output *dto.ScenarioStepQuestionOutput) (*models.Scenario, error) {
	var step models.ScenarioStep
	if err := db.Where("id = ?", output.StepID).First(&step).Error; err != nil {
		return nil, err
	}
	var scenario models.Scenario
	err := db.Where("id = ?", step.ScenarioID).First(&scenario).Error
	return &scenario, err
}

// stripScenarioStepQuestionDto zeros CorrectAnswer + Explanation in place.
// JSON `omitempty` drops the empty strings entirely from the response.
// QuestionText, Options, Order, Points stay visible — non-managers can see
// the quiz, just not the answer key.
func stripScenarioStepQuestionDto(out *dto.ScenarioStepQuestionOutput) {
	out.CorrectAnswer = ""
	out.Explanation = ""
}
