package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// --- seeding helpers -------------------------------------------------------

// createClassGroup seeds an active class-group. Unlike createTestGroupInOrg it
// takes a display name (the overview sorts on it) and an optional org.
func createClassGroup(t *testing.T, db *gorm.DB, name, ownerUserID string, orgID *uuid.UUID) groupModels.ClassGroup {
	t.Helper()
	group := groupModels.ClassGroup{
		Name: name, DisplayName: name, OwnerUserID: ownerUserID,
		OrganizationID: orgID, MaxMembers: 50,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)
	return group
}

// createTerminalSession seeds one terminal. state / org / expiry are the three
// dimensions the live-session count must discriminate on.
func createTerminalSession(t *testing.T, db *gorm.DB, userID string, state terminalModels.TerminalState, orgID *uuid.UUID, expiresAt time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&terminalModels.Terminal{
		SessionID: uuid.NewString(), UserID: userID, State: state,
		OrganizationID: orgID, ExpiresAt: expiresAt,
	}).Error)
}

func summaryByGroupID(items []services.TeacherGroupSummary) map[uuid.UUID]services.TeacherGroupSummary {
	byID := make(map[uuid.UUID]services.TeacherGroupSummary, len(items))
	for _, item := range items {
		byID[item.GroupID] = item
	}
	return byID
}

// --- service tests ---------------------------------------------------------

// TestGetManagedGroupsOverview_OwnedAndManagedGroups_ReturnsOnlyThose pins the
// scope of "MY classes": the groups the caller OWNS (class_groups.owner_user_id)
// plus those where they hold an active manager/owner membership role. A group
// where the caller is a plain member, and a group they have no tie to at all,
// must never appear.
func TestGetManagedGroupsOverview_OwnedAndManagedGroups_ReturnsOnlyThose(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	ownedA := createClassGroup(t, db, "owned-a", teacher, nil)
	ownedB := createClassGroup(t, db, "owned-b", teacher, nil)
	managed := createClassGroup(t, db, "managed-c", "other-teacher", nil)
	addGroupMember(t, db, managed.ID, teacher, groupModels.GroupMemberRoleManager)

	// The caller is only a plain member here — not "their" class.
	memberOnly := createClassGroup(t, db, "member-only-d", "other-teacher", nil)
	addGroupMember(t, db, memberOnly.ID, teacher, groupModels.GroupMemberRoleMember)
	// And a group they have nothing to do with.
	createClassGroup(t, db, "stranger-e", "other-teacher", nil)

	// Member counts differ per group so a cross-group leak would be visible.
	addGroupMember(t, db, ownedA.ID, "student-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, ownedA.ID, "student-2", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, ownedB.ID, "student-3", groupModels.GroupMemberRoleMember)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 3, "2 owned + 1 managed, never the member-only or stranger group")

	byID := summaryByGroupID(items)
	require.Contains(t, byID, ownedA.ID)
	require.Contains(t, byID, ownedB.ID)
	require.Contains(t, byID, managed.ID)
	assert.NotContains(t, byID, memberOnly.ID, "plain membership is not management")

	assert.Equal(t, "owner", byID[ownedA.ID].CallerRole)
	assert.Equal(t, "manager", byID[managed.ID].CallerRole)

	assert.Equal(t, 2, byID[ownedA.ID].MemberCount, "student-1 and student-2")
	assert.Equal(t, 1, byID[ownedB.ID].MemberCount, "student-3")
	assert.Equal(t, 1, byID[managed.ID].MemberCount, "the caller's own manager membership counts as a member row")

	// #480: member_count keeps its all-memberships meaning (capacity), while
	// learner_count answers "how many apprenants". They differ exactly where a
	// staff membership sits on the roster.
	assert.Equal(t, 2, byID[ownedA.ID].LearnerCount)
	assert.Equal(t, 1, byID[ownedB.ID].LearnerCount)
	assert.Equal(t, 0, byID[managed.ID].LearnerCount, "a manager membership is staff, not an apprenant")
}

// TestGetManagedGroupsOverview_StaffMemberships_ExcludedFromLearnerCount is the
// #480 decision in one row: teaching staff hold class memberships (the creator
// is auto-enrolled with the owner role), so "N apprenants" must count role
// `member` only, while member_count keeps counting the whole roster because the
// capacity surfaces are built on it.
func TestGetManagedGroupsOverview_StaffMemberships_ExcludedFromLearnerCount(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	group := createClassGroup(t, db, "mixed-roster", teacher, nil)
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, group.ID, "assistant", groupModels.GroupMemberRoleManager)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-2", groupModels.GroupMemberRoleMember)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, 4, items[0].MemberCount, "the whole roster, staff included — capacity")
	assert.Equal(t, 2, items[0].LearnerCount, "only the two apprenants")
}

// TestGetManagedGroupsOverview_StaffOnlyClass_ReportsZeroLearners pins the
// zero-learner class (#480): a class where only the teacher and an assistant are
// enrolled has no apprenant, so every learner-facing figure reads 0 and the
// completion rate divides by a learner count of zero without blowing up.
func TestGetManagedGroupsOverview_StaffOnlyClass_ReportsZeroLearners(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	orgID := uuid.New()
	group := createClassGroup(t, db, "staff-only", teacher, &orgID)
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, group.ID, "assistant", groupModels.GroupMemberRoleManager)

	// Both staff members are running supervisable terminals.
	future := time.Now().Add(time.Hour)
	createTerminalSession(t, db, teacher, terminalModels.StateRunning, &orgID, future)
	createTerminalSession(t, db, "assistant", terminalModels.StateRunning, &orgID, future)

	// And the teacher completed the assigned scenario while preparing it.
	scenario := createTestScenarioNoOrg(t, db, "prepared-by-teacher")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")
	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: teacher, Status: "completed",
		Grade: floatPtr(100.0), StartedAt: now.Add(-time.Hour), CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)

	row := items[0]
	assert.Equal(t, 2, row.MemberCount)
	assert.Equal(t, 0, row.LearnerCount)
	assert.Equal(t, 0, row.LiveSessionCount, "staff terminals are not learners connecting")
	assert.Equal(t, 0, row.IdleMemberCount)

	require.Len(t, row.Assignments, 1)
	assert.Equal(t, 0, row.Assignments[0].StartedCount, "the teacher's own run is not class progress")
	assert.Equal(t, 0, row.Assignments[0].CompletedCount)
	assert.Zero(t, row.Assignments[0].ClassCompletionRate, "no apprenant means no rate, not a division by zero")
}

// TestGetManagedGroupsOverview_DeletedSessions_AreNotStarted verifies that a
// scenario session which has been deleted stops counting as started.
//
// assignmentsProgressByGroup is raw SQL, so GORM's soft-delete scope never
// reaches it, and the query named no deleted_at of its own. After clearing a bad
// class launch the card still read "3 started" while every learner's row in the
// progression table read not_started — the same question answered two ways,
// which is exactly the drift the raw-SQL comment warns about.
func TestGetManagedGroupsOverview_DeletedSessions_AreNotStarted(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "deleted-sessions-teacher"

	orgID := uuid.New()
	group := createClassGroup(t, db, "deleted-sessions", teacher, &orgID)
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, group.ID, "deleted-sessions-learner", groupModels.GroupMemberRoleMember)

	scenario := createTestScenarioNoOrg(t, db, "deleted-sessions-scenario")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	session := &models.ScenarioSession{
		ScenarioID: scenario.ID,
		UserID:     "deleted-sessions-learner",
		Status:     "abandoned",
		StartedAt:  time.Now().Add(-time.Hour),
	}
	require.NoError(t, db.Create(session).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	before, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Len(t, before[0].Assignments, 1)
	require.Equal(t, 1, before[0].Assignments[0].StartedCount,
		"a live session counts as started")

	require.NoError(t, db.Delete(session).Error)

	after, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Len(t, after[0].Assignments, 1)
	assert.Equal(t, 0, after[0].Assignments[0].StartedCount,
		"a deleted session must stop counting, or the class card contradicts the "+
			"progression table it sits above")
}

// TestGetManagedGroupsOverview_PlainStudent_ReturnsEmptyList verifies a learner
// who is a member of real classes gets an EMPTY list — not an error, and above
// all not the classes they merely attend.
func TestGetManagedGroupsOverview_PlainStudent_ReturnsEmptyList(t *testing.T) {
	db := setupTestDB(t)

	group := createClassGroup(t, db, "someone-elses-class", "teacher-1", nil)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview("student-1")
	require.NoError(t, err)
	assert.NotNil(t, items, "must be an empty slice, not nil")
	assert.Empty(t, items)
}

// TestGetManagedGroupsOverview_CallerWithNoGroups_ReturnsEmptySlice pins the
// zero-input case: a caller tied to no group at all gets a valid empty slice
// (which marshals to []), never nil and never a panic.
func TestGetManagedGroupsOverview_CallerWithNoGroups_ReturnsEmptySlice(t *testing.T) {
	db := setupTestDB(t)
	createClassGroup(t, db, "unrelated", "teacher-1", nil)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview("nobody")
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)

	// An empty caller id (no userId on the context) must never degenerate into
	// "every group whose owner_user_id is blank".
	items, err = svc.GetManagedGroupsOverview("")
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}

// TestGetManagedGroupsOverview_InactiveAndExpiredGroups_AreListedAndFlagged
// documents the chosen policy: archived and expired classes stay in the list,
// FLAGGED, rather than silently vanishing — a teacher must be able to see (and
// reopen) a class they closed. Active classes sort first.
func TestGetManagedGroupsOverview_InactiveAndExpiredGroups_AreListedAndFlagged(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	live := createClassGroup(t, db, "a-live", teacher, nil)

	archived := createClassGroup(t, db, "b-archived", teacher, nil)
	require.NoError(t, db.Model(&groupModels.ClassGroup{}).Where("id = ?", archived.ID).
		Update("archived_at", time.Now()).Error)

	past := time.Now().Add(-24 * time.Hour)
	expired := createClassGroup(t, db, "c-expired", teacher, nil)
	require.NoError(t, db.Model(&groupModels.ClassGroup{}).Where("id = ?", expired.ID).
		Update("expires_at", past).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 3)

	byID := summaryByGroupID(items)
	assert.Nil(t, byID[live.ID].ArchivedAt)
	assert.True(t, byID[live.ID].IsActive)
	assert.False(t, byID[live.ID].IsExpired)

	assert.NotNil(t, byID[archived.ID].ArchivedAt, "archived class listed with archived_at set")
	assert.False(t, byID[archived.ID].IsActive, "the transitional is_active follows archived_at")
	assert.False(t, byID[archived.ID].IsExpired)

	assert.Nil(t, byID[expired.ID].ArchivedAt, "expiry is archive PENDING, not archived: the cron stamps it")
	assert.True(t, byID[expired.ID].IsExpired)
	require.NotNil(t, byID[expired.ID].ExpiresAt)

	// Open groups sort before archived ones.
	assert.NotNil(t, items[len(items)-1].ArchivedAt, "the archived class sorts last")
}

// TestGetManagedGroupsOverview_GroupWithNoActivity_ReturnsZeroedRow pins the
// all-zero row: a brand-new class with no members, no sessions and no
// assignments yields zeros and an empty (non-nil) assignment list.
func TestGetManagedGroupsOverview_GroupWithNoActivity_ReturnsZeroedRow(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	group := createClassGroup(t, db, "brand-new", teacher, nil)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)

	row := items[0]
	assert.Equal(t, group.ID, row.GroupID)
	assert.Equal(t, 0, row.MemberCount)
	assert.Equal(t, 0, row.LearnerCount)
	assert.Equal(t, 0, row.LiveSessionCount)
	assert.NotNil(t, row.Assignments, "must be an empty slice, not nil")
	assert.Empty(t, row.Assignments)
}

// TestGetManagedGroupsOverview_LiveSessionCount_CountsOnlyInOrgRunningSessions
// verifies the live count reflects ONLY the group's own APPRENANTS' sessions
// that are running right now AND were launched in the group's organization —
// the same org-context visibility rule the supervision wall applies.
//
// #480: the count reads "X/N connectés" next to the learner count, so a teacher
// or assistant who opens a terminal must not push the numerator past N.
func TestGetManagedGroupsOverview_LiveSessionCount_CountsOnlyInOrgRunningSessions(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	orgID := uuid.New()
	otherOrgID := uuid.New()
	future := time.Now().Add(time.Hour)

	group := createClassGroup(t, db, "org-class", teacher, &orgID)
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "student-2", groupModels.GroupMemberRoleMember)

	// The one session that must be counted.
	createTerminalSession(t, db, "student-1", terminalModels.StateRunning, &orgID, future)
	// The teacher's own terminal is live and in-org, but they are not an apprenant.
	createTerminalSession(t, db, teacher, terminalModels.StateRunning, &orgID, future)
	// Same member, but launched OUTSIDE the group's org — invisible to the teacher.
	createTerminalSession(t, db, "student-1", terminalModels.StateRunning, &otherOrgID, future)
	// A personal (NULL-org) session is never supervisable.
	createTerminalSession(t, db, "student-2", terminalModels.StateRunning, nil, future)
	// Stopped and past-expiry rows are not live.
	createTerminalSession(t, db, "student-2", terminalModels.StateStopped, &orgID, future)
	createTerminalSession(t, db, "student-2", terminalModels.StateRunning, &orgID, time.Now().Add(-time.Hour))
	// A non-member's in-org session belongs to another class.
	createTerminalSession(t, db, "outsider", terminalModels.StateRunning, &orgID, future)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].LiveSessionCount)
	assert.Equal(t, 2, items[0].LearnerCount, "the owner membership is staff")
	assert.Equal(t, 3, items[0].MemberCount)
}

// TestGetManagedGroupsOverview_OrglessGroup_CountsNoLiveSessions pins the safe
// default inherited from SupervisableByGroupOrgScope: a class with no
// organization supervises nothing, so its live count stays 0 even when its
// members are running sessions.
func TestGetManagedGroupsOverview_OrglessGroup_CountsNoLiveSessions(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	group := createClassGroup(t, db, "personal-class", teacher, nil)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)
	createTerminalSession(t, db, "student-1", terminalModels.StateRunning, nil, time.Now().Add(time.Hour))

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 0, items[0].LiveSessionCount)
}

// TestGetManagedGroupsOverview_Assignments_CarryProgressAggregates verifies each
// active assignment on the row carries its progress: how many distinct members
// started it, how many completed, the completion rate over the class size, and
// the average grade of completed sessions.
func TestGetManagedGroupsOverview_Assignments_CarryProgressAggregates(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	group := createClassGroup(t, db, "progress-class", teacher, nil)
	for _, uid := range []string{"student-1", "student-2", "student-3"} {
		addGroupMember(t, db, group.ID, uid, groupModels.GroupMemberRoleMember)
	}
	// #480: staff on the roster must not move the class figures in either
	// direction — neither the counts nor the rate's denominator.
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)

	scenario := createTestScenarioNoOrg(t, db, "linux-basics")
	assignment := createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	// A scenario assigned but never started must still appear, all-zero.
	untouched := createTestScenarioNoOrg(t, db, "networking")
	createScenarioAssignment(t, db, untouched.ID, &group.ID, nil, "group")

	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "completed",
		Grade: floatPtr(80.0), StartedAt: now.Add(-2 * time.Hour), CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-2", Status: "active",
		StartedAt: now.Add(-time.Hour),
	}).Error)
	// A preview session must not inflate the counts.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-3", Status: "completed",
		Grade: floatPtr(10.0), IsPreview: true, StartedAt: now, CompletedAt: &now,
	}).Error)
	// Neither must the teacher's own non-preview run through the scenario (#480).
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: teacher, Status: "completed",
		Grade: floatPtr(100.0), StartedAt: now.Add(-3 * time.Hour), CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Len(t, items[0].Assignments, 2)

	byScenario := map[uuid.UUID]services.TeacherGroupAssignment{}
	for _, a := range items[0].Assignments {
		byScenario[a.ScenarioID] = a
	}

	started := byScenario[scenario.ID]
	assert.Equal(t, assignment.ID, started.AssignmentID)
	assert.Equal(t, scenario.Title, started.ScenarioTitle)
	assert.Equal(t, 2, started.StartedCount, "student-1 and student-2, preview and teacher excluded")
	assert.Equal(t, 1, started.CompletedCount)
	assert.InDelta(t, 100.0/3.0, started.ClassCompletionRate, 0.001,
		"1 of the 3 APPRENANTS completed (#480: the owner is not in the denominator), as a 0..100 percentage")
	require.NotNil(t, started.AvgGrade)
	assert.InDelta(t, 80.0, *started.AvgGrade, 0.01)

	idle := byScenario[untouched.ID]
	assert.Equal(t, untouched.Title, idle.ScenarioTitle)
	assert.Equal(t, 0, idle.StartedCount)
	assert.Equal(t, 0, idle.CompletedCount)
	assert.Zero(t, idle.ClassCompletionRate)
	assert.Nil(t, idle.AvgGrade, "nobody completed it")
}

// TestGetManagedGroupsOverview_ClassCompletionRate_IsAPercentageNotAFraction
// pins the SCALE. A whole class finishing must read 100, not 1 — the teacher API
// already expresses ScenarioAnalytics.CompletionRate as a 0..100 percentage, and
// a silent "normalisation" to a fraction here would make every consumer wrong by
// 100x on a page nobody thought to re-open.
//
// #480 pins the DENOMINATOR alongside the scale: the rate feeds "3/12 terminé"
// on a learner-facing row, so it divides by the apprenant count. The class here
// carries a manager on top of its single learner precisely so a denominator that
// slipped back to member_count would read 50 instead of 100.
func TestGetManagedGroupsOverview_ClassCompletionRate_IsAPercentageNotAFraction(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	group := createClassGroup(t, db, "everyone-finished", teacher, nil)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "assistant", groupModels.GroupMemberRoleManager)

	scenario := createTestScenarioNoOrg(t, db, "finished-by-all")
	createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")

	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "student-1", Status: "completed",
		Grade: floatPtr(70.0), StartedAt: now.Add(-time.Hour), CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Len(t, items[0].Assignments, 1)

	assert.InDelta(t, 100.0, items[0].Assignments[0].ClassCompletionRate, 0.001,
		"the whole class completed — 100, not 1, and not 50 over a roster of 2")
}

// TestGetManagedGroupsOverview_MultipleGroups_AggregatesStayPerGroup guards the
// batching: all rows come from grouped IN queries, so a member, a live session
// or an assignment belonging to one class must never be counted in another.
func TestGetManagedGroupsOverview_MultipleGroups_AggregatesStayPerGroup(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-1"

	orgID := uuid.New()
	future := time.Now().Add(time.Hour)

	classA := createClassGroup(t, db, "class-a", teacher, &orgID)
	classB := createClassGroup(t, db, "class-b", teacher, &orgID)

	addGroupMember(t, db, classA.ID, "student-1", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, classA.ID, "student-2", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, classB.ID, "student-3", groupModels.GroupMemberRoleMember)

	createTerminalSession(t, db, "student-1", terminalModels.StateRunning, &orgID, future)
	createTerminalSession(t, db, "student-2", terminalModels.StateRunning, &orgID, future)
	createTerminalSession(t, db, "student-3", terminalModels.StateRunning, &orgID, future)

	scenarioA := createTestScenarioNoOrg(t, db, "scen-a")
	scenarioB := createTestScenarioNoOrg(t, db, "scen-b")
	createScenarioAssignment(t, db, scenarioA.ID, &classA.ID, nil, "group")
	createScenarioAssignment(t, db, scenarioB.ID, &classB.ID, nil, "group")

	now := time.Now()
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "student-1", Status: "completed",
		Grade: floatPtr(90.0), StartedAt: now.Add(-time.Hour), CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetManagedGroupsOverview(teacher)
	require.NoError(t, err)
	require.Len(t, items, 2)

	byID := summaryByGroupID(items)
	assert.Equal(t, 2, byID[classA.ID].MemberCount)
	assert.Equal(t, 1, byID[classB.ID].MemberCount)
	assert.Equal(t, 2, byID[classA.ID].LiveSessionCount)
	assert.Equal(t, 1, byID[classB.ID].LiveSessionCount)

	require.Len(t, byID[classA.ID].Assignments, 1)
	require.Len(t, byID[classB.ID].Assignments, 1)
	assert.Equal(t, scenarioA.ID, byID[classA.ID].Assignments[0].ScenarioID)
	assert.Equal(t, 1, byID[classA.ID].Assignments[0].CompletedCount)
	assert.Equal(t, scenarioB.ID, byID[classB.ID].Assignments[0].ScenarioID)
	assert.Equal(t, 0, byID[classB.ID].Assignments[0].StartedCount,
		"class A's completed session must not bleed into class B")
}

// TestGetManagedGroupsOverview_InactiveMembership_GrantsNothingAndCountsNothing
// covers the two sides of is_active on group_members: a deactivated manager no
// longer sees the class, and a deactivated learner is out of the member count.
func TestGetManagedGroupsOverview_InactiveMembership_GrantsNothingAndCountsNothing(t *testing.T) {
	db := setupTestDB(t)

	group := createClassGroup(t, db, "class", "other-teacher", nil)
	addGroupMember(t, db, group.ID, "ex-manager", groupModels.GroupMemberRoleManager)
	addGroupMember(t, db, group.ID, "ex-student", groupModels.GroupMemberRoleMember)
	addGroupMember(t, db, group.ID, "current-student", groupModels.GroupMemberRoleMember)
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("user_id IN ?", []string{"ex-manager", "ex-student"}).
		Update("is_active", false).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)

	items, err := svc.GetManagedGroupsOverview("ex-manager")
	require.NoError(t, err)
	assert.Empty(t, items, "a deactivated manager membership grants no access")

	items, err = svc.GetManagedGroupsOverview("other-teacher")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].MemberCount, "only current-student is active")
	assert.Equal(t, 1, items[0].LearnerCount, "and they are an apprenant")
}

// --- controller tests ------------------------------------------------------

// TestTeacherGroupsEndpoint_ReturnsCallerGroupsAsJSON exercises the HTTP layer:
// the handler must derive the groups from the JWT caller id and answer 200 with
// a JSON array.
func TestTeacherGroupsEndpoint_ReturnsCallerGroupsAsJSON(t *testing.T) {
	db := setupTestDB(t)
	const teacher = "teacher-http"

	group := createClassGroup(t, db, "http-class", teacher, nil)
	addGroupMember(t, db, group.ID, teacher, groupModels.GroupMemberRoleOwner)
	addGroupMember(t, db, group.ID, "student-1", groupModels.GroupMemberRoleMember)

	router := setupRealTeacherRouter(t, db, teacher, []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, group.ID.String(), response[0]["group_id"])
	assert.Equal(t, "owner", response[0]["caller_role"])

	// #480 wire contract: both keys ship, and they differ on a class whose owner
	// is enrolled. learner_count is always sent, 0 included — its absence would
	// read as "not computed", never as "no apprenant".
	assert.EqualValues(t, 2, response[0]["member_count"], "the whole roster")
	learners, present := response[0]["learner_count"]
	require.True(t, present, "learner_count must be sent on every row")
	assert.EqualValues(t, 1, learners, "student-1 only")
}

// TestTeacherGroupsEndpoint_StudentGetsEmptyJSONArray checks the learner path
// end-to-end: 200 with `[]`, never null and never someone else's class.
func TestTeacherGroupsEndpoint_StudentGetsEmptyJSONArray(t *testing.T) {
	db := setupTestDB(t)

	group := createClassGroup(t, db, "not-mine", "teacher-http", nil)
	addGroupMember(t, db, group.ID, "student-http", groupModels.GroupMemberRoleMember)

	router := setupRealTeacherRouter(t, db, "student-http", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, "[]", w.Body.String())
}

// TestTeacherGroupsEndpoint_PlatformAdminSeesOnlyOwnGroups documents the admin
// decision: this route answers "MY classes", so a platform administrator gets
// the groups THEY manage — not every group on the platform. Admins who need a
// global view use the admin surfaces.
func TestTeacherGroupsEndpoint_PlatformAdminSeesOnlyOwnGroups(t *testing.T) {
	db := setupTestDB(t)

	createClassGroup(t, db, "someone-elses", "teacher-http", nil)
	ownedByAdmin := createClassGroup(t, db, "admin-own-class", "platform-admin", nil)

	router := setupRealTeacherRouter(t, db, "platform-admin", []string{"administrator"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/teacher/groups", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1, "the admin's own class only, not the platform's")
	assert.Equal(t, ownedByAdmin.ID.String(), response[0]["group_id"])
}
