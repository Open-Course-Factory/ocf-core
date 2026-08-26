package scenarios_test

import (
	"strings"
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

// A script that still names a room is the thing the lexicon exists to remove,
// and nothing else would notice it.
//
// It works — in the language it was written in. It is only when a second
// language exists that the script sends that learner somewhere their disk does
// not have, by which point the scenario looks broken rather than untranslated.
func TestLexiconDocument_ScriptNamingARoom_IsReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Go down",
		VerifyScript: `[ "$(pwd)" = "/World/Castle/Cellar" ]`,
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	require.NotEmpty(t, document.ScriptLiterals)
	assert.Contains(t, strings.Join(document.ScriptLiterals, " | "), "Cellar")
}

// A script that composes from the vocabulary is what this is asking for, and
// must not be nagged about.
func TestLexiconDocument_ScriptUsingTheVocabulary_IsQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Go down",
		VerifyScript: `[ "$(pwd)" = "$P_CELLAR" ]`,
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, document.ScriptLiterals)
}

// Machinery keeps its own name. /etc and /opt are the same in every language,
// and flagging them would train an author to ignore the list.
func TestLexiconDocument_MachineryPaths_AreNotReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Compare",
		VerifyScript: `diff -q "$P_CELLAR/ledger" /opt/gsh/ledger`,
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, document.ScriptLiterals)
}

// A scenario with no vocabulary is not told its scripts are wrong: there is
// nothing yet to say what its world is called.
func TestLexiconDocument_NoVocabulary_ReportsNoLiterals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, _, steps := coverageScenario(t, `["en","fr"]`)
	require.NoError(t, db.Model(&steps[0]).Update("verify_script", `[ -d /World/Castle ]`).Error)

	document, err := services.LoadLexiconDocument(db, steps[0].ScenarioID)

	require.NoError(t, err)
	assert.Empty(t, document.ScriptLiterals)
}

// A name inside a heredoc is prose, not a path, and must not be reported as one.
//
// A quoted heredoc is written out verbatim: the sign a learner reads, the
// contents of a file. Telling an author to replace it with $P_CHEST is wrong
// advice — it would put a shell variable into the text rather than expand it.
// That is translation work, and it belongs to the content pass.
func TestLexiconDocument_NamesInsideHeredocs_AreNotReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Sign",
		BackgroundScript: "mkdir -p \"$P_CELLAR\"\ncat > \"$P_CELLAR/sign\" << 'EOF'\nThe Cellar is dark.\nEOF\necho done",
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, document.ScriptLiterals)
}

// Code after a heredoc closes is code again, and a room named there is still a
// room named there.
func TestLexiconDocument_CodeAfterAHeredoc_IsStillChecked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Sign",
		BackgroundScript: "cat > /tmp/sign << 'EOF'\nnothing to see\nEOF\nmkdir -p /World/Castle/Cellar",
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	require.NotEmpty(t, document.ScriptLiterals)
	assert.Contains(t, strings.Join(document.ScriptLiterals, " | "), "Cellar")
}

// A comment and a printed message are prose as much as a heredoc is. Advising
// "$P_CHEST" in an explanation, or in the sentence a learner reads when a check
// refuses, would put a variable name where a sentence belongs.
func TestLexiconDocument_CommentsAndMessages_AreNotReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Check",
		VerifyScript: "# the Cellar has to be empty by now\n" +
			"[ -z \"$(ls \"$P_CELLAR\")\" ] || { echo \"There is still something in the Cellar!\"; exit 1; }",
	}).Error)

	document, err := services.LoadLexiconDocument(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, document.ScriptLiterals)
}
