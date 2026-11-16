// tests/scenarios/bulkStartSessionUser_test.go
//
// A class bulk-start composes each learner's container the way the learner's
// own launch does (scenarioLaunchController): the scenario's console uid and
// the internet cut that reaches the learner through the scenario's assignments.
package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

func TestBulkStartScenario_SendsTheScenariosSessionUser(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "bulkuid", map[string][]string{
		"member": {"bulkuid-learner"},
	})
	require.NoError(t, sharedTestDB.Model(&models.Scenario{}).Where("id = ?", scenario.ID).Update("session_user", 1000).Error)

	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{Distribution: "Debian", Size: "xs"}, 0, "trainer")
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Len(t, ttMock.capturedInputs, 1)

	got := ttMock.capturedInputs[0].SessionUser
	require.NotNil(t, got, "bulk-start must send the scenario's session user, as the solo launch does")
	assert.Equal(t, 1000, *got)
}

// The cut stands in another class of another organization that also assigns
// the scenario: the composer's own check (the run's organization) cannot see
// it, only ScenarioNetworkCut does — as on the solo launch.
func TestBulkStartScenario_AppliesTheScenarioNetworkCut(t *testing.T) {
	const learner = "bulkcut-learner"
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "bulkcut", map[string][]string{
		"member": {learner},
	})
	db := sharedTestDB
	otherOrg := createTestOrg(t, db, "other-org-owner")
	otherClass := createTestGroupInOrg(t, db, otherOrg, "other-org-owner")
	addGroupMember(t, db, otherClass, learner, groupModels.GroupMemberRoleMember)
	createScenarioAssignment(t, db, scenario.ID, &otherClass, &otherOrg, "group")
	require.NoError(t, terminalModels.UpsertNetworkCut(db, learner, otherClass, &otherOrg, "other-trainer"))

	cut, err := terminalModels.ScenarioNetworkCut(db, learner, scenario.ID)
	require.NoError(t, err)
	require.True(t, cut, "precondition: the solo launch would cut this learner's internet")

	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{Distribution: "Debian", Size: "xs"}, 0, "trainer")
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Len(t, ttMock.capturedInputs, 1)
	assert.True(t, ttMock.capturedInputs[0].NetworkDisabled,
		"bulk-start must start the learner without internet when a cut reaches them through the scenario")
}
