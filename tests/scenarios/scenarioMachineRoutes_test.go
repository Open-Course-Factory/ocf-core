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

func TestPATCHScenario_MachineApps(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{Name: "apps", Title: "Apps", InstanceType: "S", SourceType: "builtin", CreatedByID: "creator"}
	require.NoError(t, db.Create(scenario).Error)
	r := setupScenarioPatchRouter(t, db, "creator", []string{"administrator"})
	withScenarioHooks(t, db)
	url := "/scenarios/" + scenario.ID.String()
	stored := func() string {
		var s models.Scenario
		require.NoError(t, db.First(&s, "id = ?", scenario.ID).Error)
		return s.MachineApps
	}

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_apps": `["nvim"]`}).Code)
	assert.JSONEq(t, `["nvim"]`, stored(), "set")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"title": "renamed"}).Code)
	assert.JSONEq(t, `["nvim"]`, stored(), "absent keeps")

	assert.Equal(t, http.StatusBadRequest, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_apps": `["doom"]`}).Code)
	assert.JSONEq(t, `["nvim"]`, stored(), "refused keeps")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_apps": ""}).Code)
	assert.Empty(t, stored(), "empty clears")
}

func TestPATCHScenario_MachineDisplay(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{Name: "display", Title: "Display", InstanceType: "S", SourceType: "builtin", CreatedByID: "creator"}
	require.NoError(t, db.Create(scenario).Error)
	r := setupScenarioPatchRouter(t, db, "creator", []string{"administrator"})
	withScenarioHooks(t, db)
	url := "/scenarios/" + scenario.ID.String()
	stored := func() string {
		var s models.Scenario
		require.NoError(t, db.First(&s, "id = ?", scenario.ID).Error)
		return s.MachineDisplay
	}
	assert.Equal(t, models.ScenarioMachineDisplayTerminal, stored(), "a new scenario shows a terminal")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_display": models.ScenarioMachineDisplayDesktop}).Code)
	assert.Equal(t, models.ScenarioMachineDisplayDesktop, stored(), "set")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"title": "renamed"}).Code)
	assert.Equal(t, models.ScenarioMachineDisplayDesktop, stored(), "absent keeps")

	assert.Equal(t, http.StatusBadRequest, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_display": "fullscreen"}).Code)
	assert.Equal(t, models.ScenarioMachineDisplayDesktop, stored(), "refused keeps")
}

func TestPATCHScenario_MachineBoot(t *testing.T) {
	db := freshTestDB(t)
	scenario := &models.Scenario{Name: "boot", Title: "Boot", InstanceType: "S", SourceType: "builtin", CreatedByID: "creator"}
	require.NoError(t, db.Create(scenario).Error)
	r := setupScenarioPatchRouter(t, db, "creator", []string{"administrator"})
	withScenarioHooks(t, db)
	url := "/scenarios/" + scenario.ID.String()
	stored := func() string {
		var s models.Scenario
		require.NoError(t, db.First(&s, "id = ?", scenario.ID).Error)
		return s.MachineBoot
	}
	assert.Empty(t, stored(), "a new scenario overrides nothing")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_boot": `{"jit":false}`}).Code)
	assert.JSONEq(t, `{"jit":false}`, stored(), "set")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"title": "renamed"}).Code)
	assert.JSONEq(t, `{"jit":false}`, stored(), "absent keeps")

	for _, refused := range []string{`{"append":"init=/bin/sh"}`, `{"turbo":true}`, `jit`} {
		assert.Equal(t, http.StatusBadRequest, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_boot": refused}).Code, refused)
	}
	assert.JSONEq(t, `{"jit":false}`, stored(), "refused keeps")

	require.Equal(t, http.StatusNoContent, sendJSON(t, r, http.MethodPatch, url, map[string]any{"machine_boot": ""}).Code)
	assert.Empty(t, stored(), "empty clears")
}
