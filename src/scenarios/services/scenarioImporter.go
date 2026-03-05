package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// KillerCoda index.json structures

// KillerCodaIndex represents the top-level structure of a KillerCoda-compatible index.json
type KillerCodaIndex struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Difficulty  string                `json:"difficulty"`
	Time        string                `json:"time"`
	Details     KillerCodaDetails     `json:"details"`
	Backend     KillerCodaBackend     `json:"backend"`
	Extensions  *KillerCodaExtensions `json:"extensions,omitempty"`
}

// KillerCodaDetails holds the scenario structure (intro, steps, finish)
type KillerCodaDetails struct {
	Intro  KillerCodaFile    `json:"intro"`
	Steps  []KillerCodaStep  `json:"steps"`
	Finish KillerCodaFile    `json:"finish"`
	Assets *KillerCodaAssets `json:"assets,omitempty"`
}

// KillerCodaFile references a markdown file
type KillerCodaFile struct {
	Text string `json:"text"`
}

// KillerCodaStep describes a single step with its associated files
type KillerCodaStep struct {
	Title      string `json:"title"`
	Text       string `json:"text"`       // path to text.md
	Verify     string `json:"verify"`     // path to verify.sh
	Background string `json:"background"` // path to background.sh
	Foreground string `json:"foreground"` // path to foreground.sh
	Hint       string `json:"hint"`       // OCF extension: path to hint.md
}

// KillerCodaBackend describes the backend image to use
type KillerCodaBackend struct {
	ImageID string `json:"imageid"`
}

// KillerCodaExtensions holds optional platform extensions
type KillerCodaExtensions struct {
	OCF *KillerCodaOCF `json:"ocf,omitempty"`
}

// KillerCodaOCF holds OCF-specific configuration extensions
type KillerCodaOCF struct {
	Flags      bool `json:"flags"`
	CrashTraps bool `json:"crash_traps"`
	GshEnabled bool `json:"gsh_enabled"`
}

// KillerCodaAssets describes files to copy into the environment
type KillerCodaAssets struct {
	Host01 []KillerCodaAsset `json:"host01,omitempty"`
}

// KillerCodaAsset describes a single asset file
type KillerCodaAsset struct {
	File   string `json:"file"`
	Target string `json:"target"`
	Chmod  string `json:"chmod"`
}

// ScenarioImporterService handles importing scenarios from KillerCoda-compatible directories
type ScenarioImporterService struct {
	db *gorm.DB
}

// NewScenarioImporterService creates a new importer service
func NewScenarioImporterService(db *gorm.DB) *ScenarioImporterService {
	return &ScenarioImporterService{db: db}
}

// ImportFromDirectory parses a local directory containing a KillerCoda-format scenario.
// It reads index.json and all referenced files, creating a Scenario with Steps in the database.
func (s *ScenarioImporterService) ImportFromDirectory(dirPath string, createdByID string, orgID *uuid.UUID) (*models.Scenario, error) {
	indexPath := filepath.Join(dirPath, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index.json: %w", err)
	}

	index, err := s.ParseIndexJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse index.json: %w", err)
	}

	scenario, err := s.BuildScenarioFromIndex(index, dirPath, createdByID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to build scenario: %w", err)
	}

	// Save in a transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(scenario).Error; err != nil {
			return fmt.Errorf("failed to save scenario: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return scenario, nil
}

// ParseIndexJSON parses a KillerCoda index.json file and returns the structured data.
func (s *ScenarioImporterService) ParseIndexJSON(data []byte) (*KillerCodaIndex, error) {
	var index KillerCodaIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &index, nil
}

// BuildScenarioFromIndex creates a Scenario model from parsed KillerCoda data.
// dirPath is used to read the referenced markdown and script files.
func (s *ScenarioImporterService) BuildScenarioFromIndex(index *KillerCodaIndex, dirPath string, createdByID string, orgID *uuid.UUID) (*models.Scenario, error) {
	// Determine OCF extensions
	flagsEnabled := false
	crashTraps := false
	gshEnabled := false
	if index.Extensions != nil && index.Extensions.OCF != nil {
		flagsEnabled = index.Extensions.OCF.Flags
		crashTraps = index.Extensions.OCF.CrashTraps
		gshEnabled = index.Extensions.OCF.GshEnabled
	}

	// Generate flag secret if flags are enabled
	var flagSecret string
	if flagsEnabled {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("failed to generate flag secret: %w", err)
		}
		flagSecret = hex.EncodeToString(secretBytes)
	}

	// Read intro and finish markdown
	introText := readFileContent(dirPath, index.Details.Intro.Text)
	finishText := readFileContent(dirPath, index.Details.Finish.Text)

	scenario := &models.Scenario{
		Name:           generateScenarioName(index.Title),
		Title:          index.Title,
		Description:    index.Description,
		Difficulty:     index.Difficulty,
		EstimatedTime:  index.Time,
		InstanceType:   index.Backend.ImageID,
		SourceType:     "builtin",
		FlagsEnabled:   flagsEnabled,
		FlagSecret:     flagSecret,
		GshEnabled:     gshEnabled,
		CrashTraps:     crashTraps,
		IntroText:      introText,
		FinishText:     finishText,
		CreatedByID:    createdByID,
		OrganizationID: orgID,
	}

	// Build steps
	steps := make([]models.ScenarioStep, 0, len(index.Details.Steps))
	for i, kcStep := range index.Details.Steps {
		step := models.ScenarioStep{
			Order:            i,
			Title:            kcStep.Title,
			TextContent:      readFileContent(dirPath, kcStep.Text),
			HintContent:      readFileContent(dirPath, kcStep.Hint),
			VerifyScript:     readFileContent(dirPath, kcStep.Verify),
			BackgroundScript: readFileContent(dirPath, kcStep.Background),
			ForegroundScript: readFileContent(dirPath, kcStep.Foreground),
			HasFlag:          flagsEnabled,
		}
		steps = append(steps, step)
	}

	// Sort steps by order for consistency
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Order < steps[j].Order
	})

	scenario.Steps = steps

	return scenario, nil
}

// readFileContent reads a file relative to dirPath, returning empty string if the file
// doesn't exist or the path is empty.
func readFileContent(dirPath string, relPath string) string {
	if relPath == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dirPath, relPath))
	if err != nil {
		return ""
	}
	return string(data)
}

// generateScenarioName creates a URL-friendly name from a title
func generateScenarioName(title string) string {
	name := ""
	for _, c := range title {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			name += string(c)
		} else if c >= 'A' && c <= 'Z' {
			name += string(c - 'A' + 'a')
		} else if c == ' ' || c == '_' {
			name += "-"
		}
	}
	// Remove consecutive dashes
	result := ""
	prev := byte(0)
	for i := 0; i < len(name); i++ {
		if name[i] == '-' && prev == '-' {
			continue
		}
		result += string(name[i])
		prev = name[i]
	}
	// Trim leading/trailing dashes
	for len(result) > 0 && result[0] == '-' {
		result = result[1:]
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return result
}
