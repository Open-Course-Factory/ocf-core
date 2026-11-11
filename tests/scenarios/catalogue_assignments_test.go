package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// availableCards is the learner's catalogue (personal context), by scenario id.
func availableCards(t *testing.T, db *gorm.DB, userID string) map[string]map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/scenario-sessions/available", nil)
	setupMemberRouter(db, userID).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var cards []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	byID := make(map[string]map[string]any, len(cards))
	for _, c := range cards {
		byID[c["id"].(string)] = c
	}
	return byID
}

// A removed assignment no longer brings its scenario to the learner: the launch
// refuses it, so the catalogue must not offer it either.
func TestCatalogue_RemovedAssignment_IsNotListed(t *testing.T) {
	db := freshTestDB(t)
	scenario := newLaunchableScenario(t, db, "removed-assignment")
	group := newGroupWithMember(t, db, "removed-assignment-class", "learner-removed")
	a := createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group")
	require.Contains(t, availableCards(t, db, "learner-removed"), scenario.ID.String())

	require.NoError(t, db.Delete(&models.ScenarioAssignment{}, "id = ?", a.ID).Error)

	assert.NotContains(t, availableCards(t, db, "learner-removed"), scenario.ID.String())
}

// A learner's card reads the server the assignment names: one they may not use
// says so, rather than "no compatible machine".
func TestCatalogue_ReadsTheAssignedServer_ForLearners(t *testing.T) {
	db := freshTestDB(t)
	scenario := newLaunchableScenario(t, db, "catalogue-server")
	group := newGroupWithMember(t, db, "catalogue-server-class", "learner-catalogue")
	assignWithBackend(t, db, createScenarioAssignment(t, db, scenario.ID, &group.ID, nil, "group"), "elsewhere")

	card := availableCards(t, db, "learner-catalogue")[scenario.ID.String()]
	require.NotNil(t, card)
	assert.Equal(t, false, card["launchable"])
	assert.Equal(t, "server_not_allowed", card["block_reason"])
}
