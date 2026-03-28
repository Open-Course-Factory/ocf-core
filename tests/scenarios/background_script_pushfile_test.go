package scenarios_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// waitForSetupDone waits until the session transitions out of "provisioning" (to "active" or "setup_failed").
func waitForSetupDone(t *testing.T, db *gorm.DB, sessionID any) string {
	t.Helper()
	for i := 0; i < 50; i++ { // max 500ms
		var s models.ScenarioSession
		db.First(&s, "id = ?", sessionID)
		if s.Status != "provisioning" {
			return s.Status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("session did not transition from provisioning within timeout")
	return ""
}

// bgTrackingVerificationService is a mock that tracks both ExecInContainer and PushFile calls,
// allowing tests to inspect the exact sequence and arguments of background script execution.
type bgTrackingVerificationService struct {
	execCalls     []execCall
	pushFileCalls []pushFileCall
	execErr       error
	pushFileErr   error
}

func (m *bgTrackingVerificationService) VerifyStep(terminalSessionID string, step *models.ScenarioStep) (bool, string, error) {
	return true, "", nil
}

func (m *bgTrackingVerificationService) PushFile(sessionID string, targetPath string, content string, mode string) error {
	m.pushFileCalls = append(m.pushFileCalls, pushFileCall{sessionID, targetPath, content, mode})
	return m.pushFileErr
}

func (m *bgTrackingVerificationService) ExecInContainer(sessionID string, command []string, timeout int) (int, string, string, error) {
	m.execCalls = append(m.execCalls, execCall{sessionID, command, timeout})
	if m.execErr != nil {
		return -1, "", "", m.execErr
	}
	return 0, "", "", nil
}

func TestExecuteBackgroundScript_SmallScript_UsesInline(t *testing.T) {
	db := setupTestDB(t)

	// Create a small script (well under 4000 bytes)
	smallScript := "echo 'hello world'"

	scenario := models.Scenario{
		Name:         "bg-small-inline",
		Title:        "Small Script Inline",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: false,
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:       scenario.ID,
		Order:            0,
		Title:            "Step 1",
		BackgroundScript: smallScript,
	}
	require.NoError(t, db.Create(&step).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "provisioning", session.Status)

	// Wait for async setup to complete
	status := waitForSetupDone(t, db, session.ID)
	assert.Equal(t, "active", status)

	// Small script: should use inline ExecInContainer with /bin/sh -c
	assert.Len(t, verifySvc.pushFileCalls, 0, "PushFile should NOT be called for small scripts")
	require.Len(t, verifySvc.execCalls, 1, "ExecInContainer should be called once")
	assert.Equal(t, []string{"/bin/sh", "-c", smallScript}, verifySvc.execCalls[0].command)
	assert.Equal(t, 300, verifySvc.execCalls[0].timeout) // step 0 gets 5-minute timeout
}

func TestExecuteBackgroundScript_LargeScript_UsesPushFile(t *testing.T) {
	db := setupTestDB(t)

	// Create a large script (over 4000 bytes)
	largeScript := "#!/bin/bash\n" + strings.Repeat("echo 'line of script padding to make it large enough'\n", 100)
	require.True(t, len(largeScript) > 4000, "test script must exceed 4000 bytes")

	scenario := models.Scenario{
		Name:         "bg-large-pushfile",
		Title:        "Large Script PushFile",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: false,
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:       scenario.ID,
		Order:            0,
		Title:            "Step 1",
		BackgroundScript: largeScript,
	}
	require.NoError(t, db.Create(&step).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "provisioning", session.Status)

	// Wait for async setup to complete
	status := waitForSetupDone(t, db, session.ID)
	assert.Equal(t, "active", status)

	// Large script: should use PushFile then ExecInContainer with temp path, then cleanup
	require.Len(t, verifySvc.pushFileCalls, 1, "PushFile should be called once")
	assert.Equal(t, "test-terminal", verifySvc.pushFileCalls[0].sessionID)
	assert.Equal(t, "/tmp/.ocf_bg_0.sh", verifySvc.pushFileCalls[0].targetPath)
	assert.Equal(t, largeScript, verifySvc.pushFileCalls[0].content)
	assert.Equal(t, "0700", verifySvc.pushFileCalls[0].mode)

	// Should have 2 exec calls: run script + cleanup rm
	require.Len(t, verifySvc.execCalls, 2, "ExecInContainer should be called twice (run + cleanup)")

	// First exec: run the script from temp file (uses /bin/bash from shebang)
	assert.Equal(t, []string{"/bin/bash", "/tmp/.ocf_bg_0.sh"}, verifySvc.execCalls[0].command)
	assert.Equal(t, 300, verifySvc.execCalls[0].timeout) // step 0 gets 5-minute timeout

	// Second exec: cleanup rm -f
	assert.Equal(t, []string{"rm", "-f", "/tmp/.ocf_bg_0.sh"}, verifySvc.execCalls[1].command)
	assert.Equal(t, 5, verifySvc.execCalls[1].timeout)
}

func TestExecuteBackgroundScript_EmptyScript_NoOp(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "bg-empty-noop",
		Title:        "Empty Script NoOp",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: false,
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:       scenario.ID,
		Order:            0,
		Title:            "Step 1",
		BackgroundScript: "", // Empty
	}
	require.NoError(t, db.Create(&step).Error)

	verifySvc := &bgTrackingVerificationService{}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal")

	require.NoError(t, err)
	require.NotNil(t, session)

	// Empty script: no calls at all
	assert.Len(t, verifySvc.pushFileCalls, 0, "PushFile should not be called for empty script")
	assert.Len(t, verifySvc.execCalls, 0, "ExecInContainer should not be called for empty script")
}

func TestExecuteBackgroundScript_PushFileFails_LogsAndReturns(t *testing.T) {
	db := setupTestDB(t)

	// Create a large script that will trigger PushFile path
	largeScript := "#!/bin/bash\n" + strings.Repeat("echo 'padding line for push file error test'\n", 100)
	require.True(t, len(largeScript) > 4000, "test script must exceed 4000 bytes")

	scenario := models.Scenario{
		Name:         "bg-pushfile-fail",
		Title:        "PushFile Failure",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: false,
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:       scenario.ID,
		Order:            0,
		Title:            "Step 1",
		BackgroundScript: largeScript,
	}
	require.NoError(t, db.Create(&step).Error)

	verifySvc := &bgTrackingVerificationService{
		pushFileErr: fmt.Errorf("container filesystem full"),
	}
	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, verifySvc)

	session, err := sessionSvc.StartScenario("student-1", scenario.ID, "test-terminal")

	// Session should still succeed (background script failure is best-effort)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "provisioning", session.Status)

	// Wait for async setup — should fail since PushFile errors
	status := waitForSetupDone(t, db, session.ID)
	assert.Equal(t, "setup_failed", status)

	// PushFile was called and failed
	require.Len(t, verifySvc.pushFileCalls, 1, "PushFile should be called once")

	// ExecInContainer should NOT be called since PushFile failed
	assert.Len(t, verifySvc.execCalls, 0, "ExecInContainer should not be called when PushFile fails")
}
