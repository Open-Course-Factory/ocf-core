package access

// Tests for the contextual role hierarchy (#460).
//
// The point of these is not that member < manager < owner — that never broke.
// It is that the hierarchy has exactly ONE definition. Two hardcoded copies used
// to shadow it on OrganizationMember and GroupMember, each ending in `default: 0`,
// so a role added here ranked below `member` there. Those copies now delegate,
// and the model-level tests assert the delegation.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoleHierarchy_TeacherSitsBetweenMemberAndManager(t *testing.T) {
	assert.True(t, RolePriority(RoleTeacher) > RolePriority(RoleMember),
		"a teacher must outrank a student")
	assert.True(t, RolePriority(RoleTeacher) < RolePriority(RoleManager),
		"a teacher must not gain organization administration")
}

func TestRoleHierarchy_ClassroomThresholdAdmitsTeachersAndAbove(t *testing.T) {
	cases := []struct {
		role  string
		admit bool
	}{
		{RoleMember, false},
		{RoleTeacher, true},
		{RoleManager, true},
		{RoleOwner, true},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			assert.Equal(t, tc.admit, IsRoleAtLeast(tc.role, RoleMinimumForClassrooms),
				"the classroom threshold must admit %s: %v", tc.role, tc.admit)
		})
	}
}

// An unknown role must rank below every real one rather than sorting anywhere
// surprising — IsRoleAtLeast refuses it outright, which is the fail-closed reading.
func TestRoleHierarchy_UnknownRoleIsRefused(t *testing.T) {
	assert.False(t, IsRoleAtLeast("wizard", RoleMember))
	assert.False(t, IsKnownRole("wizard"))
	assert.Equal(t, 0, RolePriority("wizard"))
}

func TestRoleHierarchy_IsKnownRoleCoversEveryRegisteredRole(t *testing.T) {
	for role := range GetRoleHierarchy() {
		assert.True(t, IsKnownRole(role),
			"%s is in the hierarchy, so validation must accept it — a role the "+
				"hierarchy knows but validators reject is unusable", role)
	}
}

// The message lists roles least-privileged first, so a user reading it sees the
// ladder rather than map iteration order.
func TestRoleHierarchy_MessageIsOrderedByPrivilege(t *testing.T) {
	assert.Equal(t, "member, teacher, manager, owner", KnownRolesForMessage())
}

// RegisterRole is the documented extension seam; a role added through it must be
// visible to every consumer, not just to IsRoleAtLeast.
func TestRoleHierarchy_RegisteredRoleIsVisibleEverywhere(t *testing.T) {
	const custom = "test-supervisor"
	RegisterRole(custom, 75)
	t.Cleanup(func() { delete(roleHierarchy, custom) })

	assert.True(t, IsKnownRole(custom))
	assert.Equal(t, 75, RolePriority(custom))
	assert.True(t, IsRoleAtLeast(custom, RoleManager))
	assert.False(t, IsRoleAtLeast(custom, RoleOwner))
}
