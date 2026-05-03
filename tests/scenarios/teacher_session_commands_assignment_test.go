package scenarios_test

// Red-phase tests for the IDOR fix on GetSessionCommands.
//
// The current GetSessionCommands implementation only verifies that the session's
// student belongs to the requested group. It does NOT verify that the scenario
// itself is assigned to the group. That allows a manager of group A to read the
// commands for a session whose student happens to also be in group A but is
// running a scenario that was only assigned to group B.
//
// The fix introduces a sentinel error `services.ErrScenarioNotAssignedToGroup`
// (matching the assignment check already enforced by GetSessionDetail) so the
// controller can map it to 404 without leaking session existence.
//
// These tests assert the NEW behavior and therefore currently fail. When the
// implementation lands:
//   - The "happy path" test (scenario assigned to group) must keep passing.
//   - The two "no assignment" tests must produce the new sentinel error.
//
// NOTE on `ErrScenarioNotAssignedToGroup`: this sentinel does not yet exist in
// services. Until it is added, the file-level helper `expectAssignmentSentinel`
// asserts on the error message string `"scenario is not assigned to this group"`
// (matching the wording already used by GetSessionDetail). After implementation,
// the helper should be flipped to `errors.Is(err, services.ErrScenarioNotAssignedToGroup)`.

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

// expectAssignmentSentinel asserts that err matches the
// "scenario not assigned to group" condition. After implementation lands and
// services.ErrScenarioNotAssignedToGroup exists, replace the body with:
//
//	assert.ErrorIs(t, err, services.ErrScenarioNotAssignedToGroup)
//
// TODO(impl): switch to ErrorIs once services.ErrScenarioNotAssignedToGroup
// is exported.
func expectAssignmentSentinel(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err, "expected an error when scenario is not assigned to group")
	assert.Contains(t, err.Error(), "scenario is not assigned to this group",
		"GetSessionCommands must reject when the scenario has no active assignment for the requested group, mirroring GetSessionDetail")
}

func TestGetSessionCommands_ScenarioAssignedToGroup_Allowed(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	studentID := "student-assigned-1"
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-assigned", Title: "Commands Assigned", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Scenario IS assigned to the group — the new check must pass.
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	terminalUUID := "tt-session-uuid-" + uuid.NewString()
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalUUID,
	}
	require.NoError(t, db.Create(&session).Error)

	body := []byte(`{"commands":[{"sequence_num":1,"command_text":"ls","executed_at":1700000000}],"total":1,"limit":50,"offset":0}`)
	tt := newCommandsCapturingTTService(body)
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, tt, sessionSvc)

	gotBody, contentType, err := dashSvc.GetSessionCommands(groupID, session.ID, 0, 0)
	require.NoError(t, err, "with a matching ScenarioAssignment, the request must be authorised")
	assert.Equal(t, terminalUUID, tt.lastSessionUUID)
	assert.Equal(t, "application/json", contentType)
	assert.NotEmpty(t, gotBody)
}

func TestGetSessionCommands_ScenarioNotAssignedToGroup_404(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	studentID := "student-no-assignment"

	// Student is in the group, has a session with a terminal — but the scenario
	// has NO assignment to this group.
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-unassigned", Title: "Commands Unassigned", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	// NOTE: no ScenarioAssignment created.

	terminalUUID := "tt-session-unassigned"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalUUID,
	}
	require.NoError(t, db.Create(&session).Error)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	body, _, err := dashSvc.GetSessionCommands(groupID, session.ID, 0, 0)
	expectAssignmentSentinel(t, err)
	assert.Nil(t, body, "no commands payload must be returned when assignment check fails")
	assert.Empty(t, tt.lastSessionUUID,
		"tt-backend must NOT be called when the scenario is not assigned to the group — prevents IDOR leak")
}

func TestGetSessionCommands_ScenarioAssignedToOtherGroup_404(t *testing.T) {
	db := setupTestDB(t)

	managedGroupID := uuid.New() // The trainer manages this group.
	otherGroupID := uuid.New()   // The scenario is assigned to a different group.
	studentID := "student-overlapping-membership"

	// The student happens to be a member of BOTH groups (overlapping enrolment),
	// so the existing membership check passes — but the scenario assignment is
	// only for the other group, so the new check must still reject.
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: managedGroupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: otherGroupID, UserID: studentID, Role: groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "cmd-assigned-elsewhere", Title: "Commands Assigned Elsewhere",
		InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Scenario is assigned to the OTHER group, not the managed group.
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &otherGroupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)

	terminalUUID := "tt-session-other-assignment"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: studentID, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalUUID,
	}
	require.NoError(t, db.Create(&session).Error)

	tt := newCommandsCapturingTTService(nil)
	dashSvc := services.NewTeacherDashboardService(db, tt, nil)

	body, _, err := dashSvc.GetSessionCommands(managedGroupID, session.ID, 0, 0)
	expectAssignmentSentinel(t, err)
	assert.Nil(t, body)
	assert.Empty(t, tt.lastSessionUUID,
		"tt-backend must NOT be called when the scenario is assigned to a different group — prevents cross-group IDOR")
}
