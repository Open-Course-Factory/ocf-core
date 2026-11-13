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

func TestGetSessionDetail_QuizStep_IncludesQuestionsArray_AfterSubmission(t *testing.T) {
	db := freshTestDB(t)

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
}

func TestGetSessionDetail_QuizStep_IncludesQuestionsArray_BeforeSubmission(t *testing.T) {
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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
	db := freshTestDB(t)

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

// A survey's answers reach the trainer as the learner gave them, with nothing
// that reads as a correction: no right answer, no verdict, no score (#568).
// Unanswered questions arrive as "" (free text) or "[]" (multi answer).
func TestGetSessionDetail_SurveyStep_ShowsAnswersWithoutCorrection(t *testing.T) {
	db := freshTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sd-survey-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sd-survey", Title: "Survey Detail", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Feedback", StepType: "survey"}
	require.NoError(t, db.Create(&step).Error)
	// A leftover correct_answer and explanation (e.g. a quiz turned survey)
	// must never surface either.
	q1 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 1, QuestionText: "Pace?", QuestionType: "multiple_choice",
		Options: `["Slow","Fast"]`, CorrectAnswer: "0", Explanation: "leftover",
	}
	q2 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 2, QuestionText: "Anything else?", QuestionType: "free_text",
	}
	q3 := models.ScenarioStepQuestion{
		StepID: step.ID, Order: 3, QuestionText: "Topics?", QuestionType: "multi_answer",
		Options: `["grep","sed"]`,
	}
	require.NoError(t, db.Create(&q1).Error)
	require.NoError(t, db.Create(&q2).Error)
	require.NoError(t, db.Create(&q3).Error)

	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sd-survey-s1", Status: "active", StartedAt: time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)
	answersJSON, err := json.Marshal(map[string]string{
		q1.ID.String(): "0",
		q2.ID.String(): "More pipes please",
		q3.ID.String(): "[]",
	})
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed", StepType: "survey",
		QuizAnswers: string(answersJSON), CompletedAt: &now,
	}).Error)

	detail, err := services.NewTeacherDashboardService(db, nil, nil).GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)
	stepDetail := detail.Steps[0]
	assert.Equal(t, "survey", stepDetail.StepType)
	assert.Nil(t, stepDetail.QuizScore, "a survey has no score")
	require.Len(t, stepDetail.Questions, 3, "the trainer sees every survey question")

	jsonBytes, err := json.Marshal(stepDetail.Questions)
	require.NoError(t, err)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &got))
	byOrder := map[int]map[string]any{}
	for _, q := range got {
		byOrder[int(q["order"].(float64))] = q
	}

	assert.Equal(t, "0", byOrder[1]["student_answer"])
	assert.Equal(t, "More pipes please", byOrder[2]["student_answer"])
	assert.Equal(t, "[]", byOrder[3]["student_answer"])
	for order, q := range byOrder {
		assert.Empty(t, q["correct_answer"], "question %d: a survey reveals no right answer", order)
		assert.Empty(t, q["explanation"], "question %d: a survey carries no explanation", order)
		if ic, ok := q["is_correct"]; ok {
			assert.Equal(t, false, ic, "question %d: a survey answer is never marked correct", order)
		}
		assert.NotEmpty(t, q["question_text"])
	}
}

// A survey's progress row keeps quiz_score and quiz_correct_count NULL. Every
// trainer view that reads those columns must take the NULLs in its stride:
// the session detail, the results table and the analytics (#568).
func TestTeacherViews_SurveyNullScores_DoNotBreakResults(t *testing.T) {
	db := freshTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sv-null-s1", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)
	scenario := models.Scenario{Name: "sv-null", Title: "Survey Nulls", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Lab", StepType: "terminal",
	}).Error)
	survey := models.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Feedback", StepType: "survey"}
	require.NoError(t, db.Create(&survey).Error)
	q := models.ScenarioStepQuestion{StepID: survey.ID, Order: 1, QuestionText: "Anything else?", QuestionType: "free_text"}
	require.NoError(t, db.Create(&q).Error)

	// Active, so the results table computes the partial grade itself.
	session := models.ScenarioSession{ScenarioID: scenario.ID, UserID: "sv-null-s1", Status: "active", StartedAt: time.Now()}
	require.NoError(t, db.Create(&session).Error)
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed", CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "completed", CompletedAt: &now, StepType: "survey",
		QuizAnswers: `{"` + q.ID.String() + `":""}`,
	}).Error)

	var stored models.ScenarioStepProgress
	require.NoError(t, db.Where("session_id = ? AND step_order = 1", session.ID).First(&stored).Error)
	require.Nil(t, stored.QuizScore, "fixture: the survey row's score is NULL")
	require.Nil(t, stored.QuizCorrectCount, "fixture: the survey row's correct count is NULL")

	svc := services.NewTeacherDashboardService(db, nil, nil)

	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 2)
	assert.Nil(t, detail.Steps[1].QuizScore)
	assert.Equal(t, int64(0), detail.TotalCorrectPossible, "survey questions are not correct-answer slots")
	require.Len(t, detail.Steps[1].Questions, 1)
	assert.Equal(t, "", detail.Steps[1].Questions[0].StudentAnswer)

	results, err := svc.GetScenarioResults(groupID, scenario.ID, nil, nil)
	require.NoError(t, err)
	require.Len(t, results.Items, 1)
	require.NotNil(t, results.Items[0].Grade)
	assert.InDelta(t, 100.0, *results.Items[0].Grade, 0.01, "the survey is left out of the grade")
	assert.Equal(t, int64(0), results.Items[0].CorrectCount)
	assert.Equal(t, int64(0), results.Items[0].TotalCorrectPossible)

	analytics, err := svc.GetScenarioAnalytics(groupID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), analytics.TotalSessions)
}
