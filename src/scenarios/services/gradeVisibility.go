package services

import (
	"log/slog"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WithheldGradeSessions is the one rule for whether a learner may see their own
// grade yet: a run of a scenario with an exam-mode quiz step keeps its grade
// hidden until the teacher of a class the learner actively belongs to
// publishes that scenario's results. A preview is the author's own test and is
// never withheld. Only learner-facing responses apply it; teacher views and the
// stored grade are untouched.
//
// It returns the ids of the withheld sessions, in two queries whatever the
// number of sessions. An empty input withholds nothing.
func WithheldGradeSessions(db *gorm.DB, sessions []models.ScenarioSession) (map[uuid.UUID]bool, error) {
	withheld := map[uuid.UUID]bool{}
	scenarioIDs, userIDs := gradedRunKeys(sessions)
	if len(scenarioIDs) == 0 {
		return withheld, nil
	}

	exam, err := ExamScenarios(db, scenarioIDs)
	if err != nil {
		return nil, err
	}
	if len(exam) == 0 {
		return withheld, nil
	}

	var published []learnerRun
	if err := db.Table("scenario_assignments sa").
		Select("DISTINCT sa.scenario_id AS scenario_id, gm.user_id AS user_id").
		Joins("JOIN group_members gm ON gm.group_id = sa.group_id AND gm.is_active = ? AND gm.deleted_at IS NULL", true).
		Where("sa.scenario_id IN ? AND gm.user_id IN ? AND sa.results_published_at IS NOT NULL AND sa.deleted_at IS NULL",
			mapKeys(exam), userIDs).
		Scan(&published).Error; err != nil {
		return nil, err
	}

	released := make(map[learnerRun]bool, len(published))
	for _, p := range published {
		released[p] = true
	}
	for _, s := range sessions {
		if !s.IsPreview && exam[s.ScenarioID] && !released[learnerRun{ScenarioID: s.ScenarioID, UserID: s.UserID}] {
			withheld[s.ID] = true
		}
	}
	return withheld, nil
}

// learnerRun is one learner's runs of one scenario, whatever the attempt.
type learnerRun struct {
	ScenarioID uuid.UUID
	UserID     string
}

// examQuizStep is the one definition of an exam-mode quiz step: a quiz whose
// correction is not shown on submit. table names the scenario_steps alias.
// Grade withholding, the exam lock and the teacher's "has an exam" flag all
// filter through it.
func examQuizStep(table string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(table+".step_type = ? AND "+table+".show_immediate_feedback = ?", "quiz", false)
	}
}

// ExamScenarios is the set of the given scenarios that have at least one
// exam-mode quiz step. An empty input returns none without a query.
func ExamScenarios(db *gorm.DB, scenarioIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	exam := map[uuid.UUID]bool{}
	if len(scenarioIDs) == 0 {
		return exam, nil
	}
	var ids []uuid.UUID
	if err := db.Model(&models.ScenarioStep{}).
		Scopes(examQuizStep("scenario_steps")).
		Where("scenario_id IN ?", scenarioIDs).
		Distinct().Pluck("scenario_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		exam[id] = true
	}
	return exam, nil
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GradeWithheldForLearner is WithheldGradeSessions for one session. A lookup
// failure withholds: showing an exam grade early cannot be taken back.
func GradeWithheldForLearner(db *gorm.DB, session *models.ScenarioSession) bool {
	withheld, err := WithheldGradeSessions(db, []models.ScenarioSession{*session})
	if err != nil {
		slog.Warn("grade visibility lookup failed, withholding", "session_id", session.ID, "err", err)
		return true
	}
	return withheld[session.ID]
}

// gradedRunKeys lists the scenarios and learners of the non-preview sessions.
func gradedRunKeys(sessions []models.ScenarioSession) ([]uuid.UUID, []string) {
	scenarioSet := map[uuid.UUID]bool{}
	userSet := map[string]bool{}
	for _, s := range sessions {
		if s.IsPreview {
			continue
		}
		scenarioSet[s.ScenarioID] = true
		userSet[s.UserID] = true
	}
	scenarioIDs := make([]uuid.UUID, 0, len(scenarioSet))
	for id := range scenarioSet {
		scenarioIDs = append(scenarioIDs, id)
	}
	userIDs := make([]string, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}
	return scenarioIDs, userIDs
}
