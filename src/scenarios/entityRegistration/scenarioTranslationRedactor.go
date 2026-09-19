package scenarioRegistration

import (
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Translations carry the same text the step and scenario redactors strip —
// hint_content above all — so a non-manager gets the row's identity (id,
// parent, locale) and nothing of what it says.

func scenarioTranslationRedactor(c *gin.Context, dtoPtr any, db *gorm.DB) error {
	return redactUnlessManager(c, dtoPtr, db, "scenarioTranslationRedactor", scenarioOfTranslationOutput, stripScenarioTranslationDto)
}

func scenarioStepTranslationRedactor(c *gin.Context, dtoPtr any, db *gorm.DB) error {
	return redactUnlessManager(c, dtoPtr, db, "scenarioStepTranslationRedactor", scenarioOfStepTranslationOutput, stripScenarioStepTranslationDto)
}

func scenarioOfTranslationOutput(db *gorm.DB, output *dto.ScenarioTranslationOutput) (*models.Scenario, error) {
	var scenario models.Scenario
	err := db.Where("id = ?", output.ScenarioID).First(&scenario).Error
	return &scenario, err
}

func scenarioOfStepTranslationOutput(db *gorm.DB, output *dto.ScenarioStepTranslationOutput) (*models.Scenario, error) {
	var step models.ScenarioStep
	if err := db.Where("id = ?", output.StepID).First(&step).Error; err != nil {
		return nil, err
	}
	var scenario models.Scenario
	err := db.Where("id = ?", step.ScenarioID).First(&scenario).Error
	return &scenario, err
}

func stripScenarioTranslationDto(out *dto.ScenarioTranslationOutput) {
	out.Title, out.Description, out.Objectives, out.Prerequisites, out.IntroText, out.FinishText = "", "", "", "", "", ""
}

func stripScenarioStepTranslationDto(out *dto.ScenarioStepTranslationOutput) {
	out.Title, out.TextContent, out.HintContent, out.IntroText, out.OutroText = "", "", "", "", ""
}
