package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func TestStartScenario_WithSetupScript_SetsProvisioningPhase(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with a setup script
	scenario := models.Scenario{
		Name:         "provisioning-test",
		Title:        "Provisioning Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		SetupScript:  "#!/bin/bash\necho setup",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a step (required)
	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-prov-1", scenario.ID, "terminal-prov-1")
	require.NoError(t, err)

	// Session should be provisioning with setup_script phase
	assert.Equal(t, "provisioning", session.Status)
	assert.Equal(t, "setup_script", session.ProvisioningPhase)

	// Verify the DB has the same values
	var dbSession models.ScenarioSession
	require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
	assert.Equal(t, "provisioning", dbSession.Status)
	assert.Equal(t, "setup_script", dbSession.ProvisioningPhase)
}

func TestStartScenario_WithoutSetupScript_NoProvisioningPhase(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario without any setup scripts
	scenario := models.Scenario{
		Name:         "no-setup-test",
		Title:        "No Setup Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a step without background script
	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	flagSvc := &mockFlagService{}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-no-setup-1", scenario.ID, "terminal-no-setup-1")
	require.NoError(t, err)

	// Session should be active with no provisioning phase
	assert.Equal(t, "active", session.Status)
	assert.Equal(t, "", session.ProvisioningPhase)
}

func TestProvisioningPhase_ClearedOnActive(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario
	scenario := models.Scenario{
		Name:         "phase-clear-test",
		Title:        "Phase Clear Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "First step",
	}
	require.NoError(t, db.Create(&step).Error)

	// Manually create a session in provisioning state with a phase
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-phase-clear",
		Status:            "provisioning",
		ProvisioningPhase: "setup_script",
		CurrentStep:       0,
		StartedAt:         db.NowFunc(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Simulate transition to active (same logic as runStep0Setup)
	result := db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", session.ID, "provisioning").
		Updates(map[string]any{
			"status":             "active",
			"provisioning_phase": "",
		})
	require.NoError(t, result.Error)
	assert.Equal(t, int64(1), result.RowsAffected)

	// Verify the session is now active with cleared phase
	var dbSession models.ScenarioSession
	require.NoError(t, db.First(&dbSession, "id = ?", session.ID).Error)
	assert.Equal(t, "active", dbSession.Status)
	assert.Equal(t, "", dbSession.ProvisioningPhase)
}
