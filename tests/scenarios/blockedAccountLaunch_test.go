// tests/scenarios/blockedAccountLaunch_test.go
//
// Blocking an account disables its terminal key, but every launch path
// re-creates a missing key on the fly. The key service now refuses a blocked
// account, and the launch paths tell the caller why.
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

func TestLaunchScenario_BlockedAccountWithoutKey_403UserBlocked(t *testing.T) {
	db := freshTestDB(t)
	userID := "launch-blocked-user"
	seedActivePlan(t, db, userID)
	require.NoError(t, authModels.SetAccountBlocked(db, userID, true))
	scenario := &models.Scenario{
		Name: "launch-blocked-test", Title: "Launch Blocked Test",
		InstanceType: "M", OsType: "deb", IsPublic: true, CreatedByID: userID,
	}
	require.NoError(t, db.Create(scenario).Error)

	ttSrv := newLaunchTTBackend(t, 8.0, 25.0)
	configureTTServerForLaunch(t, ttSrv.URL)
	router := setupLaunchRouterWithProdMiddleware(t, db, userID)

	body, _ := json.Marshal(map[string]string{"scenario_id": scenario.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/launch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"error_message":"user_blocked"`)
	var keys int64
	db.Model(&terminalModels.UserTerminalKey{}).Where("user_id = ?", userID).Count(&keys)
	assert.Zero(t, keys, "no key was re-created for the blocked account")
}

// blockedKeyTTService refuses key creation the way the real service does for
// a blocked account, and otherwise behaves like capturingTTService.
type blockedKeyTTService struct {
	*capturingTTService
	blocked map[string]bool
}

func (m *blockedKeyTTService) CreateUserKey(userID, keyName string) error {
	if m.blocked[userID] {
		return authModels.ErrAccountBlocked
	}
	return m.capturingTTService.CreateUserKey(userID, keyName)
}

func TestBulkStartScenario_BlockedLearner_SkippedWithReasonOthersStart(t *testing.T) {
	_, ttMock, scenario, groupID := bulkStartFixture(t, "bulk-blocked", map[string][]string{
		"member": {"bulk-blocked-ok", "bulk-blocked-ko"},
	})
	delete(ttMock.keys, "bulk-blocked-ko") // the block disabled the learner's key
	tt := &blockedKeyTTService{capturingTTService: ttMock, blocked: map[string]bool{"bulk-blocked-ko": true}}
	dashSvc := services.NewTeacherDashboardService(sharedTestDB, tt,
		services.NewScenarioSessionService(sharedTestDB, &mockFlagService{}, &mockVerificationService{}))

	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{
		Distribution: "debian", Size: "XS",
	}, 0, "trainer")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Created, "the other learner still starts")
	require.Len(t, result.NoKeyUsers, 1)
	assert.Equal(t, "bulk-blocked-ko", result.NoKeyUsers[0].UserID)
	assert.Equal(t, "user_blocked", result.NoKeyUsers[0].Reason)
	assert.Equal(t, []string{"bulk-blocked-ok"}, ttMock.capturedUserIDs, "no container for the blocked learner")
}
