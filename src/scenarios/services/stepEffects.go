package services

import (
	"log/slog"
	"regexp"

	"github.com/google/uuid"

	"soli/formations/src/observability"
	"soli/formations/src/scenarios/models"
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
func (s *ScenarioSessionService) emitStepTransitionBanners(session *models.ScenarioSession, fromOrder, toOrder int) {
	if session.TerminalSessionID == nil || s.verificationService == nil {
		return
	}

	if from := findStepByOrder(session.Scenario.Steps, fromOrder); from != nil {
		if banner, ok := outroBanner(from); ok {
			s.drawBanner(*session.TerminalSessionID, banner, session.ID, fromOrder)
		}
	}
	if to := findStepByOrder(session.Scenario.Steps, toOrder); to != nil {
		if banner, ok := introBanner(to); ok {
			s.drawBanner(*session.TerminalSessionID, banner, session.ID, toOrder)
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
func (s *ScenarioSessionService) drawBanner(terminalSessionID string, banner stepBanner, sessionID uuid.UUID, stepOrder int) {
	exitCode, _, stderr, err := s.verificationService.ExecInContainer(
		terminalSessionID,
		[]string{ocfBannerPath, banner.Effect, banner.Text},
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
	exitCode, _, stderr, err := s.verificationService.ExecInContainer(terminalSessionID, command, bannerTimeoutSeconds)
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
