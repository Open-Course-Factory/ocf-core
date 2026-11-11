package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// A class started on "Automatic" gets one host picked for it. A learner whose
// own plan does not allow that host is placed by their own policy rather than
// refused: nobody chose that host for them.
func TestBulkStartScenario_AutoPlacedHost_LearnerFallsBackToOwnPlacement(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "autoplaced", map[string][]string{
		"member": {"autoplaced-learner"},
	})

	provisioning := services.ScenarioProvisioning{Backend: "class-host", AutoPlaced: true, Distribution: "Debian", Size: "xs"}
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, provisioning, 0, "trainer")
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Len(t, ttMock.capturedInputs, 1)
	assert.Equal(t, "", ttMock.capturedInputs[0].Backend,
		"the capturing service allows only its default: the learner is placed, not pinned")
}

// A host the teacher chose is kept for every learner; one whose policy refuses
// it is refused, never moved.
func TestBulkStartScenario_ChosenHost_IsKeptForEveryLearner(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "chosenhost", map[string][]string{
		"member": {"chosenhost-learner"},
	})

	provisioning := services.ScenarioProvisioning{Backend: "class-host", Distribution: "Debian", Size: "xs"}
	_, err := dashSvc.BulkStartScenario(groupID, scenario.ID, provisioning, 0, "trainer")
	require.NoError(t, err)
	require.Len(t, ttMock.capturedInputs, 1)
	assert.Equal(t, "class-host", ttMock.capturedInputs[0].Backend)
}

// Resolve says whether it placed the host itself.
func TestResolve_MarksAutoPlacement(t *testing.T) {
	auto, err := newPolicyResolver(t).Resolve(debianScenario(), nil, nil, "")
	require.NoError(t, err)
	assert.True(t, auto.AutoPlaced)

	chosen, err := newPolicyResolver(t).Resolve(debianScenario(), nil, nil, "b")
	require.NoError(t, err)
	assert.False(t, chosen.AutoPlaced)
}

// A bulk start that names no host builds on the one the class's assignment
// names.
func TestClassAssignmentBackend(t *testing.T) {
	db := freshTestDB(t)
	provisioner := services.NewScenarioProvisioningService(db, newCapturingTTService())
	scenario := newLaunchableScenario(t, db, "class-backend")
	group := createClassGroup(t, db, "class-backend-class", "teacher-1", nil)

	got, err := provisioner.ClassAssignmentBackend(group.ID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got, "no assignment: automatic")

	a := createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")
	require.NoError(t, db.Model(a).Update("backend", "class-host").Error)
	got, err = provisioner.ClassAssignmentBackend(group.ID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, "class-host", got)

	require.NoError(t, db.Model(&models.ScenarioAssignment{}).Where("id = ?", a.ID).Update("is_active", false).Error)
	got, err = provisioner.ClassAssignmentBackend(group.ID, scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got, "an inactive assignment names nothing")
}

// A platform scenario started for a class lives in the class's organization:
// that org's trainers supervise it and its servers and plans apply, as when the
// learner launches it from there.
func TestBulkStartScenario_PlatformScenario_LivesInTheClassOrganization(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "platformrun", map[string][]string{
		"member": {"platformrun-learner"},
	})
	classOrg := scenario.OrganizationID // the fixture's class and scenario share it
	require.NotNil(t, classOrg)
	require.NoError(t, sharedTestDB.Model(&models.Scenario{}).Where("id = ?", scenario.ID).Update("organization_id", nil).Error)

	_, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{Distribution: "Debian", Size: "xs"}, 0, "trainer")
	require.NoError(t, err)
	require.Len(t, ttMock.capturedInputs, 1)
	assert.Equal(t, classOrg.String(), ttMock.capturedInputs[0].OrganizationID)
}

func TestClassRunOrganization(t *testing.T) {
	classOrg, scenarioOrg := uuid.New(), uuid.New()
	assert.Equal(t, &classOrg, services.ClassRunOrganization(&classOrg, &models.Scenario{OrganizationID: &scenarioOrg}))
	assert.Equal(t, &scenarioOrg, services.ClassRunOrganization(nil, &models.Scenario{OrganizationID: &scenarioOrg}))
	assert.Nil(t, services.ClassRunOrganization(nil, &models.Scenario{}))
}
