package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Mock implementations

type mockFlagService struct {
	flags       []models.ScenarioFlag
	validateRes bool
}

func (m *mockFlagService) GenerateFlags(scenario *models.Scenario, sessionID uuid.UUID, userID string) []models.ScenarioFlag {
	result := make([]models.ScenarioFlag, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		if step.HasFlag {
			flag := models.ScenarioFlag{
				SessionID:    sessionID,
				StepOrder:    step.Order,
				ExpectedFlag: "flag{test-" + userID + "}",
			}
			result = append(result, flag)
		}
	}
	m.flags = result
	return result
}

func (m *mockFlagService) ValidateFlag(expected string, submitted string) bool {
	return m.validateRes
}

type mockVerificationService struct {
	passed bool
	output string
	err    error
}

func (m *mockVerificationService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return m.passed, m.output, m.err
}

func (m *mockVerificationService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	return nil
}

func (m *mockVerificationService) ExecInContainer(sessionID string, command []string, timeout int) (int, string, string, error) {
	return 0, "", "", nil
}


func TestScenarioSessionService_StartScenario(t *testing.T) {
	db := setupTestDB(t)

	// Create a test scenario
	scenario := models.Scenario{
		Name:         "test-scenario",
		Title:        "Test Scenario",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret123",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create steps
	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "First step", HasFlag: true},
		{ScenarioID: scenario.ID, Order: 1, Title: "Step 2", TextContent: "Second step", HasFlag: true},
		{ScenarioID: scenario.ID, Order: 2, Title: "Step 3", TextContent: "Third step", HasFlag: true},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := &mockFlagService{validateRes: true}
	verifySvc := &mockVerificationService{}

	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal-123")

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, scenario.ID, session.ScenarioID)
	assert.Equal(t, "student-1", session.UserID)
	require.NotNil(t, session.TerminalSessionID)
	assert.Equal(t, "test-terminal-123", *session.TerminalSessionID)
	assert.Equal(t, 0, session.CurrentStep)
	assert.Equal(t, "active", session.Status)
	assert.False(t, session.StartedAt.IsZero())

	// Check step progress was created
	assert.Len(t, session.StepProgress, 3)

	// Find progress by step order
	progressMap := make(map[int]models.ScenarioStepProgress)
	for _, sp := range session.StepProgress {
		progressMap[sp.StepOrder] = sp
	}
	assert.Equal(t, "active", progressMap[0].Status)
	assert.Equal(t, "locked", progressMap[1].Status)
	assert.Equal(t, "locked", progressMap[2].Status)

	// Flags should have been generated
	assert.Len(t, session.Flags, 3)
}

func TestScenarioSessionService_StartScenario_NoSteps(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "empty-scenario",
		Title:        "Empty",
		InstanceType: "alpine",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal-456")

	assert.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "no steps")
}

func TestScenarioSessionService_GetCurrentStep(t *testing.T) {
	db := setupTestDB(t)

	// Create scenario and steps
	scenario := models.Scenario{
		Name:         "test-get-step",
		Title:        "Get Step Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step1 := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "First Step",
		TextContent: "Do the first thing",
		HintContent: "Here is a hint",
		HasFlag:     true,
	}
	require.NoError(t, db.Create(&step1).Error)

	step2 := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       1,
		Title:       "Second Step",
		TextContent: "Do the second thing",
	}
	require.NoError(t, db.Create(&step2).Error)

	// Create session at step 0
	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Create step progress
	progress := models.ScenarioStepProgress{
		SessionID: session.ID,
		StepOrder: 0,
		Status:    "active",
	}
	require.NoError(t, db.Create(&progress).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	result, err := sessionSvc.GetCurrentStep(session.ID)

	require.NoError(t, err)
	assert.Equal(t, 0, result.StepOrder)
	assert.Equal(t, "First Step", result.Title)
	assert.Equal(t, "Do the first thing", result.Text)
	assert.Equal(t, "Here is a hint", result.Hint)
	assert.Equal(t, "active", result.Status)
	assert.True(t, result.HasFlag)
}

func TestScenarioSessionService_VerifyAndAdvance(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "verify-test",
		Title:        "Verify Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step1 := models.ScenarioStep{
		ScenarioID:   scenario.ID,
		Order:        0,
		Title:        "Step 1",
		VerifyScript: "#!/bin/bash\ntrue",
	}
	require.NoError(t, db.Create(&step1).Error)

	step2 := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      1,
		Title:      "Step 2",
	}
	require.NoError(t, db.Create(&step2).Error)

	terminalID := "terminal-abc"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-1",
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	// Create step progress
	sp1 := models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}
	sp2 := models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}
	require.NoError(t, db.Create(&sp1).Error)
	require.NoError(t, db.Create(&sp2).Error)

	verifySvc := &mockVerificationService{passed: true, output: "OK"}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "OK", result.Output)
	require.NotNil(t, result.NextStep)
	assert.Equal(t, 1, *result.NextStep)

	// Verify session was advanced
	var updatedSession models.ScenarioSession
	db.First(&updatedSession, "id = ?", session.ID)
	assert.Equal(t, 1, updatedSession.CurrentStep)
	assert.Equal(t, "active", updatedSession.Status)

	// Verify step 1 progress was completed
	var updatedSP1 models.ScenarioStepProgress
	db.First(&updatedSP1, "session_id = ? AND step_order = 0", session.ID)
	assert.Equal(t, "completed", updatedSP1.Status)
	assert.Equal(t, 1, updatedSP1.VerifyAttempts)
	assert.NotNil(t, updatedSP1.CompletedAt)

	// Verify step 2 was unlocked
	var updatedSP2 models.ScenarioStepProgress
	db.First(&updatedSP2, "session_id = ? AND step_order = 1", session.ID)
	assert.Equal(t, "active", updatedSP2.Status)
}

func TestScenarioSessionService_VerifyLastStep_CompletesSession(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "verify-last",
		Title:        "Last Step Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:   scenario.ID,
		Order:        0,
		Title:        "Only Step",
		VerifyScript: "#!/bin/bash\ntrue",
	}
	require.NoError(t, db.Create(&step).Error)

	terminalID := "terminal-xyz"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-1",
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	sp := models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}
	require.NoError(t, db.Create(&sp).Error)

	verifySvc := &mockVerificationService{passed: true, output: "Done"}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Nil(t, result.NextStep)

	// Session should be completed
	var updatedSession models.ScenarioSession
	db.First(&updatedSession, "id = ?", session.ID)
	assert.Equal(t, "completed", updatedSession.Status)
	assert.NotNil(t, updatedSession.CompletedAt)
}

func TestScenarioSessionService_VerifyNoTerminal(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "no-terminal",
		Title:        "No Terminal",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step 1"}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
		// No terminal session ID
	}
	require.NoError(t, db.Create(&session).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	result, err := sessionSvc.VerifyCurrentStep(session.ID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no terminal session")
}

func TestScenarioSessionService_SubmitCorrectFlag(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "flag-test",
		Title:        "Flag Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", HasFlag: true}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	flag := models.ScenarioFlag{
		SessionID:    session.ID,
		StepOrder:    0,
		ExpectedFlag: "flag{correct-answer}",
	}
	require.NoError(t, db.Create(&flag).Error)

	flagSvc := &mockFlagService{validateRes: true}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, &mockVerificationService{})

	result, err := sessionSvc.SubmitFlag(session.ID, "flag{correct-answer}")

	require.NoError(t, err)
	assert.True(t, result.Correct)
	assert.Equal(t, "Correct flag", result.Message)

	// Verify flag was updated in DB
	var updatedFlag models.ScenarioFlag
	db.First(&updatedFlag, "id = ?", flag.ID)
	assert.True(t, updatedFlag.IsCorrect)
	require.NotNil(t, updatedFlag.SubmittedFlag)
	assert.Equal(t, "flag{correct-answer}", *updatedFlag.SubmittedFlag)
	assert.NotNil(t, updatedFlag.SubmittedAt)
}

func TestScenarioSessionService_SubmitIncorrectFlag(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "flag-wrong",
		Title:        "Wrong Flag Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", HasFlag: true}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	flag := models.ScenarioFlag{
		SessionID:    session.ID,
		StepOrder:    0,
		ExpectedFlag: "flag{correct-answer}",
	}
	require.NoError(t, db.Create(&flag).Error)

	flagSvc := &mockFlagService{validateRes: false}
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, &mockVerificationService{})

	result, err := sessionSvc.SubmitFlag(session.ID, "flag{wrong}")

	require.NoError(t, err)
	assert.False(t, result.Correct)
	assert.Equal(t, "Incorrect flag", result.Message)

	// Verify flag was updated but not marked correct
	var updatedFlag models.ScenarioFlag
	db.First(&updatedFlag, "id = ?", flag.ID)
	assert.False(t, updatedFlag.IsCorrect)
	require.NotNil(t, updatedFlag.SubmittedFlag)
	assert.Equal(t, "flag{wrong}", *updatedFlag.SubmittedFlag)
}

func TestScenarioSessionService_AbandonSession(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "abandon-test",
		Title:        "Abandon Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "student-1",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	err := sessionSvc.AbandonSession(session.ID)

	require.NoError(t, err)

	var updatedSession models.ScenarioSession
	db.First(&updatedSession, "id = ?", session.ID)
	assert.Equal(t, "abandoned", updatedSession.Status)
}

func TestScenarioSessionService_AbandonSession_NotFound(t *testing.T) {
	db := setupTestDB(t)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	err := sessionSvc.AbandonSession(uuid.New())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

// Verify that the DTO types match what the service returns
func TestScenarioSessionService_ResponseTypes(t *testing.T) {
	// Compile-time check that returned types match expected DTOs
	var _ *dto.CurrentStepResponse
	var _ *dto.VerifyStepResponse
	var _ *dto.SubmitFlagResponse
}
