package services

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
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
		// Check for existing active session inside the transaction to prevent race conditions
		var existingSession models.ScenarioSession
		if err := tx.Where("user_id = ? AND scenario_id = ? AND status IN ?", userID, scenarioID, []string{"in_progress", "active"}).First(&existingSession).Error; err == nil {
			// Found an active session — check if its terminal is still alive
			shouldAbandon := false

			if existingSession.TerminalSessionID == nil {
				// Orphan session with no terminal — auto-abandon
				shouldAbandon = true
			} else {
				// Look up the terminal record
				var terminal terminalModels.Terminal
				if err := tx.Where("session_id = ?", *existingSession.TerminalSessionID).First(&terminal).Error; err != nil {
					// Terminal not found (deleted or soft-deleted) — auto-abandon
					shouldAbandon = true
				} else if terminal.Status != "active" {
					// Terminal exists but is expired/stopped — auto-abandon
					shouldAbandon = true
				}
			}

			if shouldAbandon {
				slog.Info("auto-abandoning zombie scenario session",
					"session_id", existingSession.ID,
					"terminal_session_id", existingSession.TerminalSessionID,
					"user_id", userID,
				)
				if err := tx.Model(&existingSession).Update("status", "abandoned").Error; err != nil {
					return fmt.Errorf("failed to abandon zombie session: %w", err)
				}
			} else {
				return fmt.Errorf("active session already exists for this scenario")
			}
		}

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

	// Execute background script for the first step BEFORE deploying its flag,
	// because the script may create directories that the flag path depends on.
	if session.TerminalSessionID != nil && s.verificationService != nil && len(scenario.Steps) > 0 {
		s.executeBackgroundScript(*session.TerminalSessionID, &scenario.Steps[0])
	}

	// Deploy ONLY the flag for the first step (step 0) — not all flags at once.
	// Later steps' flags are deployed on step transition, after their background scripts run.
	if session.TerminalSessionID != nil && s.verificationService != nil && len(session.Flags) > 0 {
		s.deploySingleFlagToContainer(*session.TerminalSessionID, &scenario, session.Flags, 0)
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
		StepOrder:  currentStep.Order,
		TotalSteps: len(session.Scenario.Steps),
		Title:      currentStep.Title,
		Text:       currentStep.TextContent,
		Hint:       currentStep.HintContent,
		Status:     stepStatus,
		HasFlag:    currentStep.HasFlag,
	}, nil
}

// GetStepByOrder returns the content of a specific step by its order for a session.
// Only completed or active steps can be viewed — locked steps are forbidden.
func (s *ScenarioSessionService) GetStepByOrder(sessionID uuid.UUID, stepOrder int) (*dto.CurrentStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find the step at the given order
	var targetStep *models.ScenarioStep
	for i := range session.Scenario.Steps {
		if session.Scenario.Steps[i].Order == stepOrder {
			targetStep = &session.Scenario.Steps[i]
			break
		}
	}
	if targetStep == nil {
		return nil, fmt.Errorf("step (order=%d) not found in scenario", stepOrder)
	}

	// Find step progress status
	stepStatus := "locked"
	for _, sp := range session.StepProgress {
		if sp.StepOrder == stepOrder {
			stepStatus = sp.Status
			break
		}
	}

	// Only allow viewing completed or active steps
	if stepStatus == "locked" {
		return nil, fmt.Errorf("step is locked")
	}

	return &dto.CurrentStepResponse{
		StepOrder:  targetStep.Order,
		TotalSteps: len(session.Scenario.Steps),
		Title:      targetStep.Title,
		Text:       targetStep.TextContent,
		Hint:       targetStep.HintContent,
		Status:     stepStatus,
		HasFlag:    targetStep.HasFlag,
	}, nil
}

// VerifyCurrentStep runs the verify script for the current step.
func (s *ScenarioSessionService) VerifyCurrentStep(sessionID uuid.UUID) (*dto.VerifyStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
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

	if currentStep.HasFlag {
		return nil, fmt.Errorf("this step requires a flag submission, not verification")
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
			nextStep, err := s.advanceToNextStep(tx, &session, now)
			if err != nil {
				return err
			}
			response.NextStep = nextStep
		}

		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	// Execute background script for the next step (after successful DB transaction),
	// then deploy the next step's flag (after the script created any needed directories).
	if response.Passed && response.NextStep != nil && session.TerminalSessionID != nil {
		for i := range session.Scenario.Steps {
			if session.Scenario.Steps[i].Order == *response.NextStep {
				s.executeBackgroundScript(*session.TerminalSessionID, &session.Scenario.Steps[i])
				break
			}
		}
		s.deploySingleFlagToContainer(*session.TerminalSessionID, &session.Scenario, session.Flags, *response.NextStep)
	}

	return response, nil
}

// SubmitFlag validates a flag submission for the current step.
func (s *ScenarioSessionService) SubmitFlag(sessionID uuid.UUID, submittedFlag string) (*dto.SubmitFlagResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
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

	// Check brute-force lockout
	const maxFlagAttempts = 20
	if flag.FlagAttempts >= maxFlagAttempts {
		return &dto.SubmitFlagResponse{
			Correct: false,
			Message: "Too many attempts. Flag submission locked for this step.",
		}, nil
	}

	// Validate the flag
	isCorrect := s.flagService.ValidateFlag(flag.ExpectedFlag, submittedFlag)

	now := time.Now()

	response := &dto.SubmitFlagResponse{
		Correct: isCorrect,
		Message: "Incorrect flag",
	}

	if !isCorrect {
		// For incorrect flags, update outside the transaction since no step advancement is needed
		s.db.Model(flag).Updates(map[string]any{
			"submitted_flag": submittedFlag,
			"submitted_at":   now,
			"is_correct":     false,
			"flag_attempts":  gorm.Expr("flag_attempts + 1"),
		})
		return response, nil
	}

	response.Message = "Correct flag"

	// For correct flags, update the flag record inside the transaction to ensure
	// atomicity with step advancement — if the transaction fails, the flag
	// won't be incorrectly marked as correct while the step hasn't advanced.
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Update flag record inside the transaction
		if err := tx.Model(flag).Updates(map[string]any{
			"submitted_flag": submittedFlag,
			"submitted_at":   now,
			"is_correct":     true,
			"flag_attempts":  gorm.Expr("flag_attempts + 1"),
		}).Error; err != nil {
			return fmt.Errorf("failed to update flag: %w", err)
		}

		nextStep, err := s.advanceToNextStep(tx, &session, now)
		if err != nil {
			return err
		}
		response.NextStep = nextStep

		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	// Execute background script for the next step, then deploy its flag
	// (after the script created any needed directories).
	if response.NextStep != nil && session.TerminalSessionID != nil {
		for i := range session.Scenario.Steps {
			if session.Scenario.Steps[i].Order == *response.NextStep {
				s.executeBackgroundScript(*session.TerminalSessionID, &session.Scenario.Steps[i])
				break
			}
		}
		s.deploySingleFlagToContainer(*session.TerminalSessionID, &session.Scenario, session.Flags, *response.NextStep)
	}

	return response, nil
}

// GetMySessions returns all scenario sessions for the authenticated user.
func (s *ScenarioSessionService) GetMySessions(userID string) ([]dto.MySessionResponse, error) {
	var sessions []models.ScenarioSession
	if err := s.db.Preload("Scenario", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, title")
	}).Preload("StepProgress").
		Where("user_id = ?", userID).
		Order("started_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch sessions: %w", err)
	}

	result := make([]dto.MySessionResponse, 0, len(sessions))
	for _, session := range sessions {
		totalSteps := len(session.StepProgress)
		completedSteps := 0
		for _, sp := range session.StepProgress {
			if sp.Status == "completed" {
				completedSteps++
			}
		}

		resp := dto.MySessionResponse{
			ID:                session.ID,
			ScenarioID:        session.ScenarioID,
			ScenarioTitle:     session.Scenario.Title,
			Status:            session.Status,
			Grade:             session.Grade,
			CurrentStep:       session.CurrentStep,
			TotalSteps:        totalSteps,
			CompletedSteps:    completedSteps,
			StartedAt:         session.StartedAt,
			CompletedAt:       session.CompletedAt,
			TerminalSessionID: session.TerminalSessionID,
		}
		result = append(result, resp)
	}
	return result, nil
}

// AbandonSession marks a session as abandoned. Only active sessions can be abandoned.
func (s *ScenarioSessionService) AbandonSession(sessionID uuid.UUID) error {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", sessionID, "active").
		Update("status", "abandoned")

	if result.Error != nil {
		return fmt.Errorf("failed to abandon session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found or not active")
	}

	return nil
}

// advanceToNextStep handles step completion and session advancement logic.
// It marks the current step as completed, calculates time spent, and either
// completes the session (if last step) or advances to the next step.
// Returns the next step order (nil if session completed).
func (s *ScenarioSessionService) advanceToNextStep(tx *gorm.DB, session *models.ScenarioSession, now time.Time) (*int, error) {
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
			return nil, fmt.Errorf("failed to mark step completed: %w", err)
		}
	} else {
		// Fallback: update without time calculation
		if err := tx.Model(&models.ScenarioStepProgress{}).
			Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).
			Updates(map[string]any{
				"status":       "completed",
				"completed_at": now,
			}).Error; err != nil {
			return nil, fmt.Errorf("failed to mark step completed: %w", err)
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
		// Calculate grade: percentage of completed steps
		completedSteps := 0
		for _, sp := range session.StepProgress {
			if sp.Status == "completed" {
				completedSteps++
			}
		}
		completedSteps++ // current step being completed now
		totalSteps := len(session.Scenario.Steps)
		grade := float64(completedSteps) / float64(totalSteps) * 100.0

		// Mark session as completed with grade
		if err := tx.Model(session).Updates(map[string]any{
			"status":       "completed",
			"completed_at": now,
			"grade":        grade,
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to mark session completed: %w", err)
		}
		return nil, nil
	}

	// Advance to next step
	if err := tx.Model(session).Update("current_step", nextStepOrder).Error; err != nil {
		return nil, fmt.Errorf("failed to advance step: %w", err)
	}

	// Unlock next step
	if err := tx.Model(&models.ScenarioStepProgress{}).
		Where("session_id = ? AND step_order = ?", session.ID, nextStepOrder).
		Update("status", "active").Error; err != nil {
		return nil, fmt.Errorf("failed to unlock next step: %w", err)
	}

	return &nextStepOrder, nil
}

// deploySingleFlagToContainer pushes the flag for a specific step into the student's container.
// This is called on step transitions so that each flag is deployed only after its step's
// background script has run (which may create the directories the flag path depends on).
func (s *ScenarioSessionService) deploySingleFlagToContainer(terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag, stepOrder int) {
	if s.verificationService == nil {
		return
	}

	// Find the flag for this step
	var flag *models.ScenarioFlag
	for i := range flags {
		if flags[i].StepOrder == stepOrder {
			flag = &flags[i]
			break
		}
	}
	if flag == nil {
		return // No flag for this step (step may not have HasFlag enabled)
	}

	// Find the step definition for FlagPath
	var step *models.ScenarioStep
	for i := range scenario.Steps {
		if scenario.Steps[i].Order == stepOrder {
			step = &scenario.Steps[i]
			break
		}
	}
	if step == nil {
		return
	}

	// Determine the target path for the flag file
	flagPath := step.FlagPath
	if flagPath == "" {
		flagPath = fmt.Sprintf("/tmp/.flag_step_%d", flag.StepOrder)
	}

	// Validate flag path - prevent path traversal
	if strings.Contains(flagPath, "..") {
		slog.Warn("skipping flag deployment: path contains '..'", "step_order", flag.StepOrder, "path", flagPath)
		return
	}
	if !strings.HasPrefix(flagPath, "/tmp/") && !strings.HasPrefix(flagPath, "/home/") && !strings.HasPrefix(flagPath, "/World/") {
		flagPath = fmt.Sprintf("/tmp/.flag_step_%d", flag.StepOrder)
	}

	// Push the flag file to the container (with trailing newline for clean cat output)
	if err := s.verificationService.PushFile(terminalSessionID, flagPath, flag.ExpectedFlag+"\n", "0644"); err != nil {
		slog.Warn("failed to deploy flag to container", "step_order", flag.StepOrder, "path", flagPath, "err", err)
	}
}

// maxInlineScriptSize is the max script size that can be passed as a command argument.
// tt-backend limits each exec argument to 4KB; scripts larger than this
// are pushed as temp files and executed from disk.
const maxInlineScriptSize = 4000

// executeBackgroundScript runs a step's background script in the student's container.
// This is best-effort: errors are logged but don't fail the step transition,
// following the same pattern as deploySingleFlagToContainer.
//
// Small scripts (<=4000 bytes) are passed inline via /bin/sh -c.
// Large scripts are pushed as temp files via PushFile, then executed from disk
// and cleaned up afterward, to avoid tt-backend's 4KB exec argument limit.
func (s *ScenarioSessionService) executeBackgroundScript(terminalSessionID string, step *models.ScenarioStep) {
	if step.BackgroundScript == "" {
		return
	}
	if s.verificationService == nil {
		return
	}

	var exitCode int
	var stderr string
	var err error

	interpreter := parseShebang(step.BackgroundScript)

	if len(step.BackgroundScript) <= maxInlineScriptSize {
		// Small scripts: pass inline (fast, single API call)
		exitCode, _, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, "-c", step.BackgroundScript},
			30,
		)
	} else {
		// Large scripts: push as temp file then execute
		tmpPath := fmt.Sprintf("/tmp/.ocf_bg_%d.sh", step.Order)
		if pushErr := s.verificationService.PushFile(terminalSessionID, tmpPath, step.BackgroundScript, "0700"); pushErr != nil {
			slog.Warn("failed to push background script to container", "step_order", step.Order, "err", pushErr)
			return
		}
		exitCode, _, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, tmpPath},
			30,
		)
		// Best-effort cleanup
		_, _, _, _ = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{"rm", "-f", tmpPath},
			5,
		)
	}

	if err != nil {
		slog.Warn("background script failed to execute", "step_order", step.Order, "err", err)
		return
	}
	if exitCode != 0 {
		slog.Warn("background script exited with non-zero code", "step_order", step.Order, "exit_code", exitCode, "stderr", stderr)
	}
}
