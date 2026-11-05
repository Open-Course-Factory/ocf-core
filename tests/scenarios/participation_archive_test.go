package scenarios_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

var (
	archiveMonday = "2026-03-02"
	archiveFriday = "2026-03-06"
	duringCourse  = time.Date(2026, 3, 4, 12, 0, 0, 0, paris)
	afterCourse   = time.Date(2026, 3, 7, 12, 0, 0, 0, paris)
)

func (f *reportFixture) setCourseDays(t *testing.T, days ...string) {
	t.Helper()
	require.NoError(t, f.db.Model(&groupModels.ClassGroup{}).Where("id = ?", f.groupID).
		Update("course_days", pq.StringArray(days)).Error)
}

// fakeCasdoor names every user it is asked about, until the test ends.
func fakeCasdoor(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"name":"learner","displayName":"A Learner"}}`))
	}))
	casdoorsdk.InitConfig(server.URL, "dummy", "dummy", "dummy", "dummy", "dummy")
	t.Cleanup(func() {
		casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
		server.Close()
	})
}

func archivesOf(t *testing.T, db *gorm.DB) []models.ParticipationArchive {
	t.Helper()
	var archives []models.ParticipationArchive
	require.NoError(t, db.Order("created_at").Find(&archives).Error)
	return archives
}

func TestParticipationArchive_DueOnlyOnceTheLastCourseDayIsOver(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday, archiveFriday)
	archiver := services.NewParticipationArchiver(f.db, noCommands())

	require.NoError(t, archiver.ArchiveDueClasses(duringCourse))
	assert.Empty(t, archivesOf(t, f.db), "a course still running is not archived")

	require.NoError(t, archiver.ArchiveDueClasses(afterCourse))
	require.NoError(t, archiver.ArchiveDueClasses(afterCourse.AddDate(0, 0, 1)))

	archives := archivesOf(t, f.db)
	require.Len(t, archives, 1, "the same days are archived once")
	assert.Equal(t, f.orgID, archives[0].OrganizationID)
	assert.Equal(t, archiveMonday+","+archiveFriday, archives[0].Coverage)
	assert.Contains(t, archives[0].Report, f.learner)
}

func TestParticipationArchive_ClassNeverGivenCourseDaysWaitsForItsArchiving(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	archiver := services.NewParticipationArchiver(f.db, noCommands())

	require.NoError(t, archiver.ArchiveDueClasses(afterCourse))
	assert.Empty(t, archivesOf(t, f.db))

	archivedAt := afterCourse.Add(-time.Hour)
	require.NoError(t, f.db.Model(&groupModels.ClassGroup{}).Where("id = ?", f.groupID).Update("archived_at", archivedAt).Error)
	require.NoError(t, archiver.ArchiveDueClasses(afterCourse))
	require.NoError(t, archiver.ArchiveDueClasses(afterCourse.AddDate(0, 0, 3)))
	assert.Len(t, archivesOf(t, f.db), 1, "later runs on an archived class cover the same days")
}

func TestParticipationArchive_AnErasureMidCourseKeepsItsCopyBesideTheFinalOne(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday, archiveFriday)
	archiver := services.NewParticipationArchiver(f.db, noCommands())

	require.NoError(t, archiver.ArchiveClassesOfMember(f.orgID, f.learner, duringCourse))
	archives := archivesOf(t, f.db)
	require.Len(t, archives, 1)
	assert.Equal(t, archiveMonday, archives[0].Coverage, "only the days already over")

	require.NoError(t, archiver.ArchiveDueClasses(afterCourse))
	assert.Len(t, archivesOf(t, f.db), 2, "the finished course gets a copy, and the earlier one stays")
}

func TestParticipationArchive_KeepsTheLearnerAfterTheirData(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday)
	archiver := services.NewParticipationArchiver(f.db, noCommands())
	require.NoError(t, archiver.ArchiveClassesOfMember(f.orgID, f.learner, afterCourse))

	require.NoError(t, f.db.Where("user_id = ?", f.learner).Delete(&groupModels.GroupMember{}).Error)
	require.NoError(t, archiver.ArchiveClassesOfMember(f.orgID, f.learner, afterCourse))

	archives := archivesOf(t, f.db)
	require.Len(t, archives, 1)
	assert.Contains(t, archives[0].Report, f.learner, "a later run never overwrites a frozen copy")
}

func TestParticipationArchive_RefusesACopyWhileCommandsAreUnavailable(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday)
	f.seedTerminal(t, f.learner, "archive-session", mondayAt9)
	archiver := services.NewParticipationArchiver(f.db, fakeCommandActivity{err: errors.New("tt-backend down")})

	assert.ErrorIs(t, archiver.ArchiveClassesOfMember(f.orgID, f.learner, afterCourse), services.ErrCommandsUnavailable)
	assert.ErrorIs(t, archiver.ArchiveDueClasses(afterCourse), services.ErrCommandsUnavailable)
	assert.Empty(t, archivesOf(t, f.db))
}

func TestParticipationArchive_RefusesACopyWhenNoLearnerCanBeNamed(t *testing.T) {
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday)
	// A name never asked before: fetched names stay cached for a few minutes.
	namelessLearner := uuid.NewString()
	require.NoError(t, f.db.Where("user_id = ?", f.learner).Delete(&groupModels.GroupMember{}).Error)
	addGroupMember(t, f.db, f.groupID, namelessLearner, groupModels.GroupMemberRoleMember)

	err := services.NewParticipationArchiver(f.db, noCommands()).ArchiveClassesOfMember(f.orgID, namelessLearner, afterCourse)

	assert.ErrorIs(t, err, services.ErrIdentitiesUnavailable)
	assert.Empty(t, archivesOf(t, f.db))
}

func TestParticipationArchive_MemberOfAnotherOrganizationsClassIsLeftAlone(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday)
	archiver := services.NewParticipationArchiver(f.db, noCommands())

	require.NoError(t, archiver.ArchiveClassesOfMember(uuid.New(), f.learner, afterCourse))
	assert.Empty(t, archivesOf(t, f.db))
}

func TestParticipationArchive_KeptFiveYearsUnlessTheOrganizationSaysOtherwise(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		years         any
		expectedYears int
	}{
		{"default", nil, 5},
		{"organization's figure", 10, 10},
		{"a figure that would delete evidence at once", 0, 5},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("OCF_DEFAULT_REPORT_ARCHIVE_YEARS", "")
			fakeCasdoor(t)
	f := newReportFixture(t)
			f.setCourseDays(t, archiveMonday)
			require.NoError(t, f.db.Model(&orgModels.Organization{}).Where("id = ?", f.orgID).Update("report_archive_years", testCase.years).Error)
			require.NoError(t, services.NewParticipationArchiver(f.db, noCommands()).ArchiveDueClasses(afterCourse))

			archives := archivesOf(t, f.db)
			require.Len(t, archives, 1)
			expected := time.Date(2026+testCase.expectedYears, 3, 3, 0, 0, 0, 0, paris)
			assert.True(t, expected.Equal(archives[0].ExpiresAt), "kept until %s, got %s", expected, archives[0].ExpiresAt)
		})
	}
}

func archiveRouter(db *gorm.DB, readerID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", readerID) })
	controller := scenarioController.NewParticipationArchiveController(db)
	router.GET("/organizations/:id/participation-archives", controller.List)
	router.GET("/organizations/:id/participation-archives/:archiveId", controller.Get)
	return router
}

func getArchive(router *gin.Engine, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestParticipationArchiveRoutes_ListHidesReportsAndEachReadIsLogged(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	require.NoError(t, f.db.AutoMigrate(&auditModels.AuditLog{}))
	f.db.Exec("DELETE FROM audit_logs")
	f.setCourseDays(t, archiveMonday)
	require.NoError(t, services.NewParticipationArchiver(f.db, noCommands()).ArchiveDueClasses(afterCourse))
	archive := archivesOf(t, f.db)[0]
	readerID := uuid.New()
	router := archiveRouter(f.db, readerID.String())
	organizationPath := "/organizations/" + f.orgID.String() + "/participation-archives"

	listed := getArchive(router, organizationPath)
	require.Equal(t, http.StatusOK, listed.Code)
	assert.Contains(t, listed.Body.String(), archive.ID.String())
	assert.NotContains(t, listed.Body.String(), f.learner)

	read := getArchive(router, organizationPath+"/"+archive.ID.String())
	require.Equal(t, http.StatusOK, read.Code)
	var body struct {
		Report services.ParticipationReport `json:"report"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &body))
	require.Len(t, body.Report.Learners, 1)
	assert.Equal(t, f.learner, body.Report.Learners[0].UserID)

	var logged []auditModels.AuditLog
	require.NoError(t, f.db.Where("event_type = ?", auditModels.AuditEventParticipationArchiveRead).Find(&logged).Error)
	require.Len(t, logged, 1, "reading is logged, listing is not")
	assert.Equal(t, readerID, *logged[0].ActorID)
	assert.Equal(t, archive.ID, *logged[0].TargetID)
}

func TestParticipationArchiveRoutes_AnotherOrganizationsCopyIsNotFound(t *testing.T) {
	fakeCasdoor(t)
	f := newReportFixture(t)
	f.setCourseDays(t, archiveMonday)
	require.NoError(t, services.NewParticipationArchiver(f.db, noCommands()).ArchiveDueClasses(afterCourse))
	archive := archivesOf(t, f.db)[0]

	response := getArchive(archiveRouter(f.db, uuid.NewString()), "/organizations/"+uuid.NewString()+"/participation-archives/"+archive.ID.String())
	assert.Equal(t, http.StatusNotFound, response.Code)
}
