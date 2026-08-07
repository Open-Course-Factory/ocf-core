package scenarios_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Per-step intro/outro banners.
//
// The contract: on advance from N to N+1 the container is asked to draw N's
// outro then N+1's intro; a step that configures nothing produces no exec at
// all; and nothing a trainer can type into the text field can execute in the
// learner's container.

const ocfBanner = "/usr/local/bin/ocf-banner"

// bannerCalls returns the exec calls that invoked ocf-banner, in order.
func bannerCalls(svc *bgTrackingVerificationService) [][]string {
	var out [][]string
	for _, call := range svc.execCalls {
		if len(call.command) > 0 && call.command[0] == ocfBanner {
			out = append(out, call.command)
		}
	}
	return out
}

// effectsSession builds a two-step scenario with a live session parked on step
// 0, so a verify advances it to step 1.
func effectsSession(t *testing.T, db *gorm.DB, name string, step0, step1 models.ScenarioStep) *models.ScenarioSession {
	t.Helper()

	scenario := models.Scenario{
		Name:         name,
		Title:        name,
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step0.ScenarioID, step0.Order, step0.Title = scenario.ID, 0, "Step 1"
	step1.ScenarioID, step1.Order, step1.Title = scenario.ID, 1, "Step 2"
	require.NoError(t, db.Create(&step0).Error)
	require.NoError(t, db.Create(&step1).Error)

	terminalID := "terminal-" + name
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "student-" + name,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
		TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)
	return &session
}

func TestStepBanners_AdvanceDrawsOutroThenIntro(t *testing.T) {
	db := setupTestDB(t)
	session := effectsSession(t, db, "banners-order",
		models.ScenarioStep{OutroEffect: "decrypt", OutroText: "Niveau 1 terminé"},
		models.ScenarioStep{IntroEffect: "beams", IntroText: "Niveau 2 débloqué"},
	)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	calls := bannerCalls(verifySvc)
	require.Len(t, calls, 2, "one outro for the step left, one intro for the step entered")

	// Order matters to the learner: the level they finished is acknowledged
	// before the next one announces itself.
	assert.Equal(t, []string{ocfBanner, "decrypt", "Niveau 1 terminé"}, calls[0])
	assert.Equal(t, []string{ocfBanner, "beams", "Niveau 2 débloqué"}, calls[1])
}

func TestStepBanners_EffectWithoutTextDrawsNothing(t *testing.T) {
	db := setupTestDB(t)
	session := effectsSession(t, db, "banners-half-configured",
		models.ScenarioStep{OutroEffect: "decrypt"}, // no text
		models.ScenarioStep{IntroText: "no effect chosen"},
	)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Empty(t, bannerCalls(verifySvc),
		"a half-configured banner has nothing to draw or no way to draw it")
}

// The no-regression guarantee: a scenario that configures no effects must
// behave exactly as it did before these fields existed.
func TestStepBanners_ScenarioWithoutEffects_IssuesNoExtraExec(t *testing.T) {
	db := setupTestDB(t)
	session := effectsSession(t, db, "banners-absent",
		models.ScenarioStep{},
		models.ScenarioStep{},
	)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Empty(t, verifySvc.execCalls,
		"no banners configured means no container traffic at all, exactly as before this feature")
}

// A trainer's banner text reaches a container command. It travels as its own
// argv element to an exec that spawns no shell, so shell metacharacters are
// inert by construction rather than by escaping.
func TestStepBanners_TextIsNeverInterpretedAsShell(t *testing.T) {
	db := setupTestDB(t)

	hostile := "$(touch /tmp/pwned) `id` ; rm -rf / ; echo 'still text'"
	session := effectsSession(t, db, "banners-injection",
		models.ScenarioStep{},
		models.ScenarioStep{IntroEffect: "beams", IntroText: hostile},
	)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	calls := bannerCalls(verifySvc)
	require.Len(t, calls, 1)

	// The whole hostile string arrives as ONE argument, unmodified and
	// unsplit — it is data handed to ocf-banner, never a fragment of a command.
	require.Len(t, calls[0], 3, "exactly interpreter-free argv: helper, effect, text")
	assert.Equal(t, hostile, calls[0][2])

	// And no shell is involved anywhere in the invocation.
	for _, arg := range calls[0] {
		assert.NotContains(t, arg, "sh -c")
	}
	assert.NotEqual(t, "/bin/sh", calls[0][0])
}

// An effect name is written into a shell snippet on the MOTD path, so unlike
// the text it has to be constrained rather than merely quoted.
func TestStepBanners_RejectsAnEffectNameThatIsNotAnIdentifier(t *testing.T) {
	db := setupTestDB(t)
	session := effectsSession(t, db, "banners-bad-effect",
		models.ScenarioStep{},
		models.ScenarioStep{IntroEffect: "beams; rm -rf /", IntroText: "hello"},
	)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	_, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err)

	assert.Empty(t, bannerCalls(verifySvc), "an effect name that is not a bare identifier is refused, not escaped")
}

// Step 0's script runs while the session is still provisioning, before any
// console attaches, so its intro cannot be drawn — it is staged as the MOTD
// the image renders at first login instead.
func TestStepBanners_StepZeroIntroIsStagedAsMotd(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "banners-step-zero",
		Title:        "Step zero banner",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		SetupScript:  "echo setting up",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		IntroEffect: "beams",
		IntroText:   "Bienvenue dans le laboratoire",
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-zero", scenario.ID, "terminal-banners-zero")
	require.NoError(t, err)
	require.Equal(t, "active", waitForSetupDone(t, db, session.ID))

	// Nothing is drawn directly — there is no terminal yet.
	assert.Empty(t, bannerCalls(verifySvc))

	var wroteText, wroteEffect bool
	for _, call := range verifySvc.execCalls {
		joined := strings.Join(call.command, " ")
		if strings.Contains(joined, "/etc/ocf-motd.txt") {
			wroteText = true
			// The text is a positional parameter, not part of the script body,
			// so it cannot be interpreted however it is written.
			assert.Equal(t, "Bienvenue dans le laboratoire", call.command[len(call.command)-1])
			assert.NotContains(t, call.command[2], "Bienvenue",
				"the text must not be interpolated into the command string")
		}
		if strings.Contains(joined, "OCF_MOTD_EFFECT") {
			wroteEffect = true
			assert.Equal(t, "beams", call.command[len(call.command)-1])
			// The console attaches a non-login bash, which never runs
			// profile.d — the export has to land where that shell reads it.
			assert.Contains(t, joined, "/etc/bash.bashrc")
			assert.Contains(t, joined, "grep -q", "the write must be idempotent across replayed provisioning")
			// An environment variable must be set before whatever reads it
			// runs. Nothing sources the hook from bash.bashrc yet, so this is
			// not observable today — but a sourcing block added later will be
			// appended, and an export below it would never be seen.
			assert.Contains(t, joined, "cat "+"/etc/bash.bashrc"+" >> ",
				"the export must be prepended, so it still precedes a sourcing block added later")
		}
	}
	assert.True(t, wroteText, "step 0's intro text must be staged for the login shell")
	assert.True(t, wroteEffect,
		"the chosen effect must be delivered too — otherwise the hook falls back to its default and the trainer's choice is silently discarded")
}

// The simplest scenario shape — a step 0 with a banner and no scripts — takes
// a different branch in StartScenario that skips provisioning entirely. It
// still has to stage the banner, or configuring one on a script-free scenario
// silently does nothing.
func TestStepBanners_StepZeroIntroIsStagedWithoutAnyScripts(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "banners-no-scripts",
		Title:        "Banner, no scripts",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		IntroEffect: "beams",
		IntroText:   "Bienvenue",
	}).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-no-scripts", scenario.ID, "terminal-no-scripts")
	require.NoError(t, err)
	assert.Equal(t, "active", session.Status, "no scripts means no provisioning phase")

	var staged bool
	for _, call := range verifySvc.execCalls {
		if strings.Contains(strings.Join(call.command, " "), "/etc/ocf-motd.txt") {
			staged = true
			assert.Equal(t, "Bienvenue", call.command[len(call.command)-1])
		}
	}
	assert.True(t, staged, "a script-free scenario must still stage its step 0 banner")
}

func TestStepBanners_BannerFailureNeverFailsTheAdvance(t *testing.T) {
	db := setupTestDB(t)
	session := effectsSession(t, db, "banners-failure",
		models.ScenarioStep{OutroEffect: "decrypt", OutroText: "done"},
		models.ScenarioStep{IntroEffect: "beams", IntroText: "next"},
	)

	// A stock image has no ocf-banner: the exec itself fails.
	verifySvc := &bgTrackingVerificationService{execErr: assertAnError{}}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	result, err := sessionSvc.VerifyCurrentStep(session.ID)
	require.NoError(t, err, "a banner is decoration; losing one must never cost the learner the advance")
	require.NotNil(t, result.NextStep)
	assert.Equal(t, 1, *result.NextStep)

	var progress models.ScenarioStepProgress
	require.NoError(t, db.First(&progress, "session_id = ? AND step_order = 0", session.ID).Error)
	assert.Equal(t, "completed", progress.Status)
}

// assertAnError is a minimal error for the failure path above.
type assertAnError struct{}

func (assertAnError) Error() string { return "container unreachable" }
