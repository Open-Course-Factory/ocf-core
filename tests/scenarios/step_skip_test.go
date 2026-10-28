package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// POST /teacher/groups/:groupId/sessions/:sessionId/skip-to-step — a trainer
// moves a learner forward. Only the target step's setup runs: the steps in
// between are skipped, not replayed.

// threeStepSession is twoStepSession plus a third step (order 2) holding the
// given background script; step 1 holds a script of its own, so a test can
// tell which one ran.
func threeStepSession(t *testing.T, db *gorm.DB, name, lastScript string) *models.ScenarioSession {
	t.Helper()
	session := twoStepSession(t, db, name, models.ScenarioStep{
		BackgroundScript:         "echo step one setup",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: session.ScenarioID, Order: 2, Title: "Step 3",
		BackgroundScript: lastScript, BackgroundTimeoutSeconds: 5,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 2, Status: "locked"}).Error)
	return session
}

func stepStatuses(t *testing.T, db *gorm.DB, session *models.ScenarioSession) map[int]string {
	t.Helper()
	var progress []models.ScenarioStepProgress
	require.NoError(t, db.Where("session_id = ?", session.ID).Find(&progress).Error)
	statuses := map[int]string{}
	for _, p := range progress {
		statuses[p.StepOrder] = p.Status
	}
	return statuses
}

func TestSkipToStep_MovesForwardAndRunsOnlyTheTargetSetup(t *testing.T) {
	db := freshTestDB(t)
	session := threeStepSession(t, db, "skip-forward", "echo step two setup")

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.SkipToStep(session.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, result.StepOrder)

	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", session.ID).Error)
	assert.Equal(t, 2, reloaded.CurrentStep)
	assert.Equal(t, "active", reloaded.Status)
	assert.Equal(t, map[int]string{0: "skipped", 1: "skipped", 2: "active"}, stepStatuses(t, db, session),
		"skipped, not completed: the grade must not credit steps nobody did")

	require.Len(t, verifySvc.execCalls, 1, "the step skipped over must not run its setup")
	assert.Equal(t, []string{"/bin/sh", "-c", "set -e\necho step two setup"}, verifySvc.execCalls[0].command)
}

func TestSkipToStep_RejectsTheCurrentOrAnEarlierStep(t *testing.T) {
	db := freshTestDB(t)
	session := threeStepSession(t, db, "skip-backward", "echo step two setup")
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("current_step", 1).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	for _, target := range []int{0, 1} {
		_, err := sessionSvc.SkipToStep(session.ID, target)
		assert.ErrorIs(t, err, services.ErrSkipNotForward, "target %d", target)
	}
	assert.Len(t, verifySvc.execCalls, 0)
}

func TestSkipToStep_RejectsAStepTheScenarioDoesNotHave(t *testing.T) {
	db := freshTestDB(t)
	session := threeStepSession(t, db, "skip-missing", "echo step two setup")

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &bgTrackingVerificationService{})

	_, err := sessionSvc.SkipToStep(session.ID, 7)
	require.Error(t, err)
	assert.Equal(t, map[int]string{0: "active", 1: "locked", 2: "locked"}, stepStatuses(t, db, session))
}

func TestSkipToStep_RejectsASessionWhoseSetupIsRunning(t *testing.T) {
	db := freshTestDB(t)
	session := threeStepSession(t, db, "skip-provisioning", "echo step two setup")
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("status", "provisioning").Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.SkipToStep(session.ID, 2)
	assert.ErrorIs(t, err, services.ErrSessionNotActive)
	assert.Len(t, verifySvc.execCalls, 0)
}

func TestSkipToStep_AsyncTargetParksTheSessionInProvisioning(t *testing.T) {
	db := freshTestDB(t)
	session := threeStepSession(t, db, "skip-async", "echo long setup")
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("scenario_id = ? AND \"order\" = ?", session.ScenarioID, 2).
		Update("background_async", true).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.SkipToStep(session.ID, 2)
	require.NoError(t, err)
	assert.True(t, result.NextStepProvisioning, "the trainer's client polls like the learner's does")
	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	require.Len(t, verifySvc.execCalls, 1)
}
