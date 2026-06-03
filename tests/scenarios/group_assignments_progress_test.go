package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// TestGetGroupAssignmentsProgress_AggregatesPerScenario verifies the per-assignment
// progress summary used by the group's Scenarios tab. It must return ONE item per
// scenario that has (non-preview) sessions from active group members, with the
// distinct member count (TotalCount), the completed count, and the average grade
// over COMPLETED sessions only (nil when none completed).
//
// Semantics mirror getScenarioResults (SSOT): join group_members ON user_id with
// group_id=? AND is_active=true, filter ss.is_preview=false, group by scenario_id.
func TestGetGroupAssignmentsProgress_AggregatesPerScenario(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()

	// Active members of the group.
	for _, uid := range []string{"student-1", "student-2", "student-3"} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}
	// An INACTIVE member — must be excluded from counts. The false value is forced
	// into the DB independently of any GORM `default:true` tag on the model: on
	// Create, GORM omits a zero-value bool that carries a default and the DB writes
	// true, but a subsequent explicit Update DOES persist the zero value. This keeps
	// the test robust whether the model's default tag is present or absent.
	inactiveMember := groupModels.GroupMember{
		GroupID: groupID, UserID: "inactive-student", Role: "member", JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&inactiveMember).Error)
	require.NoError(t, db.Model(&inactiveMember).Update("is_active", false).Error)
	// Sanity-check the row really landed inactive (guards the seeding itself).
	var inactiveCount int64
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("user_id = ? AND is_active = ?", "inactive-student", false).
		Count(&inactiveCount).Error)
	require.Equal(t, int64(1), inactiveCount, "inactive member must be stored with is_active=false")
	// A member of a DIFFERENT group — must be excluded.
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: uuid.New(), UserID: "outsider", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	// Two scenarios with sessions.
	scenarioA := models.Scenario{Name: "scen-a", Title: "Scenario A", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	scenarioB := models.Scenario{Name: "scen-b", Title: "Scenario B", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenarioA).Error)
	require.NoError(t, db.Create(&scenarioB).Error)
	// A third scenario with NO group sessions — must not appear in the result.
	scenarioC := models.Scenario{Name: "scen-c", Title: "Scenario C", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenarioC).Error)

	now := time.Now()

	// --- Scenario A: 2 of 3 members completed, grades 80 and 100 → avg 90 ---
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "student-1", Status: "completed",
		Grade: floatPtr(80.0), StartedAt: now.Add(-2 * time.Hour), CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "student-2", Status: "completed",
		Grade: floatPtr(100.0), StartedAt: now.Add(-2 * time.Hour), CompletedAt: &now,
	}).Error)
	// student-3 has an active (not completed) session on A.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "student-3", Status: "active",
		StartedAt: now.Add(-time.Hour),
	}).Error)

	// A PREVIEW session on A from an active member — must be excluded entirely.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "student-1", Status: "completed",
		Grade: floatPtr(10.0), IsPreview: true, StartedAt: now, CompletedAt: &now,
	}).Error)
	// A session on A from the INACTIVE member — must be excluded.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "inactive-student", Status: "completed",
		Grade: floatPtr(0.0), StartedAt: now, CompletedAt: &now,
	}).Error)
	// A session on A from a non-member — must be excluded.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioA.ID, UserID: "outsider", Status: "completed",
		Grade: floatPtr(0.0), StartedAt: now, CompletedAt: &now,
	}).Error)

	// --- Scenario B: 1 member with an active session, nobody completed ---
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenarioB.ID, UserID: "student-1", Status: "active",
		StartedAt: now.Add(-time.Hour),
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetGroupAssignmentsProgress(groupID)
	require.NoError(t, err)

	// One item per scenario that has qualifying sessions: A and B only.
	require.Len(t, items, 2)

	byScenario := map[uuid.UUID]services.AssignmentProgressItem{}
	for _, it := range items {
		byScenario[it.ScenarioID] = it
	}

	a, okA := byScenario[scenarioA.ID]
	require.True(t, okA, "scenario A must be present")
	assert.Equal(t, 3, a.TotalCount, "3 distinct active members have a non-preview session on A")
	assert.Equal(t, 2, a.CompletedCount, "2 members completed A")
	require.NotNil(t, a.AvgGrade, "A has completed sessions so AvgGrade is non-nil")
	assert.InDelta(t, 90.0, *a.AvgGrade, 0.01, "avg of completed grades 80 and 100")

	b, okB := byScenario[scenarioB.ID]
	require.True(t, okB, "scenario B must be present")
	assert.Equal(t, 1, b.TotalCount, "1 active member has a session on B")
	assert.Equal(t, 0, b.CompletedCount, "nobody completed B")
	assert.Nil(t, b.AvgGrade, "no completed sessions → AvgGrade nil")

	// Scenario C had no sessions and must not be present.
	_, okC := byScenario[scenarioC.ID]
	assert.False(t, okC, "scenario C has no sessions and must be absent")
}

// TestGetGroupAssignmentsProgress_EmptyGroup pins the empty case: a group with no
// qualifying sessions returns an empty slice, not a nil panic.
func TestGetGroupAssignmentsProgress_EmptyGroup(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "lonely-student", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetGroupAssignmentsProgress(groupID)
	require.NoError(t, err)
	assert.NotNil(t, items, "must be an empty slice, not nil")
	assert.Empty(t, items)
}

// TestGetGroupAssignmentsProgress_DistinctMembers verifies TotalCount counts
// DISTINCT members, not sessions: a member with two non-preview sessions on the
// same scenario counts once.
func TestGetGroupAssignmentsProgress_DistinctMembers(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "repeat-student", Role: "member", JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{Name: "scen-d", Title: "Scenario D", InstanceType: "ubuntu:22.04", CreatedByID: "c1"}
	require.NoError(t, db.Create(&scenario).Error)

	now := time.Now()
	// Two completed sessions from the SAME member.
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "repeat-student", Status: "completed",
		Grade: floatPtr(60.0), StartedAt: now.Add(-3 * time.Hour), CompletedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "repeat-student", Status: "completed",
		Grade: floatPtr(90.0), StartedAt: now.Add(-time.Hour), CompletedAt: &now,
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	items, err := svc.GetGroupAssignmentsProgress(groupID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].TotalCount, "same member counted once")
}
