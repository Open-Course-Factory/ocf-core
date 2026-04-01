package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"

	"gorm.io/gorm"
)

// setupMemberRouter creates a test router where the user has a "member" role (not admin).
// This is needed to test the non-admin path in GetAvailableScenarios.
func setupMemberRouter(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	sessions := api.Group("/scenario-sessions")
	sessions.GET("/available", controller.GetAvailableScenarios)

	return r
}

func TestScenarioAssignment_StartDate_BeforeStartDate_NotAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db := freshTestDB(t)
	userID := "student-start-date-1"

	// Create a scenario
	scenario := models.Scenario{
		Name:         "start-date-future",
		Title:        "Future Start Date Scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a group and add the user as member
	group := groupModels.ClassGroup{
		Name: "start-date-test-group", DisplayName: "Start Date Test Group",
		OwnerUserID: "teacher-1", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID: group.ID, UserID: userID, Role: "member", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Assign the scenario to the group with a FUTURE start_date (should NOT be available)
	futureStart := time.Now().Add(48 * time.Hour)
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		CreatedByID: "admin-1",
		IsActive:    true,
		StartDate:   &futureStart,
	}
	require.NoError(t, db.Create(&assignment).Error)

	router := setupMemberRouter(db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Scenario should NOT appear because start_date is in the future
	for _, s := range response {
		assert.NotEqual(t, scenario.ID.String(), s["id"],
			"scenario with future start_date should not be available")
	}
}

func TestScenarioAssignment_StartDate_AfterStartDate_Available(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db := freshTestDB(t)
	userID := "student-start-date-2"

	// Create a scenario
	scenario := models.Scenario{
		Name:         "start-date-past",
		Title:        "Past Start Date Scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a group and add the user as member
	group := groupModels.ClassGroup{
		Name: "start-date-past-group", DisplayName: "Start Date Past Group",
		OwnerUserID: "teacher-1", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID: group.ID, UserID: userID, Role: "member", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Assign the scenario with a PAST start_date (should be available)
	pastStart := time.Now().Add(-24 * time.Hour)
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		CreatedByID: "admin-1",
		IsActive:    true,
		StartDate:   &pastStart,
	}
	require.NoError(t, db.Create(&assignment).Error)

	router := setupMemberRouter(db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Scenario SHOULD appear because start_date is in the past
	found := false
	for _, s := range response {
		if s["id"] == scenario.ID.String() {
			found = true
			break
		}
	}
	assert.True(t, found, "scenario with past start_date should be available")
}

func TestScenarioAssignment_StartDate_Nil_Available(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db := freshTestDB(t)
	userID := "student-start-date-3"

	// Create a scenario
	scenario := models.Scenario{
		Name:         "start-date-nil",
		Title:        "No Start Date Scenario",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a group and add the user as member
	group := groupModels.ClassGroup{
		Name: "start-date-nil-group", DisplayName: "Start Date Nil Group",
		OwnerUserID: "teacher-1", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID: group.ID, UserID: userID, Role: "member", IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Assign the scenario with NO start_date (nil - backward compat, should be available)
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		CreatedByID: "admin-1",
		IsActive:    true,
		// StartDate is nil
	}
	require.NoError(t, db.Create(&assignment).Error)

	router := setupMemberRouter(db, userID)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Scenario SHOULD appear because start_date is nil (backward compat)
	found := false
	for _, s := range response {
		if s["id"] == scenario.ID.String() {
			found = true
			break
		}
	}
	assert.True(t, found, "scenario with nil start_date should always be available")
}
