package services

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

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

	literals, err := findScriptLiterals(db, scenarioID)
	if err != nil {
		return nil, err
	}
	document.ScriptLiterals = literals
	if document.ScriptLiterals == nil {
		document.ScriptLiterals = []string{}
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

// machineryPrefixes are paths that mean the same thing in every language: a
// library the scenario installs, a pristine copy it compares against, the
// system files it edits. Flagging them would train an author to ignore the list.
var machineryPrefixes = []string{"/etc", "/opt", "/usr", "/var", "/tmp", "/root", "/bin", "/sbin", "/proc", "/dev"}

// findScriptLiterals reports scripts that still name a room.
//
// Matched against the default locale's names, because that is what an author
// writes by hand. A scenario with no vocabulary is not told its scripts are
// wrong — there is nothing yet to say what its world is called.
func findScriptLiterals(db *gorm.DB, scenarioID uuid.UUID) ([]string, error) {
	var scenario models.Scenario
	if err := db.First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, err
	}
	locale := scenario.DefaultLocale
	if locale == "" {
		return nil, nil
	}

	entries, names, err := loadLexicon(db, scenarioID)
	if err != nil || len(entries) == 0 {
		return nil, err
	}

	// Short names are words before they are rooms: "left" and "hall" appear in
	// ordinary English, and reporting those would bury the real ones.
	wanted := map[string]string{}
	for _, entry := range entries {
		name := names[entry.ID][locale]
		if len(strings.Trim(name, "._*-")) > 3 {
			wanted[name] = entry.Key
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}

	var steps []models.ScenarioStep
	if err := db.Where("scenario_id = ?", scenarioID).Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return nil, err
	}

	// Sorted, so the same script always reports the same way. Iterating the map
	// directly made which name got named depend on Go's map ordering, and a
	// checker that says something different each run is one nobody trusts.
	sortedNames := make([]string, 0, len(wanted))
	for name := range wanted {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	var found []string
	report := func(where, script string) {
		if script == "" {
			return
		}
		stripped := stripMachinery(scriptCode(script))
		// Every name in the script, not just the first: an author replacing one
		// and finding another still there learns to distrust the list.
		for _, name := range sortedNames {
			if namedAsAPath(stripped, name) {
				found = append(found, fmt.Sprintf(
					"%s names %q directly — use $P_%s or $W_%s", where, name, wanted[name], wanted[name]))
			}
		}
	}

	report("the setup script", scenario.SetupScript)
	for _, step := range steps {
		label := fmt.Sprintf("step %d (%s)", step.Order, step.Title)
		report(label+" verify", step.VerifyScript)
		report(label+" background", step.BackgroundScript)
		report(label+" foreground", step.ForegroundScript)
	}
	return found, nil
}

// heredocStart matches the line that opens a heredoc, capturing its delimiter.
var heredocStart = regexp.MustCompile(`<<-?\s*'?"?([A-Za-z_][A-Za-z0-9_]*)'?"?`)

// scriptCode is the part of a script that is code.
//
// A heredoc body is not: it is written out verbatim as the sign a learner
// reads or the contents of a file they open. A name there is prose, and
// advising "use $P_CHEST" would put a shell variable into the text instead of
// expanding it. Translating that text is the content pass's job, not this one's.
func scriptCode(script string) string {
	var kept []string
	terminator := ""

	for _, line := range strings.Split(script, "\n") {
		if terminator != "" {
			if strings.TrimSpace(line) == terminator {
				terminator = ""
			}
			continue
		}
		if match := heredocStart.FindStringSubmatch(line); match != nil {
			terminator = match[1]
		}

		// Comments and printed lines are prose too. A comment explains, and an
		// echo is read by the learner — replacing either with $P_CHEST would
		// put a variable name where a sentence belongs. Both are translated by
		// the content pass, not by referencing the vocabulary.
		trimmed := strings.TrimSpace(line)
		// A printed line is prose wherever the echo sits — real scripts write
		// `... || { echo "still in the Cellar"; exit 1; }`, and only checking
		// the start of the line would call that a path.
		if strings.HasPrefix(trimmed, "#") ||
			strings.Contains(line, "echo ") ||
			strings.Contains(line, "printf ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// stripMachinery removes paths that are the same in every language, so a name
// appearing only inside one is not reported.
func stripMachinery(script string) string {
	var kept []string
	for _, field := range strings.Fields(script) {
		machinery := false
		for _, prefix := range machineryPrefixes {
			if strings.Contains(field, prefix+"/") {
				machinery = true
				break
			}
		}
		if !machinery {
			kept = append(kept, field)
		}
	}
	return strings.Join(kept, " ")
}

// namedAsAPath reports a name only where it is being used as one.
//
// A room mentioned in a sentence is prose; a room written as "/World/Castle" or
// handed to `find -name "spider_*"` is a path this script will not find in
// another language. Matching the bare word instead would report every
// explanation containing it, and a checker that cries wolf gets skimmed.
func namedAsAPath(script, name string) bool {
	if strings.Contains(script, "/"+name) {
		return true
	}
	for _, field := range strings.Fields(script) {
		bare := strings.Trim(field, `"'`)
		if bare == name || strings.HasPrefix(bare, name+"*") {
			return true
		}
	}
	return false
}
