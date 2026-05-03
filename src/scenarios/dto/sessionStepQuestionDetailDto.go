package dto

import "github.com/google/uuid"

// SessionStepQuestionDetail surfaces per-question detail for a quiz step in the
// trainer dashboard. It includes the student's answer and the canonical correct
// answer so trainers can see HOW the student answered. This DTO is only ever
// embedded in the teacher-side SessionStepDetail response, which is gated by
// Layer 2 GroupRole(manager) — learners never reach the route that serializes
// this type.
type SessionStepQuestionDetail struct {
	ID            uuid.UUID `json:"id"`
	Order         int       `json:"order"`
	QuestionText  string    `json:"question_text"`
	QuestionType  string    `json:"question_type"`
	Options       string    `json:"options,omitempty"`
	CorrectAnswer string    `json:"correct_answer"`
	StudentAnswer string    `json:"student_answer"`
	IsCorrect     bool      `json:"is_correct"`
	Points        int       `json:"points"`
	Explanation   string    `json:"explanation,omitempty"`
}
