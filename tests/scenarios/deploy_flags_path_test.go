package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// pushFileCall records a single PushFile invocation for assertion.
type pushFileCall struct {
	sessionID  string
	targetPath string
	content    string
	mode       string
}

// capturingVerificationService records PushFile calls so tests can inspect
// which paths were used when deploying flags to the container.
type capturingVerificationService struct {
	pushCalls []pushFileCall
}

func (c *capturingVerificationService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return true, "", nil
}

func (c *capturingVerificationService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	c.pushCalls = append(c.pushCalls, pushFileCall{sessionID, targetPath, content, mode})
	return nil
}

func (c *capturingVerificationService) ExecInContainer(sessionID string, command []string, timeout int) (int, string, string, error) {
	return 0, "", "", nil
}


func TestDeployFlags_WorldPathPreserved(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "world-path-test",
		Title:        "World Path Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: "/World/flags/step1.flag"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	// The /World/ path should be preserved as-is, not rewritten to /tmp/
	require.Len(t, verifySvc.pushCalls, 1)
	assert.Equal(t, "/World/flags/step1.flag", verifySvc.pushCalls[0].targetPath)
	assert.Equal(t, "terminal-abc", verifySvc.pushCalls[0].sessionID)
}

func TestDeployFlags_TmpPathPreserved(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "tmp-path-test",
		Title:        "Tmp Path Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: "/tmp/myflags/step1.flag"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	require.Len(t, verifySvc.pushCalls, 1)
	assert.Equal(t, "/tmp/myflags/step1.flag", verifySvc.pushCalls[0].targetPath)
}

func TestDeployFlags_HomePathPreserved(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "home-path-test",
		Title:        "Home Path Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: "/home/student/.hidden_flag"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	require.Len(t, verifySvc.pushCalls, 1)
	assert.Equal(t, "/home/student/.hidden_flag", verifySvc.pushCalls[0].targetPath)
}

func TestDeployFlags_PathTraversalRejected(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "traversal-test",
		Title:        "Traversal Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: "/tmp/../etc/shadow"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	// Path traversal should be rejected — no PushFile call should happen for that flag
	assert.Empty(t, verifySvc.pushCalls, "path traversal should be rejected, no PushFile call expected")
}

func TestDeployFlags_DisallowedPathRewrittenToDefault(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "disallowed-path-test",
		Title:        "Disallowed Path Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: "/etc/evil.flag"},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	// Disallowed path should be rewritten to the default /tmp/.flag_step_N
	require.Len(t, verifySvc.pushCalls, 1)
	assert.Equal(t, "/tmp/.flag_step_0", verifySvc.pushCalls[0].targetPath,
		"disallowed path should be rewritten to default /tmp/.flag_step_<order>")
}

func TestDeployFlags_EmptyPathUsesDefault(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "empty-path-test",
		Title:        "Empty Path Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "test-secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1", TextContent: "Do something", HasFlag: true, FlagPath: ""},
	}
	for i := range steps {
		require.NoError(t, db.Create(&steps[i]).Error)
	}

	verifySvc := &capturingVerificationService{}
	flagSvc := services.NewFlagService()
	sessionSvc := services.NewScenarioSessionService(db, flagSvc, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "terminal-abc")
	require.NoError(t, err)
	require.NotNil(t, session)

	// Empty path should use the default /tmp/.flag_step_N
	require.Len(t, verifySvc.pushCalls, 1)
	assert.Equal(t, "/tmp/.flag_step_0", verifySvc.pushCalls[0].targetPath,
		"empty FlagPath should default to /tmp/.flag_step_<order>")
}
