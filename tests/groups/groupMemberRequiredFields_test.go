package groups_tests

// #474: POST /group-members with an empty user_id answered 201 and wrote a
// membership row with user_id="". Binding tags are inert on the generic entity
// path (#390), so the BeforeCreate hook is the only place that can refuse it.
// The same goes for the group id, the other required key of the row.

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"gorm.io/gorm"

	"soli/formations/src/groups/dto"
	groupModels "soli/formations/src/groups/models"
)

// setupGroupMemberCreateEnv is the class-archive HTTP harness with the jsonb
// Metadata column omitted on insert: the generic create path writes the full
// model, and the sqlite driver cannot bind a map.
func setupGroupMemberCreateEnv(t *testing.T) *classArchiveEnv {
	t.Helper()
	env := setupClassArchiveEnv(t)
	omitMetadata := func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "group_members" {
			tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
		}
	}
	env.db.Callback().Create().Before("gorm:create").Register("omit_group_member_metadata_on_create", omitMetadata)
	env.db.Callback().Update().Before("gorm:update").Register("omit_group_member_metadata_on_update", omitMetadata)
	return env
}

func countGroupMembersWithUserID(env *classArchiveEnv, groupID uuid.UUID, userID string) int64 {
	var count int64
	env.db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Count(&count)
	return count
}

func TestGroupMemberCreate_EmptyUserID_IsRefusedWith400(t *testing.T) {
	env := setupGroupMemberCreateEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	env.as("teacher")

	rec := env.do(http.MethodPost, "/api/v1/group-members", dto.CreateGroupMemberInput{
		GroupID: group.ID, UserID: "", Role: groupModels.GroupMemberRoleMember,
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Zero(t, countGroupMembersWithUserID(env, group.ID, ""), "no membership row may carry an empty user_id")
}

func TestGroupMemberCreate_BlankUserID_IsRefusedWith400(t *testing.T) {
	env := setupGroupMemberCreateEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	env.as("teacher")

	rec := env.do(http.MethodPost, "/api/v1/group-members", dto.CreateGroupMemberInput{
		GroupID: group.ID, UserID: "   ", Role: groupModels.GroupMemberRoleMember,
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Zero(t, countGroupMembersWithUserID(env, group.ID, "   "))
}

func TestGroupMemberCreate_NilGroupID_IsRefusedWith400(t *testing.T) {
	env := setupGroupMemberCreateEnv(t)
	env.as("teacher")

	rec := env.do(http.MethodPost, "/api/v1/group-members", dto.CreateGroupMemberInput{
		GroupID: uuid.Nil, UserID: "student", Role: groupModels.GroupMemberRoleMember,
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Zero(t, countGroupMembersWithUserID(env, uuid.Nil, "student"))
}

// The guard against over-restriction lives at the hook level:
// TestGroupMemberCreateHook_ManagerAssignsMember_Allowed accepts a well-formed
// member. This harness cannot witness a 201: the generic create path enriches
// the response from Casdoor, which has no client in tests.
