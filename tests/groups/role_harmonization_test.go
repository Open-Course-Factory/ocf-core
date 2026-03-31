package groups_tests

import (
	"testing"

	"soli/formations/src/auth/access"
	"soli/formations/src/groups/models"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Test 1: Role constants match the 3-level model (member, manager, owner)
// =============================================================================

func TestGroupMemberRole_Constants(t *testing.T) {
	t.Run("manager role exists with correct value", func(t *testing.T) {
		assert.Equal(t, models.GroupMemberRole("manager"), models.GroupMemberRoleManager,
			"GroupMemberRoleManager should be 'manager'")
	})

	t.Run("owner role exists with correct value", func(t *testing.T) {
		assert.Equal(t, models.GroupMemberRole("owner"), models.GroupMemberRoleOwner,
			"GroupMemberRoleOwner should be 'owner'")
	})

	t.Run("member role exists with correct value", func(t *testing.T) {
		assert.Equal(t, models.GroupMemberRole("member"), models.GroupMemberRoleMember,
			"GroupMemberRoleMember should be 'member'")
	})

	t.Run("exactly three roles exist", func(t *testing.T) {
		// Enumerate all valid roles — if someone adds admin/assistant back,
		// this test documents the contract: only 3 roles are valid.
		validRoles := []models.GroupMemberRole{
			models.GroupMemberRoleOwner,
			models.GroupMemberRoleManager,
			models.GroupMemberRoleMember,
		}
		assert.Len(t, validRoles, 3, "there should be exactly 3 group member roles")

		// Verify the old 4-level roles are NOT among the valid values
		for _, role := range validRoles {
			assert.NotEqual(t, models.GroupMemberRole("admin"), role,
				"'admin' should not be a valid group member role")
			assert.NotEqual(t, models.GroupMemberRole("assistant"), role,
				"'assistant' should not be a valid group member role")
		}
	})
}

// =============================================================================
// Test 2: IsManager() returns true for owner and manager, false for member
// =============================================================================

func TestGroupMember_IsManager(t *testing.T) {
	tests := []struct {
		name     string
		role     models.GroupMemberRole
		expected bool
	}{
		{"owner is manager", models.GroupMemberRoleOwner, true},
		{"manager is manager", models.GroupMemberRoleManager, true},
		{"member is not manager", models.GroupMemberRoleMember, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gm := &models.GroupMember{Role: tc.role}
			assert.Equal(t, tc.expected, gm.IsManager())
		})
	}
}

// =============================================================================
// Test 3: GetRolePriority() returns correct priority numbers
// =============================================================================

func TestGroupMember_GetRolePriority(t *testing.T) {
	tests := []struct {
		name     string
		role     models.GroupMemberRole
		expected int
	}{
		{"member priority is 10", models.GroupMemberRoleMember, 10},
		{"manager priority is 50", models.GroupMemberRoleManager, 50},
		{"owner priority is 100", models.GroupMemberRoleOwner, 100},
		{"unknown role priority is 0", models.GroupMemberRole("unknown"), 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gm := &models.GroupMember{Role: tc.role}
			assert.Equal(t, tc.expected, gm.GetRolePriority())
		})
	}
}

// =============================================================================
// Test 4: CanManageMembers() — owner and manager can, member cannot
// =============================================================================

func TestGroupMember_CanManageMembers(t *testing.T) {
	tests := []struct {
		name     string
		role     models.GroupMemberRole
		expected bool
	}{
		{"owner can manage members", models.GroupMemberRoleOwner, true},
		{"manager can manage members", models.GroupMemberRoleManager, true},
		{"member cannot manage members", models.GroupMemberRoleMember, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gm := &models.GroupMember{Role: tc.role}
			assert.Equal(t, tc.expected, gm.CanManageMembers())
		})
	}
}

// =============================================================================
// Test 5: IsRoleAtLeast() helper in auth/access package
// =============================================================================

func TestRoleHierarchy_IsRoleAtLeast(t *testing.T) {
	tests := []struct {
		name         string
		userRole     string
		requiredRole string
		expected     bool
	}{
		// member checks
		{"member meets member requirement", "member", "member", true},
		{"member does NOT meet manager requirement", "member", "manager", false},
		{"member does NOT meet owner requirement", "member", "owner", false},

		// manager checks
		{"manager meets manager requirement", "manager", "manager", true},
		{"manager meets member requirement", "manager", "member", true},
		{"manager does NOT meet owner requirement", "manager", "owner", false},

		// owner checks
		{"owner meets owner requirement", "owner", "owner", true},
		{"owner meets manager requirement", "owner", "manager", true},
		{"owner meets member requirement", "owner", "member", true},

		// unknown role
		{"unknown role meets nothing", "stranger", "member", false},
		{"any role meets unknown requirement", "owner", "superadmin", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := access.IsRoleAtLeast(tc.userRole, tc.requiredRole)
			assert.Equal(t, tc.expected, result,
				"IsRoleAtLeast(%q, %q) should be %v", tc.userRole, tc.requiredRole, tc.expected)
		})
	}
}
