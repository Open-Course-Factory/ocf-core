package scenarios_test

// Regression tests for the PATCH /api/v1/scenarios/{id} bug surfaced
// during MR !210 testing.
//
// Reported symptom:
//   - PATCH with body {"instance_type": "m", ...} returns 204
//   - On next GET, instance_type still holds the previous value
//
// Actual root cause (found by tracing editEntity.go):
//   - editEntity.go:173-179 unconditionally deletes ALL empty strings
//     from the inbound PATCH map, on the theory that "" means "no change".
//   - The frontend ScenarioEditor's <select> v-model wrote "" back to
//     instance_type when the stored value (e.g. "S") didn't match any
//     catalog option case-sensitively (catalog has "s"). The PATCH body
//     therefore contained {"instance_type": ""} — which the backend
//     silently dropped, returning 204 with no real update.
//   - The frontend has been patched (ocf-front d12a17e) to surface the
//     canonical-case key, but the backend was masking the data loss with
//     a 204. PATCH semantics require that callers can clear an optional
//     text field by sending "".
//
// Fix: cleanup is now type-aware — empty strings are dropped only when
// the target DTO field is a non-string type (e.g. *uuid.UUID, *time.Time,
// *bool) where "" can't decode meaningfully. String / *string fields
// receive the empty value and clear the column.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementController "soli/formations/src/entityManagement/routes"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"
)

// setupScenarioPatchRouter wires a minimal generic-CRUD router for the
// Scenario entity, scoped to PATCH only. Used by all PATCH tests below.
func setupScenarioPatchRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()

	originalSvc := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenario(ems.GlobalEntityRegistrationService)
	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService = originalSvc
	})

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.EnforceFunc = func(params ...any) (bool, error) { return true, nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	t.Cleanup(func() { casdoor.Enforcer = originalEnforcer })

	gen := entityManagementController.NewGenericController(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.PATCH("/scenarios/:id", gen.EditEntity)
	return r
}

// TestPATCHScenario_DescriptionExplicitEmptyIsApplied pins the PATCH
// semantics for clearing an optional text field. Sending "" must overwrite
// the stored value with empty, not be silently dropped.
//
// We use Description (nullable text) since InstanceType is `not null` and
// would fail at the GORM layer on a real PG; SQLite tolerates the latter
// but the assertion would still fail because the cleanup drops it before
// it ever reaches GORM. Description proves the contract cleanly.
func TestPATCHScenario_DescriptionExplicitEmptyIsApplied(t *testing.T) {
	db := freshTestDB(t)

	creatorID := "creator-user-2"
	scenario := &models.Scenario{
		Name:         "test-scenario-empty",
		Title:        "Test Empty",
		InstanceType: "S",
		SourceType:   "builtin",
		CreatedByID:  creatorID,
		Description:  "old description that should be cleared",
	}
	require.NoError(t, db.Create(scenario).Error)

	r := setupScenarioPatchRouter(t, db, creatorID, []string{"administrator"})

	patchBody := map[string]any{"description": ""}
	body, _ := json.Marshal(patchBody)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/scenarios/"+scenario.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, "PATCH should return 204; body=%s", w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)

	// PATCH with explicit "" must clear an optional text field. Pre-fix
	// the cleanup at editEntity.go silently dropped the key.
	assert.Equal(t, "", reloaded.Description,
		"description should be cleared when PATCH body explicitly sets it to \"\"")
}

// TestPATCHScenario_InstanceTypePersists covers the happy path: a PATCH
// body with an explicit non-empty instance_type value updates the column.
// This always passed (the bug only manifested with "" payloads), but we
// keep it as a control / regression guard.
func TestPATCHScenario_InstanceTypePersists(t *testing.T) {
	db := freshTestDB(t)

	creatorID := "creator-user-1"
	scenario := &models.Scenario{
		Name:         "test-scenario-patch",
		Title:        "Test Scenario PATCH",
		InstanceType: "S",
		SourceType:   "builtin",
		CreatedByID:  creatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	r := setupScenarioPatchRouter(t, db, creatorID, []string{"administrator"})

	// Realistic ScenarioEditor.vue payload — all fields included, several
	// empty strings (which the old cleanup blanket-dropped), legit values
	// for the rest.
	patchBody := map[string]any{
		"name":           "test-scenario-patch",
		"title":          "Updated Title",
		"difficulty":     "",
		"estimated_time": "",
		"description":    "",
		"intro_text":     "",
		"finish_text":    "",
		"objectives":     "",
		"prerequisites":  "",
		"setup_script":   "",
		"instance_type":  "m",
		"hostname":       "",
		"os_type":        "",
		"source_type":    "builtin",
		"flags_enabled":  false,
		"crash_traps":    false,
		"gsh_enabled":    false,
		"is_public":      false,
	}
	body, _ := json.Marshal(patchBody)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/scenarios/"+scenario.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, "PATCH should return 204; body=%s", w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)

	assert.Equal(t, "Updated Title", reloaded.Title, "title should update")
	assert.Equal(t, "m", reloaded.InstanceType, "instance_type should persist after PATCH")
}

// TestPATCHScenario_NonStringEmptyValuesStillSafelyDropped asserts that the
// type-aware cleanup keeps doing its original job: empty strings targeting
// non-string fields (UUID, time) still get dropped so they don't choke
// mapstructure decoders. The opposite of the description test.
func TestPATCHScenario_NonStringEmptyValuesStillSafelyDropped(t *testing.T) {
	db := freshTestDB(t)

	creatorID := "creator-user-3"
	scenario := &models.Scenario{
		Name:         "test-scenario-non-string",
		Title:        "Test Non-String",
		InstanceType: "S",
		SourceType:   "builtin",
		CreatedByID:  creatorID,
	}
	require.NoError(t, db.Create(scenario).Error)

	r := setupScenarioPatchRouter(t, db, creatorID, []string{"administrator"})

	// SetupScriptID is *uuid.UUID — sending "" here MUST be silently
	// dropped (StringToUUIDHook would convert to uuid.Nil, but the cleanup
	// is the older, safer guard). We're really just asserting no decode
	// error — the column stays nil, and the title update still goes through.
	patchBody := map[string]any{
		"title":           "Updated Non-String",
		"setup_script_id": "",
	}
	body, _ := json.Marshal(patchBody)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/scenarios/"+scenario.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, "PATCH should return 204; body=%s", w.Body.String())

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	assert.Equal(t, "Updated Non-String", reloaded.Title)
	assert.Nil(t, reloaded.SetupScriptID, "empty string for *uuid.UUID field should not corrupt the column")
}
