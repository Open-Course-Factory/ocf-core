package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// Translations carry a locale's wording for content the learner reads. They sit
// beside the entity rather than inside it: the row itself keeps the scenario's
// default locale, so every reader that does not care about language keeps
// working and nothing existing has to be migrated.
//
// Only prose lives here. Scripts do not — they are one logic shared by every
// locale, and the world names they use come from the scenario's lexicon. A
// translated verify script would be a fork, which is the whole thing this
// design exists to avoid.
//
// An empty column means "not translated yet", never "serve nothing". Readers
// fall back to the default field, so a half-finished translation degrades to
// the original wording instead of showing a learner a blank step.

type ScenarioTranslation struct {
	entityManagementModels.BaseModel
	ScenarioID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_scenario_translation_locale" json:"scenario_id"`
	Locale        string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_scenario_translation_locale" json:"locale"`
	Title         string    `gorm:"type:varchar(500)" json:"title,omitempty"`
	Description   string    `gorm:"type:text" json:"description,omitempty"`
	Objectives    string    `gorm:"type:text" json:"objectives,omitempty"`
	Prerequisites string    `gorm:"type:text" json:"prerequisites,omitempty"`
	IntroText     string    `gorm:"type:text" json:"intro_text,omitempty"`
	FinishText    string    `gorm:"type:text" json:"finish_text,omitempty"`
}

func (s ScenarioTranslation) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioTranslation) GetReferenceObject() string {
	return "ScenarioTranslation"
}

func (ScenarioTranslation) TableName() string {
	return "scenario_translations"
}

type ScenarioStepTranslation struct {
	entityManagementModels.BaseModel
	StepID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_step_translation_locale" json:"step_id"`
	Locale      string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_step_translation_locale" json:"locale"`
	Title       string    `gorm:"type:varchar(500)" json:"title,omitempty"`
	TextContent string    `gorm:"type:text" json:"text_content,omitempty"`
	HintContent string    `gorm:"type:text" json:"hint_content,omitempty"`
	IntroText   string    `gorm:"type:varchar(500)" json:"intro_text,omitempty"`
	OutroText   string    `gorm:"type:varchar(500)" json:"outro_text,omitempty"`

	// SourceHash is the default-locale text this translation was written
	// against. Without it nobody ever learns that the source was edited
	// afterwards, and the translation rots in silence — reading correctly,
	// describing something that has changed.
	SourceHash string `gorm:"type:varchar(64)" json:"source_hash,omitempty"`
}

func (s ScenarioStepTranslation) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStepTranslation) GetReferenceObject() string {
	return "ScenarioStepTranslation"
}

func (ScenarioStepTranslation) TableName() string {
	return "scenario_step_translations"
}
