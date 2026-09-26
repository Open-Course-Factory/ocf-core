package services

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/google/uuid"

	"soli/formations/src/observability"
	"soli/formations/src/scenarios/models"
)

// ErrRunNotRebuildable refuses a rebuild because the run is no longer the
// rebuildable run the caller judged: another resume reattached it first, or it
// ended in the meantime.
var ErrRunNotRebuildable = errors.New("run cannot be rebuilt")

// ReattachRunToNewTerminal moves a rebuildable run from oldTerminalID, whose
// container is gone, onto newTerminalID and rebuilds its world there in the
// background: the scenario's setup, then every step's background script
// through the current step, with the flags the run already holds. Progress,
// hints and score are not touched. The returned run is provisioning, in phase
// "replay", until the build ends.
//
// The move is one write guarded on the run still being open on
// oldTerminalID, so of two resumes racing for the same run only one gets it;
// the other gets ErrRunNotRebuildable and owns the terminal it created.
func (s *ScenarioSessionService) ReattachRunToNewTerminal(sessionID uuid.UUID, oldTerminalID, newTerminalID string) (*models.ScenarioSession, error) {
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND terminal_session_id = ? AND status IN ?",
			sessionID, oldTerminalID, []string{statusActive, statusInProgress}).
		Updates(map[string]any{
			"terminal_session_id":      newTerminalID,
			"status":                   statusProvisioning,
			"provisioning_phase":       provisioningPhaseReplay,
			"rebuild_from_terminal_id": oldTerminalID,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("failed to reattach the run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrRunNotRebuildable
	}

	var session models.ScenarioSession
	if err := s.db.Preload("Flags").First(&session, "id = ?", sessionID).Error; err != nil {
		s.abortRebuild(sessionID, newTerminalID)
		return nil, fmt.Errorf("failed to reload the reattached run: %w", err)
	}
	scenario, err := s.loadScenarioWithSteps(session.ScenarioID)
	if err != nil {
		s.abortRebuild(sessionID, newTerminalID)
		return nil, err
	}

	go s.runRebuild(buildJob{
		sessionID:    session.ID,
		terminalID:   newTerminalID,
		scenario:     scenario,
		flags:        session.Flags,
		locale:       session.Locale,
		throughOrder: session.CurrentStep,
		phase:        provisioningPhaseReplay,
	})
	return &session, nil
}

// runRebuild builds the reattached run's world and makes it active again. A
// build that fails, panics or finds the run abandoned under it goes through
// abortRebuild: the run is not lost to a failed rebuild, and the half-built
// terminal is not left holding budget.
func (s *ScenarioSessionService) runRebuild(job buildJob) {
	defer func() {
		if r := recover(); r != nil {
			observability.Metrics.ScenarioSetupPanic.Add(1)
			slog.Error("scenario rebuild panic recovered",
				"session_id", job.sessionID, "panic", r, "stack", string(debug.Stack()))
			s.abortRebuild(job.sessionID, job.terminalID)
		}
	}()

	if err := s.buildWorld(job); err != nil {
		if errors.Is(err, errBuildAbandoned) {
			slog.Warn("scenario rebuild stopped: run no longer provisioning (likely abandoned)", "session_id", job.sessionID)
		} else {
			observability.Metrics.ScenarioSetupFailed.Add(1)
			slog.Error("scenario rebuild failed", "session_id", job.sessionID, "err", err)
		}
		s.abortRebuild(job.sessionID, job.terminalID)
		return
	}

	// Only a container that is kept loses its build features; one that is
	// about to be deleted has nothing to close.
	s.finishBuild(job.sessionID, job.terminalID)
	result := s.db.Model(&models.ScenarioSession{}).
		Where("id = ? AND status = ?", job.sessionID, statusProvisioning).
		Updates(map[string]any{
			"status":                   statusActive,
			"provisioning_phase":       "",
			"rebuild_from_terminal_id": nil,
		})
	if result.Error != nil {
		slog.Error("scenario rebuild active-transition failed", "session_id", job.sessionID, "err", result.Error)
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("scenario rebuild complete but run no longer provisioning (likely abandoned)", "session_id", job.sessionID)
		s.tryDeleteTerminal(job.terminalID, job.sessionID)
		return
	}
	slog.Info("scenario rebuild complete", "session_id", job.sessionID)
}

// abortRebuild ends a rebuild that will not finish: the run goes back to the
// terminal it was rebuilt from, if it is still waiting on this rebuild, and
// the half-built terminal is deleted.
//
// Going back to the old id rather than keeping the new one is what keeps a
// failure safe: the run reads as rebuildable again, and if the delete fails
// the orphan is a plain terminal of the learner's, never a "live" resume into
// a half-built machine.
func (s *ScenarioSessionService) abortRebuild(sessionID uuid.UUID, newTerminalID string) {
	if _, err := restoreRunBeforeRebuild(s.db, sessionID); err != nil {
		slog.Error("failed to put a run back after its rebuild ended", "session_id", sessionID, "err", err)
	}
	s.tryDeleteTerminal(newTerminalID, sessionID)
}

// replayProvisioningTimeout is the budget of a whole rebuild through
// throughOrder: the scenario's setup script, then the background script of
// every step it replays. Each gets its effective timeout, as the build runs
// it.
func replayProvisioningTimeout(steps []models.ScenarioStep, throughOrder int) int {
	total := effectiveTimeout(steps, &models.ScenarioStep{Order: scenarioSetupOrder})
	for i := range steps {
		if steps[i].Order <= throughOrder {
			total += effectiveTimeout(steps, &steps[i])
		}
	}
	return total
}
