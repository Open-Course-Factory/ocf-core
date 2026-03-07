package scenarios_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.Scenario{},
		&models.ScenarioStep{},
		&models.ScenarioSession{},
		&models.ScenarioStepProgress{},
		&models.ScenarioFlag{},
		&models.ScenarioAssignment{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
	)
	require.NoError(t, err)

	return db
}

func TestScenarioImporter_ParseIndexJSON_Valid(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	jsonData := []byte(`{
		"title": "Linux Basics Lab",
		"description": "Learn basic Linux commands",
		"difficulty": "beginner",
		"time": "30m",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{
					"title": "Step 1: Navigate",
					"text": "step1/text.md",
					"verify": "step1/verify.sh",
					"background": "step1/background.sh",
					"foreground": "",
					"hint": "step1/hint.md"
				},
				{
					"title": "Step 2: Create files",
					"text": "step2/text.md",
					"verify": "step2/verify.sh",
					"background": "",
					"foreground": "step2/foreground.sh",
					"hint": ""
				}
			],
			"finish": {"text": "finish.md"},
			"assets": {
				"host01": [
					{"file": "setup.sh", "target": "/tmp", "chmod": "+x"}
				]
			}
		},
		"backend": {"imageid": "ubuntu:22.04"},
		"extensions": {
			"ocf": {
				"flags": true,
				"crash_traps": true,
				"gsh_enabled": false
			}
		}
	}`)

	index, err := importer.ParseIndexJSON(jsonData)

	require.NoError(t, err)
	assert.Equal(t, "Linux Basics Lab", index.Title)
	assert.Equal(t, "Learn basic Linux commands", index.Description)
	assert.Equal(t, "beginner", index.Difficulty)
	assert.Equal(t, "30m", index.Time)
	assert.Equal(t, "ubuntu:22.04", index.Backend.ImageID)
	assert.Len(t, index.Details.Steps, 2)
	assert.Equal(t, "Step 1: Navigate", index.Details.Steps[0].Title)
	assert.Equal(t, "step1/text.md", index.Details.Steps[0].Text)
	assert.Equal(t, "step1/verify.sh", index.Details.Steps[0].Verify)
	assert.Equal(t, "step1/hint.md", index.Details.Steps[0].Hint)
	assert.Equal(t, "Step 2: Create files", index.Details.Steps[1].Title)
	assert.Equal(t, "intro.md", index.Details.Intro.Text)
	assert.Equal(t, "finish.md", index.Details.Finish.Text)
	require.NotNil(t, index.Extensions)
	require.NotNil(t, index.Extensions.OCF)
	assert.True(t, index.Extensions.OCF.Flags)
	assert.True(t, index.Extensions.OCF.CrashTraps)
	assert.False(t, index.Extensions.OCF.GshEnabled)
	require.NotNil(t, index.Details.Assets)
	assert.Len(t, index.Details.Assets.Host01, 1)
	assert.Equal(t, "setup.sh", index.Details.Assets.Host01[0].File)
}

func TestScenarioImporter_ParseIndexJSON_Minimal(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	jsonData := []byte(`{
		"title": "Simple Lab",
		"description": "",
		"difficulty": "",
		"time": "",
		"details": {
			"intro": {"text": ""},
			"steps": [
				{"title": "Only Step", "text": "step1.md", "verify": "", "background": "", "foreground": "", "hint": ""}
			],
			"finish": {"text": ""}
		},
		"backend": {"imageid": "alpine"}
	}`)

	index, err := importer.ParseIndexJSON(jsonData)

	require.NoError(t, err)
	assert.Equal(t, "Simple Lab", index.Title)
	assert.Equal(t, "alpine", index.Backend.ImageID)
	assert.Nil(t, index.Extensions)
	assert.Len(t, index.Details.Steps, 1)
	assert.Equal(t, "Only Step", index.Details.Steps[0].Title)
	assert.Nil(t, index.Details.Assets)
}

func TestScenarioImporter_ParseIndexJSON_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	jsonData := []byte(`{invalid json`)

	index, err := importer.ParseIndexJSON(jsonData)

	assert.Error(t, err)
	assert.Nil(t, index)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestScenarioImporter_BuildScenarioFromIndex(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	// Create a temp directory with test files
	tmpDir := t.TempDir()

	// Write test files
	writeTestFile(t, tmpDir, "intro.md", "# Welcome to the lab")
	writeTestFile(t, tmpDir, "finish.md", "# Congratulations!")

	os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755)
	writeTestFile(t, tmpDir, "step1/text.md", "Navigate to /tmp")
	writeTestFile(t, tmpDir, "step1/verify.sh", "#!/bin/bash\ntest -d /tmp")
	writeTestFile(t, tmpDir, "step1/background.sh", "#!/bin/bash\nmkdir -p /tmp/test")
	writeTestFile(t, tmpDir, "step1/hint.md", "Try using cd")

	os.MkdirAll(filepath.Join(tmpDir, "step2"), 0755)
	writeTestFile(t, tmpDir, "step2/text.md", "Create a file")
	writeTestFile(t, tmpDir, "step2/verify.sh", "#!/bin/bash\ntest -f /tmp/test/file.txt")

	index := &services.KillerCodaIndex{
		Title:       "Linux Basics Lab",
		Description: "Learn Linux commands",
		Difficulty:  "beginner",
		Time:        "30m",
		Details: services.KillerCodaDetails{
			Intro:  services.KillerCodaFile{Text: "intro.md"},
			Finish: services.KillerCodaFile{Text: "finish.md"},
			Steps: []services.KillerCodaStep{
				{
					Title:      "Navigate",
					Text:       "step1/text.md",
					Verify:     "step1/verify.sh",
					Background: "step1/background.sh",
					Foreground: "",
					Hint:       "step1/hint.md",
				},
				{
					Title:      "Create file",
					Text:       "step2/text.md",
					Verify:     "step2/verify.sh",
					Background: "",
					Foreground: "step2/foreground.sh", // file does not exist
					Hint:       "",
				},
			},
		},
		Backend: services.KillerCodaBackend{ImageID: "ubuntu:22.04"},
		Extensions: &services.KillerCodaExtensions{
			OCF: &services.KillerCodaOCF{
				Flags:      true,
				CrashTraps: false,
				GshEnabled: true,
			},
		},
	}

	createdByID := "user-123"
	orgID := uuid.New()

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, createdByID, &orgID)

	require.NoError(t, err)
	assert.Equal(t, "linux-basics-lab", scenario.Name)
	assert.Equal(t, "Linux Basics Lab", scenario.Title)
	assert.Equal(t, "Learn Linux commands", scenario.Description)
	assert.Equal(t, "beginner", scenario.Difficulty)
	assert.Equal(t, "30m", scenario.EstimatedTime)
	assert.Equal(t, "ubuntu:22.04", scenario.InstanceType)
	assert.Equal(t, "builtin", scenario.SourceType)
	assert.True(t, scenario.FlagsEnabled)
	assert.NotEmpty(t, scenario.FlagSecret)
	assert.Len(t, scenario.FlagSecret, 64) // 32 bytes hex = 64 chars
	assert.True(t, scenario.GshEnabled)
	assert.False(t, scenario.CrashTraps)
	assert.Equal(t, "# Welcome to the lab", scenario.IntroText)
	assert.Equal(t, "# Congratulations!", scenario.FinishText)
	assert.Equal(t, createdByID, scenario.CreatedByID)
	assert.Equal(t, &orgID, scenario.OrganizationID)

	require.Len(t, scenario.Steps, 2)

	// Step 0
	assert.Equal(t, 0, scenario.Steps[0].Order)
	assert.Equal(t, "Navigate", scenario.Steps[0].Title)
	assert.Equal(t, "Navigate to /tmp", scenario.Steps[0].TextContent)
	assert.Equal(t, "#!/bin/bash\ntest -d /tmp", scenario.Steps[0].VerifyScript)
	assert.Equal(t, "#!/bin/bash\nmkdir -p /tmp/test", scenario.Steps[0].BackgroundScript)
	assert.Equal(t, "", scenario.Steps[0].ForegroundScript)
	assert.Equal(t, "Try using cd", scenario.Steps[0].HintContent)
	assert.True(t, scenario.Steps[0].HasFlag)

	// Step 1
	assert.Equal(t, 1, scenario.Steps[1].Order)
	assert.Equal(t, "Create file", scenario.Steps[1].Title)
	assert.Equal(t, "Create a file", scenario.Steps[1].TextContent)
	assert.Equal(t, "#!/bin/bash\ntest -f /tmp/test/file.txt", scenario.Steps[1].VerifyScript)
	assert.Equal(t, "", scenario.Steps[1].BackgroundScript)
	assert.Equal(t, "", scenario.Steps[1].ForegroundScript) // file didn't exist
	assert.Equal(t, "", scenario.Steps[1].HintContent)
	assert.True(t, scenario.Steps[1].HasFlag)
}

func TestScenarioImporter_BuildScenarioFromIndex_NoFlags(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	index := &services.KillerCodaIndex{
		Title:   "No Flags Lab",
		Details: services.KillerCodaDetails{
			Steps: []services.KillerCodaStep{
				{Title: "Step 1", Text: "step1.md"},
			},
		},
		Backend: services.KillerCodaBackend{ImageID: "alpine"},
	}

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, "user-1", nil)

	require.NoError(t, err)
	assert.False(t, scenario.FlagsEnabled)
	assert.Empty(t, scenario.FlagSecret)
	assert.False(t, scenario.Steps[0].HasFlag)
	assert.Nil(t, scenario.OrganizationID)
}

func TestScenarioImporter_ImportFromDirectory(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// Write index.json
	indexJSON := `{
		"title": "Import Test Lab",
		"description": "Testing import",
		"difficulty": "intermediate",
		"time": "15m",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Step One", "text": "step1.md", "verify": "verify1.sh", "background": "", "foreground": "", "hint": ""}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "debian:12"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)
	writeTestFile(t, tmpDir, "intro.md", "# Intro")
	writeTestFile(t, tmpDir, "finish.md", "# Done")
	writeTestFile(t, tmpDir, "step1.md", "Do step one")
	writeTestFile(t, tmpDir, "verify1.sh", "#!/bin/bash\ntrue")

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-456", nil)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, scenario.ID)
	assert.Equal(t, "Import Test Lab", scenario.Title)
	assert.Equal(t, "import-test-lab", scenario.Name)
	assert.Equal(t, "debian:12", scenario.InstanceType)
	require.Len(t, scenario.Steps, 1)
	assert.Equal(t, "Do step one", scenario.Steps[0].TextContent)

	// Verify it was persisted
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Equal(t, int64(1), count)

	var stepCount int64
	db.Model(&models.ScenarioStep{}).Count(&stepCount)
	assert.Equal(t, int64(1), stepCount)
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	err := os.WriteFile(fullPath, []byte(content), 0644)
	require.NoError(t, err)
}
