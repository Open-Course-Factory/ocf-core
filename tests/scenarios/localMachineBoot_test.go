package scenarios_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	configModels "soli/formations/src/configuration/models"
	scenarioController "soli/formations/src/scenarios/routes"
	"soli/formations/src/scenarios/services"
)

const bootDefaultsPath = "/local-machines/boot-defaults"

// setupBootDefaultsRouter mounts the boot defaults routes behind Layer-2
// enforcement, as the user with the given roles.
func setupBootDefaultsRouter(t *testing.T, db *gorm.DB, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})
	scenarioController.RegisterScenarioPermissions(mocks.NewMockEnforcer())
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", "user-1")
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())
	controller := scenarioController.NewLocalMachineBootController(db)
	api.GET(bootDefaultsPath, controller.GetBootDefaults)
	api.PUT(bootDefaultsPath, controller.PutBootDefaults)
	return r
}

func decodeBoot(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var boot map[string]any
	require.NoError(t, json.Unmarshal(body, &boot))
	return boot
}

func TestLocalMachineBootDefaults_NothingStored_GivesBuiltInValues(t *testing.T) {
	db := freshTestDB(t)
	r := setupBootDefaultsRouter(t, db, []string{"administrator"})

	w := sendJSON(t, r, http.MethodGet, bootDefaultsPath, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string]any{"jit": true, "verbose": false, "display": true, "sound": true, "append": "", "console_hidden": false},
		decodeBoot(t, w.Body.Bytes()))
}

func TestLocalMachineBootDefaults_Put_StoresAndGetReadsBack(t *testing.T) {
	db := freshTestDB(t)
	r := setupBootDefaultsRouter(t, db, []string{"administrator"})

	w := sendJSON(t, r, http.MethodPut, bootDefaultsPath, map[string]any{"jit": false, "append": "quiet loglevel=3"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored configModels.Feature
	require.NoError(t, db.First(&stored, "key = ?", services.LocalMachineBootKey).Error)
	assert.JSONEq(t, `{"jit":false,"append":"quiet loglevel=3"}`, stored.Value)

	w = sendJSON(t, r, http.MethodGet, bootDefaultsPath, nil)
	assert.Equal(t, map[string]any{"jit": false, "verbose": false, "display": true, "sound": true, "append": "quiet loglevel=3", "console_hidden": false},
		decodeBoot(t, w.Body.Bytes()), "unset keys keep the built-in values")
}

func TestLocalMachineBootDefaults_PutInvalid_RefusedAndNothingStored(t *testing.T) {
	db := freshTestDB(t)
	r := setupBootDefaultsRouter(t, db, []string{"administrator"})

	for name, body := range map[string]map[string]any{
		"unknown key":       {"jit": true, "turbo": true},
		"newline in append": {"append": "quiet\ninit=/bin/sh"},
		"append too long":   {"append": strings.Repeat("a", 513)},
		"wrong type":        {"jit": "yes"},
	} {
		t.Run(name, func(t *testing.T) {
			w := sendJSON(t, r, http.MethodPut, bootDefaultsPath, body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			var count int64
			require.NoError(t, db.Model(&configModels.Feature{}).Where("key = ?", services.LocalMachineBootKey).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestLocalMachineBootDefaults_NonAdministrator_Forbidden(t *testing.T) {
	db := freshTestDB(t)
	r := setupBootDefaultsRouter(t, db, []string{"member"})

	assert.Equal(t, http.StatusForbidden, sendJSON(t, r, http.MethodGet, bootDefaultsPath, nil).Code)
	assert.Equal(t, http.StatusForbidden, sendJSON(t, r, http.MethodPut, bootDefaultsPath, map[string]any{"jit": false}).Code)
	var count int64
	require.NoError(t, db.Model(&configModels.Feature{}).Where("key = ?", services.LocalMachineBootKey).Count(&count).Error)
	assert.Zero(t, count, "a refused PUT stores nothing")
}

// A value an administrator edited by hand in Platform Settings into something
// unreadable must not break every run: the built-in values apply.
func TestLocalMachineBootDefaults_UnreadableStoredValue_GivesBuiltInValues(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.Create(&configModels.Feature{Key: services.LocalMachineBootKey, Name: "x", Enabled: true, Value: `{"jit":`}).Error)

	boot := services.LocalMachineBootDefaults(db)
	require.NotNil(t, boot.JIT)
	assert.True(t, *boot.JIT)
}
