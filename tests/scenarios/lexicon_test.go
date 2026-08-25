package scenarios_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// lexiconScenario builds a two-language scenario with a small world:
//
//	ROOT ── CASTLE ── CELLAR ── SPIDER (a stem)
func lexiconScenario(t *testing.T) (*gorm.DB, models.Scenario) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "lexicon", Title: "Lexicon", InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)

	entries := []struct {
		key, parent, kind, en, fr string
	}{
		{"ROOT", "", "place", "World", "Monde"},
		{"CASTLE", "ROOT", "place", "Castle", "Chateau"},
		{"CELLAR", "CASTLE", "place", "Cellar", "Cave"},
		{"SPIDER", "CELLAR", "place", "spider_", "araignee_"},
		{"KING", "", "token", "King", "Roi"},
	}
	for i, e := range entries {
		entry := models.ScenarioLexiconEntry{
			ScenarioID: scenario.ID, Key: e.key, ParentKey: e.parent, Kind: e.kind, Position: i,
		}
		require.NoError(t, db.Create(&entry).Error)
		for locale, name := range map[string]string{"en": e.en, "fr": e.fr} {
			require.NoError(t, db.Create(&models.ScenarioLexiconName{
				EntryID: entry.ID, Locale: locale, Name: name,
			}).Error)
		}
	}
	return db, scenario
}

// Paths are derived from the tree, so a translator names a room and the path
// follows. Naming paths instead would let the French cellar sit somewhere the
// English one does not.
func TestLexicon_ComposesPathsFromTheTree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	en, err := services.GenerateLexicon(db, scenario.ID, "en")
	require.NoError(t, err)
	fr, err := services.GenerateLexicon(db, scenario.ID, "fr")
	require.NoError(t, err)

	assert.Contains(t, en, "W_CELLAR=Cellar")
	assert.Contains(t, en, "P_CELLAR=/World/Castle/Cellar")
	assert.Contains(t, fr, "W_CELLAR=Cave")
	assert.Contains(t, fr, "P_CELLAR=/Monde/Chateau/Cave")
}

// Text tokens are read out of files, not typed as paths, so they get a name and
// no path at all.
func TestLexicon_TokensHaveNoPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	fr, err := services.GenerateLexicon(db, scenario.ID, "fr")

	require.NoError(t, err)
	assert.Contains(t, fr, "T_KING=Roi")
	assert.NotContains(t, fr, "P_KING=")
}

// The generated file declares its own locale, so a script that has sourced it
// can tell which world it is standing in.
func TestLexicon_DeclaresItsLocale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	fr, err := services.GenerateLexicon(db, scenario.ID, "fr")

	require.NoError(t, err)
	assert.Contains(t, fr, "OCF_LOCALE=fr")
}

// A parent must be emitted before the children that compose from it, or the
// child's path expands against nothing and silently becomes a shorter one.
func TestLexicon_EmitsParentsBeforeChildren(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	en, err := services.GenerateLexicon(db, scenario.ID, "en")

	require.NoError(t, err)
	assert.Less(t, strings.Index(en, "P_CASTLE="), strings.Index(en, "P_CELLAR="),
		"a child path composed before its parent expands to the wrong place")
}

// A language that has not named every object cannot build the world at all.
// Prose falls back and reads as untranslated; a path that falls back points at
// somewhere this learner's disk does not have.
func TestLexicon_MissingName_IsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)
	require.NoError(t, db.Where("locale = ?", "fr").Delete(&models.ScenarioLexiconName{}).Error)

	_, err := services.GenerateLexicon(db, scenario.ID, "fr")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fr")
}

// Validation reports everything wrong at once, because an editor showing one
// problem at a time turns fixing a lexicon into a guessing game.
func TestLexiconValidate_CleanLexicon_HasNoProblems(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	problems, err := services.ValidateLexicon(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, problems)
}

// A place is typed into a shell by someone who has not met quoting yet, and the
// lab image generates no UTF-8 locale. Accents and spaces belong in prose.
func TestLexiconValidate_AccentedPlace_IsReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)
	require.NoError(t, db.Model(&models.ScenarioLexiconName{}).
		Where("locale = ? AND name = ?", "fr", "Cave").Update("name", "Cave à vin").Error)

	problems, err := services.ValidateLexicon(db, scenario.ID)

	require.NoError(t, err)
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, " | "), "Cave à vin")
}

// A text token is read out of a file, never typed as a path, so accents are
// fine there — reporting them would train people to ignore the checker.
func TestLexiconValidate_AccentedToken_IsAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)
	require.NoError(t, db.Model(&models.ScenarioLexiconName{}).
		Where("locale = ? AND name = ?", "fr", "Roi").Update("name", "Le Roi déchu").Error)

	problems, err := services.ValidateLexicon(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, problems)
}

// Two siblings sharing a name are one directory, and the world quietly loses a
// room rather than failing to build.
func TestLexiconValidate_SiblingsSharingAName_AreReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)
	twin := models.ScenarioLexiconEntry{
		ScenarioID: scenario.ID, Key: "DUNGEON", ParentKey: "CASTLE", Kind: "place", Position: 9,
	}
	require.NoError(t, db.Create(&twin).Error)
	require.NoError(t, db.Create(&models.ScenarioLexiconName{EntryID: twin.ID, Locale: "en", Name: "Dungeon"}).Error)
	require.NoError(t, db.Create(&models.ScenarioLexiconName{EntryID: twin.ID, Locale: "fr", Name: "Cave"}).Error)

	problems, err := services.ValidateLexicon(db, scenario.ID)

	require.NoError(t, err)
	require.NotEmpty(t, problems)
	assert.Contains(t, strings.Join(problems, " | "), "Cave")
}

// A language that has named nothing is reported once per gap, not once.
func TestLexiconValidate_MissingNames_AreReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)
	require.NoError(t, db.Where("locale = ?", "fr").Delete(&models.ScenarioLexiconName{}).Error)

	problems, err := services.ValidateLexicon(db, scenario.ID)

	require.NoError(t, err)
	assert.Len(t, problems, 5, "one per unnamed entry, so a translator can see the size of the job")
}

// The vocabulary reaches the container before anything that builds the world,
// in the language the session was started in.
func TestLexicon_ProvisioningInstallsTheSessionsLanguage(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "lexicon-provision", Title: "Lexicon", InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
		SetupScript: `mkdir -p "$P_CELLAR"`,
	}
	require.NoError(t, db.Create(&scenario).Error)

	for i, e := range []struct{ key, parent, en, fr string }{
		{"ROOT", "", "World", "Monde"},
		{"CASTLE", "ROOT", "Castle", "Chateau"},
		{"CELLAR", "CASTLE", "Cellar", "Cave"},
	} {
		entry := models.ScenarioLexiconEntry{
			ScenarioID: scenario.ID, Key: e.key, ParentKey: e.parent, Kind: "place", Position: i,
		}
		require.NoError(t, db.Create(&entry).Error)
		require.NoError(t, db.Create(&models.ScenarioLexiconName{EntryID: entry.ID, Locale: "en", Name: e.en}).Error)
		require.NoError(t, db.Create(&models.ScenarioLexiconName{EntryID: entry.ID, Locale: "fr", Name: e.fr}).Error)
	}

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "First", StepType: "info"}
	require.NoError(t, db.Create(&step).Error)

	// French has to be launchable for a French session to start at all, which
	// means its prose as well as its vocabulary.
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: step.ID, Locale: "fr", Title: "Premiere",
		SourceHash: services.StepSourceHash(step),
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := svc.StartScenario("student-1", scenario.ID, "terminal-lexicon", "fr")
	require.NoError(t, err)
	require.NotNil(t, session)

	require.Eventually(t, func() bool { return len(verifySvc.execCalls) > 0 }, 3*time.Second, 20*time.Millisecond,
		"the setup script never ran")

	// The script travels as the command tt-backend is asked to run.
	setup := strings.Join(verifySvc.execCalls[0].command, " ")
	assert.Contains(t, setup, "cat > /etc/ocf-lexicon.sh", "the vocabulary is installed first")
	assert.Contains(t, setup, "P_CELLAR=/Monde/Chateau/Cave", "in the session's language, not the default")
	assert.Less(t, strings.Index(setup, "OCF_LEXICON"), strings.Index(setup, `mkdir -p "$P_CELLAR"`),
		"a script that builds the world cannot run before the names it builds it from")
}

// A language whose vocabulary is incomplete cannot be offered, however well its
// prose is translated.
//
// This is stricter than the prose rule for a reason. Untranslated prose reads
// as English in a French world — awkward, still playable. An unnamed room means
// the world cannot be built at all: the setup script fails, and the learner
// gets a container with nothing in it.
func TestLaunchableLocales_IncompleteLexicon_IsNotOffered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario := lexiconScenario(t)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "First", StepType: "info"}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: step.ID, Locale: "fr", Title: "Premiere",
		SourceHash: services.StepSourceHash(step),
	}).Error)

	// Prose complete, so without the lexicon rule French would be offered.
	offered, err := services.LaunchableLocales(db, scenario.ID)
	require.NoError(t, err)
	require.Contains(t, offered, "fr", "precondition: French is otherwise ready")

	// Take one name away and it must withdraw.
	require.NoError(t, db.Where("locale = ? AND name = ?", "fr", "Cave").
		Delete(&models.ScenarioLexiconName{}).Error)

	offered, err = services.LaunchableLocales(db, scenario.ID)

	require.NoError(t, err)
	assert.NotContains(t, offered, "fr", "a world with an unnamed room cannot be built")
	assert.Contains(t, offered, "en")
}
