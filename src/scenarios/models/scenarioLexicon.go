package models

import (
	"log"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	ScenarioID uuid.UUID `gorm:"type:uuid;not null;index;index:idx_lexicon_scenario_key,unique" json:"scenario_id"`

	// Key is how scripts refer to this object: CELLAR becomes $W_CELLAR for the
	// name and $P_CELLAR for the path. Stable across languages by definition —
	// renaming a key is a change to the scripts, not to a translation.
	// A key may repeat under different parents: one object can sit in more than
	// one place, and must stay one object. The crown is in the Treasury until a
	// learner moves it to the Chest; two entries could be named differently, and
	// the mission that moves it would be moving something that never arrives.
	Key string `gorm:"type:varchar(120);not null;index:idx_lexicon_scenario_key,unique" json:"key"`

	// ParentKey is the entry this one sits inside. Empty means directly under
	// the world root.
	ParentKey string `gorm:"type:varchar(120);index:idx_lexicon_scenario_key,unique" json:"parent_key,omitempty"`

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

// MigrateLexiconKeyIndex widens the uniqueness rule to include the parent.
//
// An earlier shape made a key unique per scenario, which cannot express an
// object that sits in more than one place — the crown in the Treasury and then
// in the Chest. AutoMigrate will not alter an index that already exists, so the
// old one is dropped and left to be rebuilt from the model's tags.
//
// Idempotent: it checks the shape rather than assuming, so running it against a
// database that never had the narrow index does nothing.
func MigrateLexiconKeyIndex(db *gorm.DB) {
	const indexName = "idx_lexicon_scenario_key"

	if !db.Migrator().HasIndex(&ScenarioLexiconEntry{}, indexName) {
		return
	}

	var columns int64
	switch db.Dialector.Name() {
	case "postgres":
		db.Raw(`SELECT count(*) FROM pg_index i
		        JOIN pg_class c ON c.oid = i.indexrelid
		        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		        WHERE c.relname = ?`, indexName).Scan(&columns)
	default:
		// SQLite test databases are built fresh from the current model, so
		// there is never a stale index to widen.
		return
	}

	if columns >= 3 {
		return
	}
	if err := db.Migrator().DropIndex(&ScenarioLexiconEntry{}, indexName); err != nil {
		log.Printf("could not widen the lexicon key index: %v", err)
	}
}
