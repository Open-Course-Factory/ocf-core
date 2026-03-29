package scenarios_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return freshTestDB(t)
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

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, createdByID, &orgID, "builtin")

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

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, "user-1", nil, "builtin")

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

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-456", nil, "builtin")

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

func TestScenarioImporter_ParseIndexJSON_TopLevelIntro(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	// KillerCoda format with intro at top level (outside details)
	jsonData := []byte(`{
		"title": "Quiz Format",
		"description": "A quiz scenario",
		"intro": {"text": "intro.md"},
		"details": {
			"steps": [
				{"title": "Step 1", "text": "step1.md"}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu"}
	}`)

	index, err := importer.ParseIndexJSON(jsonData)

	require.NoError(t, err)
	assert.Equal(t, "Quiz Format", index.Title)
	assert.Equal(t, "intro.md", index.Intro.Text)
	// details.intro should be empty since it's at top level
	assert.Empty(t, index.Details.Intro.Text)
	assert.Equal(t, "finish.md", index.Details.Finish.Text)
}

func TestScenarioImporter_BuildScenarioFromIndex_TopLevelIntro(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "intro.md", "# Top-level intro content")

	// Index with intro at top level, empty details.intro
	index := &services.KillerCodaIndex{
		Title: "Top Level Intro Lab",
		Intro: services.KillerCodaFile{Text: "intro.md"},
		Details: services.KillerCodaDetails{
			// Intro is NOT set here — empty
			Steps: []services.KillerCodaStep{
				{Title: "Step 1", Text: "step1.md"},
			},
		},
		Backend: services.KillerCodaBackend{ImageID: "ubuntu"},
	}

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, "user-1", nil, "builtin")

	require.NoError(t, err)
	assert.Equal(t, "# Top-level intro content", scenario.IntroText)
}

func TestScenarioImporter_BuildScenarioFromIndex_DetailsIntroTakesPrecedence(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "intro-top.md", "# Top-level intro")
	writeTestFile(t, tmpDir, "intro-details.md", "# Details-level intro")
	writeTestFile(t, tmpDir, "finish-top.md", "# Top-level finish")
	writeTestFile(t, tmpDir, "finish-details.md", "# Details-level finish")

	// Both top-level and details-level intro/finish exist — details should win
	index := &services.KillerCodaIndex{
		Title:  "Precedence Lab",
		Intro:  services.KillerCodaFile{Text: "intro-top.md"},
		Finish: services.KillerCodaFile{Text: "finish-top.md"},
		Details: services.KillerCodaDetails{
			Intro:  services.KillerCodaFile{Text: "intro-details.md"},
			Finish: services.KillerCodaFile{Text: "finish-details.md"},
			Steps: []services.KillerCodaStep{
				{Title: "Step 1", Text: "step1.md"},
			},
		},
		Backend: services.KillerCodaBackend{ImageID: "ubuntu"},
	}

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, "user-1", nil, "builtin")

	require.NoError(t, err)
	assert.Equal(t, "# Details-level intro", scenario.IntroText)
	assert.Equal(t, "# Details-level finish", scenario.FinishText)
}

func TestScenarioImporter_BuildScenarioFromIndex_KillerCodaQuizFormat(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// Replicate the exact KillerCoda quiz format from the real zip:
	// - intro at top level
	// - finish inside details
	// - step text at root level (step1.md, step2.md, step3.md)
	// - scripts in subdirectories (step1/verify.sh, step1/background.sh)
	writeTestFile(t, tmpDir, "intro.md", "# Git & Docker Quiz\n\nBienvenue dans ce quiz.")
	writeTestFile(t, tmpDir, "finish.md", "# Bravo !\n\nVous avez terminé le quiz.")

	// Step 1: text at root, scripts in step1/ subdirectory
	writeTestFile(t, tmpDir, "step1.md", "## Question 1\n\nQuelle commande affiche les conteneurs ?")
	os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755)
	writeTestFile(t, tmpDir, "step1/verify.sh", `#!/bin/bash
ANSWER=$(cat /tmp/answer.txt 2>/dev/null)
if [ "$ANSWER" = "docker ps" ]; then
  echo "Correct!"
  exit 0
fi
echo "Incorrect, essayez encore."
exit 1`)
	writeTestFile(t, tmpDir, "step1/background.sh", `#!/bin/bash
cat <<'HEREDOC' > /tmp/setup.sh
echo "Setting up environment"
apt-get update -qq
HEREDOC
chmod +x /tmp/setup.sh
/tmp/setup.sh`)

	// Step 2: text at root, scripts in step2/ subdirectory
	writeTestFile(t, tmpDir, "step2.md", "## Question 2\n\nQuelle commande initialise un dépôt Git ?")
	os.MkdirAll(filepath.Join(tmpDir, "step2"), 0755)
	writeTestFile(t, tmpDir, "step2/verify.sh", `#!/bin/bash
ANSWER=$(cat /tmp/answer2.txt 2>/dev/null)
[ "$ANSWER" = "git init" ]`)
	writeTestFile(t, tmpDir, "step2/background.sh", `#!/bin/bash
mkdir -p /tmp/workspace`)

	// Step 3: text at root, scripts in step3/ subdirectory
	writeTestFile(t, tmpDir, "step3.md", "## Question 3\n\nComment construire une image Docker ?")
	os.MkdirAll(filepath.Join(tmpDir, "step3"), 0755)
	writeTestFile(t, tmpDir, "step3/verify.sh", `#!/bin/bash
ANSWER=$(cat /tmp/answer3.txt 2>/dev/null)
[ "$ANSWER" = "docker build" ]`)
	writeTestFile(t, tmpDir, "step3/background.sh", `#!/bin/bash
echo "Preparing step 3"`)

	// Build index with the exact KillerCoda quiz format
	index := &services.KillerCodaIndex{
		Title:       "Git & Docker — Quiz Théorique",
		Description: "QCM interactif sur Git et Docker",
		Intro:       services.KillerCodaFile{Text: "intro.md"},
		Details: services.KillerCodaDetails{
			Steps: []services.KillerCodaStep{
				{
					Title:      "Question 1 : Conteneurs Docker",
					Text:       "step1.md",
					Verify:     "step1/verify.sh",
					Background: "step1/background.sh",
				},
				{
					Title:      "Question 2 : Initialisation Git",
					Text:       "step2.md",
					Verify:     "step2/verify.sh",
					Background: "step2/background.sh",
				},
				{
					Title:      "Question 3 : Build Docker",
					Text:       "step3.md",
					Verify:     "step3/verify.sh",
					Background: "step3/background.sh",
				},
			},
			Finish: services.KillerCodaFile{Text: "finish.md"},
		},
		Backend: services.KillerCodaBackend{ImageID: "ubuntu"},
	}

	scenario, err := importer.BuildScenarioFromIndex(index, tmpDir, "user-quiz", nil, "builtin")

	require.NoError(t, err)

	// Verify intro from top level is populated
	assert.Contains(t, scenario.IntroText, "Git & Docker Quiz")
	assert.Contains(t, scenario.IntroText, "Bienvenue")

	// Verify finish from details level is populated
	assert.Contains(t, scenario.FinishText, "Bravo")
	assert.Contains(t, scenario.FinishText, "terminé le quiz")

	// Verify all 3 steps
	require.Len(t, scenario.Steps, 3)

	// Step 0: verify all scripts are populated (non-empty)
	assert.Equal(t, "Question 1 : Conteneurs Docker", scenario.Steps[0].Title)
	assert.Contains(t, scenario.Steps[0].TextContent, "Quelle commande affiche les conteneurs")
	assert.NotEmpty(t, scenario.Steps[0].VerifyScript, "step 1 verify script should not be empty")
	assert.Contains(t, scenario.Steps[0].VerifyScript, "docker ps")
	assert.NotEmpty(t, scenario.Steps[0].BackgroundScript, "step 1 background script should not be empty")
	assert.Contains(t, scenario.Steps[0].BackgroundScript, "HEREDOC")

	// Step 1: verify scripts
	assert.Equal(t, "Question 2 : Initialisation Git", scenario.Steps[1].Title)
	assert.Contains(t, scenario.Steps[1].TextContent, "initialise un dépôt Git")
	assert.NotEmpty(t, scenario.Steps[1].VerifyScript, "step 2 verify script should not be empty")
	assert.Contains(t, scenario.Steps[1].VerifyScript, "git init")
	assert.NotEmpty(t, scenario.Steps[1].BackgroundScript, "step 2 background script should not be empty")

	// Step 2: verify scripts
	assert.Equal(t, "Question 3 : Build Docker", scenario.Steps[2].Title)
	assert.Contains(t, scenario.Steps[2].TextContent, "construire une image Docker")
	assert.NotEmpty(t, scenario.Steps[2].VerifyScript, "step 3 verify script should not be empty")
	assert.Contains(t, scenario.Steps[2].VerifyScript, "docker build")
	assert.NotEmpty(t, scenario.Steps[2].BackgroundScript, "step 3 background script should not be empty")
}

func TestScenarioImporter_ImportFromDirectory_KillerCodaQuizFormat(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// Create realistic quiz files
	writeTestFile(t, tmpDir, "intro.md", "# Quiz Introduction\n\nWelcome to the quiz.")
	writeTestFile(t, tmpDir, "finish.md", "# Quiz Complete\n\nWell done!")

	os.MkdirAll(filepath.Join(tmpDir, "step1"), 0755)
	writeTestFile(t, tmpDir, "step1.md", "## Q1\n\nWhat is Linux?")
	writeTestFile(t, tmpDir, "step1/verify.sh", "#!/bin/bash\ntest -f /tmp/a1.txt")
	writeTestFile(t, tmpDir, "step1/background.sh", "#!/bin/bash\necho setup1")

	os.MkdirAll(filepath.Join(tmpDir, "step2"), 0755)
	writeTestFile(t, tmpDir, "step2.md", "## Q2\n\nWhat is Docker?")
	writeTestFile(t, tmpDir, "step2/verify.sh", "#!/bin/bash\ntest -f /tmp/a2.txt")
	writeTestFile(t, tmpDir, "step2/background.sh", "#!/bin/bash\necho setup2")

	// Write index.json with top-level intro (KillerCoda quiz format)
	indexJSON := `{
		"title": "Linux Docker Quiz",
		"description": "Quiz about Linux and Docker",
		"intro": {"text": "intro.md"},
		"details": {
			"steps": [
				{
					"title": "Question 1",
					"text": "step1.md",
					"verify": "step1/verify.sh",
					"background": "step1/background.sh"
				},
				{
					"title": "Question 2",
					"text": "step2.md",
					"verify": "step2/verify.sh",
					"background": "step2/background.sh"
				}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-quiz", nil, "upload")

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, scenario.ID)
	assert.Equal(t, "Linux Docker Quiz", scenario.Title)
	assert.Equal(t, "upload", scenario.SourceType)

	// Verify intro was read from top-level
	assert.Contains(t, scenario.IntroText, "Quiz Introduction")
	assert.Contains(t, scenario.IntroText, "Welcome to the quiz")

	// Verify finish was read from details level
	assert.Contains(t, scenario.FinishText, "Quiz Complete")

	// Verify steps with scripts
	require.Len(t, scenario.Steps, 2)
	assert.Equal(t, "Question 1", scenario.Steps[0].Title)
	assert.Contains(t, scenario.Steps[0].TextContent, "What is Linux")
	assert.NotEmpty(t, scenario.Steps[0].VerifyScript)
	assert.NotEmpty(t, scenario.Steps[0].BackgroundScript)

	assert.Equal(t, "Question 2", scenario.Steps[1].Title)
	assert.Contains(t, scenario.Steps[1].TextContent, "What is Docker")
	assert.NotEmpty(t, scenario.Steps[1].VerifyScript)
	assert.NotEmpty(t, scenario.Steps[1].BackgroundScript)

	// Verify persistence: reload from DB and check scripts survive
	var reloaded models.Scenario
	err = db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&reloaded, "id = ?", scenario.ID).Error
	require.NoError(t, err)

	require.Len(t, reloaded.Steps, 2)
	assert.NotEmpty(t, reloaded.Steps[0].VerifyScript, "step 1 verify script should persist in DB")
	assert.NotEmpty(t, reloaded.Steps[0].BackgroundScript, "step 1 background script should persist in DB")
	assert.NotEmpty(t, reloaded.Steps[1].VerifyScript, "step 2 verify script should persist in DB")
	assert.NotEmpty(t, reloaded.Steps[1].BackgroundScript, "step 2 background script should persist in DB")
}

func TestExtractLocalImagePaths(t *testing.T) {
	t.Run("extracts relative image paths", func(t *testing.T) {
		md := `# Title

![Browse Registry](./assets/step1-1.png)

Some text.

![View Provider](./assets/step1-2.png)
`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 2)
		assert.Contains(t, paths, "./assets/step1-1.png")
		assert.Contains(t, paths, "./assets/step1-2.png")
	})

	t.Run("skips external URLs", func(t *testing.T) {
		md := `![Logo](https://example.com/logo.svg)
![Local](./local.png)
![HTTP](http://cdn.example.com/img.jpg)`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 1)
		assert.Equal(t, "./local.png", paths[0])
	})

	t.Run("skips data URIs", func(t *testing.T) {
		md := `![Inline](data:image/png;base64,iVBOR...)
![Local](assets/real.png)`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 1)
		assert.Equal(t, "assets/real.png", paths[0])
	})

	t.Run("skips non-image extensions", func(t *testing.T) {
		md := `![PDF](./doc.pdf)
![Image](./photo.jpg)
![Script](./run.sh)`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 1)
		assert.Equal(t, "./photo.jpg", paths[0])
	})

	t.Run("deduplicates paths", func(t *testing.T) {
		md := `![A](./img.png)
![B](./img.png)
![C](./img.png)`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 1)
	})

	t.Run("handles multiple image formats", func(t *testing.T) {
		md := `![PNG](a.png)
![JPG](b.jpg)
![JPEG](c.jpeg)
![GIF](d.gif)
![SVG](e.svg)
![WEBP](f.webp)`
		paths := services.ExtractLocalImagePaths(md)
		assert.Len(t, paths, 6)
	})

	t.Run("returns empty for no images", func(t *testing.T) {
		md := "# Just text\n\nNo images here."
		paths := services.ExtractLocalImagePaths(md)
		assert.Empty(t, paths)
	})
}

func TestScenarioImporter_ImportFromDirectory_WithImages(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	// Create step directory with an image
	os.MkdirAll(filepath.Join(tmpDir, "step1", "assets"), 0755)

	// Write a small 1x1 red PNG (valid minimal PNG)
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 pixels
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, // RGB, CRC
		0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, // IDAT chunk
		0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00, 0x00, // compressed
		0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, // CRC
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, // IEND chunk
		0xae, 0x42, 0x60, 0x82,
	}
	err := os.WriteFile(filepath.Join(tmpDir, "step1", "assets", "screenshot.png"), pngData, 0644)
	require.NoError(t, err)

	// Markdown referencing the image
	writeTestFile(t, tmpDir, "step1/text.md", "# Step 1\n\n![Screenshot](./assets/screenshot.png)\n\nSee above.")
	writeTestFile(t, tmpDir, "intro.md", "# Intro\nWelcome.")
	writeTestFile(t, tmpDir, "finish.md", "# Done")

	indexJSON := `{
		"title": "Image Test Lab",
		"description": "Testing image import",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Step With Image", "text": "step1/text.md"}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-img", nil, "upload")

	require.NoError(t, err)
	require.Len(t, scenario.Steps, 1)
	assert.Contains(t, scenario.Steps[0].TextContent, "screenshot.png")

	// Verify image ProjectFile was created
	var imageFiles []models.ProjectFile
	db.Where("scenario_id = ? AND content_type = ?", scenario.ID, "image").Find(&imageFiles)

	require.Len(t, imageFiles, 1)
	assert.Equal(t, "screenshot.png", imageFiles[0].Name)
	assert.Equal(t, "step1/assets/screenshot.png", imageFiles[0].RelPath)
	assert.Equal(t, "image", imageFiles[0].ContentType)
	assert.Equal(t, "image/png", imageFiles[0].MimeType)
	assert.Equal(t, int64(len(pngData)), imageFiles[0].SizeBytes)
	assert.NotEmpty(t, imageFiles[0].Content) // base64 content
	assert.Equal(t, &scenario.ID, imageFiles[0].ScenarioID)
}

func TestScenarioImporter_ImportFromDirectory_UpsertCleansOldImages(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "step1", "assets"), 0755)

	pngData := []byte{0x89, 0x50, 0x4e, 0x47} // minimal "PNG" for test
	os.WriteFile(filepath.Join(tmpDir, "step1", "assets", "old.png"), pngData, 0644)

	writeTestFile(t, tmpDir, "step1/text.md", "![Old](./assets/old.png)")
	writeTestFile(t, tmpDir, "index.json", `{
		"title": "Upsert Image Lab",
		"details": {
			"steps": [{"title": "Step 1", "text": "step1/text.md"}]
		},
		"backend": {"imageid": "ubuntu"}
	}`)

	// First import
	scenario1, err := importer.ImportFromDirectory(tmpDir, "user-1", nil, "upload")
	require.NoError(t, err)

	var count1 int64
	db.Model(&models.ProjectFile{}).Where("scenario_id = ? AND content_type = ?", scenario1.ID, "image").Count(&count1)
	assert.Equal(t, int64(1), count1)

	// Replace image with a new one
	os.Remove(filepath.Join(tmpDir, "step1", "assets", "old.png"))
	os.WriteFile(filepath.Join(tmpDir, "step1", "assets", "new.png"), pngData, 0644)
	writeTestFile(t, tmpDir, "step1/text.md", "![New](./assets/new.png)")

	// Second import (upsert)
	scenario2, err := importer.ImportFromDirectory(tmpDir, "user-1", nil, "upload")
	require.NoError(t, err)
	assert.Equal(t, scenario1.ID, scenario2.ID) // same scenario

	// Old image should be deleted, new one created
	var imageFiles []models.ProjectFile
	db.Where("scenario_id = ? AND content_type = ?", scenario2.ID, "image").Find(&imageFiles)
	require.Len(t, imageFiles, 1)
	assert.Equal(t, "new.png", imageFiles[0].Name)
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	err := os.WriteFile(fullPath, []byte(content), 0644)
	require.NoError(t, err)
}
