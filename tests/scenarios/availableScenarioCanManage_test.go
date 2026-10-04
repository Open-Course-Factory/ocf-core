package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
)

// can_manage on the launcher card is the verdict GET /scenarios/:id applies:
// the teacher of a class in the scenario's organisation may edit it, the
// learner sitting in that class may not.
func TestGetAvailableScenarios_CanManage_TeacherTrueLearnerFalse(t *testing.T) {
	db := freshTestDB(t)
	const teacher, learner = "can-manage-teacher", "can-manage-learner"
	orgID := createTestOrg(t, db, "can-manage-org-owner")

	// The teacher wrote the lab: authorship grants can_manage under any rule.
	// Managing the class alone does not once teachers author scenarios
	// (fix/scenario-authoring-teacher-role): it lets them run a colleague's
	// lab, not edit it.
	scenario := models.Scenario{Name: "can-manage", Title: "Can Manage", InstanceType: "debian",
		CreatedByID: teacher, OrganizationID: &orgID}
	require.NoError(t, db.Create(&scenario).Error)

	class := groupModels.ClassGroup{Name: "can-manage-class", DisplayName: "Class",
		OwnerUserID: "can-manage-org-owner", OrganizationID: &orgID}
	require.NoError(t, db.Omit("Metadata").Create(&class).Error)
	for user, role := range map[string]groupModels.GroupMemberRole{
		teacher: groupModels.GroupMemberRoleManager,
		learner: groupModels.GroupMemberRoleMember,
	} {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: class.ID, UserID: user, Role: role, IsActive: true, JoinedAt: time.Now(),
		}).Error)
	}
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &class.ID, Scope: "group", IsActive: true, CreatedByID: teacher,
	}).Error)

	canManage := func(userID string) any {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available?organization_id="+orgID.String(), nil)
		setupAvailableRouter(db, userID, []string{"member"}).ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var cards []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
		require.Len(t, cards, 1)
		value, present := cards[0]["can_manage"]
		require.True(t, present, "can_manage must always be present")
		return value
	}

	assert.Equal(t, true, canManage(teacher))
	assert.Equal(t, false, canManage(learner))
}
