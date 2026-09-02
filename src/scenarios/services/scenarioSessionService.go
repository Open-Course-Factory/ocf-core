package services

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/observability"
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
	// WriteToConsole types text into the learner's live shell. Distinct from
	// ExecInContainer: that runs in a separate process, this runs in the shell
	// the learner is watching, which is what a foreground script requires.
	WriteToConsole(sessionID string, text string) error
	ExecInContainer(sessionID string, command []string, env map[string]string, timeout int) (exitCode int, stdout string, stderr string, err error)
}

// defaultAllowedFlagPaths is the fallback list of allowed path prefixes for flag deployment
// when a scenario does not define its own AllowedFlagPaths.
var defaultAllowedFlagPaths = []string{"/tmp/", "/home/", "/var/", "/opt/", "/World/"}

// parseAllowedFlagPaths splits a comma-separated string of path prefixes into a slice.
// Each prefix is trimmed of whitespace. Empty entries are ignored.
func parseAllowedFlagPaths(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isFlagPathAllowed checks whether flagPath starts with any of the allowed prefixes.
func isFlagPathAllowed(flagPath string, allowedPrefixes []string) bool {
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(flagPath, prefix) {
			return true
		}
	}
	return false
}

// findStepByOrder returns the step whose Order matches, or nil. Step orders are
// data-driven (0- or 1-based depending on the authoring path), so every lookup
// goes through the Order field rather than through slice indexing.
func findStepByOrder(steps []models.ScenarioStep, order int) *models.ScenarioStep {
	for i := range steps {
		if steps[i].Order == order {
			return &steps[i]
		}
	}
	return nil
}

// findFlagByStepOrder returns the flag generated for a given step, or nil when
// the step has none.
func findFlagByStepOrder(flags []models.ScenarioFlag, order int) *models.ScenarioFlag {
	for i := range flags {
		if flags[i].StepOrder == order {
			return &flags[i]
		}
	}
	return nil
}

// Session status vocabulary for the provisioning lifecycle. Declared together
// because the answers to "may the learner act", "may this be reprovisioned" and
// "may setup start" were each written out by hand at their call site and had
// already drifted: one accepted a status the others rejected, and the
// difference was invisible until an action was silently refused.
//
// The wider status set ("completed", "abandoned", …) still travels as literals
// elsewhere in the package; these are the ones this engine decides on.
const (
	statusActive       = "active"
	statusInProgress   = "in_progress"
	statusProvisioning = "provisioning"
	statusSetupFailed  = "setup_failed"
)

// reprovisionableStatuses are the states from which re-running a step's setup
// makes sense: a run in progress, or one whose setup already failed. A session
// already provisioning has a goroutine on the job; a completed or abandoned one
// has no run to repair.
var reprovisionableStatuses = []string{statusActive, statusInProgress, statusSetupFailed}

// ErrSessionNotActive marks a learner action rejected because the session is
// not in a state that accepts it. Controllers match on it to answer 409 rather
// than a generic failure, so the frontend can retry instead of giving up.
var ErrSessionNotActive = errors.New("session is not active")

// ErrActiveSessionExists is returned when the learner already has a run of this
// scenario that is still usable. It is a conflict the caller can act on — resume
// the existing run — not a failure, so it must not reach the client as a 500:
// an opaque server error reads as "the scenario is broken" and sends the learner
// back to relaunch, which fails again for the same reason.
var ErrActiveSessionExists = errors.New("active session already exists for this scenario")

// requireActiveSession is the single owner of "may the learner act on this
// session right now". Provisioning is the case that matters: step setup can now
// run mid-scenario, so a second browser tab could otherwise submit against a
// container being rebuilt under it.
// in_progress is accepted alongside active: it is the historical name for a
// running session and still appears on rows created before the rename, so
// rejecting it would lock those learners out of verify and submit entirely.
func requireActiveSession(session *models.ScenarioSession) error {
	if session.Status == statusActive || session.Status == statusInProgress {
		return nil
	}
	if session.Status == statusProvisioning {
		return fmt.Errorf("%w: the environment for this step is still being prepared", ErrSessionNotActive)
	}
	return fmt.Errorf("%w: session is %s", ErrSessionNotActive, session.Status)
}

// TerminalStopFunc is a callback to stop a terminal session (injected from controller layer)
type TerminalStopFunc func(terminalSessionID string) error

// TerminalBuildCompleteFunc is a callback that ends a session's provisioning
// window, removing the features it held only to be built (injected from the
// controller layer, same reason as TerminalStopFunc: no import cycle).
type TerminalBuildCompleteFunc func(terminalSessionID string) error

// ScenarioSessionService manages the lifecycle of a student's scenario session
type ScenarioSessionService struct {
	db                  *gorm.DB
	flagService         FlagServiceInterface
	verificationService VerificationServiceInterface
	stopTerminal        TerminalStopFunc
	buildComplete       TerminalBuildCompleteFunc
}

// NewScenarioSessionService creates a new session service with its dependencies
func NewScenarioSessionService(db *gorm.DB, flagService FlagServiceInterface, verificationService VerificationServiceInterface) *ScenarioSessionService {
	return &ScenarioSessionService{
		db:                  db,
		flagService:         flagService,
		verificationService: verificationService,
	}
}

// SetTerminalStopFunc sets the callback used to stop terminal sessions on failure
func (s *ScenarioSessionService) SetTerminalStopFunc(fn TerminalStopFunc) {
	s.stopTerminal = fn
}

// SetTerminalBuildCompleteFunc sets the callback that takes away the features a
// container held only while it was being provisioned.
func (s *ScenarioSessionService) SetTerminalBuildCompleteFunc(fn TerminalBuildCompleteFunc) {
	s.buildComplete = fn
}

// finishBuild closes the provisioning window, taking back any feature the
// container was given only to be built.
//
// Called on every exit from setup — success, setup failure, or panic — because
// the container outlives a failed setup: the learner sees an error state and
// the row is torn down later, and until then a NIC nobody intended is still
// attached. A scenario that declared no build features costs one call that
// removes nothing.
//
// Best-effort by design. Failing to close the window must not turn a working
// scenario into a failed one, so it is logged loudly rather than escalated —
// the session is usable, just more connected than it should be.
func (s *ScenarioSessionService) finishBuild(sessionID uuid.UUID, terminalSessionID string) {
	if s.buildComplete == nil || terminalSessionID == "" {
		return
	}
	if err := s.buildComplete(terminalSessionID); err != nil {
		slog.Error("scenario build window left open — session still holds its build-time features",
			"session_id", sessionID, "terminal_session_id", terminalSessionID, "err", err)
	}
}

// tryStopTerminal stops the linked terminal session (best-effort, logs on failure)
func (s *ScenarioSessionService) tryStopTerminal(terminalSessionID string, sessionID uuid.UUID) {
	if s.stopTerminal == nil || terminalSessionID == "" {
		return
	}
	if err := s.stopTerminal(terminalSessionID); err != nil {
		observability.Metrics.TerminalStopOnCleanupFailure.Add(1)
		slog.Error("failed to stop terminal — container may be orphaned", "terminal_session_id", terminalSessionID, "session_id", sessionID, "err", err)
	}
}

// StartScenario creates a new scenario session for a student.
// It creates the session, step progress records, generates flags, and returns session info.
// assertLocaleLaunchable refuses a locale the scenario does not fully cover.
// The empty locale — every session that predates this feature — is always
// allowed and means the scenario's own content.
func (s *ScenarioSessionService) assertLocaleLaunchable(scenarioID uuid.UUID, locale string) error {
	if locale == "" {
		return nil
	}
	launchable, err := LaunchableLocales(s.db, scenarioID)
	if err != nil {
		return fmt.Errorf("failed to resolve available languages: %w", err)
	}
	for _, available := range launchable {
		if available == locale {
			return nil
		}
	}
	return fmt.Errorf("this scenario is not available in %q", locale)
}

func (s *ScenarioSessionService) StartScenario(userID string, scenarioID uuid.UUID, terminalSessionID string, locale string) (*models.ScenarioSession, error) {
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

	// Initialize CurrentStep to the first step's actual Order, not a
	// hardcoded 0. Editor-created scenarios use 1-based ordering; legacy
	// seeded ones may use 0-based. Either way, GetCurrentStep looks up
	// the step whose Order matches CurrentStep, so seed it from data.
	// A language the scenario cannot actually deliver is refused rather than
	// quietly downgraded. A learner who asked for French and silently got
	// English would read correct text describing the wrong world — the failure
	// looks like a broken scenario, not like a missing translation.
	if err := s.assertLocaleLaunchable(scenarioID, locale); err != nil {
		return nil, err
	}

	firstStepOrder := scenario.Steps[0].Order

	now := time.Now()
	session := &models.ScenarioSession{
		ScenarioID:        scenarioID,
		UserID:            userID,
		TerminalSessionID: &terminalSessionID,
		CurrentStep:       firstStepOrder,
		Status:            "active",
		StartedAt:         now,
		Locale:            locale,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Check if the terminal already has any scenario bound (permanent binding)
		// Only enforce when a terminal session ID is actually provided
		if terminalSessionID != "" {
			var existingCount int64
			if err := tx.Model(&models.ScenarioSession{}).Where("terminal_session_id = ?", terminalSessionID).Count(&existingCount).Error; err != nil {
				return fmt.Errorf("failed to check terminal binding: %w", err)
			}
			if existingCount > 0 {
				return fmt.Errorf("terminal already has a scenario bound")
			}
		}

		// One rule, evaluated in the transaction so a concurrent launch cannot
		// slip past it. GetResumableSession answers the same question read-only
		// for the availability endpoint.
		existingSession, resumable, findErr := findExistingSession(tx, userID, scenarioID)
		if findErr != nil {
			return findErr
		}
		if existingSession != nil {
			if resumable {
				return ErrActiveSessionExists
			}
			slog.Info("auto-abandoning zombie scenario session",
				"session_id", existingSession.ID,
				"terminal_session_id", existingSession.TerminalSessionID,
				"user_id", userID,
			)
			if err := tx.Model(existingSession).Update("status", "abandoned").Error; err != nil {
				return fmt.Errorf("failed to abandon zombie session: %w", err)
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

		// Execute scenario-level setup script and/or step 0 background script.
		// Setup script runs first (global env prep), then step 0 background.
		if len(scenario.Steps) > 0 {
			setupScript := ResolveScriptContent(s.db, scenario.SetupScriptID, scenario.SetupScript)
			bgScript := ResolveScriptContent(s.db, scenario.Steps[0].BackgroundScriptID, scenario.Steps[0].BackgroundScript)
			slog.Info("StartScenario scripts", "session_id", session.ID, "setup_len", len(setupScript), "bg_len", len(bgScript))
			if setupScript != "" || bgScript != "" {
				// Set session to provisioning — frontend will poll until active
				s.db.Model(session).Updates(map[string]any{
					"status":             "provisioning",
					"provisioning_phase": "setup_script",
				})
				session.Status = "provisioning"
				session.ProvisioningPhase = "setup_script"

				go s.runStep0Setup(session.ID, *session.TerminalSessionID, &scenario, session.Flags, session.Locale)
			} else {
				// No scripts — deploy flag and stay active. Best-effort: the
				// helper logs, and reprovision-step is the retry.
				if len(session.Flags) > 0 {
					_ = s.deploySingleFlagToContainer(*session.TerminalSessionID, &scenario, session.Flags, 0)
				}
				// A step 0 carrying a banner but no scripts never reaches
				// runStep0Setup, so stage its intro here too. Without this the
				// simplest scenarios are exactly the ones whose configured
				// banner silently never appears.
				s.deliverStepZeroIntro(*session.TerminalSessionID, &scenario.Steps[0], session.ID)
				s.warnIfEffectsUnsupported(*session.TerminalSessionID, &scenario, session.ID)
				// Nothing ran, but the container was still created with the
				// build features attached — a scenario can declare them and
				// have no scripts. Close the window here too.
				s.finishBuild(session.ID, *session.TerminalSessionID)
			}
		}
	}

	return session, nil
}

// PreviewOption configures optional behavior for PreviewScenario.
type PreviewOption func(*previewConfig)

type previewConfig struct {
	isOrgManager func(userID string, orgID uuid.UUID) bool
	isAdmin      bool
}

// WithOrgManagerCheck injects a callback to check if a user is an org manager.
func WithOrgManagerCheck(fn func(userID string, orgID uuid.UUID) bool) PreviewOption {
	return func(c *previewConfig) {
		c.isOrgManager = fn
	}
}

// WithAdminBypass marks the caller as a platform admin, bypassing authorization.
func WithAdminBypass() PreviewOption {
	return func(c *previewConfig) {
		c.isAdmin = true
	}
}

// PreviewScenario creates a preview session for testing a scenario without group assignment.
// Only the scenario creator, an org manager (if the scenario belongs to an org), or a
// platform admin may preview.
func (s *ScenarioSessionService) PreviewScenario(userID string, scenarioID uuid.UUID, terminalSessionID string, opts ...PreviewOption) (*models.ScenarioSession, error) {
	cfg := &previewConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Load scenario
	var scenario models.Scenario
	if err := s.db.First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	// Authorization: creator, org manager, or admin
	authorized := cfg.isAdmin || scenario.CreatedByID == userID
	if !authorized && scenario.OrganizationID != nil && cfg.isOrgManager != nil {
		authorized = cfg.isOrgManager(userID, *scenario.OrganizationID)
	}
	if !authorized {
		return nil, fmt.Errorf("not authorized to preview this scenario")
	}

	// Delegate to StartScenario for session creation
	// Preview runs in the scenario's own language: there is no learner here to
	// have chosen one.
	session, err := s.StartScenario(userID, scenarioID, terminalSessionID, "")
	if err != nil {
		return nil, err
	}

	// Mark as preview
	if err := s.db.Model(session).Update("is_preview", true).Error; err != nil {
		return nil, fmt.Errorf("failed to set preview flag: %w", err)
	}
	session.IsPreview = true

	return session, nil
}

// runStep0Setup runs the step 0 background script asynchronously and transitions
// the session from "provisioning" to "active" once setup completes, or to
// "setup_failed" if the script fails.
func (s *ScenarioSessionService) runStep0Setup(sessionID uuid.UUID, terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag, locale string) {
	// Deferred rather than placed at the end: this function returns from a
	// dozen points — setup failures, an abandoned session, a recovered panic —
	// and every one of them leaves a container still holding the network its
	// setup asked for.
	defer s.finishBuild(sessionID, terminalSessionID)

	defer func() {
		if r := recover(); r != nil {
			observability.Metrics.ScenarioSetupPanic.Add(1)
			slog.Error("runStep0Setup panic recovered",
				"session_id", sessionID,
				"panic", r,
				"stack", string(debug.Stack()))
			// Match the existing error-path updates: gate on status='provisioning'
			// so we don't clobber an abandoned session.
			s.db.Model(&models.ScenarioSession{}).
				Where("id = ? AND status = ?", sessionID, "provisioning").
				Updates(map[string]any{
					"status":             "setup_failed",
					"provisioning_phase": "",
				})
			s.tryStopTerminal(terminalSessionID, sessionID)
		}
	}()

	// Execute scenario-level setup script first (global environment preparation)
	setupScript := ResolveScriptContent(s.db, scenario.SetupScriptID, scenario.SetupScript)

	// The world's vocabulary goes in before anything that builds the world.
	// Generated here, per session, because the locale is a property of the
	// session: a scenario seeded with one language's lexicon baked in could
	// only ever build that language's world.
	install, lexErr := s.lexiconInstall(scenario.ID, locale)
	if lexErr != nil {
		slog.Error("cannot build the world's vocabulary", "session_id", sessionID, "err", lexErr)
		s.db.Model(&models.ScenarioSession{}).
			Where("id = ? AND status = ?", sessionID, "provisioning").
			Updates(map[string]any{"status": "setup_failed", "provisioning_phase": ""})
		s.tryStopTerminal(terminalSessionID, sessionID)
		return
	}
	setupScript = install + setupScript

	if setupScript != "" {
		slog.Info("executing scenario setup script", "session_id", sessionID, "script_len", len(setupScript))
		// Create a temporary step-like structure for executeBackgroundScript
		setupStep := &models.ScenarioStep{
			Order:            -1, // sentinel value for logging
			BackgroundScript: setupScript,
		}
		// The scenario-level setup script is not a step and has no "current"
		// flag; crash_traps scenarios hand it the whole set through config.json.
		if _, err := s.executeBackgroundScript(terminalSessionID, setupStep, provisioningLocaleEnv(locale)); err != nil {
			slog.Error("scenario setup script failed", "session_id", sessionID, "err", err)
			s.db.Model(&models.ScenarioSession{}).
				Where("id = ? AND status = ?", sessionID, "provisioning").
				Updates(map[string]any{
					"status":             "setup_failed",
					"provisioning_phase": "",
				})
			observability.Metrics.ScenarioSetupFailed.Add(1)
			s.tryStopTerminal(terminalSessionID, sessionID)
			return
		}
	}

	step := &scenario.Steps[0]

	// Execute step 0 background script
	bgScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
	if bgScript != "" {
		s.db.Model(&models.ScenarioSession{}).
			Where("id = ? AND status = ?", sessionID, "provisioning").
			Update("provisioning_phase", "step_setup")
		if _, err := s.executeBackgroundScript(terminalSessionID, step, stepProvisioningEnv(scenario, flags, step.Order, locale)); err != nil {
			slog.Error("step 0 setup failed", "session_id", sessionID, "err", err)
			s.db.Model(&models.ScenarioSession{}).
				Where("id = ? AND status = ?", sessionID, "provisioning").
				Updates(map[string]any{
					"status":             "setup_failed",
					"provisioning_phase": "",
				})
			observability.Metrics.ScenarioSetupFailed.Add(1)
			s.tryStopTerminal(terminalSessionID, sessionID)
			return
		}
	}

	// Deploy the flag for step 0. Unlike a mid-scenario step, a failure here is
	// not escalated to setup_failed: that would stop the terminal over a flag
	// the learner can get back with reprovision-step.
	if len(flags) > 0 {
		_ = s.deploySingleFlagToContainer(terminalSessionID, scenario, flags, 0)
	}

	// Step 0's intro cannot be drawn now — no console has attached yet — so it
	// is staged as the MOTD and rendered when the learner's shell starts.
	s.deliverStepZeroIntro(terminalSessionID, step, sessionID)

	// Say so once, loudly, if this scenario wants banners on an image that
	// cannot draw them. The symptom is otherwise just "nothing happens".
	s.warnIfEffectsUnsupported(terminalSessionID, scenario, sessionID)

	// Transition to active — only if still provisioning (not abandoned meanwhile)
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", sessionID, "provisioning").
		Updates(map[string]any{
			"status":             "active",
			"provisioning_phase": "",
		})
	if result.Error != nil {
		slog.Error("scenario session active-transition failed", "session_id", sessionID, "err", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("scenario session setup complete but row no longer in provisioning (likely abandoned mid-setup)", "session_id", sessionID)
		return
	}
	slog.Info("scenario session setup complete", "session_id", sessionID)
}

// asyncProvisioningThresholdSeconds is the declared budget past which a step's
// provisioning is taken off the advance request. Holding the learner's
// verify/submit response open for longer than this reads as a hang, so the work
// moves to a goroutine and the session reports "provisioning" instead.
//
// It sits BELOW bgScriptTimeoutDefault on purpose, which looks like the rule
// contradicting itself and is not. A step that declares nothing keeps running
// inline under the default ceiling, because that is what it did before these
// fields existed and this feature is not allowed to change it. The threshold
// governs only what an author explicitly asked for. So declaring 30 is a
// statement — "this step really takes that long, take it off the request" —
// while inheriting 30 is a ceiling nobody claimed, and the two are treated
// differently by design.
const asyncProvisioningThresholdSeconds = 15

// runsAsynchronously is the single owner of the sync-versus-async provisioning
// rule: a step either opts in explicitly, or declares a budget long enough that
// running it inline would stall the learner's request.
//
// It reads BackgroundTimeoutSeconds directly rather than effectiveTimeout on
// purpose. A timeout is a ceiling, not an expected duration — a step allowed 60s
// usually finishes in well under a second — so the fallback default says nothing
// about how long a step actually takes. Deciding from effectiveTimeout would
// make every step that declares nothing async, which is exactly the behaviour
// change these fields exist to avoid. Zero means unspecified, and unspecified
// keeps today's behaviour.
func runsAsynchronously(step *models.ScenarioStep) bool {
	return step.BackgroundAsync || step.BackgroundTimeoutSeconds > asyncProvisioningThresholdSeconds
}

// provisionNextStep prepares the container for the step the session just
// advanced to: it runs that step's background script and deploys its flag. The
// returned status is what the advance responses hand the client — see
// dto.StepProvisioningStatus for what each field commits to.
//
// The advance itself is never rolled back when provisioning fails. By the time
// this runs the flag is burned and the progress row is committed; an async
// failure surfaces as a "setup_failed" session and a sync one as
// next_step_provisioning_failed, both retryable through reprovision-step.
func (s *ScenarioSessionService) provisionNextStep(session *models.ScenarioSession, nextStepOrder int) dto.StepProvisioningStatus {
	if session.TerminalSessionID == nil {
		return dto.StepProvisioningStatus{}
	}
	step := findStepByOrder(session.Scenario.Steps, nextStepOrder)
	if step == nil {
		return dto.StepProvisioningStatus{}
	}

	hasScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript) != ""
	hasFlag := findFlagByStepOrder(session.Flags, nextStepOrder) != nil
	// The foreground script counts as work too. Leaving it out of this check
	// meant a step that declared only a foreground script fell out here and was
	// never run at all — the one shape where it is the entire content of the
	// step.
	hasForeground := ResolveScriptContent(s.db, step.ForegroundScriptID, step.ForegroundScript) != ""
	if !hasScript && !hasFlag && !hasForeground {
		return dto.StepProvisioningStatus{}
	}

	if hasScript && runsAsynchronously(step) {
		// An advance may only start setup on a session that is actually running.
		if !s.startAsyncStepProvisioning(session, step, []string{statusActive, statusInProgress}) {
			// The session was abandoned out from under the advance; there is
			// nobody left to poll.
			return dto.StepProvisioningStatus{}
		}
		return dto.StepProvisioningStatus{
			NextStepProvisioning:       true,
			ProvisioningTimeoutSeconds: effectiveTimeout(step),
		}
	}

	if err := s.runStepProvisioning(*session.TerminalSessionID, &session.Scenario, session.Flags, step, session.Locale); err != nil {
		observability.Metrics.ScenarioStepProvisioningFailed.Add(1)
		slog.Error("step provisioning failed", "session_id", session.ID, "step_order", step.Order, "err", err)
		return dto.StepProvisioningStatus{NextStepProvisioningFailed: true}
	}
	// Setup finished inline: the step is already playable, nothing to report.
	return dto.StepProvisioningStatus{}
}

// ocfFlagCurrentEnv carries a step's own flag into its background script.
// The OCF_ prefix is load-bearing: challenge content plants decoy environment
// variables of its own, and a platform-owned name is what makes a leak audit
// ("does any process still expose it?") greppable.
const ocfFlagCurrentEnv = "OCF_FLAG_CURRENT"

// ocfLocaleEnv names the language a session's world is built in.
const ocfLocaleEnv = "OCF_LANG"

// stepProvisioningEnv builds the environment a step's background script runs
// with: its own flag, and nothing else.
//
// Passing the whole flag set would defeat the point. A learner who reaches
// root-equivalent inside the container — levels 7 and 9 hold NOPASSWD sudo by
// design — could then read every later level's flag out of one process
// environment and skip the rest of the run.
//
// Returns nil when there is nothing to pass, so a scenario without flags sends
// exactly the request it sends today rather than an empty, confusing variable.
func stepProvisioningEnv(scenario *models.Scenario, flags []models.ScenarioFlag, stepOrder int, locale string) map[string]string {
	env := provisioningLocaleEnv(locale)
	if scenario == nil || !scenario.FlagsEnabled {
		return env
	}
	flag := findFlagByStepOrder(flags, stepOrder)
	if flag == nil || flag.ExpectedFlag == "" {
		return env
	}
	if env == nil {
		env = map[string]string{}
	}
	env[ocfFlagCurrentEnv] = flag.ExpectedFlag
	return env
}

// lexiconInstall builds the shell that writes a scenario's vocabulary into the
// container, or "" when the scenario has none — which is every scenario that
// predates this and stays working untouched.
//
// The heredoc delimiter is quoted: the lexicon is data at this point, and
// letting the provisioning shell expand it would resolve every path against the
// host rather than writing it out.
func (s *ScenarioSessionService) lexiconInstall(scenarioID uuid.UUID, locale string) (string, error) {
	if locale == "" {
		var scenario models.Scenario
		if err := s.db.Select("default_locale").First(&scenario, "id = ?", scenarioID).Error; err != nil {
			return "", err
		}
		locale = scenario.DefaultLocale
	}
	if locale == "" {
		// No locale at all means a scenario that never declared languages. Its
		// scripts name their own world, as they always have.
		return "", nil
	}

	body, err := GenerateLexicon(s.db, scenarioID, locale)
	if err != nil || body == "" {
		return "", err
	}
	return fmt.Sprintf("cat > %s <<'OCF_LEXICON'\n%sOCF_LEXICON\n", LexiconContainerPath, body), nil
}

// provisioningLocaleEnv tells the container which language it is being built
// in. Content reads it to pick the right world vocabulary, so it has to reach
// the scenario's setup script as well as each step's — the setup script is
// where the world's names are installed, and a locale that arrived one step
// late would build an English world for a French session.
//
// Nil rather than an empty map when there is no locale: callers pass this
// straight through, and every session that exists today has none.
func provisioningLocaleEnv(locale string) map[string]string {
	if locale == "" {
		return nil
	}
	return map[string]string{ocfLocaleEnv: locale}
}

// stepAnswerMarker lets a background script tell OCF what the answer to its own
// step is, by printing it: "OCF_ANSWER: Tuesday".
//
// It exists because some questions only have an answer once the session picks
// one. "What day of the week was this date?" is a fine exercise and a poor
// flag: with a fixed date the answer is the same for every student and travels
// between them in one message. The scenario draws the date per session, so only
// the scenario knows the answer — and it needs a way to say so.
//
// Carried on the script's stdout, which is already read, rather than through a
// file: a file costs a second round trip into the container on every flag step,
// to serve the few that use this.
//
// The learner is root and could read the answer out of the container. That is
// fine — they could equally run `date -d` instead of `cal`. What they cannot do
// is tell a classmate, because the classmate's session asks a different
// question.
const stepAnswerMarker = "OCF_ANSWER:"

// parseStepAnswer returns the answer a background script declared, or "".
//
// The last marker wins, so a script that recomputes is not betrayed by an
// earlier draft, and a line that merely mentions the marker mid-sentence is
// ignored: it has to start the line.
func parseStepAnswer(stdout string) string {
	answer := ""
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, stepAnswerMarker) {
			answer = strings.TrimSpace(strings.TrimPrefix(trimmed, stepAnswerMarker))
		}
	}
	return answer
}

// adoptScriptChosenAnswer replaces a step's generated flag with the answer its
// background script declared, when it declared one.
//
// Silent when there is none: that is every step that does not use the
// mechanism. A failure to store leaves the generated token in place, which
// makes the step unanswerable rather than answerable by the wrong word — the
// safer direction, and it is logged.
func (s *ScenarioSessionService) adoptScriptChosenAnswer(stdout string, flags []models.ScenarioFlag, step *models.ScenarioStep) {
	if !step.HasFlag {
		return
	}
	answer := parseStepAnswer(stdout)
	if answer == "" {
		return
	}
	flag := findFlagByStepOrder(flags, step.Order)
	if flag == nil {
		return
	}

	if err := s.db.Model(&models.ScenarioFlag{}).
		Where("session_id = ? AND step_order = ?", flag.SessionID, flag.StepOrder).
		Update("expected_flag", answer).Error; err != nil {
		slog.Error("step answer declared but not stored; the step keeps its generated flag and cannot be answered",
			"session_id", flag.SessionID, "step_order", flag.StepOrder, "err", err)
		return
	}
	flag.ExpectedFlag = answer
}

// runStepProvisioning executes a step's background script and then deploys the
// step's flag. The flag is deployed even when the script failed: the advance is
// already committed, and a learner facing a half-provisioned level is better
// served by having the flag in place than by having nothing.
//
// The foreground script runs last, after the environment it acts on exists.
func (s *ScenarioSessionService) runStepProvisioning(terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag, step *models.ScenarioStep, locale string) error {
	stdout, scriptErr := s.executeBackgroundScript(terminalSessionID, step, stepProvisioningEnv(scenario, flags, step.Order, locale))
	s.adoptScriptChosenAnswer(stdout, flags, step)
	flagErr := s.deploySingleFlagToContainer(terminalSessionID, scenario, flags, step.Order)
	if scriptErr != nil {
		return scriptErr
	}
	if flagErr != nil {
		// A flag that never landed leaves the step unsolvable just as surely as
		// a failed script, so it counts as a provisioning failure too.
		return flagErr
	}
	s.runForegroundScript(terminalSessionID, step)
	return nil
}

// runForegroundScript types a step's foreground script into the learner's own
// shell, which is what distinguishes it from the background script: it runs
// where the learner is looking, in their session, so they see it happen and its
// effects on the shell — a cd, an export, a function — persist for them.
//
// Best-effort by design, and the only path here that is. A foreground script is
// a demonstration: KillerCoda scenarios use it to show a command rather than to
// build the level, and the level itself is already provisioned by the time this
// runs. Failing the advance over a demonstration would cost the learner a step
// they had legitimately earned.
//
// The commonest reason it does nothing is that no console is attached — the
// learner has not opened their terminal, or has closed it. That is not a fault,
// and it is logged at info rather than as an error so it does not read as one
// in an operator's logs.
func (s *ScenarioSessionService) runForegroundScript(terminalSessionID string, step *models.ScenarioStep) {
	if s.verificationService == nil {
		return
	}

	script := ResolveScriptContent(s.db, step.ForegroundScriptID, step.ForegroundScript)
	if script == "" {
		return
	}

	err := s.verificationService.WriteToConsole(terminalSessionID, script)
	switch {
	case err == nil:
		slog.Info("foreground script sent to console", "step_order", step.Order)
	case errors.Is(err, ErrNoLiveConsole):
		slog.Info("skipping foreground script: no console attached", "step_order", step.Order)
	default:
		slog.Warn("failed to send foreground script to console", "step_order", step.Order, "err", err)
	}
}

// startAsyncStepProvisioning moves the session to "provisioning" and runs the
// step's setup in a goroutine. The transition is guarded so an abandon racing
// the advance is not resurrected; if it lost the race no goroutine is spawned
// and the call reports false.
//
// fromStatuses is the caller's business, not this function's: an advance may
// only start setup on a running session, while a retry legitimately starts from
// one whose setup already failed. Passing it in is what lets the retry go
// straight to "provisioning" — clearing setup_failed first would mean two
// writes, with the session briefly claiming to be playable in between.
func (s *ScenarioSessionService) startAsyncStepProvisioning(session *models.ScenarioSession, step *models.ScenarioStep, fromStatuses []string) bool {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status IN ?", session.ID, fromStatuses).
		Updates(map[string]any{
			"status":             statusProvisioning,
			"provisioning_phase": "step_setup",
		})
	if result.Error != nil {
		slog.Error("failed to mark session provisioning", "session_id", session.ID, "step_order", step.Order, "err", result.Error)
		return false
	}
	if result.RowsAffected == 0 {
		slog.Warn("skipping async step provisioning: session no longer active", "session_id", session.ID, "step_order", step.Order)
		return false
	}

	go s.runAsyncStepProvisioning(session.ID, *session.TerminalSessionID, &session.Scenario, session.Flags, step, session.Locale)
	return true
}

// runAsyncStepProvisioning is the goroutine body behind
// startAsyncStepProvisioning. It mirrors runStep0Setup's safety pattern — panic
// recovery, every status write guarded on the session still being in
// "provisioning" — with one deliberate difference: a mid-scenario failure
// leaves the terminal running. The learner already has a working shell, and
// killing it would cost them the whole run for one bad level.
func (s *ScenarioSessionService) runAsyncStepProvisioning(sessionID uuid.UUID, terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag, step *models.ScenarioStep, locale string) {
	defer func() {
		if r := recover(); r != nil {
			observability.Metrics.ScenarioStepProvisioningPanic.Add(1)
			slog.Error("step provisioning panic recovered",
				"session_id", sessionID,
				"step_order", step.Order,
				"panic", r,
				"stack", string(debug.Stack()))
			s.failStepProvisioning(sessionID)
		}
	}()

	if err := s.runStepProvisioning(terminalSessionID, scenario, flags, step, locale); err != nil {
		observability.Metrics.ScenarioStepProvisioningFailed.Add(1)
		slog.Error("step provisioning failed", "session_id", sessionID, "step_order", step.Order, "err", err)
		s.failStepProvisioning(sessionID)
		if _, hasIntro := introBanner(step); hasIntro {
			s.releaseHeldScreen(&terminalSessionID)
		}
		return
	}

	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", sessionID, "provisioning").
		Updates(map[string]any{
			"status":             "active",
			"provisioning_phase": "",
		})
	if result.Error != nil {
		slog.Error("step provisioning active-transition failed", "session_id", sessionID, "err", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("step provisioning complete but row no longer in provisioning (likely abandoned mid-setup)", "session_id", sessionID)
		return
	}
	slog.Info("step provisioning complete", "session_id", sessionID, "step_order", step.Order)

	// The intro is drawn here rather than when provisioning was handed off, so
	// it announces a level that actually exists — and so the transition screen
	// the outro held stays up for the whole wait instead of being taken down at
	// the moment the wait begins.
	if banner, ok := introBanner(step); ok {
		s.drawBanner(terminalSessionID, banner, sessionID, step.Order, "--resume")
	}
}

// failStepProvisioning records a mid-scenario provisioning failure. The
// terminal is intentionally left running, unlike the step 0 failure path.
//
// A failure to record the failure is logged rather than dropped: this is the
// path that gets the session out of "provisioning", so if it silently does
// nothing the learner waits on a state that will only clear when the reaper
// gets to it, minutes later.
func (s *ScenarioSessionService) failStepProvisioning(sessionID uuid.UUID) {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", sessionID, "provisioning").
		Updates(map[string]any{
			"status":             "setup_failed",
			"provisioning_phase": "",
		})
	if result.Error != nil {
		slog.Error("failed to record step provisioning failure",
			"session_id", sessionID, "err", result.Error)
	}
}

// ReprovisionCurrentStep re-runs the current step's background script against
// the learner's existing container. It is the recovery path for a failed
// advance: the advance itself is never rolled back, so the only way back into a
// playable state is to retry the setup.
//
// force exports FORCE=1 into the script so it redoes work its idempotency
// markers would otherwise skip.
func (s *ScenarioSessionService) ReprovisionCurrentStep(sessionID uuid.UUID, force bool) (*dto.ReprovisionStepResponse, error) {
	session, err := s.loadReprovisionableSession(sessionID)
	if err != nil {
		return nil, err
	}

	step := findStepByOrder(session.Scenario.Steps, session.CurrentStep)
	if step == nil {
		return nil, fmt.Errorf("current step (order=%d) not found", session.CurrentStep)
	}

	runnable, err := s.resolveRunnableStep(step, force)
	if err != nil {
		return nil, err
	}

	if runsAsynchronously(runnable) {
		// Straight to "provisioning" from whichever state the retry was
		// accepted in, including setup_failed. From there the goroutine owns
		// the outcome.
		if !s.startAsyncStepProvisioning(session, runnable, reprovisionableStatuses) {
			return nil, fmt.Errorf("session is no longer active")
		}
		return &dto.ReprovisionStepResponse{StepOrder: step.Order, Status: statusProvisioning}, nil
	}

	// Synchronous: run first, then record the outcome, so a failed retry never
	// leaves the session claiming to be playable.
	if err := s.runStepProvisioning(*session.TerminalSessionID, &session.Scenario, session.Flags, runnable, session.Locale); err != nil {
		observability.Metrics.ScenarioStepProvisioningFailed.Add(1)
		slog.Error("step reprovisioning failed", "session_id", session.ID, "step_order", step.Order, "err", err)
		s.setSessionRunState(session.ID, statusSetupFailed)
		return nil, fmt.Errorf("step provisioning failed: %w", err)
	}
	s.setSessionRunState(session.ID, statusActive)
	return &dto.ReprovisionStepResponse{StepOrder: step.Order, Status: statusActive}, nil
}

// setSessionRunState moves a session between the two states reprovisioning can
// produce. The guard restricts it to reprovisionableStatuses, so an abandon
// racing the retry still wins.
func (s *ScenarioSessionService) setSessionRunState(sessionID uuid.UUID, status string) {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status IN ?", sessionID, reprovisionableStatuses).
		Updates(map[string]any{
			"status":             status,
			"provisioning_phase": "",
		})
	if result.Error != nil {
		slog.Error("failed to set session run state",
			"session_id", sessionID, "status", status, "err", result.Error)
	}
}

// loadReprovisionableSession loads a session and rejects the states in which
// re-running a step's setup makes no sense. A session already provisioning has
// a goroutine on the job; a completed or abandoned one has no run to repair.
func (s *ScenarioSessionService) loadReprovisionableSession(sessionID uuid.UUID) (*models.ScenarioSession, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if !slices.Contains(reprovisionableStatuses, session.Status) {
		return nil, fmt.Errorf("session status %q cannot be reprovisioned", session.Status)
	}
	if session.TerminalSessionID == nil {
		return nil, fmt.Errorf("no terminal session attached")
	}
	return &session, nil
}

// resolveRunnableStep returns a copy of the step carrying its background script
// inline, optionally force-flagged. The copy is needed because
// executeBackgroundScript re-reads the script from the ProjectFile whenever
// BackgroundScriptID is set, which would discard the injected flag.
func (s *ScenarioSessionService) resolveRunnableStep(step *models.ScenarioStep, force bool) (*models.ScenarioStep, error) {
	script := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
	if script == "" {
		return nil, fmt.Errorf("current step has no background script to run")
	}
	if force {
		script = injectForceFlag(script)
	}

	runnable := *step
	runnable.BackgroundScriptID = nil
	runnable.BackgroundScript = script
	return &runnable, nil
}

// CurrentStepProvisioningTimeout returns the effective timeout of the step a
// session is currently provisioning, or 0 when it is not provisioning at all.
// It exists so a client that polls session info — after a page reload, say, and
// so never saw the advance response — can still derive when to stop waiting.
func (s *ScenarioSessionService) CurrentStepProvisioningTimeout(session *models.ScenarioSession) int {
	if session.Status != "provisioning" {
		return 0
	}
	var step models.ScenarioStep
	if err := s.db.Where("scenario_id = ? AND \"order\" = ?", session.ScenarioID, session.CurrentStep).
		First(&step).Error; err != nil {
		return 0
	}
	return effectiveTimeout(&step)
}

// GetCurrentStep returns the current step content for a session.
// While setup is running the response carries status "provisioning" instead of
// step content, so the frontend can show its loading state. Setup is no longer
// step 0's privilege — a step can provision mid-scenario — so these responses
// report the session's real step order rather than a hardcoded 0.
func (s *ScenarioSessionService) GetCurrentStep(sessionID uuid.UUID) (*dto.CurrentStepResponse, error) {
	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if session.Status == "provisioning" {
		return &dto.CurrentStepResponse{
			StepOrder:  session.CurrentStep,
			TotalSteps: len(session.Scenario.Steps),
			Title:      "Setting up environment...",
			Status:     "provisioning",
		}, nil
	}

	if session.Status == "setup_failed" {
		return &dto.CurrentStepResponse{
			StepOrder:  session.CurrentStep,
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

	// Resolve the step's prose in the language this session was started in.
	stepText := ResolveStepText(s.db, *currentStep, session.Locale)
	textContent := stepText.Text
	hintContent := stepText.Hint

	position, stepOrders := stepPositionInfo(session.Scenario.Steps, currentStep.Order)
	response := &dto.CurrentStepResponse{
		StepOrder:             currentStep.Order,
		Position:              position,
		StepOrders:            stepOrders,
		TotalSteps:            len(session.Scenario.Steps),
		Title:                 stepText.Title,
		Text:                  textContent,
		Hint:                  hintContent,
		Status:                stepStatus,
		HasFlag:               currentStep.HasFlag,
		StepType:              normalizeStepType(currentStep.StepType),
		TextContent:           textContent,
		ShowImmediateFeedback: currentStep.ShowImmediateFeedback,
	}

	// Quiz steps: populate the sanitized question list (no correct_answer/explanation)
	if response.StepType == "quiz" {
		response.Questions = loadSanitizedQuestions(s.db, currentStep.ID)
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

// StepPosition returns the 1-based display position of the step with the given
// order among the (already order-sorted) steps. 0 means the order was not
// found.
//
// Exported because it is the single answer to "which level is this?", and every
// surface that labels a step for a learner has to give the same one. Orders are
// data-driven — 0-based from a KillerCoda import, 1-based from the editor — so
// `order + 1` is right for some scenarios and off by one for others. The
// validated-flags list showed raw orders and so numbered the first level 0.
func StepPosition(steps []models.ScenarioStep, order int) int {
	for i := range steps {
		if steps[i].Order == order {
			return i + 1
		}
	}
	return 0
}

// stepPositionInfo returns the step's display position plus the full ordered
// list of orders, which clients use to map any order to its position locally.
func stepPositionInfo(steps []models.ScenarioStep, order int) (int, []int) {
	orders := make([]int, len(steps))
	for i := range steps {
		orders[i] = steps[i].Order
	}
	return StepPosition(steps, order), orders
}

// normalizeStepType returns the canonical step_type string. Empty values from
// pre-migration rows default to "terminal" so the frontend never sees a blank.
func normalizeStepType(stepType string) string {
	if stepType == "" {
		return "terminal"
	}
	return stepType
}

// loadSanitizedQuestions fetches the quiz questions for a step and returns them
// without the CorrectAnswer/Explanation fields (those are only revealed in
// per-question results after submission).
func loadSanitizedQuestions(db *gorm.DB, stepID uuid.UUID) []dto.CurrentStepQuestion {
	var questions []models.ScenarioStepQuestion
	if err := db.Where("step_id = ?", stepID).Order("\"order\" ASC").Find(&questions).Error; err != nil {
		slog.Warn("failed to load quiz questions", "step_id", stepID, "err", err)
		return nil
	}
	out := make([]dto.CurrentStepQuestion, 0, len(questions))
	for _, q := range questions {
		out = append(out, dto.CurrentStepQuestion{
			ID:           q.ID,
			Order:        q.Order,
			QuestionText: q.QuestionText,
			QuestionType: q.QuestionType,
			Options:      q.Options,
		})
	}
	return out
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

	position, stepOrders := stepPositionInfo(session.Scenario.Steps, targetStep.Order)
	response := &dto.CurrentStepResponse{
		StepOrder:             targetStep.Order,
		Position:              position,
		StepOrders:            stepOrders,
		TotalSteps:            len(session.Scenario.Steps),
		Title:                 targetStep.Title,
		Text:                  textContent,
		Hint:                  hintContent,
		Status:                stepStatus,
		HasFlag:               targetStep.HasFlag,
		StepType:              normalizeStepType(targetStep.StepType),
		TextContent:           textContent,
		ShowImmediateFeedback: targetStep.ShowImmediateFeedback,
	}

	if response.StepType == "quiz" {
		response.Questions = loadSanitizedQuestions(s.db, targetStep.ID)
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

	if err := requireActiveSession(&session); err != nil {
		return nil, err
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

	stepType := normalizeStepType(currentStep.StepType)

	// Branch on step_type. Flag and quiz steps have dedicated submission
	// endpoints; calling /verify on them is a client error.
	switch stepType {
	case "flag":
		return nil, fmt.Errorf("this step requires flag submission via /submit-flag, not /verify")
	case "quiz":
		return nil, fmt.Errorf("this step requires quiz submission via /submit-quiz, not /verify")
	case "info":
		// Info steps have no script — clicking "next" advances the session.
		return s.completeInfoStep(&session)
	}

	// Backward compat: legacy steps with HasFlag but no step_type still route
	// to flag submission.
	if currentStep.HasFlag && stepType == "terminal" {
		return nil, fmt.Errorf("this step requires flag submission via /submit-flag, not /verify")
	}

	// Pre-populate VerifyScript from ProjectFile (VerificationService doesn't have DB access)
	currentStep.VerifyScript = ResolveScriptContent(s.db, currentStep.VerifyScriptID, currentStep.VerifyScript)

	// Steps without a verify script auto-pass when the user clicks verify
	var passed bool
	var output string
	if currentStep.VerifyScript == "" {
		passed = true
	} else {
		var err error
		passed, output, err = s.verificationService.VerifyStep(*session.TerminalSessionID, currentStep)
		if err != nil {
			return nil, fmt.Errorf("verification failed: %w", err)
		}
	}

	response := &dto.VerifyStepResponse{
		Passed: passed,
		Output: output,
	}

	// Wrap all DB updates in a transaction for consistency
	// Captured before the transaction: advanceToNextStep updates current_step
	// through GORM, which writes the new value back onto this struct, so after
	// the transaction session.CurrentStep is already the step just entered.
	leftStep := session.CurrentStep
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

	if response.Passed && response.NextStep != nil {
		response.StepProvisioningStatus = s.runStepTransition(&session, leftStep, *response.NextStep)
	}

	return response, nil
}

// completeInfoStep auto-marks an info step as completed and advances the
// session to the next step. Info steps have no verification script — the
// "verify" call is the equivalent of clicking "next".
func (s *ScenarioSessionService) completeInfoStep(session *models.ScenarioSession) (*dto.VerifyStepResponse, error) {
	now := time.Now()
	response := &dto.VerifyStepResponse{Passed: true}

	// Captured before the transaction: advanceToNextStep updates current_step
	// through GORM, which writes the new value back onto this struct, so after
	// the transaction session.CurrentStep is already the step just entered.
	leftStep := session.CurrentStep
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		nextStep, err := s.advanceToNextStep(tx, session, now)
		if err != nil {
			return err
		}
		response.NextStep = nextStep
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	if response.NextStep != nil {
		response.StepProvisioningStatus = s.runStepTransition(session, leftStep, *response.NextStep)
	}

	return response, nil
}

// SubmitQuiz scores a quiz answer set for the current step and advances the
// session. Each answer is compared with constant-time equality (mirrors the
// flag-submit pattern). The score is correct_count / total. The full submitted
// answer set is persisted as JSON on ScenarioStepProgress for the teacher
// dashboard. Quiz steps are graded but NOT gated — even a 0% quiz advances.
//
// TODO: per-question persistence — see issue #283 follow-up. Karim wants each
// answer persisted as it's submitted (so a tab close / network drop doesn't
// lose answered questions); a separate POST .../quiz-answer endpoint will
// cover that without changing this final-submission contract.
func (s *ScenarioSessionService) SubmitQuiz(sessionID uuid.UUID, input dto.SubmitQuizInput) (*dto.SubmitQuizResponse, error) {
	// Reject empty/nil submissions early (controller would map this to 400).
	if len(input.Answers) == 0 {
		return nil, fmt.Errorf("answers are required")
	}

	var session models.ScenarioSession
	if err := s.db.Preload("Scenario.Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("StepProgress").Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if err := requireActiveSession(&session); err != nil {
		return nil, err
	}

	// Find the current step
	currentStep := findStepByOrder(session.Scenario.Steps, session.CurrentStep)
	if currentStep == nil {
		return nil, fmt.Errorf("current step (order=%d) not found", session.CurrentStep)
	}

	if normalizeStepType(currentStep.StepType) != "quiz" {
		return nil, fmt.Errorf("current step is not a quiz step")
	}

	// Load every question for this step so we can validate IDs and score
	// answers against the canonical correct_answer.
	var questions []models.ScenarioStepQuestion
	if err := s.db.Where("step_id = ?", currentStep.ID).Order("\"order\" ASC").Find(&questions).Error; err != nil {
		return nil, fmt.Errorf("failed to load questions: %w", err)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("quiz step has no questions")
	}

	// Build a lookup so we can reject answers with unknown question IDs and
	// score with O(1) per submitted answer.
	byID := make(map[uuid.UUID]models.ScenarioStepQuestion, len(questions))
	for _, q := range questions {
		byID[q.ID] = q
	}
	for qID := range input.Answers {
		if _, ok := byID[qID]; !ok {
			return nil, fmt.Errorf("answer references unknown question id %s", qID)
		}
	}

	// The per-question breakdown carries correct_answer + explanation, and
	// anything in the HTTP response is readable from the browser's devtools.
	// In exam mode (show_immediate_feedback=false) the teacher chose to
	// withhold answers, so the breakdown is never even built — the flag gates
	// the API payload, not just what the UI renders (§7.5, decided 2026-08-06).
	revealBreakdown := currentStep.ShowImmediateFeedback

	total := len(questions)
	correctCount := 0
	var results []dto.QuizQuestionResult
	if revealBreakdown {
		results = make([]dto.QuizQuestionResult, 0, total)
	}
	for _, q := range questions {
		submitted := input.Answers[q.ID]
		correct := subtle.ConstantTimeCompare([]byte(submitted), []byte(q.CorrectAnswer)) == 1
		if correct {
			correctCount++
		}
		if revealBreakdown {
			results = append(results, dto.QuizQuestionResult{
				QuestionID:    q.ID,
				Correct:       correct,
				CorrectAnswer: q.CorrectAnswer,
				Explanation:   q.Explanation,
			})
		}
	}

	score := 0.0
	if total > 0 {
		score = float64(correctCount) / float64(total)
	}

	answersJSON, err := json.Marshal(input.Answers)
	if err != nil {
		return nil, fmt.Errorf("failed to encode answers: %w", err)
	}

	response := &dto.SubmitQuizResponse{
		Score:              score,
		CorrectCount:       correctCount,
		Total:              total,
		PerQuestionResults: results,
	}

	now := time.Now()
	scoreCopy := score
	// Captured before the transaction: advanceToNextStep updates current_step
	// through GORM, which writes the new value back onto this struct, so after
	// the transaction session.CurrentStep is already the step just entered.
	leftStep := session.CurrentStep
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Persist the score + answer payload + step_type on the progress row
		// so the teacher dashboard can report per-attempt grades without
		// joining ScenarioStep.
		if err := tx.Model(&models.ScenarioStepProgress{}).
			Where("session_id = ? AND step_order = ?", session.ID, session.CurrentStep).
			Updates(map[string]any{
				"step_type":    "quiz",
				"quiz_score":   &scoreCopy,
				"quiz_answers": string(answersJSON),
			}).Error; err != nil {
			return fmt.Errorf("failed to persist quiz progress: %w", err)
		}

		// Mirror the just-persisted quiz score onto the in-memory progress
		// slice so advanceToNextStep's weighted grade calculation sees it.
		// The preloaded StepProgress slice was loaded before this update.
		for i := range session.StepProgress {
			if session.StepProgress[i].StepOrder == session.CurrentStep {
				session.StepProgress[i].StepType = "quiz"
				session.StepProgress[i].QuizScore = &scoreCopy
				break
			}
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

	if response.NextStep != nil {
		response.StepProvisioningStatus = s.runStepTransition(&session, leftStep, *response.NextStep)
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

	if err := requireActiveSession(&session); err != nil {
		return nil, err
	}

	flag := findFlagByStepOrder(session.Flags, session.CurrentStep)
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
	// Captured before the transaction: advanceToNextStep updates current_step
	// through GORM, which writes the new value back onto this struct, so after
	// the transaction session.CurrentStep is already the step just entered.
	leftStep := session.CurrentStep
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

	if response.NextStep != nil {
		response.StepProvisioningStatus = s.runStepTransition(&session, leftStep, *response.NextStep)
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

	terminals, err := terminalsForSessions(s.db, sessions)
	if err != nil {
		return nil, err
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
			TrainerID:         session.TrainerID,
			Status:            session.Status,
			ProvisioningPhase: session.ProvisioningPhase,
			Grade:             session.Grade,
			CurrentStep:       session.CurrentStep,
			TotalSteps:        totalSteps,
			CompletedSteps:    completedSteps,
			StartedAt:         session.StartedAt,
			CompletedAt:       session.CompletedAt,
			TerminalSessionID: session.TerminalSessionID,
			Resumable:         sessionIsResumable(&session, terminalFor(terminals, &session)),
		}
		result = append(result, resp)
	}
	return result, nil
}

// ErrSessionNotAbandonable marks an abandon refused because the run is already
// over — completed or abandoned — or does not exist. It is a state conflict
// the caller can read, not a failure: answered as a 500 it made an abandon
// that had already succeeded look like one that had not, and the natural
// reaction to that is to retry a call that will refuse again for the same
// reason.
var ErrSessionNotAbandonable = errors.New("session is not abandonable")

// AbandonSession marks a session as abandoned. Which statuses qualify is
// owned by models.AbandonableSessionStatuses; see there for why each one is in.
//
// The running setup goroutine is safe against this: all its status writes are
// gated on WHERE status='provisioning', so once we flip to abandoned here it
// can no longer resurrect the session.
func (s *ScenarioSessionService) AbandonSession(sessionID uuid.UUID) error {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status IN ?", sessionID, models.AbandonableSessionStatuses).
		Update("status", "abandoned")

	if result.Error != nil {
		return fmt.Errorf("failed to abandon session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: session not found, already finished or already abandoned", ErrSessionNotAbandonable)
	}

	return nil
}

// FindSessionByTerminal returns the most recent scenario session attached to a
// terminal session, or an error when the terminal is not running one (an
// ordinary dashboard terminal, typically).
//
// Canonical owner of the "which run is this terminal running?" lookup: the
// by-terminal endpoint the frontend polls and the crash-trap permadeath path
// must never disagree about which run a reused terminal belongs to.
func (s *ScenarioSessionService) FindSessionByTerminal(terminalSessionID string) (*models.ScenarioSession, error) {
	var session models.ScenarioSession
	err := s.db.Where("terminal_session_id = ?", terminalSessionID).
		Order("created_at DESC").
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// EndCrashTrapRun applies permadeath: the learner's shell was SIGKILLed, so if
// it was running a crash_traps scenario the run is over — the session is
// abandoned and the container stopped.
//
// Only crash_traps scenarios arm this. Anywhere else (an ordinary scenario, or
// a terminal with no scenario session at all) a killed shell stays the
// recoverable accident it has always been, and this is a no-op.
//
// Called from a websocket teardown with no request to answer and nobody
// waiting on a result, so it reports through logs rather than an error: every
// "nothing to do here" branch is an expected outcome, not a failure.
func (s *ScenarioSessionService) EndCrashTrapRun(terminalSessionID string) {
	if terminalSessionID == "" {
		return
	}

	session, err := s.FindSessionByTerminal(terminalSessionID)
	if err != nil {
		return // plain terminal, not a scenario run
	}

	var scenario models.Scenario
	if err := s.db.Select("id", "crash_traps").
		First(&scenario, "id = ?", session.ScenarioID).Error; err != nil {
		slog.Warn("could not read the scenario behind a killed shell",
			"session_id", session.ID, "scenario_id", session.ScenarioID, "err", err)
		return
	}
	if !scenario.CrashTraps {
		return
	}

	if err := s.AbandonSession(session.ID); err != nil {
		// AbandonSession only matches active/provisioning rows, so this is the
		// ordinary outcome for a run that had already ended.
		slog.Debug("crash-trap kill hit a run that was already over",
			"session_id", session.ID, "status", session.Status)
		return
	}
	s.tryStopTerminal(terminalSessionID, session.ID)
	slog.Info("crash trap ended the run: the learner's shell was killed",
		"session_id", session.ID, "scenario_id", session.ScenarioID,
		"terminal_session_id", terminalSessionID)
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
	if err := requireActiveSession(&session); err != nil {
		return nil, err
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
		// Calculate weighted grade: each step contributes its own weight.
		//   terminal/flag/info → 1.0 if completed else 0.0
		//   quiz               → progress.QuizScore (0 if nil)
		// Mirror the just-applied "completed" status onto the in-memory
		// session.StepProgress slice for the current step so the helper
		// counts it as completed. Quiz scores must be set on the in-memory
		// slice by the caller (SubmitQuiz) before reaching here.
		for i := range session.StepProgress {
			if session.StepProgress[i].StepOrder == session.CurrentStep {
				session.StepProgress[i].Status = "completed"
				if session.StepProgress[i].CompletedAt == nil {
					completedAt := now
					session.StepProgress[i].CompletedAt = &completedAt
				}
				break
			}
		}
		grade := ComputeWeightedGradeFromLoaded(session.Scenario.Steps, session.StepProgress, nil)

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
//
// It returns an error only when a flag that should have landed did not. Having
// nothing to deploy, and the deliberate policy skips (crash_traps placing its
// own flags, a path outside the allowlist), are not failures — they are logged
// and reported as success, because no step is left unsolvable by them.
func (s *ScenarioSessionService) deploySingleFlagToContainer(terminalSessionID string, scenario *models.Scenario, flags []models.ScenarioFlag, stepOrder int) error {
	if s.verificationService == nil {
		return nil
	}

	flag := findFlagByStepOrder(flags, stepOrder)
	if flag == nil {
		return nil // No flag for this step (step may not have HasFlag enabled)
	}

	// The step definition carries FlagPath
	step := findStepByOrder(scenario.Steps, stepOrder)
	if step == nil {
		return nil
	}

	// Determine the target path for the flag file.
	//
	// No declared path means the scenario places the flag itself — through its
	// background script, which receives the value in OCF_FLAG_CURRENT. Writing
	// one anyway put the answer at a guessable /tmp path, which is a leak in
	// exactly the way the crash_traps branch below already said it was: a step
	// whose whole point is that the learner must work out where to look is
	// solved by listing /tmp instead.
	//
	// crash_traps kept its own early return for the same reason (setup.sh
	// places flags from config.json there); the rule was always about who owns
	// placement, never about that one flavour of scenario.
	flagPath := step.FlagPath
	if flagPath == "" {
		return nil
	}

	// Validate flag path - prevent path traversal
	if strings.Contains(flagPath, "..") {
		slog.Warn("skipping flag deployment: path contains '..'", "step_order", flag.StepOrder, "path", flagPath)
		return nil
	}
	allowedPrefixes := defaultAllowedFlagPaths
	if scenario.AllowedFlagPaths != "" {
		allowedPrefixes = parseAllowedFlagPaths(scenario.AllowedFlagPaths)
	}
	if !isFlagPathAllowed(flagPath, allowedPrefixes) {
		slog.Warn("skipping flag deployment: path not in allowed prefix", "step_order", flag.StepOrder, "path", flagPath)
		return nil
	}

	// Push the flag file to the container (with trailing newline for clean cat output)
	if err := s.verificationService.PushFile(terminalSessionID, flagPath, flag.ExpectedFlag+"\n", "0644"); err != nil {
		slog.Warn("failed to deploy flag to container", "step_order", flag.StepOrder, "path", flagPath, "err", err)
		return fmt.Errorf("failed to deploy flag for step %d: %w", flag.StepOrder, err)
	}
	return nil
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

// Background script execution timeouts, in seconds.
// Step 0 gets a longer timeout because it typically runs the full environment
// setup (user creation, service provisioning, package installs, etc.).
const (
	bgScriptTimeoutStep0   = 300 // 5 minutes for initial setup
	bgScriptTimeoutDefault = 30  // subsequent steps, unless the step overrides it

	// MaxBackgroundTimeoutSeconds is the ceiling on what a step may declare.
	//
	// It exists because a declared timeout is not a private matter between a
	// step and its script: CleanupStuckProvisioningSessions writes off any
	// session that has been provisioning for too long, and it cannot know which
	// step is running. Without a ceiling a step could declare a budget longer
	// than the reaper's patience, get reaped mid-run, and then have its own
	// success discarded — the goroutine's status write is guarded on
	// "provisioning", which the reaper has already cleared. The session would
	// sit in setup_failed having actually succeeded.
	//
	// So the two must be bound together rather than chosen independently. This
	// is the value the reaper derives its cutoff from; see
	// stuckProvisioningTimeout.
	MaxBackgroundTimeoutSeconds = 600
)

// effectiveTimeout resolves how long a step's background script may run.
// An explicit per-step value always wins; otherwise the initial setup (step 0,
// and the order=-1 sentinel used for the scenario-level setup script) gets the
// long budget and later steps the default.
//
// The clamp is here, at the single point every caller reads the value through,
// rather than only at the API boundary: BackgroundTimeoutSeconds also arrives
// by import, by seed and by duplication, and none of those pass through the
// entity DTOs. Clamping on read is the one place none of them can bypass.
func effectiveTimeout(step *models.ScenarioStep) int {
	if step.BackgroundTimeoutSeconds > 0 {
		return min(step.BackgroundTimeoutSeconds, MaxBackgroundTimeoutSeconds)
	}
	if step.Order <= 0 {
		return bgScriptTimeoutStep0
	}
	return bgScriptTimeoutDefault
}

// executeBackgroundScript runs a step's background script in the student's container.
// Returns nil on success, an error if the script could not be pushed/executed or exited non-zero.
//
// Small scripts (<=4000 bytes) are passed inline via /bin/sh -c.
// Large scripts are pushed as temp files via PushFile, then executed from disk
// and cleaned up afterward, to avoid tt-backend's 4KB exec argument limit.
func (s *ScenarioSessionService) executeBackgroundScript(terminalSessionID string, step *models.ScenarioStep, env map[string]string) (string, error) {
	// Resolve background script from ProjectFile if available
	bgScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
	if bgScript == "" {
		return "", nil
	}
	if s.verificationService == nil {
		return "", fmt.Errorf("verification service not available")
	}

	timeout := effectiveTimeout(step)

	var exitCode int
	var stderr string
	var err error

	interpreter := parseShebang(bgScript)

	// Inject "set -e" so any failing command stops the script immediately.
	// Without this, scripts without explicit error handling silently continue
	// after failures (e.g., failed downloads) and exit 0.
	bgScript = injectSetE(bgScript)

	var stdout string

	if len(bgScript) <= maxInlineScriptSize {
		// Small scripts: pass inline (fast, single API call)
		exitCode, stdout, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, "-c", bgScript},
			env,
			timeout,
		)
	} else {
		// Large scripts: push as temp file then execute.
		//
		// /root/, not /tmp/: the file holds a step's provisioning logic — how the
		// level is built, where it hides things — and challenge scenarios exist to
		// teach learners to go looking. A world-readable /tmp path hands them the
		// answers. /root/ is in tt-backend's push allowlist and is unreadable to
		// the learner's account, and the file is removed after the run regardless.
		tmpPath := fmt.Sprintf("/root/.ocf_bg_%d.sh", step.Order)
		if pushErr := s.verificationService.PushFile(terminalSessionID, tmpPath, bgScript, "0700"); pushErr != nil {
			slog.Warn("failed to push background script to container", "step_order", step.Order, "err", pushErr)
			return "", fmt.Errorf("failed to push script: %w", pushErr)
		}
		exitCode, stdout, stderr, err = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{interpreter, tmpPath},
			env,
			timeout,
		)
		// Best-effort cleanup — no env, the flag has nothing to do with rm.
		_, _, _, _ = s.verificationService.ExecInContainer(
			terminalSessionID,
			[]string{"rm", "-f", tmpPath},
			nil,
			5,
		)
	}

	if err != nil {
		slog.Error("background script failed to execute", "step_order", step.Order, "err", err, "stdout", truncateLog(stdout), "stderr", truncateLog(stderr))
		return stdout, fmt.Errorf("script execution failed: %w", err)
	}
	if exitCode != 0 {
		slog.Error("background script exited with non-zero code", "step_order", step.Order, "exit_code", exitCode, "stdout", truncateLog(stdout), "stderr", truncateLog(stderr))
		return stdout, fmt.Errorf("script exited with code %d: %s", exitCode, stderr)
	}
	slog.Info("background script completed", "step_order", step.Order, "exit_code", 0)
	return stdout, nil
}

// findExistingSession reports the session that stands between this learner and a
// new run of this scenario, and whether it is still usable.
//
// A session in one of the blocking statuses is not automatically a live run: its
// terminal may be gone or stopped underneath it. Such a session is a zombie and
// the caller may abandon it; a session whose terminal is still running is a real
// run the learner should resume rather than relaunch.
//
// This is the single definition of that rule. The launch path evaluates it inside
// its transaction and acts on it; the availability endpoint evaluates it read-only
// through GetResumableSession, so the button and the card cannot disagree.
func findExistingSession(db *gorm.DB, userID string, scenarioID uuid.UUID) (*models.ScenarioSession, bool, error) {
	var existing models.ScenarioSession
	err := db.Where("user_id = ? AND scenario_id = ? AND status IN ?",
		userID, scenarioID, blockingSessionStatuses).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to check for an existing session: %w", err)
	}

	var terminal *terminalModels.Terminal
	if existing.TerminalSessionID != nil {
		var t terminalModels.Terminal
		if err := db.Where("session_id = ?", *existing.TerminalSessionID).First(&t).Error; err == nil {
			terminal = &t
		}
	}
	return &existing, sessionIsResumable(&existing, terminal), nil
}

// terminalsForSessions loads the terminals backing a set of sessions, keyed by
// terminal session id, in one query. Both surfaces that judge resumability over
// a list — the launcher's availability answer and the learner's own session
// list — need exactly this context, so they load it the same way rather than
// each writing the join.
func terminalsForSessions(db *gorm.DB, sessions []models.ScenarioSession) (map[string]*terminalModels.Terminal, error) {
	byID := make(map[string]*terminalModels.Terminal, len(sessions))

	ids := make([]string, 0, len(sessions))
	for i := range sessions {
		if sessions[i].TerminalSessionID != nil {
			ids = append(ids, *sessions[i].TerminalSessionID)
		}
	}
	if len(ids) == 0 {
		return byID, nil
	}

	var terminals []terminalModels.Terminal
	if err := db.Where("session_id IN ?", ids).Find(&terminals).Error; err != nil {
		return nil, fmt.Errorf("failed to list session terminals: %w", err)
	}
	for i := range terminals {
		byID[terminals[i].SessionID] = &terminals[i]
	}
	return byID, nil
}

// terminalFor returns the terminal backing a session, or nil when it has none
// or its row is gone — both of which sessionIsResumable reads as "not a run".
func terminalFor(terminals map[string]*terminalModels.Terminal, session *models.ScenarioSession) *terminalModels.Terminal {
	if session.TerminalSessionID == nil {
		return nil
	}
	return terminals[*session.TerminalSessionID]
}

// sessionIsResumable is the rule itself, expressed once over data both callers
// already hold. A session in a blocking status is only a real run when its
// terminal is still alive; setup_failed is never resumable because the
// environment it describes is broken, and a session with no terminal — or one
// whose terminal is gone — has nothing left to return to.
//
// "Alive" is Terminal.IsLive, not `State == StateRunning`: nothing moves the
// state column off "running" when a terminal merely reaches its TTL, so the
// shortcut kept a learner's run resumable months after its container had gone,
// blocking every relaunch of that scenario with "a run is already in progress".
func sessionIsResumable(session *models.ScenarioSession, terminal *terminalModels.Terminal) bool {
	if session.Status == "setup_failed" || session.TerminalSessionID == nil {
		return false
	}
	return terminal.IsLive()
}

// GetResumableSession returns the live run of this scenario for this learner, or
// nil. Read-only: unlike the launch path it never abandons a zombie, because a
// listing must not mutate the sessions it is describing.
func (s *ScenarioSessionService) GetResumableSession(userID string, scenarioID uuid.UUID) (*models.ScenarioSession, error) {
	existing, resumable, err := findExistingSession(s.db, userID, scenarioID)
	if err != nil || !resumable {
		return nil, err
	}
	return existing, nil
}

// blockingSessionStatuses are the statuses that occupy the one-run-per-scenario
// slot. Anything else (completed, abandoned, expired) leaves the learner free to
// start again.
var blockingSessionStatuses = []string{"in_progress", "active", "provisioning", "setup_failed"}

// GetResumableSessions answers GetResumableSession for many scenarios at once,
// in two queries. The availability endpoint lists every scenario a learner can
// see, so asking per scenario would reproduce the per-row lookup that made the
// class analytics page slow.
func (s *ScenarioSessionService) GetResumableSessions(userID string, scenarioIDs []uuid.UUID) (map[uuid.UUID]*models.ScenarioSession, error) {
	resumable := make(map[uuid.UUID]*models.ScenarioSession)
	if len(scenarioIDs) == 0 {
		return resumable, nil
	}

	var sessions []models.ScenarioSession
	if err := s.db.Where("user_id = ? AND scenario_id IN ? AND status IN ?",
		userID, scenarioIDs, blockingSessionStatuses).Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("failed to list existing sessions: %w", err)
	}
	if len(sessions) == 0 {
		return resumable, nil
	}

	terminalsByID, err := terminalsForSessions(s.db, sessions)
	if err != nil {
		return nil, err
	}

	for i := range sessions {
		session := &sessions[i]
		if sessionIsResumable(session, terminalFor(terminalsByID, session)) {
			resumable[session.ScenarioID] = session
		}
	}
	return resumable, nil
}
