package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// TestScenarioSession_TrainerID_SetOnBulkStart verifies that when a trainer
// bulk-starts scenarios for students, the TrainerID is set to the trainer's user ID.
func TestScenarioSession_TrainerID_SetOnBulkStart(t *testing.T) {
	db := setupTestDB(t)

	trainerUserID := "trainer-bulk-001"

	// Create scenario with steps
	scenario := models.Scenario{
		Name: "trainer-id-bulk", Title: "Trainer ID Bulk", InstanceType: "ubuntu:22.04", CreatedByID: trainerUserID,
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	// Create group with 2 student members
	groupID := uuid.New()
	students := []string{"student-tid-1", "student-tid-2"}
	for _, uid := range students {
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: uid, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)
	}

	// Set up mocks
	ttMock := newMockTTService()
	for _, uid := range students {
		ttMock.addKey(uid)
	}
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)

	// Bulk start as trainer (no terminal creation — empty instanceType)
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, "", "", 0, trainerUserID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Created)

	// Verify each student's session has TrainerID set to the trainer
	for _, uid := range students {
		var session models.ScenarioSession
		require.NoError(t, db.Where("user_id = ? AND scenario_id = ? AND status = ?", uid, scenario.ID, "active").First(&session).Error)
		require.NotNil(t, session.TrainerID, "TrainerID should be set on bulk-started session for user %s", uid)
		assert.Equal(t, trainerUserID, *session.TrainerID, "TrainerID should be the trainer's user ID for user %s", uid)
	}
}

// TestScenarioSession_TrainerID_NilOnSelfStart verifies that when a student
// starts their own scenario (self-start), the TrainerID is nil.
func TestScenarioSession_TrainerID_NilOnSelfStart(t *testing.T) {
	db := setupTestDB(t)

	studentUserID := "student-selfstart-001"

	// Create scenario with steps
	scenario := models.Scenario{
		Name: "trainer-id-self", Title: "Trainer ID Self", InstanceType: "ubuntu:22.04", CreatedByID: "admin-1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	// Self-start via StartScenario (no trainer)
	verifySvc := &mockVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario(studentUserID, scenario.ID, "terminal-self-001")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Nil(t, session.TrainerID, "TrainerID should be nil on self-started session")
}
