package services

import (
	"log/slog"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WithheldGradeSessions is the one rule for whether a learner may see their own
// grade yet: a run of a scenario with an exam-mode quiz step keeps its grade
// hidden while a class the learner actively belongs to assigns that scenario
// and none of those classes has published its results. With no such class
// there is no teacher who could publish, so the grade is shown. A preview is
// the author's own test and is never withheld. Only learner-facing responses
// apply it; teacher views and the stored grade are untouched.
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

	awaiting, err := awaitingPublication(db, mapKeys(exam), userIDs)
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if !s.IsPreview && awaiting[learnerRun{ScenarioID: s.ScenarioID, UserID: s.UserID}] {
			withheld[s.ID] = true
		}
	}
	return withheld, nil
}

// awaitingPublication returns the (scenario, learner) pairs where at least one
// class the learner actively belongs to assigns the scenario, and none of those
// classes has published its results.
func awaitingPublication(db *gorm.DB, scenarioIDs []uuid.UUID, userIDs []string) (map[learnerRun]bool, error) {
	var rows []struct {
		ScenarioID     uuid.UUID
		UserID         string
		PublishedCount int
	}
	if err := db.Table("scenario_assignments sa").
		Select("sa.scenario_id AS scenario_id, gm.user_id AS user_id, COUNT(sa.results_published_at) AS published_count").
		Joins("JOIN group_members gm ON gm.group_id = sa.group_id AND gm.is_active = ? AND gm.deleted_at IS NULL", true).
		Where("sa.scenario_id IN ? AND gm.user_id IN ? AND sa.deleted_at IS NULL", scenarioIDs, userIDs).
		Group("sa.scenario_id, gm.user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	awaiting := make(map[learnerRun]bool, len(rows))
	for _, r := range rows {
		if r.PublishedCount == 0 {
			awaiting[learnerRun{ScenarioID: r.ScenarioID, UserID: r.UserID}] = true
		}
	}
	return awaiting, nil
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
