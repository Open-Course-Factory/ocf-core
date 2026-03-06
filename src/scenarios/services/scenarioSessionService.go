package services

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

// FlagServiceInterface defines what ScenarioSessionService needs from FlagService
type FlagServiceInterface interface {
	GenerateFlags(scenario *models.Scenario, sessionID uuid.UUID, userID string) []models.ScenarioFlag
	ValidateFlag(expected string, submitted string) bool
}

// VerificationServiceInterface defines what ScenarioSessionService needs from VerificationService
type VerificationServiceInterface interface {
	VerifyStep(terminalSessionID string, step *models.ScenarioStep) (passed bool, output string, err error)
	PushFile(sessionID string, targetPath string, content string, mode string) error
	ExecInContainer(sessionID string, command []string, timeout int) (exitCode int, stdout string, stderr string, err error)
}

// ScenarioSessionService manages the lifecycle of a student's scenario session
type ScenarioSessionService struct {
	db                  *gorm.DB
	flagService         FlagServiceInterface
	verificationService VerificationServiceInterface
}

// NewScenarioSessionService creates a new session service with its dependencies
func NewScenarioSessionService(db *gorm.DB, flagService FlagServiceInterface, verificationService VerificationServiceInterface) *ScenarioSessionService {
	return &ScenarioSessionService{
		db:                  db,
		flagService:         flagService,
		verificationService: verificationService,
	}
}

// StartScenario creates a new scenario session for a student.
// It creates the session, step progress records, generates flags, and returns session info.
func (s *ScenarioSessionService) StartScenario(userID string, scenarioID uuid.UUID, terminalSessionID string) (*models.ScenarioSession, error) {
	// Check for existing active session for same user + scenario
	var existingSession models.ScenarioSession
	if err := s.db.Where("user_id = ? AND scenario_id = ? AND status IN ?", userID, scenarioID, []string{"in_progress", "active"}).First(&existingSession).Error; err == nil {
		return nil, fmt.Errorf("active session already exists for this scenario")
	}

	// Load scenario with steps
	var scenario models.Scenario
	if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	if len(scenario.Steps) == 0 {
		return nil, fmt.Errorf("scenario has no steps")
	}

	now := time.Now()
	session := &models.ScenarioSession{
		ScenarioID:        scenarioID,
		UserID:            userID,
		TerminalSessionID: &terminalSessionID,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         now,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Create session
		if err := tx.Create(session).Error; err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		// Create step progress for each step
		for i, step := range scenario.Steps {
			status := "locked"
			if i == 0 {
				status = "active"
			}

			progress := models.ScenarioStepProgress{
				SessionID: session.ID,
				StepOrder: step.Order,
				Status:    status,
			}
			if err := tx.Create(&progress).Error; err != nil {
				return fmt.Errorf("failed to create step progress: %w", err)
			}
		}

		// Generate flags if enabled
		if scenario.FlagsEnabled && s.flagService != nil {
			flags := s.flagService.GenerateFlags(&scenario, session.ID, userID)
			for i := range flags {
				if err := tx.Create(&flags[i]).Error; err != nil {
					return fmt.Errorf("failed to create flag: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Reload session with associations
	if err := s.db.Preload("StepProgress").Preload("Flags").First(session, "id = ?", session.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload session: %w", err)
	}

	return session, nil
}

// GetCurrentStep returns the current step content for a session.
func (s *ScenarioSessionService) GetCurrentStep(sessionID uuid.UUID) (*dto.CurrentStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find the current step
	var currentStep *models.ScenarioStep
	for i := range session.Scenario.Steps {
		if session.Scenario.Steps[i].Order == session.CurrentStep {
			currentStep = &session.Scenario.Steps[i]
			break
		}
	}
	if currentStep == nil {
		return nil, fmt.Errorf("current step (order=%d) not found in scenario", session.CurrentStep)
	}

	// Find step progress status
	stepStatus := "locked"
	for _, sp := range session.StepProgress {
		if sp.StepOrder == session.CurrentStep {
			stepStatus = sp.Status
			break
		}
	}

	return &dto.CurrentStepResponse{
		StepOrder: currentStep.Order,
		Title:     currentStep.Title,
		Text:      currentStep.TextContent,
		Hint:      currentStep.HintContent,
		Status:    stepStatus,
		HasFlag:   currentStep.HasFlag,
	}, nil
}

// VerifyCurrentStep runs the verify script for the current step.
func (s *ScenarioSessionService) VerifyCurrentStep(sessionID uuid.UUID) (*dto.VerifyStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if session.TerminalSessionID == nil {
		return nil, fmt.Errorf("no terminal session attached")
	}

	// Find current step
	var currentStep *models.ScenarioStep
	for i := range session.Scenario.Steps {
		if session.Scenario.Steps[i].Order == session.CurrentStep {
			currentStep = &session.Scenario.Steps[i]
			break
		}
	}
	if currentStep == nil {
		return nil, fmt.Errorf("current step (order=%d) not found", session.CurrentStep)
	}

	// Run verification
	passed, output, err := s.verificationService.VerifyStep(*session.TerminalSessionID, currentStep)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	response := &dto.VerifyStepResponse{
		Passed: passed,
		Output: output,
	}

	// Wrap all DB updates in a transaction for consistency
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Update step progress verify attempts
		if err := tx.Model(&models.ScenarioStepProgress{}).
			Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).
			Update("verify_attempts", gorm.Expr("verify_attempts + 1")).Error; err != nil {
			return fmt.Errorf("failed to update verify attempts: %w", err)
		}

		if passed {
			now := time.Now()

			// Calculate time spent on this step and mark as completed
			var stepProgress models.ScenarioStepProgress
			if err := tx.Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).First(&stepProgress).Error; err == nil {
				timeSpent := int(now.Sub(stepProgress.CreatedAt).Seconds())
				if err := tx.Model(&models.ScenarioStepProgress{}).
					Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).
					Updates(map[string]any{
						"status":             "completed",
						"completed_at":       now,
						"time_spent_seconds": timeSpent,
					}).Error; err != nil {
					return fmt.Errorf("failed to mark step completed: %w", err)
				}
			} else {
				// Fallback: update without time calculation
				if err := tx.Model(&models.ScenarioStepProgress{}).
					Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).
					Updates(map[string]any{
						"status":       "completed",
						"completed_at": now,
					}).Error; err != nil {
					return fmt.Errorf("failed to mark step completed: %w", err)
				}
			}

			// Check if this was the last step
			isLastStep := true
			nextStepOrder := -1
			for _, step := range session.Scenario.Steps {
				if step.Order > session.CurrentStep {
					isLastStep = false
					if nextStepOrder == -1 || step.Order < nextStepOrder {
						nextStepOrder = step.Order
					}
				}
			}

			if isLastStep {
				// Mark session as completed
				if err := tx.Model(&session).Updates(map[string]any{
					"status":       "completed",
					"completed_at": now,
				}).Error; err != nil {
					return fmt.Errorf("failed to mark session completed: %w", err)
				}
			} else {
				// Advance to next step
				if err := tx.Model(&session).Update("current_step", nextStepOrder).Error; err != nil {
					return fmt.Errorf("failed to advance step: %w", err)
				}

				// Unlock next step
				if err := tx.Model(&models.ScenarioStepProgress{}).
					Where("session_id = ? AND step_order = ?", session.ID, nextStepOrder).
					Update("status", "active").Error; err != nil {
					return fmt.Errorf("failed to unlock next step: %w", err)
				}

				response.NextStep = &nextStepOrder
			}
		}

		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	return response, nil
}

// SubmitFlag validates a flag submission for the current step.
func (s *ScenarioSessionService) SubmitFlag(sessionID uuid.UUID, submittedFlag string) (*dto.SubmitFlagResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find the flag for the current step
	var flag *models.ScenarioFlag
	for i := range session.Flags {
		if session.Flags[i].StepOrder == session.CurrentStep {
			flag = &session.Flags[i]
			break
		}
	}
	if flag == nil {
		return nil, fmt.Errorf("no flag found for current step %d", session.CurrentStep)
	}

	// Validate the flag
	isCorrect := s.flagService.ValidateFlag(flag.ExpectedFlag, submittedFlag)

	now := time.Now()
	s.db.Model(flag).Updates(map[string]any{
		"submitted_flag": submittedFlag,
		"submitted_at":   now,
		"is_correct":     isCorrect,
	})

	message := "Incorrect flag"
	if isCorrect {
		message = "Correct flag"
	}

	return &dto.SubmitFlagResponse{
		Correct: isCorrect,
		Message: message,
	}, nil
}

// AbandonSession marks a session as abandoned.
func (s *ScenarioSessionService) AbandonSession(sessionID uuid.UUID) error {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ?", sessionID).
		Update("status", "abandoned")

	if result.Error != nil {
		return fmt.Errorf("failed to abandon session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found")
	}

	return nil
}
