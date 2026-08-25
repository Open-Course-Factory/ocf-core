package services

import (
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// StepText is the prose a learner reads on a step, in their session's language.
type StepText struct {
	Title string
	Text  string
	Hint  string
}

// ResolveStepText assembles a step's prose for one locale.
//
// Two layers, in order: the step's own content — which may itself live in a
// ProjectFile, hence ResolveScriptContent — and then the locale's translation
// laid over it.
//
// A field the translation leaves empty keeps the default. An empty column means
// the field has not been translated yet, and treating it as an instruction to
// serve nothing would turn a half-finished translation into blank steps. The
// same reasoning as withoutUnseenScripts on the editor side: "" is absence, not
// a value.
//
// An empty locale — every session that exists today — short-circuits before the
// query, so nothing pays for a feature it does not use.
func ResolveStepText(db *gorm.DB, step models.ScenarioStep, locale string) StepText {
	text := StepText{
		Title: step.Title,
		Text:  ResolveScriptContent(db, step.TextFileID, step.TextContent),
		Hint:  ResolveScriptContent(db, step.HintFileID, step.HintContent),
	}
	if locale == "" {
		return text
	}

	var translation models.ScenarioStepTranslation
	if err := db.Where("step_id = ? AND locale = ?", step.ID, locale).
		First(&translation).Error; err != nil {
		return text
	}

	overlay(&text.Title, translation.Title)
	overlay(&text.Text, translation.TextContent)
	overlay(&text.Hint, translation.HintContent)
	return text
}

func overlay(field *string, translated string) {
	if translated != "" {
		*field = translated
	}
}
