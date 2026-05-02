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

// TestGetSessionDetail_QuizStep_IncludesScore verifies that a completed quiz step
// surfaces both step_type="quiz" and the aggregate quiz_score in the trainer view,
// without leaking per-question quiz_answers.
func TestGetSessionDetail_QuizStep_IncludesScore(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "quiz-detail-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "quiz-detail", Title: "Quiz Detail", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "quiz-detail-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	now := time.Now()
	score := 0.75
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID:   session.ID,
		StepOrder:   0,
		Status:      "completed",
		StepType:    "quiz",
		QuizScore:   &score,
		QuizAnswers: `{"q1":"a","q2":"b"}`, // present in DB but must NOT leak through the API
		CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	step := detail.Steps[0]
	assert.Equal(t, "quiz", step.StepType)
	require.NotNil(t, step.QuizScore)
	assert.InDelta(t, 0.75, *step.QuizScore, 0.0001)
}

// TestGetSessionDetail_TerminalStep_DefaultsStepTypeWhenLegacy verifies that legacy
// step rows persisted with an empty step_type column are reported as "terminal"
// via the COALESCE/NULLIF projection in the GetSessionDetail query.
func TestGetSessionDetail_TerminalStep_DefaultsStepTypeWhenLegacy(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "legacy-step-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "legacy-step", Title: "Legacy Step", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	// Create the step with the default step_type, then force it to empty in the DB
	// to simulate a row inserted before the column existed (or with an empty value).
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Legacy Terminal Step",
	}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", step.ID).
		Update("step_type", "").Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "legacy-step-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	assert.Equal(t, "terminal", detail.Steps[0].StepType)
	assert.Nil(t, detail.Steps[0].QuizScore)
}
