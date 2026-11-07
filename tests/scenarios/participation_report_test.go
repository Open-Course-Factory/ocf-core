package scenarios_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

var (
	paris        = mustLoadLocation("Europe/Paris")
	reportStart  = time.Date(2026, 3, 2, 0, 0, 0, 0, paris) // a Monday
	reportEnd    = reportStart.AddDate(0, 0, 3)             // up to Thursday 00:00
	mondayAt9    = reportStart.Add(9 * time.Hour)
	mondayAt17   = reportStart.Add(17 * time.Hour)
	tuesdayAt10  = reportStart.AddDate(0, 0, 1).Add(10 * time.Hour)
	beforePeriod = reportStart.AddDate(0, 0, -5)
	afterPeriod  = reportEnd.AddDate(0, 0, 5)
)

func mustLoadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// fakeCommandActivity answers for tt-backend.
type fakeCommandActivity struct {
	times map[string][]time.Time
	err   error
}

func (f fakeCommandActivity) CommandTimes([]string, time.Time, time.Time) (map[string][]time.Time, error) {
	return f.times, f.err
}

type reportFixture struct {
	db       *gorm.DB
	orgID    uuid.UUID
	groupID  uuid.UUID
	scenario *models.Scenario
	learner  string
}

func newReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "report-owner")
	groupID := createTestGroupInOrg(t, db, orgID, "report-owner")
	addGroupMember(t, db, groupID, "report-owner", groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, groupID, "report-trainer", groupModels.GroupMemberRoleManager)
	addGroupMember(t, db, groupID, "report-learner", groupModels.GroupMemberRoleMember)
	scenario := createTestScenarioForOrg(t, db, orgID, "report-scenario")
	createScenarioAssignment(t, db, scenario.ID, &groupID, nil, "group")
	return &reportFixture{db: db, orgID: orgID, groupID: groupID, scenario: scenario, learner: "report-learner"}
}

func (f *reportFixture) seedTerminal(t *testing.T, userID, sessionID string, startedAt time.Time) {
	t.Helper()
	terminal := &terminalModels.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		UserTerminalKeyID: uuid.New(),
		State:             terminalModels.StateDeleted,
		ExpiresAt:         startedAt.Add(time.Hour),
		OrganizationID:    &f.orgID,
	}
	terminal.CreatedAt = startedAt.UTC()
	require.NoError(t, f.db.Create(terminal).Error)
}

func (f *reportFixture) seedRun(t *testing.T, scenarioID uuid.UUID, userID string, startedAt time.Time, stepsCompletedAt ...time.Time) *models.ScenarioSession {
	t.Helper()
	run := &models.ScenarioSession{ScenarioID: scenarioID, UserID: userID, Status: "active", StartedAt: startedAt.UTC()}
	require.NoError(t, f.db.Create(run).Error)
	for order, completedAt := range stepsCompletedAt {
		require.NoError(t, f.db.Create(&models.ScenarioStepProgress{
			SessionID:      run.ID,
			StepOrder:      order,
			Status:         "completed",
			CompletedAt:    &completedAt,
			VerifyAttempts: 2,
		}).Error)
	}
	return run
}

func (f *reportFixture) build(t *testing.T, commands services.CommandActivitySource) *services.ParticipationReport {
	t.Helper()
	return f.buildForCourseDays(t, commands, nil)
}

func (f *reportFixture) buildForCourseDays(t *testing.T, commands services.CommandActivitySource, courseDays []string) *services.ParticipationReport {
	t.Helper()
	scope, err := services.NewClassPeriodScope(f.db, f.groupID, reportStart, reportEnd, courseDays)
	require.NoError(t, err)
	report, err := services.BuildParticipationReport(f.db, scope, commands, paris)
	require.NoError(t, err)
	return report
}

func noCommands() services.CommandActivitySource {
	return fakeCommandActivity{}
}

func learnerRow(t *testing.T, report *services.ParticipationReport, userID string) services.LearnerParticipation {
	t.Helper()
	for _, learner := range report.Learners {
		if learner.UserID == userID {
			return learner
		}
	}
	require.Failf(t, "learner missing", "%s is not in the report", userID)
	return services.LearnerParticipation{}
}

func TestParticipationReport_DescribesTheClassAndPeriod(t *testing.T) {
	f := newReportFixture(t)

	report := f.build(t, noCommands())

	assert.Equal(t, "Test Group", report.Title)
	assert.Equal(t, "Test Org", report.OrganizationName)
	assert.Equal(t, "2026-03-02", report.From)
	assert.Equal(t, "2026-03-04", report.To, "the last day, not the exclusive end of the period")
	assert.Equal(t, "Europe/Paris", report.Timezone)
}

func TestParticipationReport_IdentifiesTheTrainingOrganisationAndConvention(t *testing.T) {
	f := newReportFixture(t)
	require.NoError(t, f.db.Model(&orgModels.Organization{}).Where("id = ?", f.orgID).Updates(map[string]any{
		"siret":                       "10299352400010",
		"training_declaration_number": "11755555575",
	}).Error)
	require.NoError(t, f.db.Model(&groupModels.ClassGroup{}).Where("id = ?", f.groupID).
		Update("convention_reference", "CONV-2026-014").Error)

	report := f.build(t, noCommands())

	assert.Equal(t, "10299352400010", report.OrganizationSiret)
	assert.Equal(t, "11755555575", report.OrganizationDeclarationNumber)
	assert.Equal(t, "CONV-2026-014", report.ConventionReference)
}

func TestParticipationReport_DetailsNotFilledIn_StayEmpty(t *testing.T) {
	report := newReportFixture(t).build(t, noCommands())

	assert.Empty(t, report.OrganizationSiret)
	assert.Empty(t, report.OrganizationDeclarationNumber)
	assert.Empty(t, report.ConventionReference)
}

func TestClassPeriodScope_TrainersAreTheOwnerAndManagers(t *testing.T) {
	f := newReportFixture(t)

	scope, err := services.NewClassPeriodScope(f.db, f.groupID, reportStart, reportEnd, nil)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"report-owner", "report-trainer"}, scope.TrainerIDs())
}

// An abandoned run keeps a null completed_at, so without this it would appear in
// every later period, as if the learner had worked on it.
func TestParticipationReport_RunWithNothingRecordedInThePeriod_IsLeftOut(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, reportStart.AddDate(0, -1, 0))

	report := f.build(t, noCommands())

	assert.Empty(t, learnerRow(t, report, f.learner).Scenarios)
}

func TestParticipationReport_RunStartedEarlierWithAStepCompletedInThePeriod_IsListed(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, reportStart.AddDate(0, -1, 0), mondayAt17)

	report := f.build(t, noCommands())

	learner := learnerRow(t, report, f.learner)
	assert.Len(t, learner.Scenarios, 1)
	require.Len(t, learner.Days, 1)
	assert.True(t, learner.Days[0].FirstActivity.Equal(mondayAt17), "the older start is outside the period")
}

// Absence is part of the evidence: a learner who did nothing still gets a row.
func TestParticipationReport_ListsEveryLearnerAndNoTeachingStaff(t *testing.T) {
	f := newReportFixture(t)

	report := f.build(t, noCommands())

	require.Len(t, report.Learners, 1)
	assert.Equal(t, f.learner, report.Learners[0].UserID)
	assert.Empty(t, report.Learners[0].Days)
	assert.Empty(t, report.Learners[0].Scenarios)
}

func TestParticipationReport_GroupsActivityIntoDaysWithFirstAndLast(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "monday-terminal", mondayAt9)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9.Add(time.Minute), mondayAt17)
	f.seedTerminal(t, f.learner, "tuesday-terminal", tuesdayAt10)

	days := learnerRow(t, f.build(t, noCommands()), f.learner).Days

	require.Len(t, days, 2)
	assert.Equal(t, "2026-03-02", days[0].Date)
	assert.True(t, days[0].FirstActivity.Equal(mondayAt9))
	assert.True(t, days[0].LastActivity.Equal(mondayAt17))
	assert.Equal(t, "2026-03-03", days[1].Date)
	assert.True(t, days[1].FirstActivity.Equal(tuesdayAt10))
}

// 23:30 UTC on Sunday is already Monday in Paris.
func TestParticipationReport_DaysFollowTheRequestedTimezone(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "late-terminal", time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC))

	days := learnerRow(t, f.build(t, noCommands()), f.learner).Days

	require.Len(t, days, 1)
	assert.Equal(t, "2026-03-02", days[0].Date)
}

func TestParticipationReport_IgnoresActivityOutsideThePeriod(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "too-early", beforePeriod)
	f.seedTerminal(t, f.learner, "too-late", afterPeriod)

	assert.Empty(t, learnerRow(t, f.build(t, noCommands()), f.learner).Days)
}

func TestParticipationReport_SummarisesScenarioProgress(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9, mondayAt17)

	scenarios := learnerRow(t, f.build(t, noCommands()), f.learner).Scenarios

	require.Len(t, scenarios, 1)
	assert.Equal(t, f.scenario.Title, scenarios[0].Title)
	assert.Equal(t, 1, scenarios[0].StepsCompleted)
	assert.Equal(t, 1, scenarios[0].StepsTotal)
	assert.Equal(t, 2, scenarios[0].VerifyAttempts)
}

func TestParticipationReport_LeavesOutWhatIsNotThisTraining(t *testing.T) {
	f := newReportFixture(t)
	notAssigned := createTestScenarioForOrg(t, f.db, f.orgID, "not-assigned")
	f.seedRun(t, notAssigned.ID, f.learner, mondayAt9)
	preview := f.seedRun(t, f.scenario.ID, f.learner, mondayAt9)
	require.NoError(t, f.db.Model(preview).Update("is_preview", true).Error)
	f.seedRun(t, f.scenario.ID, "someone-else", mondayAt9)
	otherOrg := uuid.New()
	personal := &terminalModels.Terminal{SessionID: "other-org", UserID: f.learner, UserTerminalKeyID: uuid.New(), State: terminalModels.StateDeleted, ExpiresAt: afterPeriod, OrganizationID: &otherOrg}
	personal.CreatedAt = mondayAt9.UTC()
	require.NoError(t, f.db.Create(personal).Error)

	row := learnerRow(t, f.build(t, noCommands()), f.learner)

	assert.Empty(t, row.Scenarios)
	assert.Empty(t, row.Days)
}

func TestParticipationReport_CountsCommandsAndUsesTheirTimes(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "with-commands", mondayAt9)
	commands := fakeCommandActivity{times: map[string][]time.Time{
		"with-commands": {mondayAt9.Add(time.Minute), mondayAt17},
	}}

	report := f.build(t, commands)

	row := learnerRow(t, report, f.learner)
	assert.True(t, report.CommandsAvailable)
	assert.Equal(t, 2, row.Commands)
	require.Len(t, row.Days, 1)
	assert.True(t, row.Days[0].LastActivity.Equal(mondayAt17))
}

// On real classes a terminal is used for days: each of those days must show.
func TestParticipationReport_TerminalUsedAcrossDays_ShowsEachDay(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "long-terminal", mondayAt9)
	tuesdayAt10 := mondayAt9.Add(25 * time.Hour)
	wednesdayAt10 := tuesdayAt10.AddDate(0, 0, 1)
	commands := fakeCommandActivity{times: map[string][]time.Time{"long-terminal": {mondayAt9, tuesdayAt10, wednesdayAt10}}}

	row := learnerRow(t, f.build(t, commands), f.learner)

	assert.Len(t, row.Days, 3)
}

func TestParticipationReport_TerminalOpenedBeforeThePeriod_CountsItsCommandsInThePeriod(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "opened-earlier", reportStart.AddDate(0, 0, -2))
	commands := fakeCommandActivity{times: map[string][]time.Time{"opened-earlier": {mondayAt9}}}

	row := learnerRow(t, f.build(t, commands), f.learner)

	assert.Equal(t, 1, row.Commands)
	require.Len(t, row.Days, 1, "the opening, two days before the period, is not activity in it")
	assert.True(t, row.Days[0].FirstActivity.Equal(mondayAt9))
}

func TestParticipationReport_RunInAClassTerminalWithoutAssignment_IsListed(t *testing.T) {
	f := newReportFixture(t)
	notAssigned := createTestScenarioForOrg(t, f.db, f.orgID, "not-assigned")
	f.seedTerminal(t, f.learner, "class-terminal", mondayAt9)
	run := f.seedRun(t, notAssigned.ID, f.learner, mondayAt9)
	require.NoError(t, f.db.Model(run).Update("terminal_session_id", "class-terminal").Error)

	row := learnerRow(t, f.build(t, noCommands()), f.learner)

	require.Len(t, row.Scenarios, 1)
	assert.Equal(t, notAssigned.Title, row.Scenarios[0].Title)
}

func TestParticipationReport_DeletedScenario_KeepsItsTitle(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9)
	require.NoError(t, f.db.Delete(f.scenario).Error)

	row := learnerRow(t, f.build(t, noCommands()), f.learner)

	require.Len(t, row.Scenarios, 1)
	assert.Equal(t, f.scenario.Title, row.Scenarios[0].Title)
}

// tt-backend being down must not hide the rest, nor read as zero commands.
func TestParticipationReport_CommandsUnavailable_StillReportsAndSaysSo(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "monday-terminal", mondayAt9)

	report := f.build(t, fakeCommandActivity{err: errors.New("tt-backend is down")})

	assert.False(t, report.CommandsAvailable)
	assert.Len(t, learnerRow(t, report, f.learner).Days, 1)
}

func TestClassPeriodScope_UnknownClass_ReportsIt(t *testing.T) {
	db := freshTestDB(t)

	_, err := services.NewClassPeriodScope(db, uuid.New(), reportStart, reportEnd, nil)

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// A class with no organisation sees no terminal, under the supervision rule.
func TestParticipationReport_ClassWithNoLearners_IsEmptyNotAnError(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "report-owner")
	groupID := createTestGroupInOrg(t, db, orgID, "report-owner")
	scope, err := services.NewClassPeriodScope(db, groupID, reportStart, reportEnd, nil)
	require.NoError(t, err)

	report, err := services.BuildParticipationReport(db, scope, noCommands(), paris)

	require.NoError(t, err)
	assert.Empty(t, report.Learners)
}

func requestParticipationReport(t *testing.T, db *gorm.DB, groupID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/teacher/groups/:groupId/participation-report", scenarioController.NewTeacherController(db).GetParticipationReport)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/teacher/groups/"+groupID.String()+"/participation-report?"+query, nil)
	require.NoError(t, err)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestGetParticipationReport_RefusesABadPeriod(t *testing.T) {
	tests := map[string]string{
		"no dates":         "",
		"no end":           "from=2026-03-02",
		"not a date":       "from=monday&to=2026-03-06",
		"end before start": "from=2026-03-06&to=2026-03-02",
		"more than a year": "from=2025-03-02&to=2026-03-02",
		"not a course day": "from=2026-03-02&to=2026-03-06&days=2026-03-02,monday",
		"unknown timezone": "from=2026-03-02&to=2026-03-06&tz=Mars/Olympus",
	}
	f := newReportFixture(t)
	for name, query := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, requestParticipationReport(t, f.db, f.groupID, query).Code)
		})
	}
}

func TestGetParticipationReport_AcceptsAWholeYear(t *testing.T) {
	f := newReportFixture(t)

	recorder := requestParticipationReport(t, f.db, f.groupID, "from=2025-03-02&to=2026-03-01")

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func TestGetParticipationReport_ReportsTheCourseDaysAsked(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9)

	recorder := requestParticipationReport(t, f.db, f.groupID, "from=2026-03-02&to=2026-03-04&tz=Europe/Paris&days=2026-03-03,2026-03-02,2026-03-03")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var report services.ParticipationReport
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &report))
	assert.Equal(t, []string{"2026-03-02", "2026-03-03"}, report.Days, "sorted, without duplicates")
}

func TestGetParticipationReport_UnknownClass_IsNotFound(t *testing.T) {
	db := freshTestDB(t)

	recorder := requestParticipationReport(t, db, uuid.New(), "from=2026-03-02&to=2026-03-06")

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

// The last day is included: a course ending on Friday has its Friday.
func TestGetParticipationReport_IncludesTheLastDayInTheTimezoneAsked(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9)

	recorder := requestParticipationReport(t, f.db, f.groupID, "from=2026-03-02&to=2026-03-02&tz=Europe/Paris")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var report services.ParticipationReport
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &report))
	require.Len(t, report.Learners, 1)
	require.Len(t, report.Learners[0].Days, 1)
	assert.Equal(t, "2026-03-02", report.Learners[0].Days[0].Date)
}

func TestParticipationReport_LearnerWhoLeft_StaysListedAndMarked(t *testing.T) {
	f := newReportFixture(t)
	addGroupMember(t, f.db, f.groupID, "report-dropout", groupModels.GroupMemberRoleMember)
	require.NoError(t, f.db.Model(&groupModels.GroupMember{}).
		Where("user_id = ?", "report-dropout").Update("is_active", false).Error)

	report := f.build(t, noCommands())

	assert.True(t, learnerRow(t, report, "report-dropout").HasLeft)
	assert.False(t, learnerRow(t, report, f.learner).HasLeft)
}

func TestParticipationReport_LearnerEnrolledAgain_HasOneRowAndHasNotLeft(t *testing.T) {
	f := newReportFixture(t)
	require.NoError(t, f.db.Model(&groupModels.GroupMember{}).
		Where("user_id = ?", f.learner).Update("is_active", false).Error)
	addGroupMember(t, f.db, f.groupID, f.learner, groupModels.GroupMemberRoleMember)

	report := f.build(t, noCommands())

	require.Len(t, report.Learners, 1)
	assert.False(t, report.Learners[0].HasLeft)
}

func TestParticipationReport_CourseDays_LeaveOutOtherDaysAndTheirCommands(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, "long-terminal", mondayAt9)
	commands := fakeCommandActivity{times: map[string][]time.Time{"long-terminal": {mondayAt17, tuesdayAt10, tuesdayAt10.Add(time.Hour)}}}

	report := f.buildForCourseDays(t, commands, []string{"2026-03-02"})

	assert.Equal(t, []string{"2026-03-02"}, report.Days)
	assert.Equal(t, 1, report.OtherActivityDays)
	row := learnerRow(t, report, f.learner)
	require.Len(t, row.Days, 1)
	assert.Equal(t, 1, row.Days[0].Commands)
	assert.Equal(t, 1, row.Commands, "commands of other days must not sit beside the kept days")
}

func TestParticipationReport_CourseDayOutsideThePeriod_IsNotAColumn(t *testing.T) {
	f := newReportFixture(t)

	report := f.buildForCourseDays(t, noCommands(), []string{"2026-03-02", "2026-04-01"})

	assert.Equal(t, []string{"2026-03-02"}, report.Days)
}

func TestParticipationReport_CourseDayWithNoActivity_RemainsAColumn(t *testing.T) {
	f := newReportFixture(t)

	report := f.buildForCourseDays(t, noCommands(), []string{"2026-03-03"})

	assert.Equal(t, []string{"2026-03-03"}, report.Days)
	assert.Empty(t, learnerRow(t, report, f.learner).Days)
}

func TestParticipationReport_ActivityDays_CountLearnersWhateverTheCourseDays(t *testing.T) {
	f := newReportFixture(t)
	addGroupMember(t, f.db, f.groupID, "report-second", groupModels.GroupMemberRoleMember)
	f.seedTerminal(t, f.learner, "first", mondayAt9)
	f.seedTerminal(t, f.learner, "first-again", mondayAt17)
	f.seedTerminal(t, "report-second", "second", mondayAt9)
	f.seedTerminal(t, "report-second", "second-tuesday", tuesdayAt10)

	report := f.buildForCourseDays(t, noCommands(), []string{"2026-03-03"})

	assert.Equal(t, []services.ActivityDay{
		{Date: "2026-03-02", ActiveLearners: 2},
		{Date: "2026-03-03", ActiveLearners: 1},
	}, report.ActivityDays)
}

// Work done outside the course days still counts, without saying when it happened.
func TestParticipationReport_RunOutsideTheCourseDays_KeepsItsResultButNotItsDay(t *testing.T) {
	f := newReportFixture(t)
	f.seedRun(t, f.scenario.ID, f.learner, tuesdayAt10)

	row := learnerRow(t, f.buildForCourseDays(t, noCommands(), []string{"2026-03-02"}), f.learner)

	assert.Len(t, row.Scenarios, 1)
	assert.Empty(t, row.Days)
}

func TestParticipationReport_ReservationPlaceholder_IsNotActivity(t *testing.T) {
	f := newReportFixture(t)
	f.seedTerminal(t, f.learner, terminalModels.TerminalReservationSessionIDPrefix+"pending", mondayAt9)

	assert.Empty(t, learnerRow(t, f.build(t, noCommands()), f.learner).Days)
}

func TestParticipationReport_RemovedStep_DoesNotCountInTheTotal(t *testing.T) {
	f := newReportFixture(t)
	removed := &models.ScenarioStep{ScenarioID: f.scenario.ID, Order: 99, Title: "removed"}
	require.NoError(t, f.db.Create(removed).Error)
	require.NoError(t, f.db.Delete(removed).Error)
	f.seedRun(t, f.scenario.ID, f.learner, mondayAt9)

	scenarios := learnerRow(t, f.build(t, noCommands()), f.learner).Scenarios

	require.Len(t, scenarios, 1)
	assert.Equal(t, 1, scenarios[0].StepsTotal)
}

func TestValidateCourseDays(t *testing.T) {
	assert.NoError(t, groupModels.ValidateCourseDays(nil))
	assert.NoError(t, groupModels.ValidateCourseDays([]string{"2026-03-02"}))
	assert.Error(t, groupModels.ValidateCourseDays([]string{"2026-03-02", "monday"}))
}
