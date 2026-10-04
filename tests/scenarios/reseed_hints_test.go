package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// A re-seed reuses each step's row through Updates, which never writes the
// Hints association, after the old hint rows were deleted: every re-seeded
// scenario lost its progressive hints.

func seedOneStepWithHint(t *testing.T, db *gorm.DB, hintContent string) *models.Scenario {
	t.Helper()
	scenario, _, err := services.NewScenarioSeedService(db).SeedScenario(dto.SeedScenarioInput{
		Title:        "reseed-hints",
		InstanceType: "debian",
		Steps:        []dto.SeedStepInput{{Title: "Only", TextContent: "Look around.", HintContent: hintContent}},
	}, "reseed-user", nil)
	require.NoError(t, err)
	return scenario
}

func storedHintContents(t *testing.T, db *gorm.DB, scenarioID any) []string {
	t.Helper()
	var hints []models.ScenarioStepHint
	require.NoError(t, db.Joins("JOIN scenario_steps s ON s.id = scenario_step_hints.step_id AND s.deleted_at IS NULL").
		Where("s.scenario_id = ?", scenarioID).Order("level ASC").Find(&hints).Error)
	contents := make([]string, len(hints))
	for i, h := range hints {
		assert.Equal(t, i+1, h.Level)
		contents[i] = h.Content
	}
	return contents
}

func TestReseed_KeepsProgressiveHints(t *testing.T) {
	cases := []struct {
		name   string
		reseed string
		want   []string
	}{
		{"same hints", "### Hint 1\nTry ls\n### Hint 2\nTry ls -a", []string{"Try ls", "Try ls -a"}},
		{"changed hints", "### Hint 1\nTry cd\n### Hint 2\nTry cd ~\n### Hint 3\nTry pwd", []string{"Try cd", "Try cd ~", "Try pwd"}},
		{"hint removed", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			first := seedOneStepWithHint(t, db, "### Hint 1\nTry ls\n### Hint 2\nTry ls -a")
			require.Equal(t, []string{"Try ls", "Try ls -a"}, storedHintContents(t, db, first.ID))

			reseeded := seedOneStepWithHint(t, db, tc.reseed)
			require.Equal(t, first.ID, reseeded.ID, "the re-seed updates the same scenario")

			assert.Equal(t, tc.want, storedHintContents(t, db, reseeded.ID))
			require.Len(t, reseeded.Steps, 1)
			assert.Len(t, reseeded.Steps[0].Hints, len(tc.want), "the returned scenario carries the hints too")
		})
	}
}
