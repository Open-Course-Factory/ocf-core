package groups_tests

// #503: GroupMember.IsActive carried gorm:"default:true", so
// db.Create(&GroupMember{IsActive: false}) stored true. Latent in production
// (members are created active) but every test seeding an inactive member had
// to Create then Update("is_active", false).

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
)

func TestGroupMemberCreate_ExplicitInactive_IsStored(t *testing.T) {
	db := newGroupRoleCapDB(t)

	member := &groupModels.GroupMember{
		GroupID:  uuid.New(),
		UserID:   "inactive-on-create-503",
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: false,
	}
	require.NoError(t, db.Omit("Metadata").Create(member).Error)

	var stored groupModels.GroupMember
	require.NoError(t, db.First(&stored, "id = ?", member.ID).Error)
	assert.False(t, stored.IsActive, "a member created inactive must be stored inactive")
}
