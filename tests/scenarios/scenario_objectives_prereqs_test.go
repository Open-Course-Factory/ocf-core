package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

func TestScenario_CreateWithObjectives_Success(t *testing.T) {
	db := freshTestDB(t)

	objectives := "By the end of this scenario, you will be able to: navigate the Linux filesystem, create and manage files using basic commands, understand file permissions and ownership."

	scenario := models.Scenario{
		Name:         "objectives-test",
		Title:        "Objectives Test Scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Objectives:   objectives,
	}
	require.NoError(t, db.Create(&scenario).Error)

	var saved models.Scenario
	require.NoError(t, db.First(&saved, "id = ?", scenario.ID).Error)
	assert.Equal(t, objectives, saved.Objectives)
}

func TestScenario_CreateWithPrerequisites_Success(t *testing.T) {
	db := freshTestDB(t)

	prerequisites := "Basic familiarity with using a computer. No prior Linux experience required. A web browser with JavaScript enabled."

	scenario := models.Scenario{
		Name:          "prerequisites-test",
		Title:         "Prerequisites Test Scenario",
		InstanceType:  "ubuntu:22.04",
		CreatedByID:   "creator-1",
		Prerequisites: prerequisites,
	}
	require.NoError(t, db.Create(&scenario).Error)

	var saved models.Scenario
	require.NoError(t, db.First(&saved, "id = ?", scenario.ID).Error)
	assert.Equal(t, prerequisites, saved.Prerequisites)
}

func TestScenario_EditObjectives_Success(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "edit-objectives-test",
		Title:        "Edit Objectives Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Objectives:   "Original objectives text.",
	}
	require.NoError(t, db.Create(&scenario).Error)

	updatedObjectives := "Updated objectives: master advanced Linux networking, configure firewalls, manage system services."
	require.NoError(t, db.Model(&scenario).Update("objectives", updatedObjectives).Error)

	var saved models.Scenario
	require.NoError(t, db.First(&saved, "id = ?", scenario.ID).Error)
	assert.Equal(t, updatedObjectives, saved.Objectives)
}
