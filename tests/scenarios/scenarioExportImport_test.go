package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// --- Export Service Tests ---

func TestExportService_ExportAsJSON_Success(t *testing.T) {
	db := freshTestDB(t)

	// Create a scenario with steps (including scripts that are json:"-" on the model)
	scenario := models.Scenario{
		Name:          "export-test",
		Title:         "Export Test",
		Description:   "A test scenario for export",
		Difficulty:    "beginner",
		EstimatedTime: "15m",
		InstanceType:  "ubuntu:22.04",
		OsType:        "deb",
		SourceType:    "seed",
		FlagsEnabled:  true,
		FlagSecret:    "test-secret",
		GshEnabled:    true,
		CrashTraps:    false,
		IntroText:     "# Welcome",
		FinishText:    "# Done",
		CreatedByID:   "user-1",
		Steps: []models.ScenarioStep{
			{
				Order:            0,
				Title:            "Step 1",
				TextContent:      "Do step 1",
				HintContent:      "Hint for step 1",
				VerifyScript:     "#!/bin/bash\ntrue",
				BackgroundScript: "#!/bin/bash\nsetup",
				ForegroundScript: "",
				HasFlag:          true,
				FlagPath:         "/tmp/flag",
				FlagLevel:        1,
			},
			{
				Order:            1,
				Title:            "Step 2",
				TextContent:      "Do step 2",
				HintContent:      "",
				VerifyScript:     "#!/bin/bash\ncheck",
				BackgroundScript: "",
				ForegroundScript: "#!/bin/bash\nfg",
				HasFlag:          false,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	export, err := exportService.ExportAsJSON(scenario.ID)

	require.NoError(t, err)
	require.NotNil(t, export)

	// Verify all fields are present
	assert.Equal(t, "Export Test", export.Title)
	assert.Equal(t, "A test scenario for export", export.Description)
	assert.Equal(t, "beginner", export.Difficulty)
	assert.Equal(t, "15m", export.EstimatedTime)
	assert.Equal(t, "ubuntu:22.04", export.InstanceType)
	assert.Equal(t, "deb", export.OsType)
	assert.True(t, export.FlagsEnabled)
	assert.True(t, export.GshEnabled)
	assert.False(t, export.CrashTraps)
	assert.Equal(t, "# Welcome", export.IntroText)
	assert.Equal(t, "# Done", export.FinishText)

	// Verify steps include scripts (which are json:"-" on the model, but GORM loads them)
	require.Len(t, export.Steps, 2)

	assert.Equal(t, 0, export.Steps[0].Order)
	assert.Equal(t, "Step 1", export.Steps[0].Title)
	assert.Equal(t, "Do step 1", export.Steps[0].TextContent)
	assert.Equal(t, "Hint for step 1", export.Steps[0].HintContent)
	assert.Equal(t, "#!/bin/bash\ntrue", export.Steps[0].VerifyScript)
	assert.Equal(t, "#!/bin/bash\nsetup", export.Steps[0].BackgroundScript)
	assert.Empty(t, export.Steps[0].ForegroundScript)
	assert.True(t, export.Steps[0].HasFlag)
	assert.Equal(t, "/tmp/flag", export.Steps[0].FlagPath)
	assert.Equal(t, 1, export.Steps[0].FlagLevel)

	assert.Equal(t, 1, export.Steps[1].Order)
	assert.Equal(t, "Step 2", export.Steps[1].Title)
	assert.Equal(t, "#!/bin/bash\ncheck", export.Steps[1].VerifyScript)
	assert.Equal(t, "#!/bin/bash\nfg", export.Steps[1].ForegroundScript)
	assert.False(t, export.Steps[1].HasFlag)
}

func TestExportService_ExportAsJSON_NotFound(t *testing.T) {
	db := freshTestDB(t)

	exportService := services.NewScenarioExportService(db)
	export, err := exportService.ExportAsJSON(uuid.New())

	assert.Error(t, err)
	assert.Nil(t, export)
	assert.Contains(t, err.Error(), "scenario not found")
}

func TestExportService_ExportAsArchive_Success(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:          "archive-test",
		Title:         "Archive Test",
		Description:   "Archive scenario",
		Difficulty:    "intermediate",
		EstimatedTime: "30m",
		InstanceType:  "debian:12",
		SourceType:    "seed",
		FlagsEnabled:  true,
		FlagSecret:    "secret",
		GshEnabled:    false,
		CrashTraps:    true,
		IntroText:     "# Intro Text",
		FinishText:    "# Finish Text",
		CreatedByID:   "user-1",
		Steps: []models.ScenarioStep{
			{
				Order:            0,
				Title:            "Navigate",
				TextContent:      "Navigate to /tmp",
				HintContent:      "Try cd /tmp",
				VerifyScript:     "#!/bin/bash\ntest -d /tmp",
				BackgroundScript: "#!/bin/bash\nmkdir -p /tmp",
				HasFlag:          true,
				FlagPath:         "/tmp/flag1",
			},
			{
				Order:            1,
				Title:            "Create File",
				TextContent:      "Create a file",
				VerifyScript:     "#!/bin/bash\ntest -f /tmp/file",
				ForegroundScript: "#!/bin/bash\necho hello",
				HasFlag:          false,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	zipBytes, filename, err := exportService.ExportAsArchive(scenario.ID)

	require.NoError(t, err)
	assert.Equal(t, "archive-test.zip", filename)
	assert.NotEmpty(t, zipBytes)

	// Verify zip is valid and contains expected files
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	fileMap := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		fileMap[f.Name] = string(data)
	}

	// Check index.json exists and is valid
	indexJSON, ok := fileMap["index.json"]
	require.True(t, ok, "zip must contain index.json")

	var index services.KillerCodaIndex
	err = json.Unmarshal([]byte(indexJSON), &index)
	require.NoError(t, err)
	assert.Equal(t, "Archive Test", index.Title)
	assert.Equal(t, "Archive scenario", index.Description)
	assert.Equal(t, "intermediate", index.Difficulty)
	assert.Equal(t, "30m", index.Time)
	assert.Equal(t, "debian:12", index.Backend.ImageID)
	require.Len(t, index.Details.Steps, 2)
	assert.Equal(t, "Navigate", index.Details.Steps[0].Title)
	assert.Equal(t, "Create File", index.Details.Steps[1].Title)

	// Check extensions
	require.NotNil(t, index.Extensions)
	require.NotNil(t, index.Extensions.OCF)
	assert.True(t, index.Extensions.OCF.Flags)
	assert.True(t, index.Extensions.OCF.CrashTraps)
	assert.False(t, index.Extensions.OCF.GshEnabled)

	// Check intro and finish
	assert.Equal(t, "# Intro Text", fileMap["intro.md"])
	assert.Equal(t, "# Finish Text", fileMap["finish.md"])

	// Check step files
	assert.Equal(t, "Navigate to /tmp", fileMap["step1/text.md"])
	assert.Equal(t, "Try cd /tmp", fileMap["step1/hint.md"])
	assert.Equal(t, "#!/bin/bash\ntest -d /tmp", fileMap["step1/verify.sh"])
	assert.Equal(t, "#!/bin/bash\nmkdir -p /tmp", fileMap["step1/background.sh"])

	assert.Equal(t, "Create a file", fileMap["step2/text.md"])
	assert.Equal(t, "#!/bin/bash\ntest -f /tmp/file", fileMap["step2/verify.sh"])
	assert.Equal(t, "#!/bin/bash\necho hello", fileMap["step2/foreground.sh"])

	// step2 should not have hint or background
	_, hasHint := fileMap["step2/hint.md"]
	assert.False(t, hasHint)
	_, hasBg := fileMap["step2/background.sh"]
	assert.False(t, hasBg)
}

func TestExportService_ExportMultiple_Success(t *testing.T) {
	db := freshTestDB(t)

	scenario1 := models.Scenario{
		Name:         "multi-1",
		Title:        "Multi 1",
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step A", TextContent: "Content A"},
		},
	}
	scenario2 := models.Scenario{
		Name:         "multi-2",
		Title:        "Multi 2",
		InstanceType: "debian:12",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step B", TextContent: "Content B"},
			{Order: 1, Title: "Step C", TextContent: "Content C"},
		},
	}
	require.NoError(t, db.Create(&scenario1).Error)
	require.NoError(t, db.Create(&scenario2).Error)

	exportService := services.NewScenarioExportService(db)
	exports, err := exportService.ExportMultipleAsJSON([]uuid.UUID{scenario1.ID, scenario2.ID})

	require.NoError(t, err)
	require.Len(t, exports, 2)

	// Order may vary, find each by title
	titles := map[string]dto.ScenarioExportOutput{}
	for _, e := range exports {
		titles[e.Title] = e
	}

	m1, ok := titles["Multi 1"]
	require.True(t, ok)
	assert.Equal(t, "ubuntu:22.04", m1.InstanceType)
	require.Len(t, m1.Steps, 1)
	assert.Equal(t, "Step A", m1.Steps[0].Title)

	m2, ok := titles["Multi 2"]
	require.True(t, ok)
	assert.Equal(t, "debian:12", m2.InstanceType)
	require.Len(t, m2.Steps, 2)
}

func TestExportService_ExportMultiple_NotFound(t *testing.T) {
	db := freshTestDB(t)

	exportService := services.NewScenarioExportService(db)
	exports, err := exportService.ExportMultipleAsJSON([]uuid.UUID{uuid.New(), uuid.New()})

	assert.Error(t, err)
	assert.Nil(t, exports)
	assert.Contains(t, err.Error(), "no scenarios found")
}

// --- Seed Service Tests ---

func TestSeedService_Create_Success(t *testing.T) {
	db := freshTestDB(t)

	seedService := services.NewScenarioSeedService(db)

	input := dto.SeedScenarioInput{
		Title:         "Seed Create Test",
		Description:   "Testing seed create",
		Difficulty:    "beginner",
		EstimatedTime: "10m",
		InstanceType:  "ubuntu:22.04",
		OsType:        "deb",
		FlagsEnabled:  true,
		GshEnabled:    false,
		CrashTraps:    true,
		IntroText:     "# Intro",
		FinishText:    "# Finish",
		Steps: []dto.SeedStepInput{
			{
				Title:        "Step One",
				TextContent:  "Do step one",
				HintContent:  "Hint one",
				VerifyScript: "#!/bin/bash\ntrue",
				HasFlag:      true,
				FlagPath:     "/tmp/flag",
			},
			{
				Title:            "Step Two",
				TextContent:      "Do step two",
				BackgroundScript: "#!/bin/bash\nsetup",
				HasFlag:          false,
			},
		},
	}

	scenario, isUpdate, err := seedService.SeedScenario(input, "user-1", nil)

	require.NoError(t, err)
	assert.False(t, isUpdate)
	assert.NotEqual(t, uuid.Nil, scenario.ID)
	assert.Equal(t, "seed-create-test", scenario.Name)
	assert.Equal(t, "Seed Create Test", scenario.Title)
	assert.Equal(t, "Testing seed create", scenario.Description)
	assert.Equal(t, "beginner", scenario.Difficulty)
	assert.Equal(t, "ubuntu:22.04", scenario.InstanceType)
	assert.Equal(t, "deb", scenario.OsType)
	assert.True(t, scenario.FlagsEnabled)
	assert.NotEmpty(t, scenario.FlagSecret)
	assert.Equal(t, "seed", scenario.SourceType)
	assert.Equal(t, "user-1", scenario.CreatedByID)
	assert.Nil(t, scenario.OrganizationID)

	require.Len(t, scenario.Steps, 2)
	assert.Equal(t, "Step One", scenario.Steps[0].Title)
	assert.Equal(t, "Do step one", scenario.Steps[0].TextContent)
	assert.True(t, scenario.Steps[0].HasFlag)
	assert.Equal(t, "Step Two", scenario.Steps[1].Title)
	assert.False(t, scenario.Steps[1].HasFlag)
}

func TestSeedService_Upsert_Updates(t *testing.T) {
	db := freshTestDB(t)

	seedService := services.NewScenarioSeedService(db)

	// Create initial scenario
	input1 := dto.SeedScenarioInput{
		Title:        "Upsert Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		Steps: []dto.SeedStepInput{
			{Title: "Original Step", TextContent: "Original content"},
		},
	}

	scenario1, isUpdate1, err := seedService.SeedScenario(input1, "user-1", nil)
	require.NoError(t, err)
	assert.False(t, isUpdate1)
	originalID := scenario1.ID
	originalSecret := scenario1.FlagSecret
	assert.NotEmpty(t, originalSecret)

	// Update with same title (same slug -> same name -> upsert)
	input2 := dto.SeedScenarioInput{
		Title:        "Upsert Test",
		Description:  "Updated description",
		InstanceType: "debian:12",
		FlagsEnabled: true,
		Steps: []dto.SeedStepInput{
			{Title: "Updated Step 1", TextContent: "New content 1"},
			{Title: "Updated Step 2", TextContent: "New content 2"},
		},
	}

	scenario2, isUpdate2, err := seedService.SeedScenario(input2, "user-1", nil)
	require.NoError(t, err)
	assert.True(t, isUpdate2)

	// Should be same scenario (same ID)
	assert.Equal(t, originalID, scenario2.ID)
	// Description updated
	assert.Equal(t, "Updated description", scenario2.Description)
	// Instance type updated
	assert.Equal(t, "debian:12", scenario2.InstanceType)
	// Flag secret preserved
	assert.Equal(t, originalSecret, scenario2.FlagSecret)
	// Steps replaced
	require.Len(t, scenario2.Steps, 2)
	assert.Equal(t, "Updated Step 1", scenario2.Steps[0].Title)
	assert.Equal(t, "Updated Step 2", scenario2.Steps[1].Title)

	// Verify old steps are gone from DB
	var stepCount int64
	db.Model(&models.ScenarioStep{}).Where("scenario_id = ?", originalID).Count(&stepCount)
	assert.Equal(t, int64(2), stepCount)
}

func TestSeedService_WithOrgID(t *testing.T) {
	db := freshTestDB(t)

	seedService := services.NewScenarioSeedService(db)

	orgID := uuid.New()
	input := dto.SeedScenarioInput{
		Title:        "Org Scenario",
		InstanceType: "alpine",
		Steps: []dto.SeedStepInput{
			{Title: "Step 1", TextContent: "Content"},
		},
	}

	scenario, _, err := seedService.SeedScenario(input, "user-1", &orgID)

	require.NoError(t, err)
	require.NotNil(t, scenario.OrganizationID)
	assert.Equal(t, orgID, *scenario.OrganizationID)
}

// --- Archive Structure Tests ---

func TestExportArchive_ContainsIndexJSON(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "index-check",
		Title:        "Index Check",
		InstanceType: "ubuntu:22.04",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Only Step", TextContent: "Content"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	zipBytes, _, err := exportService.ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	found := false
	for _, f := range r.File {
		if f.Name == "index.json" {
			found = true
			rc, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			rc.Close()

			var index services.KillerCodaIndex
			err = json.Unmarshal(data, &index)
			require.NoError(t, err)
			assert.Equal(t, "Index Check", index.Title)
			assert.Equal(t, "ubuntu:22.04", index.Backend.ImageID)
			require.Len(t, index.Details.Steps, 1)
			assert.Equal(t, "Only Step", index.Details.Steps[0].Title)
		}
	}
	assert.True(t, found, "zip must contain index.json")
}

func TestExportArchive_ContainsStepFiles(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "step-files-check",
		Title:        "Step Files Check",
		InstanceType: "alpine",
		SourceType:   "seed",
		IntroText:    "# Intro",
		FinishText:   "# Finish",
		CreatedByID:  "user-1",
		Steps: []models.ScenarioStep{
			{
				Order:            0,
				Title:            "Step 1",
				TextContent:      "Step 1 text",
				HintContent:      "Step 1 hint",
				VerifyScript:     "#!/bin/bash\nverify1",
				BackgroundScript: "#!/bin/bash\nbg1",
				ForegroundScript: "#!/bin/bash\nfg1",
			},
			{
				Order:        1,
				Title:        "Step 2",
				TextContent:  "Step 2 text",
				VerifyScript: "#!/bin/bash\nverify2",
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	exportService := services.NewScenarioExportService(db)
	zipBytes, _, err := exportService.ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	// Step 1 should have all files
	assert.True(t, fileNames["step1/text.md"], "should have step1/text.md")
	assert.True(t, fileNames["step1/hint.md"], "should have step1/hint.md")
	assert.True(t, fileNames["step1/verify.sh"], "should have step1/verify.sh")
	assert.True(t, fileNames["step1/background.sh"], "should have step1/background.sh")
	assert.True(t, fileNames["step1/foreground.sh"], "should have step1/foreground.sh")

	// Step 2 should only have text and verify
	assert.True(t, fileNames["step2/text.md"], "should have step2/text.md")
	assert.True(t, fileNames["step2/verify.sh"], "should have step2/verify.sh")
	assert.False(t, fileNames["step2/hint.md"], "should not have step2/hint.md")
	assert.False(t, fileNames["step2/background.sh"], "should not have step2/background.sh")
	assert.False(t, fileNames["step2/foreground.sh"], "should not have step2/foreground.sh")

	// Intro and finish
	assert.True(t, fileNames["intro.md"], "should have intro.md")
	assert.True(t, fileNames["finish.md"], "should have finish.md")
	assert.True(t, fileNames["index.json"], "should have index.json")
}

// --- Roundtrip Test: Export JSON -> re-import via seed ---

func TestExportImport_JSONRoundtrip(t *testing.T) {
	db := freshTestDB(t)

	// Create a scenario
	seedService := services.NewScenarioSeedService(db)
	exportService := services.NewScenarioExportService(db)

	originalInput := dto.SeedScenarioInput{
		Title:         "Roundtrip Test",
		Description:   "Testing roundtrip",
		Difficulty:    "advanced",
		EstimatedTime: "1h",
		InstanceType:  "ubuntu:22.04",
		OsType:        "deb",
		FlagsEnabled:  true,
		GshEnabled:    true,
		CrashTraps:    false,
		IntroText:     "# Welcome",
		FinishText:    "# Bye",
		Steps: []dto.SeedStepInput{
			{
				Title:            "Step 1",
				TextContent:      "Do step 1",
				HintContent:      "Hint 1",
				VerifyScript:     "#!/bin/bash\ntrue",
				BackgroundScript: "#!/bin/bash\nsetup",
				HasFlag:          true,
				FlagPath:         "/tmp/flag",
			},
			{
				Title:            "Step 2",
				TextContent:      "Do step 2",
				ForegroundScript: "#!/bin/bash\nfg",
				HasFlag:          false,
			},
		},
	}

	scenario, _, err := seedService.SeedScenario(originalInput, "user-1", nil)
	require.NoError(t, err)

	// Export as JSON
	export, err := exportService.ExportAsJSON(scenario.ID)
	require.NoError(t, err)

	// Convert export to SeedScenarioInput format for re-import
	// The export format is designed to match SeedScenarioInput
	reimportInput := dto.SeedScenarioInput{
		Title:         export.Title + " (reimported)",
		Description:   export.Description,
		Difficulty:    export.Difficulty,
		EstimatedTime: export.EstimatedTime,
		InstanceType:  export.InstanceType,
		OsType:        export.OsType,
		FlagsEnabled:  export.FlagsEnabled,
		GshEnabled:    export.GshEnabled,
		CrashTraps:    export.CrashTraps,
		IntroText:     export.IntroText,
		FinishText:    export.FinishText,
	}
	for _, step := range export.Steps {
		reimportInput.Steps = append(reimportInput.Steps, dto.SeedStepInput{
			Title:            step.Title,
			TextContent:      step.TextContent,
			HintContent:      step.HintContent,
			VerifyScript:     step.VerifyScript,
			BackgroundScript: step.BackgroundScript,
			ForegroundScript: step.ForegroundScript,
			HasFlag:          step.HasFlag,
			FlagPath:         step.FlagPath,
		})
	}

	// Re-import with different title (so it creates a new scenario)
	reimported, isUpdate, err := seedService.SeedScenario(reimportInput, "user-2", nil)
	require.NoError(t, err)
	assert.False(t, isUpdate) // different title -> new scenario
	assert.NotEqual(t, scenario.ID, reimported.ID)

	// Verify content matches
	assert.Equal(t, "Testing roundtrip", reimported.Description)
	assert.Equal(t, "advanced", reimported.Difficulty)
	assert.Equal(t, "1h", reimported.EstimatedTime)
	assert.Equal(t, "ubuntu:22.04", reimported.InstanceType)
	assert.Equal(t, "deb", reimported.OsType)
	assert.True(t, reimported.FlagsEnabled)
	assert.True(t, reimported.GshEnabled)
	assert.False(t, reimported.CrashTraps)
	assert.Equal(t, "# Welcome", reimported.IntroText)
	assert.Equal(t, "# Bye", reimported.FinishText)

	require.Len(t, reimported.Steps, 2)
	assert.Equal(t, "Step 1", reimported.Steps[0].Title)
	assert.Equal(t, "Do step 1", reimported.Steps[0].TextContent)
	assert.Equal(t, "#!/bin/bash\ntrue", reimported.Steps[0].VerifyScript)
	assert.Equal(t, "#!/bin/bash\nsetup", reimported.Steps[0].BackgroundScript)
	assert.True(t, reimported.Steps[0].HasFlag)
	assert.Equal(t, "/tmp/flag", reimported.Steps[0].FlagPath)

	assert.Equal(t, "Step 2", reimported.Steps[1].Title)
	assert.Equal(t, "#!/bin/bash\nfg", reimported.Steps[1].ForegroundScript)
	assert.False(t, reimported.Steps[1].HasFlag)
}
