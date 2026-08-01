package groups_tests

// #460: OrganizationMember and GroupMember each carried their own copy of the
// role hierarchy, ending in `default: 0`. A role registered in auth/access
// therefore ranked BELOW member in those two — silently, because nothing compared
// the tables.
//
// These tests assert the delegation rather than the numbers: they read the
// hierarchy as the expectation, so they cannot drift from it the way the copies
// did.

import (
	"testing"

	access "soli/formations/src/auth/access"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"

	"github.com/stretchr/testify/assert"
)

func TestOrganizationMemberRolePriority_DelegatesToTheHierarchy(t *testing.T) {
	for role, want := range access.GetRoleHierarchy() {
		member := &organizationModels.OrganizationMember{
			Role: organizationModels.OrganizationMemberRole(role),
		}
		assert.Equalf(t, want, member.GetRolePriority(),
			"OrganizationMember must rank %q through the single hierarchy, not a private copy", role)
	}
}

func TestGroupMemberRolePriority_DelegatesToTheHierarchy(t *testing.T) {
	for role, want := range access.GetRoleHierarchy() {
		member := &groupModels.GroupMember{
			Role: groupModels.GroupMemberRole(role),
		}
		assert.Equalf(t, want, member.GetRolePriority(),
			"GroupMember must rank %q through the single hierarchy, not a private copy", role)
	}
}

// The regression in one line: before the consolidation this returned 0, ranking a
// teacher below a student.
func TestOrganizationMemberRolePriority_KnowsTeacher(t *testing.T) {
	teacher := &organizationModels.OrganizationMember{Role: organizationModels.OrganizationMemberRole(access.RoleTeacher)}
	student := &organizationModels.OrganizationMember{Role: organizationModels.OrgRoleMember}

	assert.True(t, teacher.GetRolePriority() > student.GetRolePriority(),
		"a teacher must outrank a student — a private priority table returned 0 for the unknown role")
	assert.True(t, teacher.HasHigherRoleThan(organizationModels.OrgRoleMember))
}

func TestCanManageGroups_AdmitsTeachersButNotStudents(t *testing.T) {
	cases := []struct {
		role  organizationModels.OrganizationMemberRole
		admit bool
	}{
		{organizationModels.OrgRoleMember, false},
		{organizationModels.OrganizationMemberRole(access.RoleTeacher), true},
		{organizationModels.OrgRoleManager, true},
		{organizationModels.OrgRoleOwner, true},
	}

	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			member := &organizationModels.OrganizationMember{Role: tc.role}
			assert.Equal(t, tc.admit, member.CanManageGroups())
		})
	}
}

// Running a class and administering the organization are different jobs: a teacher
// gets the first and not the second.
func TestTeacher_CanManageGroupsWithoutOrganizationAdministration(t *testing.T) {
	teacher := &organizationModels.OrganizationMember{
		Role: organizationModels.OrganizationMemberRole(access.RoleTeacher),
	}

	assert.True(t, teacher.CanManageGroups(), "a teacher runs classes")
	assert.False(t, teacher.IsManager(), "a teacher does not administer the organization")
	assert.False(t, teacher.CanManageOrganization(), "a teacher does not touch members or billing")
}
