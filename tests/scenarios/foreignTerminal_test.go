// tests/scenarios/foreignTerminal_test.go
//
// A run whose terminal_session_id names a terminal of another user must never
// act on that container: no script, no start, no read of its history. Such a
// run cannot be made through the API any more, but rows made before can
// remain. services.TerminalOfAnotherUser is the one predicate.
package scenarios_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

const foreignTerminalOwner = "foreign-terminal-owner"

// seedForeignTerminal records terminalID as a running terminal of
// foreignTerminalOwner.
func seedForeignTerminal(t *testing.T, db *gorm.DB, terminalID string) {
	t.Helper()
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID: terminalID,
		UserID:    foreignTerminalOwner,
		State:     terminalModels.StateRunning,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)
}

// --- the predicate ----------------------------------------------------------

func TestTerminalOfAnotherUser_MissingTerminalRow_IsNotAnotherUsers(t *testing.T) {
	db := freshTestDB(t)
	terminalID := "no-row-" + uuid.NewString()
	run := &models.ScenarioSession{UserID: "run-owner", TerminalSessionID: &terminalID}

	assert.False(t, services.TerminalOfAnotherUser(db, run),
		"a terminal with no row belongs to nobody else: the run keeps acting on it")
}

func TestTerminalOfAnotherUser_OwnAndForeignTerminal(t *testing.T) {
	db := freshTestDB(t)
	foreign := "foreign-" + uuid.NewString()
	seedForeignTerminal(t, db, foreign)

	assert.True(t, services.TerminalOfAnotherUser(db, &models.ScenarioSession{UserID: "run-owner", TerminalSessionID: &foreign}))
	assert.False(t, services.TerminalOfAnotherUser(db, &models.ScenarioSession{UserID: foreignTerminalOwner, TerminalSessionID: &foreign}))
	assert.False(t, services.TerminalOfAnotherUser(db, &models.ScenarioSession{UserID: "run-owner"}), "no terminal at all")
}

func TestTerminalOfAnotherUser_UnreadableTerminalRow_Refuses(t *testing.T) {
	// A database without the terminals table: every read of the owner fails.
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	terminalID := "unreadable-" + uuid.NewString()

	assert.True(t, services.TerminalOfAnotherUser(broken, &models.ScenarioSession{UserID: "run-owner", TerminalSessionID: &terminalID}),
		"an owner that cannot be read must fail closed")
}

// A soft-deleted terminal keeps its owner: deleting it never hands its id to
// whoever else's run names it.
func TestTerminalOfAnotherUser_SoftDeletedForeignTerminal_IsAnotherUsers(t *testing.T) {
	db := freshTestDB(t)
	foreign := "soft-deleted-" + uuid.NewString()
	seedForeignTerminal(t, db, foreign)
	require.NoError(t, db.Where("session_id = ?", foreign).Delete(&terminalModels.Terminal{}).Error)

	assert.True(t, services.TerminalOfAnotherUser(db, &models.ScenarioSession{UserID: "run-owner", TerminalSessionID: &foreign}))
	assert.False(t, services.TerminalOfAnotherUser(db, &models.ScenarioSession{UserID: foreignTerminalOwner, TerminalSessionID: &foreign}))
}

// The by-terminal lookup returns the owner's run even when a newer run of
// another user names the same, soft-deleted, terminal.
func TestFindSessionByTerminal_ForeignRunOnSoftDeletedTerminal_DoesNotShadowOwnersRun(t *testing.T) {
	db := freshTestDB(t)
	scenario := seedSessionScenario(t, db)
	terminalID := "shadowed-" + uuid.NewString()
	seedForeignTerminal(t, db, terminalID)
	ownerRun := models.ScenarioSession{ScenarioID: scenario.ID, UserID: foreignTerminalOwner, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID}
	require.NoError(t, db.Create(&ownerRun).Error)
	forged := models.ScenarioSession{ScenarioID: scenario.ID, UserID: "forger", Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID}
	forged.CreatedAt = ownerRun.CreatedAt.Add(time.Hour)
	require.NoError(t, db.Create(&forged).Error)
	require.NoError(t, db.Where("session_id = ?", terminalID).Delete(&terminalModels.Terminal{}).Error)

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	found, err := sessionSvc.FindSessionByTerminal(terminalID)

	require.NoError(t, err)
	assert.Equal(t, ownerRun.ID, found.ID, "the newer run of another user must not shadow the owner's")
}

// --- learner exec paths not guarded ----------------------------------------

// A correct flag advances the run and sets up the next step: its background
// script must not run in another user's container.
func TestSubmitFlag_RunOnAnotherUsersTerminal_RunsNothingThere(t *testing.T) {
	db := freshTestDB(t)
	session := flaggedTwoStepSession(t, db, "foreign-flag", models.ScenarioStep{
		BackgroundScript:         "echo setting up level one",
		BackgroundTimeoutSeconds: 5, // inline, so the exec has happened on return
		ForegroundScript:         "echo hello",
	})
	seedForeignTerminal(t, db, *session.TerminalSessionID)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)
	_, _ = sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")

	assert.Empty(t, verifySvc.execCalls, "no script may run in another user's container")
	assert.Empty(t, verifySvc.pushFileCalls, "no file may be pushed into another user's container")
	assert.Empty(t, verifySvc.consoleWrites, "nothing may be typed into another user's console")
}

// surveyThenSetupSession seeds a run on terminalID at a survey step (which any
// answer passes) followed by a step with a background script.
func surveyThenSetupSession(t *testing.T, db *gorm.DB, terminalID string) models.ScenarioSession {
	t.Helper()
	scenario := models.Scenario{Name: "foreign-quiz-" + uuid.NewString()[:8], Title: "Foreign Quiz", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario).Error)
	survey := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Feedback", StepType: "survey"}
	require.NoError(t, db.Create(&survey).Error)
	require.NoError(t, db.Create(&models.ScenarioStepQuestion{
		StepID: survey.ID, Order: 1, QuestionText: "Anything to add?", QuestionType: "free_text",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Next", StepType: "terminal",
		BackgroundScript: "echo setting up", BackgroundTimeoutSeconds: 5,
	}).Error)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-foreign-quiz", Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)
	return session
}

// The same advance through a quiz (here a survey, which any answer passes).
func TestSubmitQuiz_RunOnAnotherUsersTerminal_RunsNothingThere(t *testing.T) {
	db := freshTestDB(t)
	terminalID := "foreign-quiz-terminal"
	seedForeignTerminal(t, db, terminalID)
	session := surveyThenSetupSession(t, db, terminalID)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	result, err := sessionSvc.SubmitQuiz(session.ID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{}})
	require.NoError(t, err)
	require.NotNil(t, result.NextStep, "control: the survey advances the run")

	assert.Empty(t, verifySvc.execCalls, "no script may run in another user's container")
}

// failTerminalReads makes every read of the terminals table fail until the
// test ends, as a database error would.
func failTerminalReads(t *testing.T, db *gorm.DB) {
	t.Helper()
	const name = "test:fail_terminal_reads"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "terminals" {
			_ = tx.AddError(errors.New("terminals unreadable"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

// When the terminal's owner cannot be read, an advance is refused before it is
// written: it must not go through with the next step's setup silently skipped.
func TestSubmitFlag_TerminalOwnerUnreadable_RefusesTheAdvance(t *testing.T) {
	db := freshTestDB(t)
	session := flaggedTwoStepSession(t, db, "unreadable-flag", models.ScenarioStep{
		BackgroundScript: "echo setting up level one", BackgroundTimeoutSeconds: 5,
	})
	failTerminalReads(t, db)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{validateRes: true}, verifySvc)
	_, err := sessionSvc.SubmitFlag(session.ID, "flag{step-zero}")

	require.Error(t, err)
	assert.Equal(t, 0, currentStepOf(t, db, *session), "the run must not advance")
	assert.Empty(t, verifySvc.execCalls)
}

func TestSubmitQuiz_TerminalOwnerUnreadable_RefusesTheAdvance(t *testing.T) {
	db := freshTestDB(t)
	session := surveyThenSetupSession(t, db, "unreadable-quiz-terminal")
	failTerminalReads(t, db)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	_, err := sessionSvc.SubmitQuiz(session.ID, dto.SubmitQuizInput{Answers: map[uuid.UUID]string{}})

	require.Error(t, err)
	assert.Equal(t, 0, currentStepOf(t, db, session), "the run must not advance")
	assert.Empty(t, verifySvc.execCalls)
}

// test-verify runs an arbitrary script: on a preview run pointing at another
// user's terminal it must not reach that container.
func TestTestVerifyScript_PreviewOnAnotherUsersTerminal_RunsNothing(t *testing.T) {
	db := freshTestDB(t)
	f := newTestVerifyFixture(t, db, testVerifyAuthor, true, 0, "", "")
	seedForeignTerminal(t, db, *f.session.TerminalSessionID)

	w := postTestVerify(t, db, testVerifyAuthor, f.session.ID.String(), dto.TestVerifyScriptInput{Script: "id"})

	assert.NotEqual(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, *f.execHits, "the script must not run in another user's container")
}

// Resuming a paused run starts its terminal in place: never someone else's.
func TestResume_PausedRunOnAnotherUsersTerminal_DoesNotStartIt(t *testing.T) {
	f := seedResumableRun(t, "resume-foreign", resumeSeed{terminalState: terminalModels.StateStopped})
	require.NoError(t, f.db.Model(&terminalModels.Terminal{}).Where("session_id = ?", f.oldTerminal).
		Update("user_id", foreignTerminalOwner).Error)
	tt := newPreviewTTBackend(t)

	w := resumeRun(t, f)

	assert.Empty(t, tt.startedSessions(), "another user's stopped container must not be started; body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), f.oldTerminal, "nor handed to the caller")
}

// --- teacher side -----------------------------------------------------------

// A class manager reads a learner's command history through the learner's
// run: not the history of a terminal the learner does not own.
func TestGetSessionCommands_RunOnAnotherUsersTerminal_ReadsNothing(t *testing.T) {
	db := freshTestDB(t)
	groupID := uuid.New()
	studentID := "student-foreign-cmd"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)
	scenario := models.Scenario{Name: "foreign-cmd", Title: "Foreign Commands", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)
	terminalID := "foreign-cmd-terminal"
	seedForeignTerminal(t, db, terminalID)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)

	tt := newCommandsCapturingTTService([]byte(`{"commands":[]}`))
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, tt, sessionSvc)

	_, _, err := dashSvc.GetSessionCommands(groupID, session.ID, 50, 0)

	assert.Error(t, err)
	assert.Empty(t, tt.lastSessionUUID, "another user's terminal history must not be read")
}
