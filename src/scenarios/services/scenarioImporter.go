package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/utils"
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
	HasFlag    *bool  `json:"has_flag"`   // OCF extension: per-step flag override (nil = use scenario default)
	FlagPath   string `json:"flag_path"`  // OCF extension: where to place flag in container
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

	// Upsert: check if scenario with same name already exists
	var existing models.Scenario
	if err := s.db.Where("name = ?", scenario.Name).First(&existing).Error; err == nil {
		// Update existing scenario
		if existing.FlagSecret != "" && scenario.FlagsEnabled {
			scenario.FlagSecret = existing.FlagSecret // preserve flag secret
		}

		err = s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&existing).Updates(map[string]any{
				"title":          scenario.Title,
				"description":    scenario.Description,
				"difficulty":     scenario.Difficulty,
				"estimated_time": scenario.EstimatedTime,
				"instance_type":  scenario.InstanceType,
				"flags_enabled":  scenario.FlagsEnabled,
				"flag_secret":    scenario.FlagSecret,
				"gsh_enabled":    scenario.GshEnabled,
				"crash_traps":    scenario.CrashTraps,
				"intro_text":     scenario.IntroText,
				"finish_text":    scenario.FinishText,
			}).Error; err != nil {
				return fmt.Errorf("failed to update scenario: %w", err)
			}

			// Delete old steps, create new ones
			if err := tx.Where("scenario_id = ?", existing.ID).Delete(&models.ScenarioStep{}).Error; err != nil {
				return fmt.Errorf("failed to delete old steps: %w", err)
			}
			for i := range scenario.Steps {
				scenario.Steps[i].ScenarioID = existing.ID
				if err := tx.Create(&scenario.Steps[i]).Error; err != nil {
					return fmt.Errorf("failed to create step: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Reload
		if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).First(&existing, "id = ?", existing.ID).Error; err != nil {
			return nil, fmt.Errorf("failed to reload scenario: %w", err)
		}
		return &existing, nil
	}

	// Create new scenario
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
		Name:           utils.GenerateSlug(index.Title),
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
		// Per-step has_flag override: if specified, use it; otherwise fall back to scenario-level flagsEnabled
		stepHasFlag := flagsEnabled
		if kcStep.HasFlag != nil {
			stepHasFlag = *kcStep.HasFlag
		}

		step := models.ScenarioStep{
			Order:            i,
			Title:            kcStep.Title,
			TextContent:      readFileContent(dirPath, kcStep.Text),
			HintContent:      readFileContent(dirPath, kcStep.Hint),
			VerifyScript:     readFileContent(dirPath, kcStep.Verify),
			BackgroundScript: readFileContent(dirPath, kcStep.Background),
			ForegroundScript: readFileContent(dirPath, kcStep.Foreground),
			HasFlag:          stepHasFlag,
			FlagPath:         kcStep.FlagPath,
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
// doesn't exist or the path is empty. It includes path traversal protection to ensure
// the resolved path stays within dirPath.
func readFileContent(dirPath string, relPath string) string {
	if relPath == "" {
		return ""
	}
	// Sanitize: resolve the full path and ensure it stays within dirPath
	fullPath := filepath.Join(dirPath, relPath)
	cleanPath := filepath.Clean(fullPath)
	cleanDir := filepath.Clean(dirPath)
	if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) && cleanPath != cleanDir {
		return ""
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return ""
	}
	return string(data)
}

