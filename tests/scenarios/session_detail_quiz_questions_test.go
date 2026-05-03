package scenarios_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// findQuestion returns the question detail with the given Order, or nil.
// Imported via the dto package by reflection-of-tests once the wip tag is dropped.
// We keep the helper signature flexible to avoid pinning to the exact dto type
// import path before the implementation lands.
func findQuestionByOrder(t *testing.T, step services.SessionStepDetail, order int) any {
	t.Helper()
	for _, q := range step.Questions {
		if q.Order == order {
			return q
		}
	}
	t.Fatalf("question with order=%d not found in step questions array (got %d entries)", order, len(step.Questions))
	return nil
}

func TestGetSessionDetail_QuizStep_IncludesQuestionsArray_AfterSubmission(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "qd-after-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "qd-after", Title: "Quiz Detail After", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	q1 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 1,
		QuestionText: "List files?", QuestionType: "multiple_choice",
		Options: `["ls","cd","rm"]`, CorrectAnswer: "ls", Points: 1,
	}
	require.NoError(t, db.Create(&q1).Error)
	q2 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 2,
		QuestionText: "/dev/null discards data?", QuestionType: "true_false",
		CorrectAnswer: "true", Points: 1,
	}
	require.NoError(t, db.Create(&q2).Error)
	q3 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 3,
		QuestionText: "Print working dir?", QuestionType: "free_text",
		CorrectAnswer: "pwd", Points: 1,
	}
	require.NoError(t, db.Create(&q3).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "qd-after-s1",
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Student answered: q1 correct, q2 wrong, q3 correct → 2/3 ≈ 0.667
	answers := map[string]string{
		q1.ID.String(): "ls",
		q2.ID.String(): "false",
		q3.ID.String(): "pwd",
	}
	answersJSON, err := json.Marshal(answers)
	require.NoError(t, err)
	score := 2.0 / 3.0
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID:   session.ID,
		StepOrder:   0,
		Status:      "completed",
		StepType:    "quiz",
		QuizScore:   &score,
		QuizAnswers: string(answersJSON),
		CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	stepDetail := detail.Steps[0]
	assert.Equal(t, "quiz", stepDetail.StepType)
	require.Len(t, stepDetail.Questions, 3,
		"a quiz step must surface its questions array on the trainer view")

	// Use the typed access via JSON round-trip to avoid pinning to the dto import path.
	jsonBytes, err := json.Marshal(stepDetail.Questions)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &got))

	byOrder := map[int]map[string]any{}
	for _, q := range got {
		ord, ok := q["order"].(float64)
		require.True(t, ok, "each question must include an 'order' field")
		byOrder[int(ord)] = q
	}

	require.Contains(t, byOrder, 1)
	require.Contains(t, byOrder, 2)
	require.Contains(t, byOrder, 3)

	// q1 → correct
	assert.Equal(t, "ls", byOrder[1]["student_answer"])
	assert.Equal(t, "ls", byOrder[1]["correct_answer"])
	assert.Equal(t, true, byOrder[1]["is_correct"])

	// q2 → wrong
	assert.Equal(t, "false", byOrder[2]["student_answer"])
	assert.Equal(t, "true", byOrder[2]["correct_answer"])
	assert.Equal(t, false, byOrder[2]["is_correct"])

	// q3 → correct
	assert.Equal(t, "pwd", byOrder[3]["student_answer"])
	assert.Equal(t, "pwd", byOrder[3]["correct_answer"])
	assert.Equal(t, true, byOrder[3]["is_correct"])

	// Touch helper to keep it referenced even if unused above.
	_ = findQuestionByOrder
}

func TestGetSessionDetail_QuizStep_IncludesQuestionsArray_BeforeSubmission(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "qd-before-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "qd-before", Title: "Quiz Detail Before", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	q1 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 1, QuestionText: "Q1", QuestionType: "multiple_choice",
		Options: `["a","b"]`, CorrectAnswer: "a", Points: 1,
	}
	require.NoError(t, db.Create(&q1).Error)
	q2 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 2, QuestionText: "Q2", QuestionType: "true_false",
		CorrectAnswer: "true", Points: 1,
	}
	require.NoError(t, db.Create(&q2).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "qd-before-s1",
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Quiz not yet submitted: no QuizScore, no QuizAnswers.
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID:   session.ID,
		StepOrder:   0,
		Status:      "active",
		StepType:    "quiz",
		QuizScore:   nil,
		QuizAnswers: "",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	stepDetail := detail.Steps[0]
	require.Len(t, stepDetail.Questions, 2,
		"unsubmitted quiz step must still expose the questions metadata so the trainer sees the quiz structure")

	jsonBytes, err := json.Marshal(stepDetail.Questions)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &got))

	for _, q := range got {
		assert.Equal(t, "", q["student_answer"],
			"before submission, every question's student_answer must be empty")
		// is_correct may be omitted if zero-value, so allow either nil or false.
		ic, hasKey := q["is_correct"]
		if hasKey {
			assert.Equal(t, false, ic, "before submission, every question's is_correct must be false")
		}
		assert.NotEmpty(t, q["question_text"], "metadata must still be populated")
	}
}

func TestGetSessionDetail_NonQuizStep_QuestionsArrayOmitted(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "qd-nonquiz-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "qd-nonquiz", Title: "Non-Quiz Step", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Terminal Step", StepType: "terminal",
	}).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "qd-nonquiz-s1",
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	// Marshal the whole step and assert the JSON does not contain a "questions" key.
	jsonBytes, err := json.Marshal(detail.Steps[0])
	require.NoError(t, err)
	var stepMap map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &stepMap))

	_, hasQuestions := stepMap["questions"]
	assert.False(t, hasQuestions,
		"non-quiz steps must omit the 'questions' key from their JSON projection (omitempty)")
}

func TestGetSessionDetail_QuizStep_AnswersJSONMalformed_DegradesGracefully(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "qd-bad-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "qd-bad", Title: "Quiz Bad JSON", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	q1 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 1, QuestionText: "Q1", QuestionType: "multiple_choice",
		Options: `["a"]`, CorrectAnswer: "a", Points: 1,
	}
	require.NoError(t, db.Create(&q1).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "qd-bad-s1",
		Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// QuizAnswers is malformed JSON. The service must NOT error — it must
	// degrade gracefully and report empty student answers.
	score := 0.0
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID:   session.ID,
		StepOrder:   0,
		Status:      "completed",
		StepType:    "quiz",
		QuizScore:   &score,
		QuizAnswers: "not-json",
		CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err,
		"malformed QuizAnswers JSON must NOT propagate as an error from GetSessionDetail")
	require.Len(t, detail.Steps, 1)
	require.Len(t, detail.Steps[0].Questions, 1)

	jsonBytes, err := json.Marshal(detail.Steps[0].Questions)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &got))

	assert.Equal(t, "", got[0]["student_answer"],
		"with malformed answers JSON, every student_answer must be empty (graceful degradation)")
}
