package scenarios_test

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scenario says where it runs: an Incus container (the default), a machine in
// the learner's browser, or whichever the learner's plan prefers. The value is a
// closed set, refused at every door when unknown: an unknown value would quietly
// read as Incus at launch and the author would never learn why.
func TestScenarioMachineProblem_ClosedSet(t *testing.T) {
	for _, value := range []string{"", models.ScenarioMachineIncus, models.ScenarioMachineLocal, models.ScenarioMachineByAccount} {
		assert.Empty(t, models.ScenarioMachineProblem(value), "%q is a machine", value)
	}
	assert.NotEmpty(t, models.ScenarioMachineProblem("cloud"))
}

func TestScenarioMachineHook_RefusesUnknownValues(t *testing.T) {
	h := scenarioHooks.NewScenarioMachineHook()
	run := func(hookType hooks.HookType, entity any) error {
		return h.Execute(&hooks.HookContext{EntityName: "Scenario", HookType: hookType, NewEntity: entity})
	}
	require.NoError(t, run(hooks.BeforeCreate, &models.Scenario{Machine: models.ScenarioMachineLocal}))
	require.Error(t, run(hooks.BeforeCreate, &models.Scenario{Machine: "cloud"}))

	local, cloud := models.ScenarioMachineLocal, "cloud"
	require.NoError(t, run(hooks.BeforeUpdate, map[string]any{"machine": local}))
	require.Error(t, run(hooks.BeforeUpdate, map[string]any{"machine": cloud}))
	require.NoError(t, run(hooks.BeforeUpdate, map[string]any{"title": "renamed"}), "a patch without machine")
}

func TestScenarioMachine_ImportRefusesUnknownAndDefaultsToIncus(t *testing.T) {
	db := freshTestDB(t)
	seeder := services.NewScenarioSeedService(db)
	org := uuid.New()

	input := fullyPopulatedScenarioInput()
	input.Machine = nil
	seeded, _, err := seeder.SeedScenario(input, "author", &org)
	require.NoError(t, err)
	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", seeded.ID).Error)
	assert.Equal(t, models.ScenarioMachineIncus, stored.Machine, "a scenario that says nothing runs on Incus")

	cloud := "cloud"
	input.Title, input.Machine = "Elsewhere", &cloud
	_, _, err = seeder.SeedScenario(input, "author", &org)
	require.Error(t, err, "an unknown machine is refused at import")
}

// A scenario names the apps its machine in the browser offers to install, from
// the platform's list only: games and the like are not in it, and an unknown id
// is refused at every door rather than silently dropped.
func TestScenarioMachineAppsProblem_OnlyThePlatformList(t *testing.T) {
	assert.Empty(t, models.ScenarioMachineAppsProblem(""), "none offers nothing")
	assert.Empty(t, models.ScenarioMachineAppsProblem(`["python","postgres"]`))
	assert.NotEmpty(t, models.ScenarioMachineAppsProblem(`["python","doom"]`))
	assert.NotEmpty(t, models.ScenarioMachineAppsProblem(`python`), "not a JSON array")
}

func TestScenarioMachineHook_RefusesAppsOutsideTheList(t *testing.T) {
	h := scenarioHooks.NewScenarioMachineHook()
	run := func(hookType hooks.HookType, entity any) error {
		return h.Execute(&hooks.HookContext{EntityName: "Scenario", HookType: hookType, NewEntity: entity})
	}
	require.NoError(t, run(hooks.BeforeCreate, &models.Scenario{MachineApps: `["nvim"]`}))
	require.Error(t, run(hooks.BeforeCreate, &models.Scenario{MachineApps: `["doom"]`}))

	require.NoError(t, run(hooks.BeforeUpdate, map[string]any{"machine_apps": `["nvim"]`}))
	require.Error(t, run(hooks.BeforeUpdate, map[string]any{"machine_apps": `["doom"]`}))
}

func TestScenarioMachineApps_SeedKeepsWhenAbsentAndRefusesUnknown(t *testing.T) {
	db := freshTestDB(t)
	seeder := services.NewScenarioSeedService(db)
	org := uuid.New()

	input := fullyPopulatedScenarioInput()
	input.MachineApps = []string{"python"}
	seeded, _, err := seeder.SeedScenario(input, "author", &org)
	require.NoError(t, err)

	input.MachineApps = nil
	_, _, err = seeder.SeedScenario(input, "author", &org)
	require.NoError(t, err)
	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", seeded.ID).Error)
	assert.JSONEq(t, `["python"]`, stored.MachineApps, "a re-seed that says nothing keeps the list")

	input.Title, input.MachineApps = "With a game", []string{"doom"}
	_, _, err = seeder.SeedScenario(input, "author", &org)
	require.Error(t, err, "an app outside the platform's list is refused at import")
}
