package scenarios_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
)

func withScenarioHooks(t *testing.T, db *gorm.DB) {
	t.Helper()
	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.DisableAllHooks(false)
	scenarioHooks.InitScenarioHooks(db)
	t.Cleanup(hooks.GlobalHookRegistry.ClearAllHooks)
}

func TestGroupCreateScenario_AppOutsideThePlatformList_Refused(t *testing.T) {
	db := freshTestDB(t)
	withScenarioHooks(t, db)
	managerID := "group-manager-machine-apps"
	orgID := createTestOrg(t, db, "org-owner-machine-apps")
	groupID := createTestGroupInOrg(t, db, orgID, "org-owner-machine-apps")
	addGroupMember(t, db, groupID, managerID, groupModels.GroupMemberRoleManager)
	router := setupOrgTestRouterWithUserAndRoles(t, db, managerID, []string{"Member"})

	w := sendJSON(t, router, http.MethodPost, "/groups/"+groupID.String()+"/scenarios", map[string]any{
		"name": "with-a-game", "title": "With a game", "instance_type": "ubuntu:22.04", "machine_apps": `["doom","firefox"]`,
	})

	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Zero(t, count, "a refused scenario is not stored")
}

func TestOrgCreateScenario_AppsNotAJSONList_Refused(t *testing.T) {
	db := freshTestDB(t)
	withScenarioHooks(t, db)
	ownerID := "org-owner-machine-apps-2"
	orgID := createTestOrg(t, db, ownerID)
	addOrgMember(t, db, orgID, ownerID, orgModels.OrgRoleOwner)
	router := setupOrgTestRouterWithUserAndRoles(t, db, ownerID, []string{"Member"})

	for _, field := range []map[string]any{{"machine_apps": "nvim"}, {"machine": "bogus"}, {"machine_display": "fullscreen"}} {
		payload := map[string]any{"name": "bad-machine", "title": "Bad machine", "instance_type": "ubuntu:22.04"}
		for k, v := range field {
			payload[k] = v
		}
		w := sendJSON(t, router, http.MethodPost, "/organizations/"+orgID.String()+"/scenarios", payload)
		assert.Equal(t, http.StatusBadRequest, w.Code, "%v: body: %s", field, w.Body.String())
	}
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Zero(t, count, "a refused scenario is not stored")
}

// Every machine field PATCHes as the editor sends it: a valid value is stored
// and read back, an absent one is kept, a refused one changes nothing, and
// an empty one clears an optional field.
func TestPATCHScenario_MachineFields(t *testing.T) {
	cases := []struct {
		field, valid string
		refused      []string
		clears, json bool
		stored       func(models.Scenario) string
	}{
		{field: "machine", valid: models.ScenarioMachineLocal, refused: []string{"bogus"},
			stored: func(s models.Scenario) string { return s.Machine }},
		{field: "machine_apps", valid: `["nvim"]`, refused: []string{`["doom"]`, "nvim"}, clears: true, json: true,
			stored: func(s models.Scenario) string { return s.MachineApps }},
		{field: "machine_display", valid: models.ScenarioMachineDisplayDesktop, refused: []string{"fullscreen"},
			stored: func(s models.Scenario) string { return s.MachineDisplay }},
		{field: "machine_boot", valid: `{"jit":false}`, refused: []string{`{"append":"init=/bin/sh"}`, `{"turbo":true}`, `jit`},
			clears: true, json: true, stored: func(s models.Scenario) string { return s.MachineBoot }},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			db := freshTestDB(t)
			scenario := &models.Scenario{Name: c.field, Title: c.field, InstanceType: "S", SourceType: "builtin", CreatedByID: "creator"}
			require.NoError(t, db.Create(scenario).Error)
			r := setupScenarioPatchRouter(t, db, "creator", []string{"administrator"})
			withScenarioHooks(t, db)
			url := "/scenarios/" + scenario.ID.String()
			assertStored := func(want, msg string) {
				t.Helper()
				var s models.Scenario
				require.NoError(t, db.First(&s, "id = ?", scenario.ID).Error)
				if c.json && want != "" {
					assert.JSONEq(t, want, c.stored(s), msg)
				} else {
					assert.Equal(t, want, c.stored(s), msg)
				}
			}

			require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{c.field: c.valid}).Code)
			assertStored(c.valid, "set")

			require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"title": "renamed"}).Code)
			assertStored(c.valid, "absent keeps")

			for _, refused := range c.refused {
				assert.Equal(t, http.StatusBadRequest, sendJSON(t, r, http.MethodPatch, url, map[string]any{c.field: refused}).Code, refused)
			}
			assertStored(c.valid, "refused keeps")

			if c.clears {
				require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{c.field: ""}).Code)
				assertStored("", "empty clears")
			}
		})
	}
}

// A new scenario runs in a container and shows a terminal, overriding no boot
// setting and offering no app.
func TestCreateScenario_MachineFieldDefaults(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{Name: "defaults", Title: "Defaults", InstanceType: "S", SourceType: "builtin", CreatedByID: "creator"}
	require.NoError(t, db.Create(scenario).Error)
	var stored models.Scenario
	require.NoError(t, db.First(&stored, "id = ?", scenario.ID).Error)
	assert.Equal(t, models.ScenarioMachineDisplayTerminal, stored.MachineDisplay)
	assert.Empty(t, stored.MachineBoot)
	assert.Empty(t, stored.MachineApps)
	assert.Equal(t, models.ScenarioMachineIncus, stored.Machine)
}
