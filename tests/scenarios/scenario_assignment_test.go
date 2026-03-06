package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

func TestScenarioAssignment_Create_Success(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "assign-test",
		Title:        "Assignment Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	groupID := uuid.New()
	deadline := time.Now().Add(7 * 24 * time.Hour)

	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: "admin-1",
		Deadline:    &deadline,
		IsActive:    true,
	}
	require.NoError(t, db.Create(&assignment).Error)

	// Verify persisted
	var found models.ScenarioAssignment
	err := db.Preload("Scenario").First(&found, "id = ?", assignment.ID).Error
	require.NoError(t, err)

	assert.Equal(t, scenario.ID, found.ScenarioID)
	assert.Equal(t, &groupID, found.GroupID)
	assert.Equal(t, "group", found.Scope)
	assert.Equal(t, "admin-1", found.CreatedByID)
	assert.True(t, found.IsActive)
	assert.NotNil(t, found.Deadline)
	assert.Equal(t, "Assignment Test", found.Scenario.Title)
}

func TestScenarioAssignment_Create_OrgScope(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "assign-org-test",
		Title:        "Org Assignment Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	orgID := uuid.New()

	assignment := models.ScenarioAssignment{
		ScenarioID:     scenario.ID,
		OrganizationID: &orgID,
		Scope:          "org",
		CreatedByID:    "admin-1",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&assignment).Error)

	var found models.ScenarioAssignment
	err := db.First(&found, "id = ?", assignment.ID).Error
	require.NoError(t, err)

	assert.Equal(t, "org", found.Scope)
	assert.Equal(t, &orgID, found.OrganizationID)
	assert.Nil(t, found.GroupID)
}

func TestScenarioAssignment_ListByGroup(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "assign-filter-test",
		Title:        "Filter Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	groupA := uuid.New()
	groupB := uuid.New()

	// Create two assignments for groupA, one for groupB
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioAssignment{
			ScenarioID:  scenario.ID,
			GroupID:     &groupA,
			Scope:       "group",
			CreatedByID: "admin-1",
			IsActive:    true,
		}).Error)
	}
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupB,
		Scope:       "group",
		CreatedByID: "admin-1",
		IsActive:    true,
	}).Error)

	var assignments []models.ScenarioAssignment
	err := db.Where("group_id = ?", groupA).Find(&assignments).Error
	require.NoError(t, err)
	assert.Len(t, assignments, 2)
}

func TestScenarioAssignment_Delete(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "assign-delete-test",
		Title:        "Delete Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		Scope:       "group",
		CreatedByID: "admin-1",
		IsActive:    true,
	}
	require.NoError(t, db.Create(&assignment).Error)

	// Delete (soft delete via BaseModel)
	err := db.Delete(&assignment).Error
	require.NoError(t, err)

	// Should not be found with default scope
	var found models.ScenarioAssignment
	err = db.First(&found, "id = ?", assignment.ID).Error
	assert.Error(t, err) // record not found
}
