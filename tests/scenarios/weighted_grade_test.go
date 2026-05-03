package scenarios_test

// Red-phase tests for the weighted grade computation introduced alongside the
// step-type-aware quiz feature. The legacy `CalculateGrade` simply divides
// completed steps by total steps. The new helper distinguishes step types:
//
//   - terminal/flag/info → counts 1.0 if status="completed", else 0
//   - quiz                → counts the per-step QuizScore (a fraction in [0,1])
//                            if QuizScore is non-nil; otherwise 0
//
// Legacy rows with empty step_type are treated as "terminal" so older sessions
// keep their grades.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// --- Pure-helper tests on ComputeWeightedGradeFromLoaded ---

func TestWeightedGrade_AllTerminalSteps_AllCompleted_100(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "terminal"},
		{Order: 1, StepType: "terminal"},
		{Order: 2, StepType: "terminal"},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now},
		{StepOrder: 1, Status: "completed", CompletedAt: &now},
		{StepOrder: 2, Status: "completed", CompletedAt: &now},
	}

	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 100.0, grade, 0.01)
}

func TestWeightedGrade_AllTerminalSteps_TwoOfThreeCompleted_66Pct(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "terminal"},
		{Order: 1, StepType: "terminal"},
		{Order: 2, StepType: "terminal"},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now},
		{StepOrder: 1, Status: "completed", CompletedAt: &now},
		{StepOrder: 2, Status: "active"},
	}

	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 66.666, grade, 0.01)
}

func TestWeightedGrade_QuizStep_HalfScore_Counts50(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "terminal"},
		{Order: 1, StepType: "quiz"},
	}
	now := time.Now()
	score := 0.5
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now},
		{StepOrder: 1, Status: "completed", CompletedAt: &now, StepType: "quiz", QuizScore: &score},
	}

	// (1.0 + 0.5) / 2 = 0.75 → 75%
	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 75.0, grade, 0.01)
}

func TestWeightedGrade_QuizStep_NotSubmitted_CountsZero(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "terminal"},
		{Order: 1, StepType: "quiz"},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now},
		// Quiz step exists but no QuizScore yet — must count as 0.
		{StepOrder: 1, Status: "active", StepType: "quiz"},
	}

	// (1.0 + 0.0) / 2 = 0.5 → 50%
	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 50.0, grade, 0.01)
}

func TestWeightedGrade_InfoStep_Completed_CountsOne(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "info"},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now, StepType: "info"},
	}

	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 100.0, grade, 0.01)
}

func TestWeightedGrade_FlagStep_Completed_CountsOne(t *testing.T) {
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "flag"},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now, StepType: "flag"},
	}

	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 100.0, grade, 0.01)
}

func TestWeightedGrade_NoSteps_ReturnsZero(t *testing.T) {
	grade := services.ComputeWeightedGradeFromLoaded(nil, nil, nil)
	assert.InDelta(t, 0.0, grade, 0.01)
}

func TestWeightedGrade_LegacyEmptyStepType_TreatedAsTerminal(t *testing.T) {
	// Legacy rows: step_type is empty string. Must be treated as "terminal".
	steps := []models.ScenarioStep{
		{Order: 0, StepType: ""},
		{Order: 1, StepType: ""},
	}
	now := time.Now()
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "completed", CompletedAt: &now},
		{StepOrder: 1, Status: "completed", CompletedAt: &now},
	}

	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 100.0, grade, 0.01)
}

func TestWeightedGrade_QuizScoreFullCredit_AndTerminalIncomplete(t *testing.T) {
	// Quiz fully correct (1.0), terminal step still active. Mid-run grade.
	steps := []models.ScenarioStep{
		{Order: 0, StepType: "terminal"},
		{Order: 1, StepType: "quiz"},
	}
	now := time.Now()
	full := 1.0
	progress := []models.ScenarioStepProgress{
		{StepOrder: 0, Status: "active"}, // terminal not yet completed
		{StepOrder: 1, Status: "completed", CompletedAt: &now, StepType: "quiz", QuizScore: &full},
	}

	// (0.0 + 1.0) / 2 = 0.5 → 50%
	grade := services.ComputeWeightedGradeFromLoaded(steps, progress, nil)
	assert.InDelta(t, 50.0, grade, 0.01)
}

// --- ComputeWeightedGrade (DB-backed wrapper) integration tests ---

func TestCalculateGrade_QuizSession_UsesWeightedFormula(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "wgrade-quiz", Title: "Weighted Grade Quiz",
		InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Terminal", StepType: "terminal",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz", StepType: "quiz",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "wg-student-1", Status: "active",
		StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	now := time.Now()
	score := 0.6
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed", CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "completed", CompletedAt: &now,
		StepType: "quiz", QuizScore: &score,
	}).Error)

	// (1.0 + 0.6) / 2 = 0.8 → 80%
	grade, err := services.ComputeWeightedGrade(db, session.ID)
	require.NoError(t, err)
	assert.InDelta(t, 80.0, grade, 0.01,
		"a quiz at 60% combined with a completed terminal must yield 80% under the weighted formula")
}

func TestGetScenarioResults_PartialGrade_UsesWeightedFormula(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO group_members (id, group_id, user_id, role, joined_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), groupID, "wg-results-s1", "member", time.Now(), true).Error)

	scenario := models.Scenario{
		Name: "wgrade-results", Title: "Weighted Results",
		InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Terminal", StepType: "terminal",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz", StepType: "quiz",
	}).Error)

	// Active (not completed) session — no Grade set yet, partial is computed.
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "wg-results-s1", Status: "active",
		StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	now := time.Now()
	score := 0.5
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed", CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "completed", CompletedAt: &now,
		StepType: "quiz", QuizScore: &score,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	paginated, err := svc.GetScenarioResults(groupID, scenario.ID, nil, nil)
	require.NoError(t, err)
	require.Len(t, paginated.Items, 1)

	// Old (broken) behaviour computed completed_steps / total_steps = 2/2 = 100%,
	// or 1/2 = 50% if it didn't count the quiz as completed. Neither is correct.
	// New behaviour: (1.0 terminal + 0.5 quiz) / 2 = 0.75 → 75%.
	require.NotNil(t, paginated.Items[0].Grade,
		"partial grade must be computed for active session with progress")
	assert.InDelta(t, 75.0, *paginated.Items[0].Grade, 0.01,
		"partial grade in scenario results must use the weighted formula (terminal=1.0, quiz=score)")
}
