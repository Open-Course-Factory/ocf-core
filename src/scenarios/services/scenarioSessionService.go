package services

import (
	"encoding/json"
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
		if err := tx.Where("user_id = ? AND scenario_id = ? AND status IN ?", userID, scenarioID, []string{"in_progress", "active", "provisioning", "setup_failed"}).First(&existingSession).Error; err == nil {
			// setup_failed sessions are always auto-abandoned — the environment is broken
			shouldAbandon := existingSession.Status == "setup_failed"

			if !shouldAbandon {
				// For other statuses, check if the terminal is still alive
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

	if session.TerminalSessionID != nil && s.verificationService != nil {
		slog.Info("StartScenario post-create",
			"session_id", session.ID,
			"terminal_id", *session.TerminalSessionID,
			"crash_traps", scenario.CrashTraps,
			"flags_count", len(session.Flags),
			"steps_count", len(scenario.Steps),
		)

		// For crash_traps scenarios: push /etc/challenge/config.json with all flags
		// BEFORE running the background script (setup.sh reads this file)
		if scenario.CrashTraps && len(session.Flags) > 0 {
			if err := s.deployChallengeConfig(*session.TerminalSessionID, &scenario, session, userID); err != nil {
				slog.Error("failed to deploy challenge config", "session_id", session.ID, "err", err)
				return nil, fmt.Errorf("failed to deploy challenge config: %w", err)
			}
		}

		// Execute background script for the first step.
		// If Step 0 has a background script, run it asynchronously and set status
		// to "provisioning". A goroutine handles execution, flag deployment, and
		// status transition to "active" once setup completes.
		if len(scenario.Steps) > 0 {
			bgScript := ResolveScriptContent(s.db, scenario.Steps[0].BackgroundScriptID, scenario.Steps[0].BackgroundScript)
			slog.Info("StartScenario background script", "session_id", session.ID, "script_len", len(bgScript))
			if bgScript != "" {
				// Set session to provisioning — frontend will poll until active
				s.db.Model(session).Update("status", "provisioning")
				session.Status = "provisioning"

				go s.runStep0Setup(session.ID, *session.TerminalSessionID, &scenario, session.Flags)
			} else {
				// No background script — deploy flag and stay active
				if len(session.Flags) > 0 {
					s.deploySingleFlagToContainer(*session.TerminalSessionID, &scenario, session.Flags, 0)
				}
			}
		}
	}

	return session, nil
}

// runStep0Setup runs the step 0 background script asynchronously and transitions
// the session from "provisioning" to "active" once setup completes, or to
// "setup_failed" if the script fails.
func (s *ScenarioSessionService) runStep0Setup(sessionID uuid.UUID, terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag) {
	step := &scenario.Steps[0]

	// Execute the background script (uses the 5-minute timeout for step 0)
	if err := s.executeBackgroundScript(terminalSessionID, step); err != nil {
		slog.Error("step 0 setup failed", "session_id", sessionID, "err", err)
		s.db.Model(&models.ScenarioSession{}).
			Where("id = ? AND status = ?", sessionID, "provisioning").
			Update("status", "setup_failed")
		return
	}

	// Deploy the flag for step 0
	if len(flags) > 0 {
		s.deploySingleFlagToContainer(terminalSessionID, scenario, flags, 0)
	}

	// Transition to active — only if still provisioning (not abandoned meanwhile)
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", sessionID, "provisioning").
		Update("status", "active")
	if result.RowsAffected > 0 {
		slog.Info("scenario session setup complete", "session_id", sessionID)
	}
}

// GetCurrentStep returns the current step content for a session.
// If the session is still provisioning (step 0 setup running), returns a
// response with status "provisioning" so the frontend can show a loading state.
func (s *ScenarioSessionService) GetCurrentStep(sessionID uuid.UUID) (*dto.CurrentStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if session.Status == "provisioning" {
		return &dto.CurrentStepResponse{
			StepOrder:  0,
			TotalSteps: len(session.Scenario.Steps),
			Title:      "Setting up environment...",
			Status:     "provisioning",
		}, nil
	}

	if session.Status == "setup_failed" {
		return &dto.CurrentStepResponse{
			StepOrder:  0,
			TotalSteps: len(session.Scenario.Steps),
			Title:      "Environment setup failed",
			Status:     "setup_failed",
		}, nil
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

	// Resolve text/hint content from ProjectFile if available
	textContent := ResolveScriptContent(s.db, currentStep.TextFileID, currentStep.TextContent)
	hintContent := ResolveScriptContent(s.db, currentStep.HintFileID, currentStep.HintContent)

	response := &dto.CurrentStepResponse{
		StepOrder:  currentStep.Order,
		TotalSteps: len(session.Scenario.Steps),
		Title:      currentStep.Title,
		Text:       textContent,
		Hint:       hintContent,
		Status:     stepStatus,
		HasFlag:    currentStep.HasFlag,
	}

	// Add progressive hint metadata
	var totalHints int64
	s.db.Model(&models.ScenarioStepHint{}).Where("step_id = ?", currentStep.ID).Count(&totalHints)
	if totalHints > 0 {
		response.HintsTotalCount = int(totalHints)
		// Find hints_revealed from step progress
		for _, sp := range session.StepProgress {
			if sp.StepOrder == session.CurrentStep {
				response.HintsRevealed = sp.HintsRevealed
				break
			}
		}
		// Don't leak single hint content when progressive hints exist
		response.Hint = ""
	}

	return response, nil
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

	// Resolve text/hint content from ProjectFile if available
	textContent := ResolveScriptContent(s.db, targetStep.TextFileID, targetStep.TextContent)
	hintContent := ResolveScriptContent(s.db, targetStep.HintFileID, targetStep.HintContent)

	response := &dto.CurrentStepResponse{
		StepOrder:  targetStep.Order,
		TotalSteps: len(session.Scenario.Steps),
		Title:      targetStep.Title,
		Text:       textContent,
		Hint:       hintContent,
		Status:     stepStatus,
		HasFlag:    targetStep.HasFlag,
	}

	// Add progressive hint metadata
	var totalHints int64
	s.db.Model(&models.ScenarioStepHint{}).Where("step_id = ?", targetStep.ID).Count(&totalHints)
	if totalHints > 0 {
		response.HintsTotalCount = int(totalHints)
		for _, sp := range session.StepProgress {
			if sp.StepOrder == stepOrder {
				response.HintsRevealed = sp.HintsRevealed
				break
			}
		}
		response.Hint = ""
	}

	return response, nil
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

	// Pre-populate VerifyScript from ProjectFile (VerificationService doesn't have DB access)
	currentStep.VerifyScript = ResolveScriptContent(s.db, currentStep.VerifyScriptID, currentStep.VerifyScript)

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
				_ = s.executeBackgroundScript(*session.TerminalSessionID, &session.Scenario.Steps[i])
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
				_ = s.executeBackgroundScript(*session.TerminalSessionID, &session.Scenario.Steps[i])
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

// RevealHint reveals a progressive hint for a given step in a session.
// Hints must be revealed sequentially (level 1 before level 2, etc.).
// Re-reading an already revealed hint is idempotent.
func (s *ScenarioSessionService) RevealHint(sessionID uuid.UUID, stepOrder int, level int) (*dto.RevealHintResponse, error) {
	// 1. Load session, verify it's active
	var session models.ScenarioSession
	if err := s.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if session.Status != "active" {
		return nil, fmt.Errorf("session is not active")
	}

	// 2. Load step progress, verify step is not locked
	var progress models.ScenarioStepProgress
	if err := s.db.Where("session_id = ? AND step_order = ?", sessionID, stepOrder).First(&progress).Error; err != nil {
		return nil, fmt.Errorf("step progress not found: %w", err)
	}
	if progress.Status == "locked" {
		return nil, fmt.Errorf("step is locked")
	}

	// 3. Find the step model (by scenario_id + order)
	var step models.ScenarioStep
	if err := s.db.Where("scenario_id = ? AND \"order\" = ?", session.ScenarioID, stepOrder).First(&step).Error; err != nil {
		return nil, fmt.Errorf("step not found: %w", err)
	}

	// 4. Count total hints for this step
	var totalHints int64
	s.db.Model(&models.ScenarioStepHint{}).Where("step_id = ?", step.ID).Count(&totalHints)
	if totalHints == 0 {
		return nil, fmt.Errorf("no hints available for this step")
	}

	// 5. Validate level bounds
	if level < 1 || level > int(totalHints) {
		return nil, fmt.Errorf("invalid hint level %d (must be between 1 and %d)", level, totalHints)
	}

	// 6. Enforce sequential reveal: level must be <= hints_revealed + 1
	if level > progress.HintsRevealed+1 {
		return nil, fmt.Errorf("must reveal hint %d before hint %d", progress.HintsRevealed+1, level)
	}

	// 7. Fetch hint content by step_id + level
	var hint models.ScenarioStepHint
	if err := s.db.Where("step_id = ? AND level = ?", step.ID, level).First(&hint).Error; err != nil {
		return nil, fmt.Errorf("hint not found: %w", err)
	}

	// 8. If level > hints_revealed: update hints_revealed (idempotent for re-reads)
	if level > progress.HintsRevealed {
		s.db.Model(&models.ScenarioStepProgress{}).
			Where("session_id = ? AND step_order = ?", sessionID, stepOrder).
			Update("hints_revealed", level)
	}

	return &dto.RevealHintResponse{
		Level:   level,
		Content: hint.Content,
		Total:   int(totalHints),
	}, nil
}

// advanceToNextStep handles step completion and session advancement logic.
// It marks the current step as completed, calculates time spent, and either
// completes the session (if last step) or advances to the next step.
// Returns the next step order (nil if session completed).
func (s *ScenarioSessionService) advanceToNextStep(tx *gorm.DB, session *models.ScenarioSession, now time.Time) (*int, error) {
	// Calculate time spent on this step and mark as completed.
	// Time is measured from when the student started this step:
	// - Step 0: from session start
	// - Other steps: from when the previous step was completed
	var stepProgress models.ScenarioStepProgress
	if err := tx.Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).First(&stepProgress).Error; err == nil {
		stepStartTime := session.StartedAt
		// Find the previous step's completion time
		var prevStep models.ScenarioStepProgress
		if err := tx.Where("session_id = ? AND step_order < ? AND status = ?",
			session.ID, session.CurrentStep, "completed").
			Order("step_order DESC").First(&prevStep).Error; err == nil && prevStep.CompletedAt != nil {
			stepStartTime = *prevStep.CompletedAt
		}
		timeSpent := int(now.Sub(stepStartTime).Seconds())
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
		// For crash_traps scenarios, setup.sh handles flag placement via config.json.
		// Do NOT write a fallback file — it would leak all flags as world-readable files in /tmp/.
		if scenario.CrashTraps {
			return
		}
		flagPath = fmt.Sprintf("/tmp/.flag_step_%d", flag.StepOrder)
	}

	// Validate flag path - prevent path traversal
	if strings.Contains(flagPath, "..") {
		slog.Warn("skipping flag deployment: path contains '..'", "step_order", flag.StepOrder, "path", flagPath)
		return
	}
	if !strings.HasPrefix(flagPath, "/tmp/") && !strings.HasPrefix(flagPath, "/home/") && !strings.HasPrefix(flagPath, "/var/") && !strings.HasPrefix(flagPath, "/opt/") && !strings.HasPrefix(flagPath, "/World/") {
		slog.Warn("skipping flag deployment: path not in allowed prefix", "step_order", flag.StepOrder, "path", flagPath)
		return
	}

	// Push the flag file to the container (with trailing newline for clean cat output)
	if err := s.verificationService.PushFile(terminalSessionID, flagPath, flag.ExpectedFlag+"\n", "0644"); err != nil {
		slog.Warn("failed to deploy flag to container", "step_order", flag.StepOrder, "path", flagPath, "err", err)
	}
}

// deployChallengeConfig pushes /etc/challenge/config.json to the container.
// This is required for crash_traps scenarios where setup.sh reads flags from this file.
// The config contains the student ID, all flags (keyed by step order), and the initial password.
func (s *ScenarioSessionService) deployChallengeConfig(terminalSessionID string, scenario *models.Scenario, session *models.ScenarioSession, userID string) error {
	if s.verificationService == nil {
		return fmt.Errorf("verification service not available")
	}

	// Build the flags map: {"0": "FLAG{...}", "1": "FLAG{...}", ...}
	flagsMap := make(map[string]string)
	for _, flag := range session.Flags {
		flagsMap[fmt.Sprintf("%d", flag.StepOrder)] = flag.ExpectedFlag
	}

	config := map[string]any{
		"student_id":       userID,
		"challenge":        scenario.Name,
		"initial_password": "challenge2026",
		"flags":            flagsMap,
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal challenge config: %w", err)
	}

	// Push to /tmp/ (tt-backend restricts paths), setup.sh will move it
	if err := s.verificationService.PushFile(terminalSessionID, "/tmp/challenge_config.json", string(configJSON), "0600"); err != nil {
		return fmt.Errorf("failed to push challenge config: %w", err)
	}

	slog.Info("deployed challenge config", "session_id", session.ID, "flags_count", len(flagsMap))
	return nil
}

// maxInlineScriptSize is the max script size that can be passed as a command argument.
// tt-backend limits each exec argument to 4KB; scripts larger than this
// are pushed as temp files and executed from disk.
const maxInlineScriptSize = 4000

// Background script execution timeouts.
// Step 0 gets a longer timeout because it typically runs the full environment
// setup (user creation, service provisioning, package installs, etc.).
const (
	bgScriptTimeoutStep0   = 300 // 5 minutes for initial setup
	bgScriptTimeoutDefault = 30  // 30 seconds for subsequent steps
)

// executeBackgroundScript runs a step's background script in the student's container.
// Returns nil on success, an error if the script could not be pushed/executed or exited non-zero.
// For non-step-0 scripts the caller may choose to ignore the error (best-effort).
//
// Small scripts (<=4000 bytes) are passed inline via /bin/sh -c.
// Large scripts are pushed as temp files via PushFile, then executed from disk
// and cleaned up afterward, to avoid tt-backend's 4KB exec argument limit.
func (s *ScenarioSessionService) executeBackgroundScript(terminalSessionID string, step *models.ScenarioStep) error {
	// Resolve background script from ProjectFile if available
	bgScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
	if bgScript == "" {
		return nil
	}
	if s.verificationService == nil {
		return fmt.Errorf("verification service not available")
	}

	timeout := bgScriptTimeoutDefault
	if step.Order == 0 {
		timeout = bgScriptTimeoutStep0
	}

	var exitCode int
	var stderr string
	var err error

	interpreter := parseShebang(bgScript)

	if len(bgScript) <= maxInlineScriptSize {
		// Small scripts: pass inline (fast, single API call)
		exitCode, _, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, "-c", bgScript},
			timeout,
		)
	} else {
		// Large scripts: push as temp file then execute
		tmpPath := fmt.Sprintf("/tmp/.ocf_bg_%d.sh", step.Order)
		if pushErr := s.verificationService.PushFile(terminalSessionID, tmpPath, bgScript, "0700"); pushErr != nil {
			slog.Warn("failed to push background script to container", "step_order", step.Order, "err", pushErr)
			return fmt.Errorf("failed to push script: %w", pushErr)
		}
		exitCode, _, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, tmpPath},
			timeout,
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
		return fmt.Errorf("script execution failed: %w", err)
	}
	if exitCode != 0 {
		slog.Warn("background script exited with non-zero code", "step_order", step.Order, "exit_code", exitCode, "stderr", stderr)
		return fmt.Errorf("script exited with code %d: %s", exitCode, stderr)
	}
	return nil
}
