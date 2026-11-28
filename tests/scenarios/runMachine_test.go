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
			assert.Equal(t, c.want, got.Machine)
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
		assert.Equal(t, incus, got.Machine, scenario)
	}
}

// Rule 4: a class or organization assignment runs a By account scenario on
// Incus (supervision, bulk start); an assigned Local scenario stays local.
func TestScenarioMachine_AssignmentKeepsByAccountOnIncus(t *testing.T) {
	ctx := services.MachineContext{SwitchOn: true, PlanSetting: paymentModels.LocalMachinesByAccountDefault, Assigned: true}

	ctx.ScenarioMachine = models.ScenarioMachineByAccount
	got, err := services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine)

	ctx.ScenarioMachine = models.ScenarioMachineLocal
	got, err = services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, local, got.Machine)
}

// Rule 5, and what the learner owning the machine breaks: an exam, and a
// crash-trap scenario (its config holds every flag of the run), never run
// locally. Set to Local, it is refused with a reason; set to By account, it goes
// to Incus, which that setting allows.
func TestScenarioMachine_RefusesWhatTheLearnerCouldFake(t *testing.T) {
	for name, ctx := range map[string]services.MachineContext{
		"exam":        {Exam: true},
		"crash traps": {CrashTraps: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx.SwitchOn, ctx.PlanSetting = true, paymentModels.LocalMachinesByAccountDefault

			ctx.ScenarioMachine = models.ScenarioMachineLocal
			_, err := services.ScenarioMachine(ctx)
			require.ErrorIs(t, err, services.ErrScenarioNeedsContainer)

			ctx.ScenarioMachine = models.ScenarioMachineByAccount
			got, err := services.ScenarioMachine(ctx)
			require.NoError(t, err)
			assert.Equal(t, incus, got.Machine)
		})
	}

	// A plan that would put it on Incus anyway does not refuse it.
	got, err := services.ScenarioMachine(services.MachineContext{
		SwitchOn: true, PlanSetting: paymentModels.LocalMachinesOff, ScenarioMachine: models.ScenarioMachineLocal, Exam: true,
	})
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine)
}

// Flag steps run locally (decided 2026-11-25): a step's flag is as readable to
// a root learner in a container, and a verify step is as fakeable.
func TestRunMachine_FlagScenarioRunsLocally(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	scenario := models.Scenario{Name: "flags-local", Title: "Flags Local", InstanceType: "debian",
		CreatedByID: "author", Machine: models.ScenarioMachineLocal, FlagsEnabled: true}
	require.NoError(t, db.Create(&scenario).Error)

	got, err := services.ReadMachineSwitches(db).RunMachine(db, &scenario, services.RunRequest{Plan: &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}})
	require.NoError(t, err)
	assert.Equal(t, local, got.Machine)
}

// The reading from the database: the switch fails closed, a nil plan is off,
// and the exam and crash-trap facts come from the scenario itself.
func TestRunMachine_ReadsTheSwitchPlanAndScenario(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "runs-here", Title: "Runs Here", InstanceType: "debian",
		CreatedByID: "author", Machine: models.ScenarioMachineLocal}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Do", StepType: "terminal"}).Error)
	plan := &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}

	got, err := services.ReadMachineSwitches(db).RunMachine(db, &scenario, services.RunRequest{Plan: plan})
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine, "no switch row: off")

	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	got, err = services.ReadMachineSwitches(db).RunMachine(db, &scenario, services.RunRequest{Plan: plan})
	require.NoError(t, err)
	assert.Equal(t, local, got.Machine)

	got, err = services.ReadMachineSwitches(db).RunMachine(db, &scenario, services.RunRequest{Plan: nil})
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine, "no plan: off")

	require.NoError(t, db.Create(&models.ScenarioStep{ScenarioID: scenario.ID, Order: 2, Title: "Exam",
		StepType: models.StepTypeQuiz, ShowImmediateFeedback: false}).Error)
	_, err = services.ReadMachineSwitches(db).RunMachine(db, &scenario, services.RunRequest{Plan: plan})
	require.ErrorIs(t, err, services.ErrScenarioNeedsContainer, "an exam-mode quiz makes it an exam")
}

// A browser that cannot run a machine (the tab says why) sends a By account
// scenario to Incus, which that setting allows, and refuses a Local one.
func TestScenarioMachine_LocalMachineUnavailable(t *testing.T) {
	ctx := services.MachineContext{SwitchOn: true, PlanSetting: paymentModels.LocalMachinesByAccountDefault,
		LocalMachineUnavailable: "memory"}

	ctx.ScenarioMachine = models.ScenarioMachineByAccount
	got, err := services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine)

	ctx.ScenarioMachine = models.ScenarioMachineLocal
	_, err = services.ScenarioMachine(ctx)
	require.ErrorIs(t, err, services.ErrLocalMachineUnavailable)

	ctx.ScenarioMachine = models.ScenarioMachineIncus
	got, err = services.ScenarioMachine(ctx)
	require.NoError(t, err)
	assert.Equal(t, incus, got.Machine, "an Incus scenario never needed the browser")
}

// Plan rule 3 (2026-11-28): on an Allowed plan, a By account run goes to a
// container while one fits the learner's budget, and to the browser when none
// does, saying why. An assignment, a browser that cannot run it, or what the
// learner could fake keep it on a container.
func TestScenarioMachine_ByAccountOnAnAllowedPlanFollowsTheBudget(t *testing.T) {
	base := services.MachineContext{SwitchOn: true, PlanSetting: paymentModels.LocalMachinesAllowed,
		ScenarioMachine: models.ScenarioMachineByAccount}
	cases := map[string]struct {
		edit func(*services.MachineContext)
		want services.MachineDecision
	}{
		"fits":              {func(*services.MachineContext) {}, services.MachineDecision{Machine: incus}},
		"full":              {func(c *services.MachineContext) { c.ContainerFull = true }, services.MachineDecision{Machine: local, Reason: services.MachineReasonQuota}},
		"full but assigned": {func(c *services.MachineContext) { c.ContainerFull, c.Assigned = true, true }, services.MachineDecision{Machine: incus}},
		"full, browser cannot": {func(c *services.MachineContext) { c.ContainerFull, c.LocalMachineUnavailable = true, "memory" },
			services.MachineDecision{Machine: incus}},
		"full, crash traps": {func(c *services.MachineContext) { c.ContainerFull, c.CrashTraps = true, true }, services.MachineDecision{Machine: incus}},
		"full, Default for By account": {func(c *services.MachineContext) {
			c.ContainerFull, c.PlanSetting = true, paymentModels.LocalMachinesByAccountDefault
		}, services.MachineDecision{Machine: local}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := base
			c.edit(&ctx)
			got, err := services.ScenarioMachine(ctx)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

// The budget is weighed only where it decides: never for a Local scenario or
// on a Default for By account plan.
func TestRunMachine_AsksTheBudgetOnlyWhenItDecides(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.LocalMachinesFeatureKey, Name: "x", Enabled: true}).Error)
	scenario := models.Scenario{Name: "weighs", Title: "Weighs", InstanceType: "s", CreatedByID: "author",
		Machine: models.ScenarioMachineByAccount}
	require.NoError(t, db.Create(&scenario).Error)
	asked := 0
	full := func() (bool, error) { asked++; return false, nil }
	switches := services.ReadMachineSwitches(db)

	got, err := switches.RunMachine(db, &scenario, services.RunRequest{
		Plan: &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}, ContainerFits: full})
	require.NoError(t, err)
	assert.Equal(t, services.MachineDecision{Machine: local, Reason: services.MachineReasonQuota}, got)
	assert.Equal(t, 1, asked)

	_, err = switches.RunMachine(db, &scenario, services.RunRequest{
		Plan: &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesByAccountDefault}, ContainerFits: full})
	require.NoError(t, err)
	scenario.Machine = models.ScenarioMachineLocal
	_, err = switches.RunMachine(db, &scenario, services.RunRequest{
		Plan: &paymentModels.SubscriptionPlan{LocalMachines: paymentModels.LocalMachinesAllowed}, ContainerFits: full})
	require.NoError(t, err)
	assert.Equal(t, 1, asked, "not asked when the plan's table decides")
}
