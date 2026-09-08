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

	"soli/formations/src/groups/dto"
	groupModels "soli/formations/src/groups/models"
)

func countGroupMembersWithUserID(env *classArchiveEnv, groupID uuid.UUID, userID string) int64 {
	var count int64
	env.db.Model(&groupModels.GroupMember{}).Where("group_id = ? AND user_id = ?", groupID, userID).Count(&count)
	return count
}

func TestGroupMemberCreate_MissingRequiredKey_IsRefusedWith400(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "promo", "teacher", nil)
	seedClassMember(t, env.db, group.ID, "teacher", groupModels.GroupMemberRoleOwner)
	env.as("teacher")

	cases := []struct {
		name    string
		groupID uuid.UUID
		userID  string
	}{
		{"empty user_id", group.ID, ""},
		{"blank user_id", group.ID, "   "},
		{"nil group_id", uuid.Nil, "student"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.do(http.MethodPost, "/api/v1/group-members", dto.CreateGroupMemberInput{
				GroupID: tc.groupID, UserID: tc.userID, Role: groupModels.GroupMemberRoleMember,
			})

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Zero(t, countGroupMembersWithUserID(env, tc.groupID, tc.userID), "no membership row may be written")
		})
	}
}

// The guard against over-restriction lives at the hook level:
// TestGroupMemberCreateHook_ManagerAssignsMember_Allowed accepts a well-formed
// member. This harness cannot witness a 201: the generic create path enriches
// the response from Casdoor, which has no client in tests.
