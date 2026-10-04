package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

// The model refuses a public organisation scenario on every write that knows
// the row's organisation: a struct save, and a column-map update of a loaded
// row, which is how re-seeds and generic PATCHes write.
func TestScenarioModel_OrgScenarioNeverStoredPublic(t *testing.T) {
	db := freshTestDB(t)
	orgID := createTestOrg(t, db, "vis-owner")

	created := models.Scenario{Name: "vis-org", Title: "Org", InstanceType: "debian", OrganizationID: &orgID, IsPublic: true}
	require.NoError(t, db.Create(&created).Error)
	assertStoredPublic(t, created, false, "create")

	require.NoError(t, db.Model(&created).Updates(map[string]any{"is_public": true}).Error)
	assertStoredPublic(t, created, false, "map update")

	require.NoError(t, db.Model(&created).Update("is_public", true).Error)
	assertStoredPublic(t, created, false, "single column update")

	platform := models.Scenario{Name: "vis-platform", Title: "Platform", InstanceType: "debian"}
	require.NoError(t, db.Create(&platform).Error)
	require.NoError(t, db.Model(&platform).Updates(map[string]any{"is_public": true}).Error)
	assertStoredPublic(t, platform, true, "a platform scenario may be published")
}

func assertStoredPublic(t *testing.T, scenario models.Scenario, want bool, how string) {
	t.Helper()
	var stored models.Scenario
	require.NoError(t, sharedTestDB.First(&stored, "id = ?", scenario.ID).Error)
	assert.Equal(t, want, stored.IsPublic, how)
}
