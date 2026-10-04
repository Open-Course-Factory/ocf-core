package groups_tests

// /users/me?includes=group_memberships preloaded a "ClassGroup" relation that
// GroupMember does not have. The query failed, the error was swallowed, and the
// include came back empty: the front then believed no one managed any class,
// and a class manager could not reach the scenario editor.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	usersRoutes "soli/formations/src/auth/routes/usersRoutes"
	groupModels "soli/formations/src/groups/models"
)

func TestActiveGroupMemberships_ReturnsTheClassWithItsRole(t *testing.T) {
	db := freshTestDB(t)
	group := &groupModels.ClassGroup{Name: "test-class", DisplayName: "Test Class", OwnerUserID: "owner-1", MaxMembers: 30}
	group.ID = uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: group.ID, UserID: "manager-1", Role: groupModels.GroupMemberRoleManager, JoinedAt: time.Now(), IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: group.ID, UserID: "manager-1", Role: groupModels.GroupMemberRoleMember, JoinedAt: time.Now(), IsActive: false,
	}).Error)

	memberships, err := usersRoutes.ActiveGroupMemberships(db, "manager-1")

	require.NoError(t, err)
	require.Len(t, memberships, 1, "only the active membership")
	assert.Equal(t, groupModels.GroupMemberRoleManager, memberships[0].Role)
	assert.Equal(t, "Test Class", memberships[0].Group.DisplayName, "the class is preloaded")
}
