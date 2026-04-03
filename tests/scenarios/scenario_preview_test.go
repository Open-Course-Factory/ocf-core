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

func TestPreviewScenario_CreatorCanPreview(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "preview-test",
		Title:        "Preview Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, nil)

	// Creator starts a preview session
	session, err := sessionSvc.PreviewScenario("creator-1", scenario.ID, "terminal-preview-1")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.True(t, session.IsPreview, "preview session should have IsPreview=true")
	assert.Equal(t, "creator-1", session.UserID)
}

func TestPreviewScenario_NonCreatorDenied(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "preview-denied-test",
		Title:        "Preview Denied Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, nil)

	// Random user tries to preview — should be denied
	session, err := sessionSvc.PreviewScenario("random-user", scenario.ID, "terminal-preview-2")
	assert.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "not authorized")
}

func TestPreviewScenario_OrgManagerCanPreview(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	scenario := models.Scenario{
		Name:           "preview-org-test",
		Title:          "Preview Org Test",
		InstanceType:   "ubuntu:22.04",
		CreatedByID:    "creator-1",
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, nil)

	// Org manager tries to preview — should succeed
	session, err := sessionSvc.PreviewScenario("org-manager-1", scenario.ID, "terminal-preview-3", services.WithOrgManagerCheck(func(userID string, orgID uuid.UUID) bool {
		return userID == "org-manager-1"
	}))
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.True(t, session.IsPreview)
}

func TestPreviewScenario_IsPreviewFlag(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "preview-flag-test",
		Title:        "Preview Flag Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, nil)

	session, err := sessionSvc.PreviewScenario("creator-1", scenario.ID, "terminal-preview-4")
	require.NoError(t, err)
	require.NotNil(t, session)

	// Verify the session was persisted with IsPreview=true
	var loaded models.ScenarioSession
	require.NoError(t, db.First(&loaded, "id = ?", session.ID).Error)
	assert.True(t, loaded.IsPreview, "persisted session should have IsPreview=true")
}

func TestPreviewScenario_ExcludedFromResults(t *testing.T) {
	db := freshTestDB(t)

	// Create a group with a member
	group := groupModels.ClassGroup{Name: "test-group", DisplayName: "Test Group", OwnerUserID: "creator-1", IsActive: true}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "creator-1",
		Role:     "manager",
		JoinedAt: time.Now(),
		IsActive: true,
	}).Error)

	scenario := models.Scenario{
		Name:         "preview-results-test",
		Title:        "Preview Results Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, nil)

	// Create a normal session
	normalSession, err := sessionSvc.StartScenario("creator-1", scenario.ID, "terminal-normal")
	require.NoError(t, err)
	require.NotNil(t, normalSession)

	// Abandon the normal session so we can start a preview
	require.NoError(t, db.Model(normalSession).Update("status", "completed").Error)

	// Create a preview session
	previewSession, err := sessionSvc.PreviewScenario("creator-1", scenario.ID, "terminal-preview-5")
	require.NoError(t, err)
	require.NotNil(t, previewSession)

	// Check teacher dashboard results — preview session should be excluded
	dashboardSvc := services.NewTeacherDashboardService(db, nil, sessionSvc)
	results, err := dashboardSvc.GetScenarioResults(group.ID, scenario.ID, nil, nil)
	require.NoError(t, err)

	// Only the normal (completed) session should appear, not the preview
	for _, item := range results.Items {
		assert.NotEqual(t, previewSession.ID.String(), item.SessionID.String(),
			"preview session should not appear in teacher dashboard results")
	}
}
