package services

import (
	"log/slog"
	"regexp"

	"github.com/google/uuid"

	"soli/formations/src/observability"
	"soli/formations/src/scenarios/models"

	"soli/formations/src/scenarios/dto"
)

// Per-step intro/outro banners.
//
// A trainer picks an effect name and types a line; this turns that into an
// ocf-banner call inside the learner's container. Two properties of the call
// site shape everything here:
//
//   - Banners are decoration. A failure must never fail a step — the learner
//     has already earned the advance, and losing it over an animation would be
//     absurd. Every path here logs at most.
//   - The text comes from a trainer's form field and ends up in a container
//     command. It is passed as an argv element to an exec that runs no shell,
//     so a `$(...)`, a backtick or a `;` in the text is inert by construction
//     rather than by escaping.

const (
	// ocfBannerPath is where the challenge image installs the banner helper.
	// An image without it makes the exec fail, which is logged and ignored —
	// scenarios must run on stock images.
	ocfBannerPath = "/usr/local/bin/ocf-banner"

	// ocfMotdPath is read by the image's login-shell hook. Step 0's intro
	// cannot be drawn directly: its background script runs during provisioning,
	// before the learner's console attaches, so there is no terminal to draw
	// on. Writing the file instead defers the banner to first shell.
	ocfMotdPath = "/etc/ocf-motd.txt"

	// ocfMotdEffectPath carries step 0's effect to the same hook, exactly the
	// way the text does. It reads the first line, tolerates a trailing newline,
	// and falls back to its own default for anything malformed.
	//
	// This used to be an exported environment variable, which made it depend on
	// sourcing order and on the export reaching the right shell at all. Three
	// separate failures came out of that asymmetry and every one was silent,
	// because an unset variable simply means the default wins. A file has none
	// of those failure modes.
	ocfMotdEffectPath = "/etc/ocf-motd-effect.txt"

	// bannerTimeoutSeconds bounds the exec. ocf-banner caps its own render at
	// 5s; this is the outer bound on the round trip.
	bannerTimeoutSeconds = 10
)

// effectNamePattern is the shape of a tte effect name. Effects are chosen from
// a list in the editor, but the field is a plain string on the API, so it is
// validated here too — this value reaches a container command, and unlike the
// banner text it is also written into a shell snippet for the MOTD path.
var effectNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

// stepBanner is one renderable banner: an effect and the line it draws.
type stepBanner struct {
	Effect string
	Text   string
}

// introBanner and outroBanner report the banner configured on a step, and
// whether there is one at all. Both halves are required: an effect with no
// text has nothing to draw, and text with no effect has no way to draw it.
func introBanner(step *models.ScenarioStep) (stepBanner, bool) {
	return newStepBanner(step.IntroEffect, step.IntroText)
}

func outroBanner(step *models.ScenarioStep) (stepBanner, bool) {
	return newStepBanner(step.OutroEffect, step.OutroText)
}

func newStepBanner(effect, text string) (stepBanner, bool) {
	if effect == "" || text == "" {
		return stepBanner{}, false
	}
	if !effectNamePattern.MatchString(effect) {
		slog.Warn("ignoring step banner: effect name is not a valid tte effect", "effect", effect)
		return stepBanner{}, false
	}
	return stepBanner{Effect: effect, Text: text}, true
}

// scenarioUsesEffects reports whether any step in the scenario configures a
// banner, which is what decides that the container needs tte at all.
func scenarioUsesEffects(scenario *models.Scenario) bool {
	for i := range scenario.Steps {
		if _, ok := introBanner(&scenario.Steps[i]); ok {
			return true
		}
		if _, ok := outroBanner(&scenario.Steps[i]); ok {
			return true
		}
	}
	return false
}

// emitStepTransitionBanners draws the outro of the step just left and then the
// intro of the step just entered, in that order — the learner should see the
// level they finished acknowledged before the next one announces itself.
//
// Step 0's intro is not drawn here; it is delivered as a MOTD at container
// setup, because at that point no console has attached yet.
// emitOutroBanner draws the banner belonging to the step the learner just
// finished. It must run BEFORE the next step is provisioned: that provisioning
// executes the next step's background script, which may itself write to the
// learner's terminal, and the outro then lands after output from a step the
// learner has not reached yet — reading as if the two steps had swapped.
func (s *ScenarioSessionService) emitOutroBanner(session *models.ScenarioSession, fromOrder int) {
	if session.TerminalSessionID == nil || s.verificationService == nil {
		return
	}
	if from := findStepByOrder(session.Scenario.Steps, fromOrder); from != nil {
		if banner, ok := outroBanner(from); ok {
			s.drawBanner(*session.TerminalSessionID, banner, session.ID, fromOrder)
		}
	}
}

// emitIntroBanner draws the banner of the step being entered. It runs AFTER
// provisioning, so the environment the banner announces actually exists by the
// time the learner reads it.
func (s *ScenarioSessionService) emitIntroBanner(session *models.ScenarioSession, toOrder int) {
	if session.TerminalSessionID == nil || s.verificationService == nil {
		return
	}
	if to := findStepByOrder(session.Scenario.Steps, toOrder); to != nil {
		if banner, ok := introBanner(to); ok {
			// --resume is inert when no screen is held, so the terminal's own
			// state decides whether this continues a held transition. Threading
			// a flag from the outro would be the engine guessing at something
			// the container already knows.
			s.drawBanner(*session.TerminalSessionID, banner, session.ID, toOrder, "--resume")
		}
	}
}

// drawBanner runs ocf-banner in the container. The effect and text travel as
// separate argv elements to an exec that spawns no shell, so nothing in either
// can be interpreted as a command.
//
// ocf-banner exits 0 on every path it controls, including a missing tte, so a
// non-zero result here means the helper itself is absent — an older or stock
// image. That is expected and logged at debug volume, never surfaced.
func (s *ScenarioSessionService) drawBanner(terminalSessionID string, banner stepBanner, sessionID uuid.UUID, stepOrder int, args ...string) {
	// No env: a banner carries the trainer's own words, never a flag.
	argv := append([]string{ocfBannerPath}, args...)
	argv = append(argv, banner.Effect, banner.Text)
	exitCode, _, stderr, err := s.verificationService.ExecInContainer(
		terminalSessionID,
		argv,
		nil,
		bannerTimeoutSeconds,
	)
	if err != nil {
		slog.Info("step banner not drawn", "session_id", sessionID, "step_order", stepOrder, "err", err)
		return
	}
	if exitCode != 0 {
		slog.Info("step banner not drawn: ocf-banner unavailable on this image",
			"session_id", sessionID, "step_order", stepOrder, "exit_code", exitCode, "stderr", truncateLog(stderr))
	}
}

// deliverStepZeroIntro stages the first step's banner for the learner's login
// shell, which is the only moment it can be seen: the step 0 background script
// runs while the session is still provisioning, with no console attached.
//
// The text is written through a positional parameter rather than interpolated
// into the command, so it cannot be interpreted as shell however it is
// written. /etc is outside tt-backend's file-push allowlist, which is why this
// goes through exec at all.
func (s *ScenarioSessionService) deliverStepZeroIntro(terminalSessionID string, step *models.ScenarioStep, sessionID uuid.UUID) {
	banner, ok := introBanner(step)
	if !ok {
		return
	}
	if s.verificationService == nil {
		return
	}

	s.execBestEffort(terminalSessionID, sessionID, "step 0 intro text",
		[]string{"/bin/sh", "-c", `printf '%s\n' "$1" > ` + ocfMotdPath, "sh", banner.Text})

	// Written the same way as the text, for the same reason: a positional
	// parameter rather than part of the command string, so nothing in it can be
	// interpreted however it is authored. effectNamePattern has already
	// constrained this to a bare identifier, which is narrower than what the
	// hook accepts, so whatever reaches the file passes its validation.
	s.execBestEffort(terminalSessionID, sessionID, "step 0 intro effect",
		[]string{"/bin/sh", "-c", `printf '%s\n' "$1" > ` + ocfMotdEffectPath, "sh", banner.Effect})
}

// execBestEffort runs a container command whose failure is not worth failing
// anything over, and says so in the log rather than silently.
func (s *ScenarioSessionService) execBestEffort(terminalSessionID string, sessionID uuid.UUID, what string, command []string) {
	exitCode, _, stderr, err := s.verificationService.ExecInContainer(terminalSessionID, command, nil, bannerTimeoutSeconds)
	if err != nil {
		slog.Info("could not deliver "+what, "session_id", sessionID, "err", err)
		return
	}
	if exitCode != 0 {
		slog.Info("could not deliver "+what, "session_id", sessionID, "exit_code", exitCode, "stderr", truncateLog(stderr))
	}
}

// warnIfEffectsUnsupported checks, once per session, that a scenario asking for
// banners landed on an image that can draw them.
//
// This is the visible half of the toolchain question. Installing tte at setup
// time would need the network feature, which is off by default, so the answer
// is to run these scenarios on an image that already carries it. When that has
// not happened the effects simply never appear, and without this the trainer
// and the operator both have nothing to go on.
func (s *ScenarioSessionService) warnIfEffectsUnsupported(terminalSessionID string, scenario *models.Scenario, sessionID uuid.UUID) {
	if s.verificationService == nil || !scenarioUsesEffects(scenario) {
		return
	}

	exitCode, _, _, err := s.verificationService.ExecInContainer(
		terminalSessionID,
		[]string{"/bin/sh", "-c", "command -v tte >/dev/null 2>&1"},
		nil,
		bannerTimeoutSeconds,
	)
	if err != nil {
		slog.Warn("could not check whether the container supports step effects",
			"session_id", sessionID, "scenario", scenario.Name, "err", err)
		return
	}
	if exitCode != 0 {
		observability.Metrics.ScenarioEffectsUnsupported.Add(1)
		slog.Warn("scenario configures step effects but the container has no tte — no banner will render",
			"session_id", sessionID,
			"scenario", scenario.Name,
			"instance_type", scenario.InstanceType,
			"hint", "run this scenario on a challenge image that ships tte; installing it at setup needs the network feature, which is disabled by default")
	}
}

// runStepTransition carries the learner from one step to the next: the outro of
// the level they finished, the provisioning of the level they are entering, and
// its intro.
//
// When both banners exist the alternate screen is held across the gap, so the
// learner does not drop back to their shell mid-transition and wait there in
// front of a terminal the next level has not been built for yet. With only one
// of the two banners there is nothing coming to replace a held screen, so the
// screen is not held.
//
// The hold is released on every path that does not end in an intro. It also
// expires on its own inside the container, because none of these paths run if
// the engine dies mid-transition.
func (s *ScenarioSessionService) runStepTransition(session *models.ScenarioSession, fromOrder, toOrder int) dto.StepProvisioningStatus {
	from := findStepByOrder(session.Scenario.Steps, fromOrder)
	to := findStepByOrder(session.Scenario.Steps, toOrder)

	var outro stepBanner
	hasOutro, hasIntro := false, false
	if from != nil {
		outro, hasOutro = outroBanner(from)
	}
	if to != nil {
		_, hasIntro = introBanner(to)
	}

	// A screen is only held when a banner is coming to replace it.
	held := hasOutro && hasIntro

	if hasOutro && session.TerminalSessionID != nil && s.verificationService != nil {
		if held {
			s.drawBanner(*session.TerminalSessionID, outro, session.ID, fromOrder, "--hold")
		} else {
			s.drawBanner(*session.TerminalSessionID, outro, session.ID, fromOrder)
		}
	}

	status := s.provisionNextStep(session, toOrder)

	switch {
	case status.NextStepProvisioning:
		// Setup moved to a goroutine. The intro belongs to whoever finishes it:
		// drawing it here would announce a level whose environment is still
		// being built, and would drop the held screen at the very moment the
		// learner most needs something other than an idle prompt to look at.
		return status
	case status.NextStepProvisioningFailed:
		// The panel takes over with an infrastructure error. Give the terminal
		// back rather than leaving the learner in front of a waiting screen for
		// something that is not coming — but only if this transition actually
		// held one. A scenario with no banners must not pay for an extra exec
		// on every failure.
		if held {
			s.releaseHeldScreen(session.TerminalSessionID)
		}
		return status
	}

	s.emitIntroBanner(session, toOrder)
	return status
}

// releaseHeldScreen takes down a held transition screen. Safe to call when
// nothing is held — ocf-banner checks the terminal's own state rather than
// trusting the engine's belief about it — so callers do not have to track
// whether a hold is outstanding.
func (s *ScenarioSessionService) releaseHeldScreen(terminalSessionID *string) {
	if terminalSessionID == nil || s.verificationService == nil {
		return
	}
	if _, _, _, err := s.verificationService.ExecInContainer(
		*terminalSessionID,
		[]string{ocfBannerPath, "--release"},
		nil,
		bannerTimeoutSeconds,
	); err != nil {
		slog.Debug("could not release the held transition screen", "err", err)
	}
}
