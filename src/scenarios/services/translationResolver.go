package services

import (
	"soli/formations/src/scenarios/dto"
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

// ScenarioProse is everything a learner reads that is not a step: the card's
// words, the briefing that opens the session, and the text that closes it.
type ScenarioProse struct {
	Title       string
	Description string
	Intro       string
	Finish      string
}

// ResolveScenarioText assembles a scenario's own prose for one locale.
//
// The steps already resolve this way, and the briefing did not: it was fetched
// straight off the scenario, so a session played in French opened with an
// English welcome and closed with an English farewell. Same text, two paths,
// one of which forgot — so this is the path, and the card's per-locale map
// below lays its two fields over the same rule.
//
// Empty locale short-circuits, and an untranslated field keeps the original:
// prose that falls back is merely untranslated, which is not the same hazard as
// a path that falls back and points at a directory nobody has.
func ResolveScenarioText(db *gorm.DB, scenario models.Scenario, locale string) ScenarioProse {
	prose := ScenarioProse{
		Title:       scenario.Title,
		Description: scenario.Description,
		Intro:       ResolveScriptContent(db, scenario.IntroFileID, scenario.IntroText),
		Finish:      ResolveScriptContent(db, scenario.FinishFileID, scenario.FinishText),
	}
	if locale == "" {
		return prose
	}

	var translation models.ScenarioTranslation
	if err := db.Where("scenario_id = ? AND locale = ?", scenario.ID, locale).
		First(&translation).Error; err != nil {
		return prose
	}

	applyScenarioTranslation(&prose, translation)
	return prose
}

// applyScenarioTranslation is the one place a scenario's translation is laid
// over its defaults, so the card and the player cannot disagree about what a
// half-filled translation means.
func applyScenarioTranslation(prose *ScenarioProse, translation models.ScenarioTranslation) {
	overlay(&prose.Title, translation.Title)
	overlay(&prose.Description, translation.Description)
	overlay(&prose.Intro, translation.IntroText)
	overlay(&prose.Finish, translation.FinishText)
}

// ScenarioTextByLocale gives a card its text in every language it is offered
// in, keyed by locale.
//
// All of them at once, because the picker sits on the card: a learner changing
// a dropdown expects the words beside it to change, and a card that blanked
// while it asked the server would make choosing a language feel like loading a
// page.
//
// An untranslated field falls back to the original. On a card that is the right
// answer — a description still in English is readable, and the alternative is a
// blank card for a language whose steps are all translated. It differs from the
// world's names, where falling back would point at a directory that does not
// exist.
//
// Empty for a single-language scenario: there is no choice to preview.
func ScenarioTextByLocale(db *gorm.DB, scenario models.Scenario) (map[string]dto.ScenarioText, error) {
	locales, err := scenario.GetLocales()
	if err != nil || len(locales) < 2 {
		return nil, err
	}

	var rows []models.ScenarioTranslation
	if err := db.Where("scenario_id = ?", scenario.ID).Find(&rows).Error; err != nil {
		return nil, err
	}
	byLocale := make(map[string]models.ScenarioTranslation, len(rows))
	for _, row := range rows {
		byLocale[row.Locale] = row
	}

	text := make(map[string]dto.ScenarioText, len(locales))
	for _, locale := range locales {
		prose := ScenarioProse{Title: scenario.Title, Description: scenario.Description}
		if translation, ok := byLocale[locale]; ok {
			applyScenarioTranslation(&prose, translation)
		}
		// The card shows two of the four; the rows are read in one query here
		// rather than per locale, because this runs over the whole catalogue.
		text[locale] = dto.ScenarioText{Title: prose.Title, Description: prose.Description}
	}
	return text, nil
}
