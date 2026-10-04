package services

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

var (
	// ErrNoStepsToCopy refuses a copy that names no step.
	ErrNoStepsToCopy = errors.New("no steps to copy")
	// ErrStepPositionOutOfRange refuses a position outside 0..len(target steps).
	ErrStepPositionOutOfRange = errors.New("position is outside the target's steps")
	// ErrSourceStepNotFound reports a source step id that names no step.
	ErrSourceStepNotFound = errors.New("source step not found")
)

// StepOutline lists a scenario's steps as ScenarioStepOutline: what each step
// is, never how it is graded. The step columns are selected by name, so a
// script or a flag path is never even loaded.
func StepOutline(db *gorm.DB, scenarioID uuid.UUID) ([]dto.ScenarioStepOutline, error) {
	var steps []models.ScenarioStep
	if err := db.Select("id", "\"order\"", "title", "step_type", "text_content", "has_flag").
		Where("scenario_id = ?", scenarioID).Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("load steps: %w", err)
	}
	stepIDs := make([]uuid.UUID, len(steps))
	for i := range steps {
		stepIDs[i] = steps[i].ID
	}
	hintCounts, err := countPerStep(db, &models.ScenarioStepHint{}, stepIDs)
	if err != nil {
		return nil, err
	}
	questionCounts, err := countPerStep(db, &models.ScenarioStepQuestion{}, stepIDs)
	if err != nil {
		return nil, err
	}
	translations, err := outlineTranslations(db, stepIDs)
	if err != nil {
		return nil, err
	}

	outline := make([]dto.ScenarioStepOutline, len(steps))
	for i, step := range steps {
		outline[i] = dto.ScenarioStepOutline{
			ID:            step.ID,
			Order:         step.Order,
			Title:         step.Title,
			StepType:      step.StepType,
			TextContent:   step.TextContent,
			HasFlag:       step.HasFlag,
			HintCount:     hintCounts[step.ID],
			QuestionCount: questionCounts[step.ID],
			Translations:  translations[step.ID],
		}
	}
	return outline, nil
}

func countPerStep(db *gorm.DB, model any, stepIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	var rows []struct {
		StepID uuid.UUID
		Count  int
	}
	if len(stepIDs) > 0 {
		if err := db.Model(model).Select("step_id, COUNT(*) AS count").
			Where("step_id IN ?", stepIDs).Group("step_id").Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("count per step: %w", err)
		}
	}
	counts := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		counts[row.StepID] = row.Count
	}
	return counts, nil
}

func outlineTranslations(db *gorm.DB, stepIDs []uuid.UUID) (map[uuid.UUID][]dto.ScenarioStepOutlineTranslation, error) {
	var rows []models.ScenarioStepTranslation
	if len(stepIDs) > 0 {
		if err := db.Select("step_id", "locale", "title", "text_content").
			Where("step_id IN ?", stepIDs).Order("locale ASC").Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("load step translations: %w", err)
		}
	}
	byStep := map[uuid.UUID][]dto.ScenarioStepOutlineTranslation{}
	for _, row := range rows {
		byStep[row.StepID] = append(byStep[row.StepID], dto.ScenarioStepOutlineTranslation{
			Locale: row.Locale, Title: row.Title, TextContent: row.TextContent,
		})
	}
	return byStep, nil
}

// SourceScenarioIDsOfSteps returns the scenario of every named step, so the
// caller can authorise each before CopySteps. A step id that names no step is
// ErrSourceStepNotFound.
func SourceScenarioIDsOfSteps(db *gorm.DB, stepIDs []uuid.UUID) ([]uuid.UUID, error) {
	var steps []models.ScenarioStep
	if err := db.Select("id", "scenario_id").Where("id IN ?", stepIDs).Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("load source steps: %w", err)
	}
	found := make(map[uuid.UUID]bool, len(steps))
	seen := map[uuid.UUID]bool{}
	var scenarioIDs []uuid.UUID
	for _, step := range steps {
		found[step.ID] = true
		if !seen[step.ScenarioID] {
			seen[step.ScenarioID] = true
			scenarioIDs = append(scenarioIDs, step.ScenarioID)
		}
	}
	for _, id := range stepIDs {
		if !found[id] {
			return nil, ErrSourceStepNotFound
		}
	}
	return scenarioIDs, nil
}

// CopySteps copies the source steps — content, scripts, hint rows, quiz
// questions, step translations — into the target scenario, in the order given,
// starting at position (nil appends). The target's own steps from position on
// move down, and every order is renumbered 0..n-1, in one transaction.
// Authorisation is the caller's.
func (s *ScenarioDuplicateService) CopySteps(targetID uuid.UUID, sourceStepIDs []uuid.UUID, position *int) ([]models.ScenarioStep, error) {
	if len(sourceStepIDs) == 0 {
		return nil, ErrNoStepsToCopy
	}
	sources, err := s.loadStepsInRequestOrder(sourceStepIDs)
	if err != nil {
		return nil, err
	}

	var copies []models.ScenarioStep
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing []models.ScenarioStep
		if err := tx.Select("id", "\"order\"").Where("scenario_id = ?", targetID).
			Order("\"order\" ASC").Find(&existing).Error; err != nil {
			return fmt.Errorf("load target steps: %w", err)
		}
		at := len(existing)
		if position != nil {
			at = *position
		}
		if at < 0 || at > len(existing) {
			return ErrStepPositionOutOfRange
		}
		if err := renumberAroundInsertion(tx, existing, at, len(sources)); err != nil {
			return err
		}
		for i, src := range sources {
			step, err := copyStepInto(tx, src, targetID, at+i)
			if err != nil {
				return err
			}
			copies = append(copies, step)
		}
		return nil
	})
	return copies, err
}

func (s *ScenarioDuplicateService) loadStepsInRequestOrder(stepIDs []uuid.UUID) ([]models.ScenarioStep, error) {
	var steps []models.ScenarioStep
	if err := s.db.
		Preload("Hints", func(db *gorm.DB) *gorm.DB { return db.Order("level ASC") }).
		Preload("Questions", func(db *gorm.DB) *gorm.DB { return db.Order("\"order\" ASC") }).
		Where("id IN ?", stepIDs).Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("load source steps: %w", err)
	}
	byID := make(map[uuid.UUID]models.ScenarioStep, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	ordered := make([]models.ScenarioStep, len(stepIDs))
	for i, id := range stepIDs {
		step, ok := byID[id]
		if !ok {
			return nil, ErrSourceStepNotFound
		}
		ordered[i] = step
	}
	return ordered, nil
}

// renumberAroundInsertion gives the target's steps contiguous orders, leaving
// a gap of width at index at for the copies.
func renumberAroundInsertion(tx *gorm.DB, existing []models.ScenarioStep, at, width int) error {
	for i, step := range existing {
		order := i
		if i >= at {
			order = i + width
		}
		if order == step.Order {
			continue
		}
		if err := tx.Model(&models.ScenarioStep{}).Where("id = ?", step.ID).Update("order", order).Error; err != nil {
			return fmt.Errorf("renumber step %s: %w", step.ID, err)
		}
	}
	return nil
}
