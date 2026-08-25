package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// A scenario's lexicon is the vocabulary its world is built from: one entry per
// object, one name per language.
//
// It exists so that a scenario's scripts can stop naming rooms. A verify script
// that says `[ "$here" = "$P_CELLAR" ]` works in every language; one that says
// `/World/Castle/Cellar` has to be forked to be translated, and two copies of a
// check drift the first time one is fixed.
//
// Entries form a tree, and paths are derived from it rather than stored. That
// is the whole reason for the shape: composing "the cellar is inside the castle"
// once means a translator supplies names, never paths, and cannot put the French
// cellar somewhere the English one is not.
type ScenarioLexiconEntry struct {
	entityManagementModels.BaseModel
	ScenarioID uuid.UUID `gorm:"type:uuid;not null;index" json:"scenario_id"`

	// Key is how scripts refer to this object: CELLAR becomes $W_CELLAR for the
	// name and $P_CELLAR for the path. Stable across languages by definition —
	// renaming a key is a change to the scripts, not to a translation.
	Key string `gorm:"type:varchar(120);not null;index:idx_lexicon_scenario_key,unique" json:"key"`

	// ParentKey is the entry this one sits inside. Empty means directly under
	// the world root.
	ParentKey string `gorm:"type:varchar(120)" json:"parent_key,omitempty"`

	// Kind separates what a name is for, because the rules differ:
	//
	//   place — a directory or file. Typed into a shell by beginners, so it must
	//           stay ASCII and space-free whatever language it is in.
	//   token — text living inside a world file, matched by grep or sed. Free to
	//           carry accents; it is read, not typed as a path.
	//   stem  — a fragment that must survive inside other names, for the case
	//           where one search has to find several objects at once.
	Kind string `gorm:"type:varchar(20);not null;default:'place'" json:"kind"`

	// Position orders siblings, so a generated file and an editor list agree.
	Position int `gorm:"default:0" json:"position"`

	Names []ScenarioLexiconName `gorm:"foreignKey:EntryID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"names,omitempty"`
}

func (s ScenarioLexiconEntry) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioLexiconEntry) GetReferenceObject() string { return "ScenarioLexiconEntry" }

func (ScenarioLexiconEntry) TableName() string { return "scenario_lexicon_entries" }

// ScenarioLexiconName is what one language calls one entry.
//
// A missing name is not an empty name: a language that has not named an object
// cannot build the world at all, which is why coverage refuses to call such a
// locale complete rather than falling back the way prose does. Prose that falls
// back is merely untranslated; a path that falls back points somewhere that
// does not exist on this learner's disk.
type ScenarioLexiconName struct {
	entityManagementModels.BaseModel
	EntryID uuid.UUID `gorm:"type:uuid;not null;index:idx_lexicon_name_locale,unique" json:"entry_id"`
	Locale  string    `gorm:"type:varchar(10);not null;index:idx_lexicon_name_locale,unique" json:"locale"`
	Name    string    `gorm:"type:varchar(200);not null" json:"name"`
}

func (s ScenarioLexiconName) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioLexiconName) GetReferenceObject() string { return "ScenarioLexiconName" }

func (ScenarioLexiconName) TableName() string { return "scenario_lexicon_names" }
