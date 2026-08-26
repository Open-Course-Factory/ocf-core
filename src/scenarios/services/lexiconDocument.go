package services

import (
	"fmt"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LoadLexiconDocument reads a scenario's whole vocabulary, and what is wrong
// with it.
func LoadLexiconDocument(db *gorm.DB, scenarioID uuid.UUID) (*dto.LexiconDocumentOutput, error) {
	entries, names, err := loadLexicon(db, scenarioID)
	if err != nil {
		return nil, err
	}

	document := &dto.LexiconDocumentOutput{Entries: make([]dto.LexiconEntryOutput, 0, len(entries))}
	for _, entry := range entries {
		perLocale := map[string]string{}
		for locale, name := range names[entry.ID] {
			perLocale[locale] = name
		}
		document.Entries = append(document.Entries, dto.LexiconEntryOutput{
			Key:       entry.Key,
			ParentKey: entry.ParentKey,
			Kind:      entry.Kind,
			Names:     perLocale,
		})
	}

	problems, err := ValidateLexicon(db, scenarioID)
	if err != nil {
		return nil, err
	}
	document.Problems = problems
	if document.Problems == nil {
		document.Problems = []string{}
	}
	return document, nil
}

// ReplaceLexicon stores a whole vocabulary, in one transaction.
//
// Two classes of wrong are treated differently, on purpose.
//
// Unfinished — a room nobody has named in French yet — is stored and reported.
// Being mid-edit is the normal state of a lexicon being written, and refusing
// it would lose the work every time a translator stops halfway.
//
// Impossible — a parent that does not exist, a room inside itself, one key used
// twice — is refused and nothing is written. No amount of further typing makes
// those resolvable, and storing one would break generation for a language that
// was working a moment ago.
func ReplaceLexicon(db *gorm.DB, scenarioID uuid.UUID, entries []dto.LexiconEntryInput) error {
	if err := assertResolvable(entries); err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var existing []models.ScenarioLexiconEntry
		if err := tx.Where("scenario_id = ?", scenarioID).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			ids := make([]uuid.UUID, len(existing))
			for i, e := range existing {
				ids[i] = e.ID
			}
			if err := tx.Unscoped().Where("entry_id IN ?", ids).
				Delete(&models.ScenarioLexiconName{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("scenario_id = ?", scenarioID).
				Delete(&models.ScenarioLexiconEntry{}).Error; err != nil {
				return err
			}
		}

		for position, input := range entries {
			kind := input.Kind
			if kind == "" {
				kind = "place"
			}
			entry := models.ScenarioLexiconEntry{
				ScenarioID: scenarioID,
				Key:        input.Key,
				ParentKey:  input.ParentKey,
				Kind:       kind,
				Position:   position,
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			for locale, name := range input.Names {
				if name == "" {
					continue
				}
				if err := tx.Create(&models.ScenarioLexiconName{
					EntryID: entry.ID, Locale: locale, Name: name,
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// assertResolvable rejects a lexicon no amount of naming could rescue.
func assertResolvable(entries []dto.LexiconEntryInput) error {
	byKey := make(map[string]dto.LexiconEntryInput, len(entries))
	positions := map[string]int{}
	occupied := map[string]bool{}

	for _, entry := range entries {
		if entry.Key == "" {
			return fmt.Errorf("lexicon: an entry has no key")
		}
		// The same key in the same place twice is one entry claiming to be two.
		// In two different places it is one object that can be in either — the
		// crown before and after a learner moves it.
		slot := entry.ParentKey + "/" + entry.Key
		if occupied[slot] {
			return fmt.Errorf("lexicon: %q appears twice in the same place", entry.Key)
		}
		occupied[slot] = true
		positions[entry.Key]++
		byKey[entry.Key] = entry
	}

	// A repeated key has no single place, so nothing can be inside it: a child
	// would have two possible paths and no way to say which was meant.
	for _, entry := range entries {
		if entry.ParentKey != "" && positions[entry.ParentKey] > 1 {
			return fmt.Errorf(
				"lexicon: %q sits inside %q, which is in more than one place — nothing can be inside an object that moves",
				entry.Key, entry.ParentKey)
		}
	}

	for _, entry := range entries {
		seen := map[string]bool{}
		for cursor := entry; cursor.ParentKey != ""; {
			if seen[cursor.Key] {
				return fmt.Errorf("lexicon: %q is inside itself", entry.Key)
			}
			seen[cursor.Key] = true

			parent, ok := byKey[cursor.ParentKey]
			if !ok {
				return fmt.Errorf("lexicon: %q sits inside %q, which does not exist", cursor.Key, cursor.ParentKey)
			}
			cursor = parent
		}
	}
	return nil
}
