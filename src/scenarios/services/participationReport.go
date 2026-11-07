package services

import (
	"cmp"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// ParticipationScope says what a participation report covers. The report knows
// nothing else about where this comes from, so a real training session entity
// can replace a class over a date range by implementing this interface.
type ParticipationScope interface {
	Title() string
	ConventionReference() string // the training agreement, empty when not filled in
	OrganizationID() *uuid.UUID
	Period() (from, to time.Time) // to is exclusive
	// CourseDays lists the days that belong to the training, YYYY-MM-DD. Empty means
	// every day with activity counts.
	CourseDays() []string
	LearnerIDs() []string       // everyone who took part, including those who left
	FormerLearnerIDs() []string // those of LearnerIDs who no longer belong
	TrainerIDs() []string
	ScenarioIDs() []uuid.UUID
}

// CommandActivitySource says when each terminal session's commands ran within
// [from, to). A session outlives its day, so per-session totals would not do.
type CommandActivitySource interface {
	CommandTimes(sessionUUIDs []string, from, to time.Time) (map[string][]time.Time, error)
}

// ParticipationReport is evidence of what each learner did over a period. It
// reports recorded activity, never hours attended: a learner may be present
// between two recorded events, or absent.
type ParticipationReport struct {
	Title            string                 `json:"title"`
	OrganizationName string                 `json:"organization_name"`
	// Empty when the organization or the class has not filled them in.
	OrganizationSiret             string `json:"organization_siret"`
	OrganizationDeclarationNumber string `json:"organization_declaration_number"`
	ConventionReference           string `json:"convention_reference"`
	From             string                 `json:"from"`     // first day, YYYY-MM-DD in Timezone
	To               string                 `json:"to"`       // last day, inclusive
	Timezone         string                 `json:"timezone"` // IANA name the days are cut in
	GeneratedAt      time.Time              `json:"generated_at"`
	Trainers         []string               `json:"trainers"`
	// Days are the report's columns: the course days in the period, or every day
	// with activity when no course day is set.
	Days []string `json:"days"`
	// ActivityDays lists every day of the period with activity, whether or not it
	// is reported, so a manager can pick the course days from them.
	ActivityDays []ActivityDay `json:"activity_days"`
	// OtherActivityDays counts the days with activity that the report leaves out. It
	// says so, or the report would read as edited evidence.
	OtherActivityDays int                    `json:"other_activity_days"`
	Learners          []LearnerParticipation `json:"learners"`
	// CommandsAvailable is false when tt-backend could not answer: the counts are
	// then unknown, not zero.
	CommandsAvailable bool `json:"commands_available"`
}

// ActivityDay says how many learners were active on a day.
type ActivityDay struct {
	Date           string `json:"date"` // YYYY-MM-DD in the report's timezone
	ActiveLearners int    `json:"active_learners"`
}

// LearnerParticipation is one learner's row; empty lists mean no recorded activity.
type LearnerParticipation struct {
	UserID    string                  `json:"user_id"`
	Name      string                  `json:"name"`
	Email     string                  `json:"email"`
	HasLeft   bool                    `json:"has_left"` // no longer in the class
	Commands  int                     `json:"commands"` // on the reported days
	Days      []ParticipationDay      `json:"days"`
	Scenarios []ScenarioParticipation `json:"scenarios"`
}

// ParticipationDay holds a day's first and last recorded activity and its commands.
type ParticipationDay struct {
	Date          string    `json:"date"` // YYYY-MM-DD in the report's timezone
	Commands      int       `json:"commands"`
	FirstActivity time.Time `json:"first_activity"`
	LastActivity  time.Time `json:"last_activity"`
}

// ScenarioParticipation is a learner's progress in one run of a scenario. It has no
// dates: a lab finished outside the course days keeps its result, not when it happened.
type ScenarioParticipation struct {
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Grade          *float64 `json:"grade,omitempty"`
	StepsCompleted int      `json:"steps_completed"`
	StepsTotal     int      `json:"steps_total"`
	VerifyAttempts int      `json:"verify_attempts"`
}

// classPeriodScope is a class over a date range, until training sessions exist.
type classPeriodScope struct {
	group            groupModels.ClassGroup
	from, to         time.Time
	courseDays       []string
	learnerIDs       []string
	formerLearnerIDs []string
	trainerIDs       []string
	scenarioIDs      []uuid.UUID
}

// NewClassPeriodScope loads a class. An unknown class returns gorm.ErrRecordNotFound.
func NewClassPeriodScope(db *gorm.DB, groupID uuid.UUID, from, to time.Time, courseDays []string) (ParticipationScope, error) {
	scope := &classPeriodScope{from: from, to: to, courseDays: courseDays}
	if err := db.First(&scope.group, "id = ?", groupID).Error; err != nil {
		return nil, err
	}
	members := db.Model(&groupModels.GroupMember{}).Where("group_id = ?", groupID)
	learners := members.Session(&gorm.Session{}).Scopes(groupModels.LearnerRoleScope("group_members"))
	if err := learners.Session(&gorm.Session{}).Distinct().Pluck("user_id", &scope.learnerIDs).Error; err != nil {
		return nil, err
	}
	// Someone removed and enrolled again has both an inactive and an active row.
	var currentLearnerIDs []string
	if err := learners.Session(&gorm.Session{}).Where("is_active = ?", true).Pluck("user_id", &currentLearnerIDs).Error; err != nil {
		return nil, err
	}
	for _, userID := range scope.learnerIDs {
		if !slices.Contains(currentLearnerIDs, userID) {
			scope.formerLearnerIDs = append(scope.formerLearnerIDs, userID)
		}
	}
	if err := members.Session(&gorm.Session{}).
		Where("is_active = ? AND role IN ?", true, groupModels.ManagerRoles).
		Pluck("user_id", &scope.trainerIDs).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&models.ScenarioAssignment{}).
		Where("group_id = ?", groupID).
		Pluck("scenario_id", &scope.scenarioIDs).Error; err != nil {
		return nil, err
	}
	return scope, nil
}

func (s *classPeriodScope) Title() string                  { return s.group.DisplayName }
func (s *classPeriodScope) ConventionReference() string   { return s.group.ConventionReference }
func (s *classPeriodScope) OrganizationID() *uuid.UUID     { return s.group.OrganizationID }
func (s *classPeriodScope) Period() (time.Time, time.Time) { return s.from, s.to }
func (s *classPeriodScope) CourseDays() []string           { return s.courseDays }
func (s *classPeriodScope) LearnerIDs() []string           { return s.learnerIDs }
func (s *classPeriodScope) FormerLearnerIDs() []string     { return s.formerLearnerIDs }
func (s *classPeriodScope) TrainerIDs() []string           { return s.trainerIDs }
func (s *classPeriodScope) ScenarioIDs() []uuid.UUID       { return s.scenarioIDs }

// BuildParticipationReport assembles the report. Days are cut in location.
func BuildParticipationReport(db *gorm.DB, scope ParticipationScope, commands CommandActivitySource, location *time.Location) (*ParticipationReport, error) {
	from, to := scope.Period()
	organization := reportOrganization(db, scope.OrganizationID())
	report := &ParticipationReport{
		Title:             scope.Title(),
		OrganizationName:  organization.DisplayName,
		OrganizationSiret: organization.Siret,
		OrganizationDeclarationNumber: organization.TrainingDeclarationNumber,
		ConventionReference: scope.ConventionReference(),
		From:              from.In(location).Format(time.DateOnly),
		To:                to.Add(-time.Nanosecond).In(location).Format(time.DateOnly),
		Timezone:          location.String(),
		GeneratedAt:       time.Now(),
		Trainers:          displayNames(scope.TrainerIDs()),
		CommandsAvailable: true,
	}

	collector := newActivityCollector(from, to, location)

	terminals, err := loadScopeTerminals(db, scope)
	if err != nil {
		return nil, err
	}
	sessionIDs := sessionIDsOf(terminals)
	commandTimes, err := commands.CommandTimes(sessionIDs, from, to)
	if err != nil {
		slog.Warn("participation report: commands unavailable", "err", err)
		report.CommandsAvailable = false
	}
	collector.addTerminals(terminals, commandTimes)

	runs, err := loadScenarioRuns(db, scope, sessionIDs)
	if err != nil {
		return nil, err
	}
	runs = slices.DeleteFunc(runs, func(run models.ScenarioSession) bool {
		return !slices.ContainsFunc(runTimestamps(run), collector.inPeriod)
	})
	for _, run := range runs {
		for _, at := range runTimestamps(run) {
			collector.addActivity(run.UserID, at)
		}
	}
	scenarioRows, err := scenarioRowsByLearner(db, runs)
	if err != nil {
		return nil, err
	}

	report.ActivityDays = collector.activityDays()
	report.Days = reportedDays(scope.CourseDays(), report.From, report.To, report.ActivityDays)
	for _, day := range report.ActivityDays {
		if !slices.Contains(report.Days, day.Date) {
			report.OtherActivityDays++
		}
	}

	report.Learners = make([]LearnerParticipation, 0, len(scope.LearnerIDs()))
	for _, userID := range scope.LearnerIDs() {
		days := collector.daysOf(userID, report.Days)
		report.Learners = append(report.Learners, LearnerParticipation{
			UserID:    userID,
			HasLeft:   slices.Contains(scope.FormerLearnerIDs(), userID),
			Commands:  totalCommands(days),
			Days:      days,
			Scenarios: append([]ScenarioParticipation{}, scenarioRows[userID]...), // an empty list in JSON, not null
		})
	}
	resolveUserIdentities(report.Learners,
		func(learner LearnerParticipation) string { return learner.UserID },
		func(learner *LearnerParticipation, info userInfo) {
			learner.Name, learner.Email = info.Name, info.Email
		})
	slices.SortFunc(report.Learners, func(a, b LearnerParticipation) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UserID, b.UserID))
	})
	return report, nil
}

// reportedDays are the course days within the period (YYYY-MM-DD strings sort as dates
// do), or every day with activity when the scope sets no course day.
func reportedDays(courseDays []string, firstDay, lastDay string, activityDays []ActivityDay) []string {
	days := []string{}
	if len(courseDays) == 0 {
		for _, day := range activityDays {
			days = append(days, day.Date)
		}
		return days
	}
	for _, day := range courseDays {
		if firstDay <= day && day <= lastDay {
			days = append(days, day)
		}
	}
	return days
}

func totalCommands(days []ParticipationDay) int {
	total := 0
	for _, day := range days {
		total += day.Commands
	}
	return total
}

// reportOrganization returns a blank organization when there is none: the report
// is still worth having without its header.
func reportOrganization(db *gorm.DB, organizationID *uuid.UUID) orgModels.Organization {
	var organization orgModels.Organization
	if organizationID == nil {
		return organization
	}
	if err := db.Select("display_name", "siret", "training_declaration_number").First(&organization, "id = ?", *organizationID).Error; err != nil {
		slog.Warn("participation report: organization not found", "id", organizationID, "err", err)
		return orgModels.Organization{}
	}
	return organization
}

func displayNames(userIDs []string) []string {
	users := fetchUserMap(userIDs)
	names := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		names = append(names, cmp.Or(users[userID].Name, userID))
	}
	slices.Sort(names)
	return names
}

// loadScopeTerminals follows the supervision rule: a class sees the terminals its
// learners opened in its organisation, and a class with no organisation sees none.
// Opened before the period is fine: a terminal can still be used days later.
func loadScopeTerminals(db *gorm.DB, scope ParticipationScope) ([]terminalModels.Terminal, error) {
	_, to := scope.Period()
	var terminals []terminalModels.Terminal
	err := db.
		Where("terminals.user_id IN ?", scope.LearnerIDs()).
		Where("terminals.created_at < ?", to.UTC()).
		Scopes(terminalModels.SupervisableByGroupOrgScope(scope.OrganizationID())).
		Find(&terminals).Error
	// A reservation placeholder holds budget: nobody has used a terminal yet.
	return slices.DeleteFunc(terminals, func(terminal terminalModels.Terminal) bool {
		return terminalModels.IsReservationPlaceholderSessionID(terminal.SessionID)
	}), err
}

func sessionIDsOf(terminals []terminalModels.Terminal) []string {
	ids := make([]string, 0, len(terminals))
	for _, terminal := range terminals {
		ids = append(ids, terminal.SessionID)
	}
	return ids
}

// loadScenarioRuns returns each learner's runs that may fall in the period; the
// caller keeps those with a timestamp in it, since an abandoned run keeps a null
// completed_at. A run belongs to the training if the class assigns its scenario or it
// ran in one of the class's terminals: a scenario can be started without being assigned.
func loadScenarioRuns(db *gorm.DB, scope ParticipationScope, terminalSessionIDs []string) ([]models.ScenarioSession, error) {
	from, to := scope.Period()
	var runs []models.ScenarioSession
	err := db.
		Preload("StepProgress").
		Where("user_id IN ? AND is_preview = ?", scope.LearnerIDs(), false).
		Where("scenario_id IN ? OR terminal_session_id IN ?", scope.ScenarioIDs(), terminalSessionIDs).
		Where("started_at < ? AND (completed_at IS NULL OR completed_at >= ?)", to.UTC(), from.UTC()).
		Order("started_at").
		Find(&runs).Error
	return runs, err
}

func scenarioRowsByLearner(db *gorm.DB, runs []models.ScenarioSession) (map[string][]ScenarioParticipation, error) {
	scenarioIDs := make([]uuid.UUID, 0, len(runs))
	for _, run := range runs {
		scenarioIDs = append(scenarioIDs, run.ScenarioID)
	}
	var scenarios []models.Scenario
	// Unscoped keeps a deleted scenario's title, as evidence of past work. It carries
	// over to the preload, whose removed steps must still not count.
	if err := db.Unscoped().Preload("Steps", "deleted_at IS NULL").Find(&scenarios, "id IN ?", scenarioIDs).Error; err != nil {
		return nil, err
	}
	scenarioByID := make(map[uuid.UUID]models.Scenario, len(scenarios))
	for _, scenario := range scenarios {
		scenarioByID[scenario.ID] = scenario
	}

	rows := map[string][]ScenarioParticipation{}
	for _, run := range runs {
		rows[run.UserID] = append(rows[run.UserID], scenarioParticipationOf(run, scenarioByID[run.ScenarioID]))
	}
	return rows, nil
}

func runTimestamps(run models.ScenarioSession) []time.Time {
	timestamps := []time.Time{run.StartedAt}
	if run.CompletedAt != nil {
		timestamps = append(timestamps, *run.CompletedAt)
	}
	for _, step := range run.StepProgress {
		if step.CompletedAt != nil {
			timestamps = append(timestamps, *step.CompletedAt)
		}
	}
	return timestamps
}

func scenarioParticipationOf(run models.ScenarioSession, scenario models.Scenario) ScenarioParticipation {
	row := ScenarioParticipation{
		Title:      scenario.Title,
		Status:     run.Status,
		Grade:      run.Grade,
		StepsTotal: len(scenario.Steps),
	}
	for _, step := range run.StepProgress {
		row.VerifyAttempts += step.VerifyAttempts
		if step.Status == "completed" {
			row.StepsCompleted++
		}
	}
	return row
}

// activityCollector turns timestamps into first and last activity per learner and
// day, ignoring anything outside the period.
type activityCollector struct {
	from, to time.Time
	location *time.Location
	days     map[string]map[string]*ParticipationDay
}

func newActivityCollector(from, to time.Time, location *time.Location) *activityCollector {
	return &activityCollector{from: from, to: to, location: location, days: map[string]map[string]*ParticipationDay{}}
}

func (c *activityCollector) inPeriod(at time.Time) bool {
	return !at.Before(c.from) && at.Before(c.to)
}

func (c *activityCollector) addTerminals(terminals []terminalModels.Terminal, commandTimes map[string][]time.Time) {
	for _, terminal := range terminals {
		c.addActivity(terminal.UserID, terminal.CreatedAt)
		for _, at := range commandTimes[terminal.SessionID] {
			if day := c.addActivity(terminal.UserID, at); day != nil {
				day.Commands++
			}
		}
	}
}

// addActivity returns the day the activity falls on, nil when outside the period.
func (c *activityCollector) addActivity(userID string, at time.Time) *ParticipationDay {
	if !c.inPeriod(at) {
		return nil
	}
	date := at.In(c.location).Format(time.DateOnly)
	if c.days[userID] == nil {
		c.days[userID] = map[string]*ParticipationDay{}
	}
	day, seen := c.days[userID][date]
	if !seen {
		day = &ParticipationDay{Date: date, FirstActivity: at, LastActivity: at}
		c.days[userID][date] = day
	}
	if at.Before(day.FirstActivity) {
		day.FirstActivity = at
	}
	if at.After(day.LastActivity) {
		day.LastActivity = at
	}
	return day
}

func (c *activityCollector) activityDays() []ActivityDay {
	activeLearners := map[string]int{}
	for _, days := range c.days {
		for date := range days {
			activeLearners[date]++
		}
	}
	activityDays := make([]ActivityDay, 0, len(activeLearners))
	for date, count := range activeLearners {
		activityDays = append(activityDays, ActivityDay{Date: date, ActiveLearners: count})
	}
	slices.SortFunc(activityDays, func(a, b ActivityDay) int { return cmp.Compare(a.Date, b.Date) })
	return activityDays
}

// daysOf returns the learner's activity on the given days, sorted.
func (c *activityCollector) daysOf(userID string, reported []string) []ParticipationDay {
	days := make([]ParticipationDay, 0, len(c.days[userID]))
	for date, day := range c.days[userID] {
		if slices.Contains(reported, date) {
			days = append(days, *day)
		}
	}
	slices.SortFunc(days, func(a, b ParticipationDay) int { return cmp.Compare(a.Date, b.Date) })
	return days
}
