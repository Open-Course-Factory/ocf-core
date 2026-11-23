package services

import (
	"errors"
	"fmt"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrWaitingAtGate refuses an answer to a step the learner already completed
// and is waiting after. It is a session-state refusal (409), like a step whose
// setup is still running.
var ErrWaitingAtGate = fmt.Errorf("%w: this step is done, the next one opens when the teacher allows it", ErrSessionNotActive)

// ErrStepNotCompleted refuses to continue a run whose current step is not done.
var ErrStepNotCompleted = errors.New("the current step is not completed")

// ErrValidateQuizStep refuses a teacher validation of a quiz step: its score
// comes from the learner's answers.
var ErrValidateQuizStep = errors.New("a quiz step is graded from its answers and cannot be validated by hand")

// stepGateOf is the gate a run stops at: the strictest step gate among the
// classes the learner actively belongs to that assign the scenario
// (learnerClassAssignments). A preview, or a run no such class assigns, has
// none and returns nil.
func stepGateOf(db *gorm.DB, session *models.ScenarioSession) (*int, error) {
	if session.IsPreview {
		return nil, nil
	}
	classes, err := learnerClassAssignments(db, []uuid.UUID{session.ScenarioID}, []string{session.UserID})
	if err != nil {
		return nil, err
	}
	return classes[learnerRun{ScenarioID: session.ScenarioID, UserID: session.UserID}].StepGate, nil
}

// gateHoldsAfter reports whether the gate keeps a learner who finished
// currentStep out of the step after it. The last step is never held: finishing
// the scenario is not gated.
func gateHoldsAfter(steps []models.ScenarioStep, currentStep int, gate *int) bool {
	next, hasNext := nextStepOrder(steps, currentStep)
	return hasNext && gate != nil && next >= *gate
}

// nextStepOrder is the order of the step after currentStep, whatever the
// order of the slice; false when currentStep is the last one.
func nextStepOrder(steps []models.ScenarioStep, currentStep int) (int, bool) {
	next, found := 0, false
	for _, step := range steps {
		if step.Order > currentStep && (!found || step.Order < next) {
			next, found = step.Order, true
		}
	}
	return next, found
}

// currentStepCompleted reports whether the run's current step is already
// completed: on a run still going, the learner is waiting at the gate.
func currentStepCompleted(session *models.ScenarioSession) bool {
	for _, p := range session.StepProgress {
		if p.StepOrder == session.CurrentStep {
			return p.Status == "completed"
		}
	}
	return false
}

// ContinueAfterGate moves a run held at the gate into its next step once the
// gate no longer covers it, with the next step's setup and intro as after any
// advance. Still held, it reports so and changes nothing. A second call finds
// the new step not completed and is refused, so it never advances twice.
func (s *ScenarioSessionService) ContinueAfterGate(sessionID uuid.UUID) (*dto.VerifyStepResponse, error) {
	session, err := s.loadRunForAdvance(sessionID)
	if err != nil {
		return nil, err
	}
	if !currentStepCompleted(session) {
		return nil, ErrStepNotCompleted
	}
	nextOrder, hasNext := nextStepOrder(session.Scenario.Steps, session.CurrentStep)
	if !hasNext {
		return nil, ErrStepNotCompleted
	}
	gate, err := stepGateOf(s.db, session)
	if err != nil {
		return nil, fmt.Errorf("failed to read the step gate: %w", err)
	}
	if gateHoldsAfter(session.Scenario.Steps, session.CurrentStep, gate) {
		return &dto.VerifyStepResponse{Passed: true, WaitingAtGate: true}, nil
	}

	leftStep := session.CurrentStep
	if err := s.db.Transaction(func(tx *gorm.DB) error { return enterStep(tx, session, nextOrder) }); err != nil {
		return nil, err
	}
	return &dto.VerifyStepResponse{
		Passed:                 true,
		NextStep:               &nextOrder,
		StepProvisioningStatus: s.runStepTransition(session, leftStep, nextOrder),
	}, nil
}

// ValidateCurrentStep is a teacher passing the learner's current step by hand.
// It advances like any passed step, so the gate still applies and the last
// step completes the run with its grade. A quiz is refused: its grade comes
// from answers.
func (s *ScenarioSessionService) ValidateCurrentStep(sessionID uuid.UUID) (*dto.VerifyStepResponse, error) {
	session, err := s.loadRunForAdvance(sessionID)
	if err != nil {
		return nil, err
	}
	if currentStepCompleted(session) {
		return nil, ErrWaitingAtGate
	}
	step := FindStepByOrder(session.Scenario.Steps, session.CurrentStep)
	if step == nil {
		return nil, fmt.Errorf("current step (order=%d) not found", session.CurrentStep)
	}
	if step.StepType == models.StepTypeQuiz {
		return nil, ErrValidateQuizStep
	}
	return s.passCurrentStep(session)
}

// loadRunForAdvance loads an active run with what an advance reads: its steps
// in order, its progress and its flags.
func (s *ScenarioSessionService) loadRunForAdvance(sessionID uuid.UUID) (*models.ScenarioSession, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	// An advance is written before the next step is set up: a failed read
	// must not pass for "no terminal" and skip that setup silently.
	if err := DropForeignTerminal(s.db, &session); err != nil {
		return nil, err
	}
	if err := requireActiveSession(&session); err != nil {
		return nil, err
	}
	return &session, nil
}

// ErrStepNotInScenario refuses a gate at a step order the scenario does not have.
var ErrStepNotInScenario = errors.New("step_order is not a step of this scenario")

// SetStepGate locks the class's runs of the scenario at stepOrder, or releases
// them (nil), on every assignment of the scenario to the class.
func (s *TeacherDashboardService) SetStepGate(groupID, scenarioID uuid.UUID, stepOrder *int) error {
	if stepOrder != nil {
		var count int64
		if err := s.db.Model(&models.ScenarioStep{}).
			Where("scenario_id = ? AND \"order\" = ?", scenarioID, *stepOrder).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrStepNotInScenario
		}
	}
	result := s.db.Model(&models.ScenarioAssignment{}).
		Where("group_id = ? AND scenario_id = ?", groupID, scenarioID).
		Update("step_gate", stepOrder)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrScenarioNotAssignedToGroup
	}
	return nil
}
