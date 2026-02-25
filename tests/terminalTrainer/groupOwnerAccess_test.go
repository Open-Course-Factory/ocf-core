package terminalTrainer_tests

import (
	"testing"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test cases for group owner implicit terminal access ---

// TestGroupOwnerAccess_OwnerGetsWriteAccess verifies that the owner of a group
// gets implicit write access to terminals of group members, even without an
// explicit terminal share.
func TestGroupOwnerAccess_OwnerGetsWriteAccess(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-1"
	studentUserID := "student-1"

	// Create a terminal owned by the student
	userKey, err := createTestUserKey(db, studentUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, studentUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create an active group owned by the trainer
	group := &groupModels.ClassGroup{
		Name:        "devops-class",
		DisplayName: "DevOps Class",
		OwnerUserID: trainerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	// Add student as an active member of the group
	createTestGroupMember(t, db, group.ID, studentUserID, groupModels.GroupMemberRoleMember)

	// Service-level check: trainer (group owner) should have write access
	svc := services.NewTerminalTrainerService(db)
	hasAccess, err := svc.HasTerminalAccess(terminal.ID.String(), trainerUserID, models.AccessLevelWrite)
	assert.NoError(t, err)
	assert.True(t, hasAccess, "group owner should have implicit write access to member's terminal")
}

// TestGroupOwnerAccess_OwnerGetsReadAccess verifies that the owner of a group
// also gets implicit read access to terminals of group members (read <= write).
func TestGroupOwnerAccess_OwnerGetsReadAccess(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-1"
	studentUserID := "student-1"

	// Create a terminal owned by the student
	userKey, err := createTestUserKey(db, studentUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, studentUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create an active group owned by the trainer
	group := &groupModels.ClassGroup{
		Name:        "devops-class",
		DisplayName: "DevOps Class",
		OwnerUserID: trainerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	// Add student as an active member
	createTestGroupMember(t, db, group.ID, studentUserID, groupModels.GroupMemberRoleMember)

	// Service-level check: trainer should also have read access
	svc := services.NewTerminalTrainerService(db)
	hasAccess, err := svc.HasTerminalAccess(terminal.ID.String(), trainerUserID, models.AccessLevelRead)
	assert.NoError(t, err)
	assert.True(t, hasAccess, "group owner should have implicit read access to member's terminal")
}

// TestGroupOwnerAccess_RegularMemberNoAccess verifies that a regular group member
// does NOT get implicit access to another member's terminal (only the owner does).
func TestGroupOwnerAccess_RegularMemberNoAccess(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-1"
	studentUserID := "student-1"
	otherStudentID := "student-2"

	// Create a terminal owned by student-1
	userKey, err := createTestUserKey(db, studentUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, studentUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create an active group owned by the trainer
	group := &groupModels.ClassGroup{
		Name:        "devops-class",
		DisplayName: "DevOps Class",
		OwnerUserID: trainerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	// Add both students as members
	createTestGroupMember(t, db, group.ID, studentUserID, groupModels.GroupMemberRoleMember)
	createTestGroupMember(t, db, group.ID, otherStudentID, groupModels.GroupMemberRoleMember)

	// student-2 should NOT get implicit access to student-1's terminal
	svc := services.NewTerminalTrainerService(db)
	hasAccess, err := svc.HasTerminalAccess(terminal.ID.String(), otherStudentID, models.AccessLevelRead)
	assert.NoError(t, err)
	assert.False(t, hasAccess, "regular group member should NOT have implicit access to another member's terminal")
}

// TestGroupOwnerAccess_InactiveGroup_NoAccess verifies that an inactive group
// does NOT grant implicit access to the group owner.
func TestGroupOwnerAccess_InactiveGroup_NoAccess(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-1"
	studentUserID := "student-1"

	// Create a terminal owned by the student
	userKey, err := createTestUserKey(db, studentUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, studentUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create a group then deactivate it (GORM skips false zero-value for bool with default:true)
	group := &groupModels.ClassGroup{
		Name:        "archived-class",
		DisplayName: "Archived Class",
		OwnerUserID: trainerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)
	// Deactivate after creation to bypass GORM zero-value skip
	err = db.Model(group).Update("is_active", false).Error
	require.NoError(t, err)

	// Add student as an active member of the inactive group
	createTestGroupMember(t, db, group.ID, studentUserID, groupModels.GroupMemberRoleMember)

	// Group owner should NOT have access because the group is inactive
	svc := services.NewTerminalTrainerService(db)
	hasAccess, err := svc.HasTerminalAccess(terminal.ID.String(), trainerUserID, models.AccessLevelWrite)
	assert.NoError(t, err)
	assert.False(t, hasAccess, "inactive group should NOT grant implicit terminal access")
}

// TestGroupOwnerAccess_UserNotInGroup_NoAccess verifies that if the terminal owner
// is NOT a member of the group, the group owner does NOT get access.
func TestGroupOwnerAccess_UserNotInGroup_NoAccess(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-1"
	studentUserID := "student-1"
	otherStudentID := "student-2"

	// Create a terminal owned by student-1 (who is NOT in the group)
	userKey, err := createTestUserKey(db, studentUserID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, studentUserID, "active", userKey.ID)
	require.NoError(t, err)

	// Create an active group owned by the trainer
	group := &groupModels.ClassGroup{
		Name:        "devops-class",
		DisplayName: "DevOps Class",
		OwnerUserID: trainerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	// Only add student-2, NOT student-1 (the terminal owner)
	createTestGroupMember(t, db, group.ID, otherStudentID, groupModels.GroupMemberRoleMember)

	// Group owner should NOT have access because the terminal owner is not in the group
	svc := services.NewTerminalTrainerService(db)
	hasAccess, err := svc.HasTerminalAccess(terminal.ID.String(), trainerUserID, models.AccessLevelWrite)
	assert.NoError(t, err)
	assert.False(t, hasAccess, "group owner should NOT have access when terminal owner is not in the group")
}
