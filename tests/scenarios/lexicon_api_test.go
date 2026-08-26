package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// The lexicon is read and written whole rather than one entry at a time.
//
// It is a table with a shape: entries point at parents, and names must line up
// across languages. Saving it piecemeal means every intermediate state is a
// lexicon with a dangling parent or a half-renamed room, and the checks would
// have to be suspended for exactly as long as it takes an editor to finish —
// which is the window in which a broken one reaches a learner.

func TestLexiconDocument_ReplacesTheWholeThing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	err := services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "World", "fr": "Monde"}},
		{Key: "GARDEN", ParentKey: "ROOT", Kind: "place", Names: map[string]string{"en": "Garden", "fr": "Jardin"}},
	})
	require.NoError(t, err)

	document, err := services.LoadLexiconDocument(db, scenario.ID)
	require.NoError(t, err)

	assert.Len(t, document.Entries, 2, "the castle is gone because the new lexicon does not mention it")
	assert.Equal(t, "GARDEN", document.Entries[1].Key)
	assert.Equal(t, "Jardin", document.Entries[1].Names["fr"])
}

// Order is kept as sent, so a generated file and the editor's table agree.
func TestLexiconDocument_KeepsTheOrderItWasGiven(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "World", "fr": "Monde"}},
		{Key: "B", ParentKey: "ROOT", Kind: "place", Names: map[string]string{"en": "Bee", "fr": "Abeille"}},
		{Key: "A", ParentKey: "ROOT", Kind: "place", Names: map[string]string{"en": "Ant", "fr": "Fourmi"}},
	}))

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Equal(t, []string{"ROOT", "B", "A"}, []string{
		document.Entries[0].Key, document.Entries[1].Key, document.Entries[2].Key,
	})
}

// A half-finished lexicon saves and reports. Being mid-edit is the normal state
// of one being written, and refusing to store it would mean losing the work
// every time a translator stops halfway.
func TestLexiconDocument_UnfinishedWork_SavesAndReports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	err := services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "World", "fr": "Monde"}},
		{Key: "CELLAR", ParentKey: "ROOT", Kind: "place", Names: map[string]string{"en": "Cellar"}},
	})
	require.NoError(t, err, "unfinished is not invalid")

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Len(t, document.Entries, 2)
	assert.NotEmpty(t, document.Problems, "and the gap is reported rather than passed over")
}

// A structurally impossible lexicon is refused outright, and nothing is stored.
// It is not unfinished work — no amount of further typing makes a room that is
// inside itself resolvable, and storing it would break generation for a locale
// that was previously fine.
func TestLexiconDocument_UnknownParent_IsRefusedAndChangesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	err := services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "World", "fr": "Monde"}},
		{Key: "ATTIC", ParentKey: "NOWHERE", Kind: "place", Names: map[string]string{"en": "Attic", "fr": "Grenier"}},
	})

	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&models.ScenarioLexiconEntry{}).
		Where("scenario_id = ?", scenario.ID).Count(&count).Error)
	assert.Equal(t, int64(5), count, "the lexicon that was already there is untouched")
}

func TestLexiconDocument_CycleIsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	err := services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "A", ParentKey: "B", Kind: "place", Names: map[string]string{"en": "A", "fr": "A"}},
		{Key: "B", ParentKey: "A", Kind: "place", Names: map[string]string{"en": "B", "fr": "B"}},
	})

	require.Error(t, err)
}

// Two entries under one key would make $P_CELLAR mean two things.
func TestLexiconDocument_DuplicateKey_IsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	err := services.ReplaceLexicon(db, scenario.ID, []dto.LexiconEntryInput{
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "World", "fr": "Monde"}},
		{Key: "ROOT", Kind: "place", Names: map[string]string{"en": "Realm", "fr": "Royaume"}},
	})

	require.Error(t, err)
}
