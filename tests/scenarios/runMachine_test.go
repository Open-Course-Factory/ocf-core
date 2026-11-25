package scenarios_test

import (
	"fmt"
	"testing"

	configModels "soli/formations/src/configuration/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	incus = models.ScenarioMachineIncus
	local = models.ScenarioMachineLocal
)

// Every cell of the plan's table, with the switch on, outside any assignment.
//
//	| Plan setting           | Local scenario | By account scenario |
//	| Off                    | Incus          | Incus               |
//	| Allowed                | local          | Incus               |
//	| Default for By account | local          | local               |
//
// An Incus scenario is Incus whatever the plan says, and an empty value on
// either side reads as its default (scenario: Incus, plan: off).
func TestScenarioMachine_PlanTable(t *testing.T) {
	cases := []struct {
		plan, scenario, want string
	}{
		{paymentModels.LocalMachinesOff, models.ScenarioMachineLocal, incus},
		{paymentModels.LocalMachinesOff, models.ScenarioMachineByAccount, incus},
		{paymentModels.LocalMachinesOff, models.ScenarioMachineIncus, incus},
		{paymentModels.LocalMachinesAllowed, models.ScenarioMachineLocal, local},
		{paymentModels.LocalMachinesAllowed, models.ScenarioMachineByAccount, incus},
		{paymentModels.LocalMachinesAllowed, models.ScenarioMachineIncus, incus},
		{paymentModels.LocalMachinesByAccountDefault, models.ScenarioMachineLocal, local},
		{paymentModels.LocalMachinesByAccountDefault, models.ScenarioMachineByAccount, local},
		{paymentModels.LocalMachinesByAccountDefault, models.ScenarioMachineIncus, incus},
		{"", models.ScenarioMachineLocal, incus},
		{paymentModels.LocalMachinesByAccountDefault, "", incus},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("plan=%q scenario=%q", c.plan, c.scenario), func(t *testing.T) {
			got, err := services.ScenarioMachine(services.MachineContext{
				SwitchOn: true, PlanSetting: c.plan, ScenarioMachine: c.scenario,
			})
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

// Switch off: every new run goes to Incus, whatever the plan and scenario say.
func TestScenarioMachine_SwitchOffIsIncus(t *testing.T) {
	for _, scenario := range []string{models.ScenarioMachineLocal, models.ScenarioMachineByAccount} {
		got, err := services.ScenarioMachine(services.MachineContext{
			PlanSetting: paymentModels.LocalMachinesByAccountDefault, ScenarioMachine: scenario,
		})
		require.NoError(t, err)
		assert.Equal(t, incus, got, scenario)
	}
}

// Rule 4: a class or organization assignment runs a By account scenario on
// Incus (supervision, bulk start); an assigned Local scenario stays local.
func TestScenarioMachine_AssignmentKeepsByAccountOnIncus(t *testing.T) {
	ctx := services.MachineContext{SwitchOn: true, PlanSetting: paymentModels.LocalMachinesByAccountDefault, Assigned: true}

	ctx.ScenarioMachine = models.ScenarioMachineByAccount
	got, err := services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, incus, got)

	ctx.ScenarioMachine = models.ScenarioMachineLocal
	got, err = services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, local, got)
}

// Rule 5, and what the learner owning the machine breaks: a scenario whose
// result must not be in the learner's hands (an exam, crash traps, flags) never
// runs locally. Set to Local, it is refused with a reason; set to By account, it
// goes to Incus, which that setting allows.
func TestScenarioMachine_RefusesWhatTheLearnerCouldFake(t *testing.T) {
	for name, ctx := range map[string]services.MachineContext{
		"exam":        {Exam: true},
		"crash traps": {CrashTraps: true},
		"flags":       {Flags: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx.SwitchOn, ctx.PlanSetting = true, paymentModels.LocalMachinesByAccountDefault

			ctx.ScenarioMachine = models.ScenarioMachineLocal
			_, err := services.ScenarioMachine(ctx)
			require.ErrorIs(t, err, services.ErrScenarioNeedsContainer)

			ctx.ScenarioMachine = models.ScenarioMachineByAccount
			got, err := services.ScenarioMachine(ctx)
			require.NoError(t, err)
			assert.Equal(t, incus, got)
		})
	}

	// A plan that would put it on Incus anyway does not refuse it.
	got, err := services.ScenarioMachine(services.MachineContext{
		SwitchOn: true, PlanSetting: paymentModels.LocalMachinesOff, ScenarioMachine: models.ScenarioMachineLocal, Exam: true,
	})
	require.NoError(t, err)
	assert.Equal(t, incus, got)
}

// The reading from the database: the switch fails closed, a nil plan is off,
// and the exam, crash-trap and flag facts come from the scenario itself.
func TestRunMachine_ReadsTheSwitchPlanAndScenario(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "runs-here", Title: "Runs Here", InstanceType: "debian",
		CreatedByID: "author", Machine: models.ScenarioMachineLocal}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Do", StepType: "terminal"}).Error)
	plan := &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}

	got, err := services.RunMachine(db, &scenario, plan, false)
	require.NoError(t, err)
	assert.Equal(t, incus, got, "no switch row: off")

	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	got, err = services.RunMachine(db, &scenario, plan, false)
	require.NoError(t, err)
	assert.Equal(t, local, got)

	got, err = services.RunMachine(db, &scenario, nil, false)
	require.NoError(t, err)
	assert.Equal(t, incus, got, "no plan: off")

	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 2, Title: "Exam",
		StepType: models.StepTypeQuiz, ShowImmediateFeedback: false}).Error)
	_, err = services.RunMachine(db, &scenario, plan, false)
	require.ErrorIs(t, err, services.ErrScenarioNeedsContainer, "an exam-mode quiz makes it an exam")
}
