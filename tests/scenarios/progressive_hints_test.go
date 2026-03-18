package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// --- RevealHint tests ---

func TestRevealHint_Success(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-success", Title: "Hint Success", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something",
	}
	require.NoError(t, db.Create(&step).Error)

	// Create 3 progressive hints for this step
	for i := 1; i <= 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStepHint{
			StepID:  step.ID,
			Level:   i,
			Content: "Hint content level " + string(rune('0'+i)),
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	progress := models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}
	require.NoError(t, db.Create(&progress).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	resp, err := sessionSvc.RevealHint(session.ID, 0, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Level)
	assert.NotEmpty(t, resp.Content)
	assert.Equal(t, 3, resp.Total)

	// Verify HintsRevealed was updated in DB
	var updatedProgress models.ScenarioStepProgress
	db.First(&updatedProgress, "session_id = ? AND step_order = 0", session.ID)
	assert.Equal(t, 1, updatedProgress.HintsRevealed)
}

func TestRevealHint_SequentialEnforcement(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-seq", Title: "Hint Sequential", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	for i := 1; i <= 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStepHint{
			StepID:  step.ID,
			Level:   i,
			Content: "Hint " + string(rune('0'+i)),
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	// Try to reveal level 2 without revealing level 1 first
	_, err := sessionSvc.RevealHint(session.ID, 0, 2)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must reveal hint 1 before")
}

func TestRevealHint_IdempotentReRead(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-idem", Title: "Hint Idempotent", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	require.NoError(t, db.Create(&models.ScenarioStepHint{
		StepID: step.ID, Level: 1, Content: "First hint content",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepHint{
		StepID: step.ID, Level: 2, Content: "Second hint content",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	// Reveal level 1 first time
	resp1, err := sessionSvc.RevealHint(session.ID, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, "First hint content", resp1.Content)

	// Reveal level 1 again (idempotent)
	resp2, err := sessionSvc.RevealHint(session.ID, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, "First hint content", resp2.Content)

	// HintsRevealed should still be 1, not 2
	var progress models.ScenarioStepProgress
	db.First(&progress, "session_id = ? AND step_order = 0", session.ID)
	assert.Equal(t, 1, progress.HintsRevealed)
}

func TestRevealHint_LockedStep(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-locked", Title: "Hint Locked", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	require.NoError(t, db.Create(&models.ScenarioStepHint{
		StepID: step.ID, Level: 1, Content: "A hint",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Step progress is "locked" (not "active")
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "locked",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	_, err := sessionSvc.RevealHint(session.ID, 0, 1)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "locked")
}

func TestRevealHint_NoHints(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-none", Title: "No Hints", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Step with NO ScenarioStepHint records
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	_, err := sessionSvc.RevealHint(session.ID, 0, 1)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no hints")
}

func TestRevealHint_OutOfBounds(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-oob", Title: "Hint Out of Bounds", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	// Only 2 hints
	require.NoError(t, db.Create(&models.ScenarioStepHint{
		StepID: step.ID, Level: 1, Content: "Hint 1",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepHint{
		StepID: step.ID, Level: 2, Content: "Hint 2",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	// Reveal level 1 and 2 successfully
	_, err := sessionSvc.RevealHint(session.ID, 0, 1)
	require.NoError(t, err)
	_, err = sessionSvc.RevealHint(session.ID, 0, 2)
	require.NoError(t, err)

	// Try level 3 which doesn't exist
	_, err = sessionSvc.RevealHint(session.ID, 0, 3)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hint level")
}

// --- GetCurrentStep hint metadata tests ---

func TestGetCurrentStep_WithHintMetadata(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-meta", Title: "Hint Metadata", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something",
	}
	require.NoError(t, db.Create(&step).Error)

	// Create 3 ScenarioStepHint records
	for i := 1; i <= 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStepHint{
			StepID:  step.ID,
			Level:   i,
			Content: "Hint " + string(rune('0'+i)),
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	result, err := sessionSvc.GetCurrentStep(session.ID)

	require.NoError(t, err)
	assert.Equal(t, 3, result.HintsTotalCount)
	assert.Equal(t, 0, result.HintsRevealed)
	// Hint content should NOT be leaked when no hints have been revealed
	assert.Empty(t, result.Hint)
}

func TestGetCurrentStep_WithHints_AfterReveal(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "hint-after", Title: "Hint After Reveal", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something",
	}
	require.NoError(t, db.Create(&step).Error)

	for i := 1; i <= 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStepHint{
			StepID:  step.ID,
			Level:   i,
			Content: "Hint " + string(rune('0'+i)),
		}).Error)
	}

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", CurrentStep: 0, Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Manually set HintsRevealed=2 on progress
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active", HintsRevealed: 2,
	}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	result, err := sessionSvc.GetCurrentStep(session.ID)

	require.NoError(t, err)
	assert.Equal(t, 2, result.HintsRevealed)
}

// --- SplitHintContent helper tests ---

func TestSplitHintContent_MultipleHeaders(t *testing.T) {
	input := "### Indice 1\nFirst hint\n### Indice 2\nSecond hint\n### Indice 3\nThird hint"
	result := services.SplitHintContent(input)

	require.Len(t, result, 3)
	assert.Equal(t, "First hint", result[0])
	assert.Equal(t, "Second hint", result[1])
	assert.Equal(t, "Third hint", result[2])
}

func TestSplitHintContent_SingleContent(t *testing.T) {
	input := "Just a single hint with no headers"
	result := services.SplitHintContent(input)

	require.Len(t, result, 1)
	assert.Equal(t, "Just a single hint with no headers", result[0])
}

func TestSplitHintContent_ColonVariant(t *testing.T) {
	input := "### Indice 1 :\nFirst\n### Indice 2 :\nSecond"
	result := services.SplitHintContent(input)

	require.Len(t, result, 2)
}

// --- SessionStepDetail includes HintsRevealed ---

func TestSessionStepDetail_IncludesHintsRevealed(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "hint-detail-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "hint-detail", Title: "Hint Detail", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create scenario assignment so GetSessionDetail validation passes
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "hint-detail-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Create step progress with HintsRevealed=2
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active", HintsRevealed: 2,
	}).Error)

	teacherSvc := services.NewTeacherDashboardService(db, nil, nil)

	detail, err := teacherSvc.GetSessionDetail(groupID, session.ID)

	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)
	assert.Equal(t, 2, detail.Steps[0].HintsRevealed)
}
