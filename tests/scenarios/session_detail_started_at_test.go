package scenarios_test

// Red-phase tests for `started_at` derivation in `services.SessionStepDetail`.
//
// The trainer dashboard must show, for each step:
//   - step 0:  StartedAt == session.StartedAt
//   - step N>0: StartedAt == StepProgress[N-1].CompletedAt   (if previous completed)
//   - if any earlier step in the chain has no CompletedAt, the StartedAt is nil.
//
// This mirrors the time-tracking behaviour already implemented inside
// `advanceToNextStep` (scenarioSessionService.go) and surfaces it through the
// teacher dashboard so trainers can see when a student actually began each step.

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

func TestGetSessionDetail_StartedAt_FirstStepEqualsSessionStart(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sa-first-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sa-first", Title: "StartedAt First", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 0",
	}).Error)

	sessionStart := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sa-first-s1",
		Status: "active", StartedAt: sessionStart,
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 1)

	require.NotNil(t, detail.Steps[0].StartedAt,
		"step 0 must always have StartedAt populated (= session.StartedAt)")
	assert.WithinDuration(t, sessionStart, *detail.Steps[0].StartedAt, time.Second,
		"step 0 StartedAt must equal session.StartedAt")
}

func TestGetSessionDetail_StartedAt_NthStepEqualsPreviousCompletion(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sa-chain-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sa-chain", Title: "StartedAt Chain", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	sessionStart := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Second)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sa-chain-s1",
		Status: "active", StartedAt: sessionStart,
	}
	require.NoError(t, db.Create(&session).Error)

	step0CompletedAt := sessionStart.Add(15 * time.Minute)
	step1CompletedAt := sessionStart.Add(45 * time.Minute)

	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "completed",
		CompletedAt: &step0CompletedAt,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "completed",
		CompletedAt: &step1CompletedAt,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 2, Status: "active",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 3)

	// step 0 → session.StartedAt
	require.NotNil(t, detail.Steps[0].StartedAt)
	assert.WithinDuration(t, sessionStart, *detail.Steps[0].StartedAt, time.Second)

	// step 1 → step 0 CompletedAt
	require.NotNil(t, detail.Steps[1].StartedAt,
		"step 1 must use step 0's CompletedAt as its StartedAt")
	assert.WithinDuration(t, step0CompletedAt, *detail.Steps[1].StartedAt, time.Second)

	// step 2 → step 1 CompletedAt
	require.NotNil(t, detail.Steps[2].StartedAt,
		"step 2 must use step 1's CompletedAt as its StartedAt")
	assert.WithinDuration(t, step1CompletedAt, *detail.Steps[2].StartedAt, time.Second)
}

func TestGetSessionDetail_StartedAt_NilWhenPreviousIncomplete(t *testing.T) {
	db := setupTestDB(t)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: "sa-nil-s1", Role: "member",
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name: "sa-nil", Title: "StartedAt Nil", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group",
		CreatedByID: "c1", IsActive: true,
	}).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step",
		}).Error)
	}

	sessionStart := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: "sa-nil-s1",
		Status: "active", StartedAt: sessionStart,
	}
	require.NoError(t, db.Create(&session).Error)

	// Step 0 still active (no CompletedAt). Step 1 is locked.
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 1, Status: "locked",
	}).Error)

	svc := services.NewTeacherDashboardService(db, nil, nil)
	detail, err := svc.GetSessionDetail(groupID, session.ID)
	require.NoError(t, err)
	require.Len(t, detail.Steps, 2)

	// Step 0 → session.StartedAt
	require.NotNil(t, detail.Steps[0].StartedAt)
	assert.WithinDuration(t, sessionStart, *detail.Steps[0].StartedAt, time.Second)

	// Step 1 → previous step has no CompletedAt → StartedAt must be nil.
	assert.Nil(t, detail.Steps[1].StartedAt,
		"when the previous step has no CompletedAt, the next step's StartedAt must be nil")
}
