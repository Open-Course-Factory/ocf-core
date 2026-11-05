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
	OrganizationID() *uuid.UUID
	Period() (from, to time.Time) // to is exclusive
	LearnerIDs() []string
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
	From             string                 `json:"from"`     // first day, YYYY-MM-DD in Timezone
	To               string                 `json:"to"`       // last day, inclusive
	Timezone         string                 `json:"timezone"` // IANA name the days are cut in
	GeneratedAt      time.Time              `json:"generated_at"`
	Trainers         []string               `json:"trainers"`
	Learners         []LearnerParticipation `json:"learners"`
	// CommandsAvailable is false when tt-backend could not answer: the counts are
	// then unknown, not zero.
	CommandsAvailable bool `json:"commands_available"`
}

// LearnerParticipation is one learner's row; empty lists mean no recorded activity.
type LearnerParticipation struct {
	UserID    string                  `json:"user_id"`
	Name      string                  `json:"name"`
	Email     string                  `json:"email"`
	Commands  int                     `json:"commands"`
	Days      []ParticipationDay      `json:"days"`
	Scenarios []ScenarioParticipation `json:"scenarios"`
}

// ParticipationDay holds a day's first and last recorded activity.
type ParticipationDay struct {
	Date          string    `json:"date"` // YYYY-MM-DD in the report's timezone
	FirstActivity time.Time `json:"first_activity"`
	LastActivity  time.Time `json:"last_activity"`
}

// ScenarioParticipation is a learner's progress in one run of a scenario.
type ScenarioParticipation struct {
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Grade          *float64   `json:"grade,omitempty"`
	StepsCompleted int        `json:"steps_completed"`
	StepsTotal     int        `json:"steps_total"`
	VerifyAttempts int        `json:"verify_attempts"`
}

// classPeriodScope is a class over a date range, until training sessions exist.
type classPeriodScope struct {
	group       groupModels.ClassGroup
	from, to    time.Time
	learnerIDs  []string
	trainerIDs  []string
	scenarioIDs []uuid.UUID
}

// NewClassPeriodScope loads a class. An unknown class returns gorm.ErrRecordNotFound.
func NewClassPeriodScope(db *gorm.DB, groupID uuid.UUID, from, to time.Time) (ParticipationScope, error) {
	scope := &classPeriodScope{from: from, to: to}
	if err := db.First(&scope.group, "id = ?", groupID).Error; err != nil {
		return nil, err
	}
	activeMembers := db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND is_active = ?", groupID, true)
	if err := activeMembers.Session(&gorm.Session{}).
		Scopes(groupModels.LearnerRoleScope("group_members")).
		Pluck("user_id", &scope.learnerIDs).Error; err != nil {
		return nil, err
	}
	if err := activeMembers.Session(&gorm.Session{}).
		Where("role IN ?", []groupModels.GroupMemberRole{groupModels.GroupMemberRoleOwner, groupModels.GroupMemberRoleManager}).
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
func (s *classPeriodScope) OrganizationID() *uuid.UUID     { return s.group.OrganizationID }
func (s *classPeriodScope) Period() (time.Time, time.Time) { return s.from, s.to }
func (s *classPeriodScope) LearnerIDs() []string           { return s.learnerIDs }
func (s *classPeriodScope) TrainerIDs() []string           { return s.trainerIDs }
func (s *classPeriodScope) ScenarioIDs() []uuid.UUID       { return s.scenarioIDs }

// BuildParticipationReport assembles the report. Days are cut in location.
func BuildParticipationReport(db *gorm.DB, scope ParticipationScope, commands CommandActivitySource, location *time.Location) (*ParticipationReport, error) {
	from, to := scope.Period()
	report := &ParticipationReport{
		Title:             scope.Title(),
		OrganizationName:  organizationName(db, scope.OrganizationID()),
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
	commandCounts := map[string]int{}
	for _, terminal := range terminals {
		collector.add(terminal.UserID, terminal.CreatedAt)
		commandCounts[terminal.UserID] += len(commandTimes[terminal.SessionID])
		for _, at := range commandTimes[terminal.SessionID] {
			collector.add(terminal.UserID, at)
		}
	}

	scenarioRows, err := loadScenarioParticipation(db, scope, sessionIDs, collector)
	if err != nil {
		return nil, err
	}

	report.Learners = make([]LearnerParticipation, 0, len(scope.LearnerIDs()))
	for _, userID := range scope.LearnerIDs() {
		report.Learners = append(report.Learners, LearnerParticipation{
			UserID:    userID,
			Commands:  commandCounts[userID],
			Days:      collector.daysOf(userID),
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

func organizationName(db *gorm.DB, organizationID *uuid.UUID) string {
	if organizationID == nil {
		return ""
	}
	var organization orgModels.Organization
	if err := db.Select("display_name").First(&organization, "id = ?", *organizationID).Error; err != nil {
		slog.Warn("participation report: organization not found", "id", organizationID, "err", err)
		return ""
	}
	return organization.DisplayName
}

func displayNames(userIDs []string) []string {
	names := make([]string, 0, len(userIDs))
	for _, info := range fetchUserMap(userIDs) {
		names = append(names, info.Name)
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
	return terminals, err
}

func sessionIDsOf(terminals []terminalModels.Terminal) []string {
	ids := make([]string, 0, len(terminals))
	for _, terminal := range terminals {
		ids = append(ids, terminal.SessionID)
	}
	return ids
}

// loadScenarioParticipation returns each learner's runs with something recorded in
// the period, and records those timestamps as activity. A run belongs to the training
// if the class assigns its scenario or it ran in one of the class's terminals: a
// scenario can be started without being assigned.
func loadScenarioParticipation(db *gorm.DB, scope ParticipationScope, terminalSessionIDs []string, collector *activityCollector) (map[string][]ScenarioParticipation, error) {
	from, to := scope.Period()
	var runs []models.ScenarioSession
	err := db.
		Preload("StepProgress").
		Where("user_id IN ? AND is_preview = ?", scope.LearnerIDs(), false).
		Where("scenario_id IN ? OR terminal_session_id IN ?", scope.ScenarioIDs(), terminalSessionIDs).
		Where("started_at < ? AND (completed_at IS NULL OR completed_at >= ?)", to.UTC(), from.UTC()).
		Order("started_at").
		Find(&runs).Error
	if err != nil {
		return nil, err
	}

	scenarioIDs := make([]uuid.UUID, 0, len(runs))
	for _, run := range runs {
		scenarioIDs = append(scenarioIDs, run.ScenarioID)
	}
	var scenarios []models.Scenario
	// Unscoped: evidence of past work must keep its title once a scenario is deleted.
	if err := db.Unscoped().Preload("Steps").Find(&scenarios, "id IN ?", scenarioIDs).Error; err != nil {
		return nil, err
	}
	scenarioByID := make(map[uuid.UUID]models.Scenario, len(scenarios))
	for _, scenario := range scenarios {
		scenarioByID[scenario.ID] = scenario
	}

	rows := make(map[string][]ScenarioParticipation, len(scope.LearnerIDs()))
	for _, run := range runs {
		timestamps := runTimestamps(run)
		// An abandoned run keeps a null completed_at, so the query above cannot tell.
		if !slices.ContainsFunc(timestamps, collector.inPeriod) {
			continue
		}
		for _, at := range timestamps {
			collector.add(run.UserID, at)
		}
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
		Title:       scenario.Title,
		Status:      run.Status,
		StartedAt:   run.StartedAt,
		CompletedAt: run.CompletedAt,
		Grade:       run.Grade,
		StepsTotal:  len(scenario.Steps),
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

func (c *activityCollector) add(userID string, at time.Time) {
	if !c.inPeriod(at) {
		return
	}
	date := at.In(c.location).Format(time.DateOnly)
	if c.days[userID] == nil {
		c.days[userID] = map[string]*ParticipationDay{}
	}
	day, seen := c.days[userID][date]
	if !seen {
		c.days[userID][date] = &ParticipationDay{Date: date, FirstActivity: at, LastActivity: at}
		return
	}
	if at.Before(day.FirstActivity) {
		day.FirstActivity = at
	}
	if at.After(day.LastActivity) {
		day.LastActivity = at
	}
}

func (c *activityCollector) daysOf(userID string) []ParticipationDay {
	days := make([]ParticipationDay, 0, len(c.days[userID]))
	for _, day := range c.days[userID] {
		days = append(days, *day)
	}
	slices.SortFunc(days, func(a, b ParticipationDay) int { return cmp.Compare(a.Date, b.Date) })
	return days
}
