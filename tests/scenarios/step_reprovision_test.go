package scenarios_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// POST /scenario-sessions/:id/reprovision-step — the recovery path for a step
// whose setup failed. The advance is never rolled back, so re-running the
// current step's script against the same container is the only way back into a
// playable run.

func TestReprovisionCurrentStep_RerunsTheCurrentStepsScript(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "reprovision-basic", models.ScenarioStep{
		BackgroundScript:         "echo setup level",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("current_step", 1).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.StepOrder)
	assert.Equal(t, "active", result.Status)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, []string{"/bin/sh", "-c", "set -e\necho setup level"}, verifySvc.execCalls[0].command)
}

func TestReprovisionCurrentStep_Force_ExportsForceFlagIntoTheScript(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "reprovision-force", models.ScenarioStep{
		BackgroundScript:         "#!/bin/bash\necho setup level",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("current_step", 1).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.ReprovisionCurrentStep(session.ID, true)
	require.NoError(t, err)

	require.Len(t, verifySvc.execCalls, 1)
	assert.Equal(t, []string{"/bin/bash", "-c", "#!/bin/bash\nset -e\nexport FORCE=1\necho setup level"},
		verifySvc.execCalls[0].command,
		"FORCE lands after the shebang, so the interpreter is still resolved from it")
}

func TestReprovisionCurrentStep_RecoversASetupFailedSession(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "reprovision-recover", models.ScenarioStep{
		BackgroundScript:         "echo retry",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"current_step": 1, "status": "setup_failed"}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, "active", sessionStatus(t, db, session.ID),
		"a successful retry puts the learner back in the run")
}

func TestReprovisionCurrentStep_FailedRetry_LeavesTheSessionMarkedBroken(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "reprovision-fails-again", models.ScenarioStep{
		BackgroundScript:         "echo retry",
		BackgroundTimeoutSeconds: 5,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{"current_step": 1, "status": "setup_failed"}).Error)

	verifySvc := &bgTrackingVerificationService{execErr: fmt.Errorf("container still unreachable")}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.Error(t, err)
	assert.Equal(t, "setup_failed", sessionStatus(t, db, session.ID),
		"a retry that fails must not leave the session claiming to be playable")
}

func TestReprovisionCurrentStep_AsyncStep_ParksTheSessionInProvisioning(t *testing.T) {
	db := setupTestDB(t)
	session := twoStepSession(t, db, "reprovision-async", models.ScenarioStep{
		BackgroundScript: "echo long setup",
		BackgroundAsync:  true,
	})
	require.NoError(t, db.Model(&models.ScenarioSession{}).
		Where("id = ?", session.ID).Update("current_step", 1).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "provisioning", result.Status)
	assert.Equal(t, "active", waitForSetupDone(t, db, session.ID))
	require.Len(t, verifySvc.execCalls, 1)
}

func TestReprovisionCurrentStep_RejectsSessionsWithNothingToRepair(t *testing.T) {
	db := setupTestDB(t)

	t.Run("abandoned session", func(t *testing.T) {
		session := twoStepSession(t, db, "reprovision-abandoned", models.ScenarioStep{
			BackgroundScript: "echo setup",
		})
		require.NoError(t, db.Model(&models.ScenarioSession{}).
			Where("id = ?", session.ID).
			Updates(map[string]any{"current_step": 1, "status": "abandoned"}).Error)

		verifySvc := &bgTrackingVerificationService{}
		sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

		_, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "abandoned")
		assert.Len(t, verifySvc.execCalls, 0)
	})

	t.Run("step without a background script", func(t *testing.T) {
		session := twoStepSession(t, db, "reprovision-no-script", models.ScenarioStep{
			TextContent: "nothing to provision",
		})
		require.NoError(t, db.Model(&models.ScenarioSession{}).
			Where("id = ?", session.ID).Update("current_step", 1).Error)

		verifySvc := &bgTrackingVerificationService{}
		sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

		_, err := sessionSvc.ReprovisionCurrentStep(session.ID, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no background script")
		assert.Len(t, verifySvc.execCalls, 0)
	})
}
