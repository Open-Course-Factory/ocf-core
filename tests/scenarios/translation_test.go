package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// setupTranslatedSession builds a scenario whose single step has English
// content, optionally a French translation, and a session pinned to a locale.
//
// The locale lives on the session rather than being read per request: the
// container was built in one language and cannot be rebuilt, so a learner who
// switches the interface mid-run must keep reading the language their world is
// actually in.
func setupTranslatedSession(t *testing.T, sessionLocale string, translate func(*gorm.DB, uuid.UUID)) uuid.UUID {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "translated-" + sessionLocale,
		Title:        "Down to the Cellar",
		InstanceType: "debian",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Down to the Cellar",
		StepType:    "terminal",
		TextContent: "Make your way down to the Cellar.",
		HintContent: "Use cd to move.",
	}
	require.NoError(t, db.Create(&step).Error)

	if translate != nil {
		translate(db, step.ID)
	}

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
		Locale:      sessionLocale,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID,
		StepOrder: 0,
		Status:    "active",
	}).Error)

	return session.ID
}

func frenchStep(db *gorm.DB, stepID uuid.UUID) {
	db.Create(&models.ScenarioStepTranslation{
		StepID:      stepID,
		Locale:      "fr",
		Title:       "Descendre a la Cave",
		TextContent: "Descendez jusqu'a la Cave.",
		HintContent: "Utilisez cd pour vous deplacer.",
	})
}

func currentStep(t *testing.T, sessionID uuid.UUID) *dtoCurrentStep {
	t.Helper()
	svc := services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{})
	response, err := svc.GetCurrentStep(sessionID)
	require.NoError(t, err)
	require.NotNil(t, response)
	return &dtoCurrentStep{Title: response.Title, Text: response.Text, Hint: response.Hint}
}

type dtoCurrentStep struct{ Title, Text, Hint string }

// A session pinned to French must read the French step, not the English one.
func TestGetCurrentStep_SessionLocale_ServesTheTranslation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID := setupTranslatedSession(t, "fr", frenchStep)

	step := currentStep(t, sessionID)

	assert.Equal(t, "Descendre a la Cave", step.Title)
	assert.Equal(t, "Descendez jusqu'a la Cave.", step.Text)
	assert.Equal(t, "Utilisez cd pour vous deplacer.", step.Hint)
}

// A session with no locale is every session that exists today. It must keep
// reading exactly what it read before.
func TestGetCurrentStep_NoLocale_ServesTheDefaultContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID := setupTranslatedSession(t, "", frenchStep)

	step := currentStep(t, sessionID)

	assert.Equal(t, "Down to the Cellar", step.Title)
	assert.Equal(t, "Make your way down to the Cellar.", step.Text)
}

// A locale with no translation for this step falls back to the default rather
// than serving an empty step.
func TestGetCurrentStep_UntranslatedLocale_FallsBackToDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID := setupTranslatedSession(t, "es", frenchStep)

	step := currentStep(t, sessionID)

	assert.Equal(t, "Down to the Cellar", step.Title)
	assert.Equal(t, "Make your way down to the Cellar.", step.Text)
}

// A translation that fills only some fields must not blank the others: an empty
// column is an untranslated field, not an instruction to serve nothing.
func TestGetCurrentStep_PartialTranslation_KeepsDefaultForEmptyFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	sessionID := setupTranslatedSession(t, "fr", func(db *gorm.DB, stepID uuid.UUID) {
		db.Create(&models.ScenarioStepTranslation{
			StepID: stepID,
			Locale: "fr",
			Title:  "Descendre a la Cave",
		})
	})

	step := currentStep(t, sessionID)

	assert.Equal(t, "Descendre a la Cave", step.Title)
	assert.Equal(t, "Make your way down to the Cellar.", step.Text,
		"an untranslated field keeps the default rather than becoming empty")
}

// A session's language has to reach the container, because the world's own
// directory names are built from it. It travels the same channel the current
// step's flag already uses.
func TestBackgroundScript_SessionLocale_ReachesTheContainer(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "locale-env", models.ScenarioStep{
		BackgroundScript:         "echo building in $OCF_LANG",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(session).Update("locale", "fr").Error)
	session.Locale = "fr"

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, "fr", verifySvc.execCalls[0].env["OCF_LANG"],
		"the container has to know which language its world was built in")
}

// A session with no locale is every session that exists today: it must send
// exactly what it sent before, which for a scenario without flags is nothing at
// all.
func TestBackgroundScript_NoLocale_SendsNoEnvAtAll(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "locale-env-absent", models.ScenarioStep{
		BackgroundScript:         "echo no locale here",
		BackgroundTimeoutSeconds: 5,
	})

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Nil(t, verifySvc.execCalls[0].env,
		"an empty OCF_LANG would be a visible difference for every session that predates locales")
}

// The catalogue card has to be able to switch with the picker beside it, so
// every offered language's title and description travel with the scenario.
//
// Sent together rather than fetched per choice: the learner is changing a
// dropdown, and a card that went blank while it asked the server would make
// choosing a language feel like loading a page.
func TestScenarioCardText_CarriesEveryOfferedLanguage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "card", Title: "Down to the Cellar", Description: "A castle adventure.",
		InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: scenario.ID, Locale: "fr",
		Title: "Descendre a la Cave", Description: "Une aventure au chateau.",
	}).Error)

	text, err := services.ScenarioTextByLocale(db, scenario)

	require.NoError(t, err)
	assert.Equal(t, "Down to the Cellar", text["en"].Title, "the default language is the scenario itself")
	assert.Equal(t, "Descendre a la Cave", text["fr"].Title)
	assert.Equal(t, "Une aventure au chateau.", text["fr"].Description)
}

// A language that has not been given a card title falls back to the original
// rather than showing an empty card.
func TestScenarioCardText_UntranslatedLanguage_KeepsTheOriginal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "card-partial", Title: "Down to the Cellar", Description: "A castle adventure.",
		InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: scenario.ID, Locale: "fr", Title: "Descendre a la Cave",
	}).Error)

	text, err := services.ScenarioTextByLocale(db, scenario)

	require.NoError(t, err)
	assert.Equal(t, "Descendre a la Cave", text["fr"].Title)
	assert.Equal(t, "A castle adventure.", text["fr"].Description,
		"an untranslated description reads in the original, never blank")
}

// A single-language scenario carries nothing: there is no choice to preview.
func TestScenarioCardText_SingleLanguage_CarriesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "card-single", Title: "Down to the Cellar",
		InstanceType: "debian", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	text, err := services.ScenarioTextByLocale(db, scenario)

	require.NoError(t, err)
	assert.Empty(t, text)
}

// ---------------------------------------------------------------------------
// The briefing, in the language the session is being played in
// ---------------------------------------------------------------------------

// The steps were resolved for the session's locale from the start; the briefing
// was read straight off the scenario. So a session played entirely in French —
// French world, French steps, French refusals — opened with an English welcome
// and closed with an English farewell, and nothing in the suite looked at
// either.
func TestResolveScenarioText_UsesTheSessionsLanguage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "briefing", Title: "GameShell Basics", Description: "A castle adventure.",
		IntroText: "Welcome to the castle!", FinishText: "The adventure ends.",
		InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: scenario.ID, Locale: "fr",
		Title: "GameShell — les bases", Description: "Une aventure au chateau.",
		IntroText: "Bienvenue au chateau !", FinishText: "L'aventure se termine.",
	}).Error)

	french := services.ResolveScenarioText(db, scenario, "fr")

	assert.Equal(t, "Bienvenue au chateau !", french.Intro, "the briefing opens the session")
	assert.Equal(t, "L'aventure se termine.", french.Finish, "and this closes it")
	assert.Equal(t, "GameShell — les bases", french.Title)
	assert.Equal(t, "Une aventure au chateau.", french.Description)
}

// No locale is every session that existed before this feature.
func TestResolveScenarioText_NoLocale_IsTheScenarioItself(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "untranslated", Title: "GameShell Basics",
		IntroText: "Welcome to the castle!",
		InstanceType: "debian", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	assert.Equal(t, "Welcome to the castle!", services.ResolveScenarioText(db, scenario, "").Intro)
}

// A half-filled translation keeps the original for what it does not carry.
// Prose that falls back is merely untranslated; the alternative is a session
// that opens on a blank briefing.
func TestResolveScenarioText_PartialTranslation_KeepsWhatIsMissing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "half", Title: "GameShell Basics",
		IntroText: "Welcome to the castle!", FinishText: "The adventure ends.",
		InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: scenario.ID, Locale: "fr",
		IntroText: "Bienvenue au chateau !",
	}).Error)

	french := services.ResolveScenarioText(db, scenario, "fr")

	assert.Equal(t, "Bienvenue au chateau !", french.Intro)
	assert.Equal(t, "The adventure ends.", french.Finish, "untranslated, so the original stands")
	assert.Equal(t, "GameShell Basics", french.Title)
}

// The card and the player must agree about what a translation means. They read
// the same rows through the same overlay, and this is what says so.
func TestScenarioCardText_AgreesWithTheSessionResolver(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "agree", Title: "GameShell Basics", Description: "A castle adventure.",
		InstanceType: "debian", CreatedByID: "creator-1",
		DefaultLocale: "en", Locales: `["en","fr"]`,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: scenario.ID, Locale: "fr",
		Title: "GameShell — les bases", Description: "Une aventure au chateau.",
	}).Error)

	card, err := services.ScenarioTextByLocale(db, scenario)
	require.NoError(t, err)
	session := services.ResolveScenarioText(db, scenario, "fr")

	assert.Equal(t, card["fr"].Title, session.Title)
	assert.Equal(t, card["fr"].Description, session.Description)
}
