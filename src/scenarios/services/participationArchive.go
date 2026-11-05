package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	ttServices "soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// archiveTimezone cuts days in archived reports. A job has no browser to ask.
// ponytail: one timezone for every class, store one per organization when a
// customer trains outside France.
const archiveTimezone = "Europe/Paris"

// ErrCommandsUnavailable refuses a copy while tt-backend cannot answer: a frozen
// copy would keep unknown command counts for years, and the next run can retry.
var ErrCommandsUnavailable = errors.New("command activity is unavailable")

// ErrIdentitiesUnavailable refuses a copy while Casdoor cannot name a single
// learner: the identity may be erased right after the copy, and with it the names.
var ErrIdentitiesUnavailable = errors.New("learner identities are unavailable")

// ParticipationArchiver freezes participation reports of organizations' classes.
type ParticipationArchiver struct {
	db       *gorm.DB
	commands CommandActivitySource
}

func NewParticipationArchiver(db *gorm.DB, commands CommandActivitySource) *ParticipationArchiver {
	return &ParticipationArchiver{db: db, commands: commands}
}

// NewTTBackendParticipationArchiver reads commands from tt-backend.
func NewTTBackendParticipationArchiver(db *gorm.DB) *ParticipationArchiver {
	return NewParticipationArchiver(db, ttServices.NewCommandActivityReader())
}

// ArchiveDueClasses archives every class whose course days are over or that was
// archived. One failing class does not stop the others; the next run retries it.
func (a *ParticipationArchiver) ArchiveDueClasses(now time.Time) error {
	var classes []groupModels.ClassGroup
	if err := a.db.Where("organization_id IS NOT NULL").Find(&classes).Error; err != nil {
		return fmt.Errorf("load classes to archive: %w", err)
	}
	var failures []error
	for _, class := range classes {
		if !isDueForArchive(class, now) {
			continue
		}
		if err := a.archiveClass(class, now); err != nil {
			failures = append(failures, fmt.Errorf("class %s: %w", class.ID, err))
		}
	}
	return errors.Join(failures...)
}

// ArchiveClassesOfMember archives what a member's erasure is about to remove:
// their part of every class of the organization they ever belonged to.
func (a *ParticipationArchiver) ArchiveClassesOfMember(organizationID uuid.UUID, userID string, now time.Time) error {
	memberships := a.db.Model(&groupModels.GroupMember{}).Select("group_id").Where("user_id = ?", userID)
	var classes []groupModels.ClassGroup
	if err := a.db.Where("organization_id = ? AND id IN (?)", organizationID, memberships).Find(&classes).Error; err != nil {
		return fmt.Errorf("load classes of member: %w", err)
	}
	for _, class := range classes {
		if err := a.archiveClass(class, now); err != nil {
			return fmt.Errorf("class %s: %w", class.ID, err)
		}
	}
	return nil
}

func isDueForArchive(class groupModels.ClassGroup, now time.Time) bool {
	if class.IsArchived() {
		return true
	}
	today := now.In(archiveLocation()).Format(time.DateOnly)
	return len(class.CourseDays) > 0 && slices.Max(class.CourseDays) < today
}

func (a *ParticipationArchiver) archiveClass(class groupModels.ClassGroup, now time.Time) error {
	period := archivePeriodOf(class, now, archiveLocation())
	var existing int64
	if err := a.db.Model(&models.ParticipationArchive{}).
		Where("group_id = ? AND coverage = ?", class.ID, period.coverage).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	report, err := a.reportToFreeze(class, period)
	if err != nil || report == nil {
		return err
	}
	return a.saveCopy(class, period, report)
}

// reportToFreeze builds the report, refusing one too incomplete to keep for years.
// A class that never had a learner has nothing to freeze: it returns nil.
func (a *ParticipationArchiver) reportToFreeze(class groupModels.ClassGroup, period archivePeriod) (*ParticipationReport, error) {
	scope, err := NewClassPeriodScope(a.db, class.ID, period.from, period.to, period.courseDays)
	if err != nil {
		return nil, err
	}
	report, err := BuildParticipationReport(a.db, scope, a.commands, archiveLocation())
	if err != nil {
		return nil, err
	}
	if !report.CommandsAvailable {
		return nil, ErrCommandsUnavailable
	}
	if len(report.Learners) == 0 {
		return nil, nil
	}
	if !slices.ContainsFunc(report.Learners, func(learner LearnerParticipation) bool { return learner.Name != "" }) {
		return nil, ErrIdentitiesUnavailable
	}
	return report, nil
}

func (a *ParticipationArchiver) saveCopy(class groupModels.ClassGroup, period archivePeriod, report *ParticipationReport) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	years, err := a.archiveYearsOf(*class.OrganizationID)
	if err != nil {
		return err
	}
	archive := models.ParticipationArchive{
		GroupID:        class.ID,
		OrganizationID: *class.OrganizationID,
		Title:          report.Title,
		Coverage:       period.coverage,
		Report:         string(encoded),
		ExpiresAt:      period.to.AddDate(years, 0, 0),
	}
	return a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&archive).Error
}

func (a *ParticipationArchiver) archiveYearsOf(organizationID uuid.UUID) (int, error) {
	var organization organizationModels.Organization
	if err := a.db.Select("id", "report_archive_years").First(&organization, "id = ?", organizationID).Error; err != nil {
		return 0, fmt.Errorf("load organization: %w", err)
	}
	return organization.EffectiveReportArchiveYears(), nil
}

type archivePeriod struct {
	from, to   time.Time
	courseDays []string // nil reports every day with activity
	coverage   string
}

// archivePeriodOf covers the course days already over, or, when none is, every
// day since the class began. It stops at the class's archiving so that repeated
// runs on an archived class agree on one coverage.
func archivePeriodOf(class groupModels.ClassGroup, now time.Time, location *time.Location) archivePeriod {
	end := now
	if class.ArchivedAt != nil && class.ArchivedAt.Before(now) {
		end = *class.ArchivedAt
	}
	lastDay := end.In(location).Format(time.DateOnly)

	elapsed := slices.DeleteFunc(slices.Clone([]string(class.CourseDays)), func(day string) bool { return day > lastDay })
	slices.Sort(elapsed)
	if len(elapsed) == 0 {
		firstDay := class.CreatedAt.In(location).Format(time.DateOnly)
		return archivePeriod{
			from:     dayStart(firstDay, location),
			to:       dayStart(lastDay, location).AddDate(0, 0, 1),
			coverage: firstDay + ".." + lastDay,
		}
	}
	return archivePeriod{
		from:       dayStart(elapsed[0], location),
		to:         dayStart(elapsed[len(elapsed)-1], location).AddDate(0, 0, 1),
		courseDays: elapsed,
		coverage:   strings.Join(elapsed, ","),
	}
}

// dayStart reads a day ValidateCourseDays or time.Format already vouched for.
func dayStart(day string, location *time.Location) time.Time {
	start, _ := time.ParseInLocation(time.DateOnly, day, location)
	return start
}

func archiveLocation() *time.Location {
	location, err := time.LoadLocation(archiveTimezone)
	if err != nil {
		return time.UTC // no timezone database in the image: dates shift, nothing is lost
	}
	return location
}
