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

// Progressive hints are revealed from rows built from the step's own hint, so
// a French session revealed English hints even when the step's French
// translation had its own. The translated hint is split by the same rule.

func createStepWithTranslatedHints(t *testing.T, db *gorm.DB) models.ScenarioStep {
	t.Helper()
	scenario := models.Scenario{Name: "translated-hints", Title: "Translated Hints", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", TextContent: "Look around.",
		HintContent: "### Hint 1\nTry ls\n### Hint 2\nTry ls -a",
		Hints:       services.BuildStepHints("### Hint 1\nTry ls\n### Hint 2\nTry ls -a"),
	}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: step.ID, Locale: "fr", Title: "Étape",
		HintContent: "### Indice 1\nEssayez ls\n### Indice 2\nEssayez ls -a\n### Indice 3\nRegardez les fichiers cachés",
	}).Error)
	return step
}

func startHintSession(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, locale string) uuid.UUID {
	t.Helper()
	session := models.ScenarioSession{
		ScenarioID: scenarioID, UserID: "student-1", CurrentStep: 0, Status: "active",
		StartedAt: time.Now(), Locale: locale,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	return session.ID
}

func TestRevealHint_FollowsTheSessionLocale(t *testing.T) {
	cases := []struct {
		locale, wantFirst string
		wantTotal         int
	}{
		{"fr", "Essayez ls", 3},
		{"", "Try ls", 2},
		{"de", "Try ls", 2}, // no German translation: the step's own hints
	}
	for _, tc := range cases {
		t.Run("locale="+tc.locale, func(t *testing.T) {
			db := freshTestDB(t)
			step := createStepWithTranslatedHints(t, db)
			sessionID := startHintSession(t, db, step.ScenarioID, tc.locale)
			svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

			current, err := svc.GetCurrentStep(sessionID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, current.HintsTotalCount, "the count matches the hints revealed")

			first, err := svc.RevealHint(sessionID, 0, 1)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFirst, first.Content)
			assert.Equal(t, tc.wantTotal, first.Total)

			last, err := svc.RevealHint(sessionID, 0, 2)
			require.NoError(t, err)
			assert.Equal(t, 2, last.Level)
			if tc.wantTotal == 3 {
				third, err := svc.RevealHint(sessionID, 0, 3)
				require.NoError(t, err)
				assert.Equal(t, "Regardez les fichiers cachés", third.Content)
			}
		})
	}
}

// A step without progressive hints keeps its single hint in every language: a
// translation changes the words, not the kind of hint.
func TestRevealHint_StepWithoutHintRows_TranslationAddsNone(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "single-hint", Title: "Single Hint", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", HintContent: "Use cd."}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: step.ID, Locale: "fr", HintContent: "### Indice 1\nUtilisez cd\n### Indice 2\ncd ~",
	}).Error)
	sessionID := startHintSession(t, db, scenario.ID, "fr")
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	current, err := svc.GetCurrentStep(sessionID)
	require.NoError(t, err)
	assert.Zero(t, current.HintsTotalCount)
	_, err = svc.RevealHint(sessionID, 0, 1)
	assert.Error(t, err)
}
