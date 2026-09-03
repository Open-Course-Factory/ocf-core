package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// A scenario that names the uid its console runs as must still name it after a
// round trip.
//
// The importer has always read `session_user`; export was not writing it. So
// exporting a scenario and importing the copy silently changed what it is: the
// copy ran as root, and every permission mission in it went quiet — the exact
// failure the field exists to prevent, reintroduced by the act of copying.
func TestExportService_CarriesTheSessionUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	learner := 1000
	scenario := models.Scenario{
		Name:         "export-session-user",
		Title:        "Export Session User",
		InstanceType: "debian",
		OsType:       "deb",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		SessionUser:  &learner,
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Do it", VerifyScript: "true"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exported, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.NotNil(t, exported.SessionUser,
		"export dropped the uid, so importing this copy would give it a root console")
	assert.Equal(t, 1000, *exported.SessionUser)
}

// And a scenario that names none must not gain one.
func TestExportService_OmitsAnAbsentSessionUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "export-no-session-user",
		Title:        "Export No Session User",
		InstanceType: "debian",
		OsType:       "deb",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", TextContent: "Do it", VerifyScript: "true"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exported, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	assert.Nil(t, exported.SessionUser)
}
