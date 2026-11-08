package services

import (
	"crypto/subtle"
	"encoding/json"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
)

// QuestionScore is one question's outcome. Correct means answered exactly
// right, in every scoring mode; Earned may be negative under partial scoring.
type QuestionScore struct {
	Correct bool
	Earned  float64
	Max     float64
}

// QuizGrade is a quiz step's outcome. Questions is index-aligned with the
// questions that were scored.
type QuizGrade struct {
	Score        float64
	CorrectCount int
	Questions    []QuestionScore
}

// ScoreQuiz is the one place a quiz answer set is graded — the learner's
// submission and the teacher dashboard both go through it, so they can never
// disagree about what was right.
//
// All-or-nothing (the default) scores correct_count / total and ignores
// points, which is what every grade before partial scoring was computed with.
// Partial weights questions by points, credits each right tick of a
// multi_answer question and takes penalty × its share back per wrong tick;
// the step total floors at 0. Partial-question-floor also floors each
// question at 0.
func ScoreQuiz(step models.ScenarioStep, questions []models.ScenarioStepQuestion, answers map[uuid.UUID]string) QuizGrade {
	result := QuizGrade{Questions: make([]QuestionScore, 0, len(questions))}
	if len(questions) == 0 {
		return result
	}
	partial := step.IsPartialScoring()
	floorEachQuestion := step.ScoringMode() == models.QuizScoringPartialQuestionFloor
	penalty := step.WrongPenalty()
	earned, possible := 0.0, 0.0
	for _, q := range questions {
		submitted := answers[q.ID]
		qs := QuestionScore{Correct: answerIsExactlyRight(submitted, q.CorrectAnswer), Max: questionPoints(q)}
		if partial {
			qs.Earned = partialEarned(q, submitted, qs.Max, penalty)
			if floorEachQuestion {
				qs.Earned = max(0, qs.Earned)
			}
		} else if qs.Correct {
			qs.Earned = qs.Max
		}
		if qs.Correct {
			result.CorrectCount++
		}
		earned += qs.Earned
		possible += qs.Max
		result.Questions = append(result.Questions, qs)
	}

	if partial {
		result.Score = min(1, max(0, earned)/possible)
	} else {
		result.Score = float64(result.CorrectCount) / float64(len(questions))
	}
	return result
}

// answerIsExactlyRight compares in constant time, like a flag submission. An
// empty answer is never right, even against an empty correct_answer.
func answerIsExactlyRight(submitted, correct string) bool {
	return submitted != "" && subtle.ConstantTimeCompare([]byte(submitted), []byte(correct)) == 1
}

func questionPoints(q models.ScenarioStepQuestion) float64 {
	if q.Points <= 0 {
		return 1
	}
	return float64(q.Points)
}

func partialEarned(q models.ScenarioStepQuestion, submitted string, points, penalty float64) float64 {
	if q.QuestionType == "multi_answer" {
		if earned, ok := multiAnswerEarned(q.CorrectAnswer, submitted, points, penalty); ok {
			return earned
		}
	}
	switch {
	case answerIsExactlyRight(submitted, q.CorrectAnswer):
		return points
	case submitted == "":
		return 0
	default:
		return -penalty * points
	}
}

// multiAnswerEarned credits points/k per right tick and takes penalty × points/k
// per wrong one, k being the number of right options. ok is false when the
// correct set is unusable (no right option), so the caller falls back to the
// exact comparison. An unparseable submission ticks nothing.
func multiAnswerEarned(correctAnswer, submitted string, points, penalty float64) (float64, bool) {
	var correctIdx []int
	if json.Unmarshal([]byte(correctAnswer), &correctIdx) != nil || len(correctIdx) == 0 {
		return 0, false
	}
	rightSet := make(map[int]bool, len(correctIdx))
	for _, i := range correctIdx {
		rightSet[i] = true
	}
	var ticked []int
	_ = json.Unmarshal([]byte(submitted), &ticked)
	share := points / float64(len(rightSet))
	earned := 0.0
	seen := make(map[int]bool, len(ticked))
	for _, i := range ticked {
		if seen[i] {
			continue
		}
		seen[i] = true
		if rightSet[i] {
			earned += share
		} else {
			earned -= penalty * share
		}
	}
	return earned, true
}
