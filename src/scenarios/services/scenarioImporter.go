package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
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
	Intro       KillerCodaFile        `json:"intro"`
	Finish      KillerCodaFile        `json:"finish"`
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
	Text       string `json:"text"`
	Background string `json:"background"` // path to background.sh (intro only)
	Foreground string `json:"foreground"` // path to foreground.sh (intro only)
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
// sourceType indicates the origin of the scenario (e.g. "builtin", "upload", "seed").
// If empty, it defaults to "builtin".
func (s *ScenarioImporterService) ImportFromDirectory(dirPath string, createdByID string, orgID *uuid.UUID, sourceType string) (*models.Scenario, error) {
	if sourceType == "" {
		sourceType = "builtin"
	}
	indexPath := filepath.Join(dirPath, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read index.json: %w", err)
	}

	index, err := s.ParseIndexJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse index.json: %w", err)
	}

	scenario, err := s.BuildScenarioFromIndex(index, dirPath, createdByID, orgID, sourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to build scenario: %w", err)
	}

	// Build step path metadata from the KillerCoda index for ProjectFile RelPaths
	stepRelPaths := buildStepRelPaths(index)

	// Upsert: check if scenario with same name already exists
	// When orgID is set (group-level import), scope lookup to the same organization
	// to prevent cross-tenant overwrites.
	var existing models.Scenario
	upsertQuery := s.db.Where("name = ?", scenario.Name)
	if orgID != nil {
		upsertQuery = upsertQuery.Where("organization_id = ?", *orgID)
	}
	if err := upsertQuery.First(&existing).Error; err == nil {
		// Update existing scenario
		if existing.FlagSecret != "" && scenario.FlagsEnabled {
			scenario.FlagSecret = existing.FlagSecret // preserve flag secret
		}

		err = s.db.Transaction(func(tx *gorm.DB) error {
			// Collect old ProjectFile IDs from scenario and steps
			oldFileIDs := collectProjectFileIDs(tx, existing.ID)

			// Null out scenario-level FKs before deleting files
			if err := tx.Model(&existing).Updates(map[string]any{
				"title":           scenario.Title,
				"description":     scenario.Description,
				"difficulty":      scenario.Difficulty,
				"estimated_time":  scenario.EstimatedTime,
				"instance_type":   scenario.InstanceType,
				"flags_enabled":   scenario.FlagsEnabled,
				"flag_secret":     scenario.FlagSecret,
				"gsh_enabled":     scenario.GshEnabled,
				"crash_traps":     scenario.CrashTraps,
				"intro_text":      scenario.IntroText,
				"finish_text":     scenario.FinishText,
				"setup_script":    scenario.SetupScript,
				"setup_script_id": nil,
				"intro_file_id":   nil,
				"finish_file_id":  nil,
			}).Error; err != nil {
				return fmt.Errorf("failed to update scenario: %w", err)
			}

			// Delete old hints before steps (soft-delete won't cascade)
			if err := tx.Where("step_id IN (?)",
				tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", existing.ID),
			).Delete(&models.ScenarioStepHint{}).Error; err != nil {
				return fmt.Errorf("failed to delete old hints: %w", err)
			}
			// Delete old steps
			if err := tx.Where("scenario_id = ?", existing.ID).Delete(&models.ScenarioStep{}).Error; err != nil {
				return fmt.Errorf("failed to delete old steps: %w", err)
			}
			// Delete old ProjectFiles
			if len(oldFileIDs) > 0 {
				if err := tx.Where("id IN ?", oldFileIDs).Delete(&models.ProjectFile{}).Error; err != nil {
					return fmt.Errorf("failed to delete old project files: %w", err)
				}
			}

			// Create new steps
			for i := range scenario.Steps {
				scenario.Steps[i].ScenarioID = existing.ID
				if err := tx.Create(&scenario.Steps[i]).Error; err != nil {
					return fmt.Errorf("failed to create step: %w", err)
				}
			}

			// Create ProjectFiles for scenario and steps (dual-write)
			if err := createProjectFilesForScenario(tx, &existing, scenario, stepRelPaths); err != nil {
				return err
			}

			// Import images referenced in markdown content
			if err := importScenarioImages(tx, existing.ID, dirPath, index, scenario); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return nil, err
		}

		// Reload
		if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).Preload("Steps.Hints", func(db *gorm.DB) *gorm.DB {
			return db.Order("level ASC")
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

		// Create ProjectFiles for scenario and steps (dual-write)
		if err := createProjectFilesForScenario(tx, scenario, scenario, stepRelPaths); err != nil {
			return err
		}

		// Import images referenced in markdown content
		if err := importScenarioImages(tx, scenario.ID, dirPath, index, scenario); err != nil {
			return err
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
// sourceType indicates the origin of the scenario (e.g. "builtin", "upload", "seed").
func (s *ScenarioImporterService) BuildScenarioFromIndex(index *KillerCodaIndex, dirPath string, createdByID string, orgID *uuid.UUID, sourceType string) (*models.Scenario, error) {
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

	// Read intro and finish markdown (support both details-level and top-level locations)
	introFile := index.Details.Intro.Text
	if introFile == "" {
		introFile = index.Intro.Text
	}
	finishFile := index.Details.Finish.Text
	if finishFile == "" {
		finishFile = index.Finish.Text
	}
	introText := readFileContent(dirPath, introFile)
	finishText := readFileContent(dirPath, finishFile)

	// Read global background/foreground scripts (KillerCoda intro-level scripts)
	setupScript := readFileContent(dirPath, index.Details.Intro.Background)
	if setupScript == "" {
		setupScript = readFileContent(dirPath, index.Intro.Background)
	}

	scenario := &models.Scenario{
		Name:           utils.GenerateSlug(index.Title),
		Title:          index.Title,
		Description:    index.Description,
		Difficulty:     index.Difficulty,
		EstimatedTime:  index.Time,
		InstanceType:   index.Backend.ImageID,
		SourceType:     sourceType,
		FlagsEnabled:   flagsEnabled,
		FlagSecret:     flagSecret,
		GshEnabled:     gshEnabled,
		CrashTraps:     crashTraps,
		IntroText:      introText,
		FinishText:     finishText,
		SetupScript:    setupScript,
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

		// Build progressive hints from hint content
		if step.HintContent != "" {
			parts := SplitHintContent(step.HintContent)
			hints := make([]models.ScenarioStepHint, len(parts))
			for j, part := range parts {
				hints[j] = models.ScenarioStepHint{
					Level:   j + 1,
					Content: part,
				}
			}
			step.Hints = hints
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

// stepRelPathInfo holds KillerCoda original relative paths for a step's files.
type stepRelPathInfo struct {
	Verify     string
	Background string
	Foreground string
	Text       string
	Hint       string
}

// buildStepRelPaths extracts the original KillerCoda relative paths from the index
// so they can be preserved in ProjectFile records for round-trip fidelity.
func buildStepRelPaths(index *KillerCodaIndex) []stepRelPathInfo {
	result := make([]stepRelPathInfo, len(index.Details.Steps))
	for i, kcStep := range index.Details.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)
		result[i] = stepRelPathInfo{
			Verify:     defaultRelPath(kcStep.Verify, stepDir+"/verify.sh"),
			Background: defaultRelPath(kcStep.Background, stepDir+"/background.sh"),
			Foreground: defaultRelPath(kcStep.Foreground, stepDir+"/foreground.sh"),
			Text:       defaultRelPath(kcStep.Text, stepDir+"/text.md"),
			Hint:       defaultRelPath(kcStep.Hint, stepDir+"/hint.md"),
		}
	}
	return result
}

// defaultRelPath returns kcPath if non-empty, otherwise fallback.
func defaultRelPath(kcPath, fallback string) string {
	if kcPath != "" {
		return kcPath
	}
	return fallback
}

// collectProjectFileIDs gathers all ProjectFile IDs referenced by a scenario and its steps.
func collectProjectFileIDs(tx *gorm.DB, scenarioID uuid.UUID) []uuid.UUID {
	var ids []uuid.UUID

	// Scenario-level FKs
	var scenario models.Scenario
	if err := tx.Select("setup_script_id, intro_file_id, finish_file_id").First(&scenario, "id = ?", scenarioID).Error; err == nil {
		if scenario.SetupScriptID != nil {
			ids = append(ids, *scenario.SetupScriptID)
		}
		if scenario.IntroFileID != nil {
			ids = append(ids, *scenario.IntroFileID)
		}
		if scenario.FinishFileID != nil {
			ids = append(ids, *scenario.FinishFileID)
		}
	}

	// Step-level FKs
	var steps []models.ScenarioStep
	tx.Select("verify_script_id, background_script_id, foreground_script_id, text_file_id, hint_file_id").
		Where("scenario_id = ?", scenarioID).Find(&steps)
	for _, step := range steps {
		if step.VerifyScriptID != nil {
			ids = append(ids, *step.VerifyScriptID)
		}
		if step.BackgroundScriptID != nil {
			ids = append(ids, *step.BackgroundScriptID)
		}
		if step.ForegroundScriptID != nil {
			ids = append(ids, *step.ForegroundScriptID)
		}
		if step.TextFileID != nil {
			ids = append(ids, *step.TextFileID)
		}
		if step.HintFileID != nil {
			ids = append(ids, *step.HintFileID)
		}
	}

	// Image files linked via ScenarioID
	var imageFiles []models.ProjectFile
	tx.Select("id").Where("scenario_id = ? AND content_type = ?", scenarioID, "image").Find(&imageFiles)
	for _, f := range imageFiles {
		ids = append(ids, f.ID)
	}

	return ids
}

// createProjectFilesForScenario creates ProjectFile records for a scenario and its steps,
// and updates the FKs accordingly. dbScenario is the persisted scenario (with valid ID),
// srcScenario provides the inline content, and stepRelPaths provides original KillerCoda paths.
func createProjectFilesForScenario(tx *gorm.DB, dbScenario *models.Scenario, srcScenario *models.Scenario, stepRelPaths []stepRelPathInfo) error {
	// Scenario-level setup script (global background)
	if srcScenario.SetupScript != "" {
		file := models.ProjectFile{
			Name:        "background.sh",
			RelPath:     "background.sh",
			ContentType: "script",
			Content:     srcScenario.SetupScript,
			StorageType: "database",
			SizeBytes:   int64(len(srcScenario.SetupScript)),
		}
		if err := tx.Create(&file).Error; err != nil {
			return fmt.Errorf("failed to create setup script ProjectFile: %w", err)
		}
		if err := tx.Model(dbScenario).Update("setup_script_id", file.ID).Error; err != nil {
			return fmt.Errorf("failed to update setup_script_id: %w", err)
		}
	}

	// Scenario-level files
	if srcScenario.IntroText != "" {
		file := models.ProjectFile{
			Name:        "intro.md",
			RelPath:     "intro.md",
			ContentType: "markdown",
			Content:     srcScenario.IntroText,
			StorageType: "database",
			SizeBytes:   int64(len(srcScenario.IntroText)),
		}
		if err := tx.Create(&file).Error; err != nil {
			return fmt.Errorf("failed to create intro ProjectFile: %w", err)
		}
		if err := tx.Model(dbScenario).Update("intro_file_id", file.ID).Error; err != nil {
			return fmt.Errorf("failed to update intro_file_id: %w", err)
		}
	}

	if srcScenario.FinishText != "" {
		file := models.ProjectFile{
			Name:        "finish.md",
			RelPath:     "finish.md",
			ContentType: "markdown",
			Content:     srcScenario.FinishText,
			StorageType: "database",
			SizeBytes:   int64(len(srcScenario.FinishText)),
		}
		if err := tx.Create(&file).Error; err != nil {
			return fmt.Errorf("failed to create finish ProjectFile: %w", err)
		}
		if err := tx.Model(dbScenario).Update("finish_file_id", file.ID).Error; err != nil {
			return fmt.Errorf("failed to update finish_file_id: %w", err)
		}
	}

	// Step-level files — reload steps from DB to get their persisted IDs
	var dbSteps []models.ScenarioStep
	tx.Where("scenario_id = ?", dbScenario.ID).Order("\"order\" ASC").Find(&dbSteps)

	for _, dbStep := range dbSteps {
		// Find matching source step by order
		var srcStep *models.ScenarioStep
		for j := range srcScenario.Steps {
			if srcScenario.Steps[j].Order == dbStep.Order {
				srcStep = &srcScenario.Steps[j]
				break
			}
		}
		if srcStep == nil {
			continue
		}

		// Get rel path info for this step (by order index)
		var relPaths stepRelPathInfo
		if dbStep.Order < len(stepRelPaths) {
			relPaths = stepRelPaths[dbStep.Order]
		} else {
			stepDir := fmt.Sprintf("step%d", dbStep.Order+1)
			relPaths = stepRelPathInfo{
				Verify:     stepDir + "/verify.sh",
				Background: stepDir + "/background.sh",
				Foreground: stepDir + "/foreground.sh",
				Text:       stepDir + "/text.md",
				Hint:       stepDir + "/hint.md",
			}
		}

		if srcStep.VerifyScript != "" {
			file := models.ProjectFile{
				Name:        "verify.sh",
				RelPath:     relPaths.Verify,
				ContentType: "script",
				Content:     srcStep.VerifyScript,
				StorageType: "database",
				SizeBytes:   int64(len(srcStep.VerifyScript)),
			}
			if err := tx.Create(&file).Error; err != nil {
				return fmt.Errorf("failed to create verify ProjectFile: %w", err)
			}
			if err := tx.Model(&dbStep).Update("verify_script_id", file.ID).Error; err != nil {
				return fmt.Errorf("failed to update verify_script_id: %w", err)
			}
		}

		if srcStep.BackgroundScript != "" {
			file := models.ProjectFile{
				Name:        "background.sh",
				RelPath:     relPaths.Background,
				ContentType: "script",
				Content:     srcStep.BackgroundScript,
				StorageType: "database",
				SizeBytes:   int64(len(srcStep.BackgroundScript)),
			}
			if err := tx.Create(&file).Error; err != nil {
				return fmt.Errorf("failed to create background ProjectFile: %w", err)
			}
			if err := tx.Model(&dbStep).Update("background_script_id", file.ID).Error; err != nil {
				return fmt.Errorf("failed to update background_script_id: %w", err)
			}
		}

		if srcStep.ForegroundScript != "" {
			file := models.ProjectFile{
				Name:        "foreground.sh",
				RelPath:     relPaths.Foreground,
				ContentType: "script",
				Content:     srcStep.ForegroundScript,
				StorageType: "database",
				SizeBytes:   int64(len(srcStep.ForegroundScript)),
			}
			if err := tx.Create(&file).Error; err != nil {
				return fmt.Errorf("failed to create foreground ProjectFile: %w", err)
			}
			if err := tx.Model(&dbStep).Update("foreground_script_id", file.ID).Error; err != nil {
				return fmt.Errorf("failed to update foreground_script_id: %w", err)
			}
		}

		if srcStep.TextContent != "" {
			file := models.ProjectFile{
				Name:        "text.md",
				RelPath:     relPaths.Text,
				ContentType: "markdown",
				Content:     srcStep.TextContent,
				StorageType: "database",
				SizeBytes:   int64(len(srcStep.TextContent)),
			}
			if err := tx.Create(&file).Error; err != nil {
				return fmt.Errorf("failed to create text ProjectFile: %w", err)
			}
			if err := tx.Model(&dbStep).Update("text_file_id", file.ID).Error; err != nil {
				return fmt.Errorf("failed to update text_file_id: %w", err)
			}
		}

		if srcStep.HintContent != "" {
			file := models.ProjectFile{
				Name:        "hint.md",
				RelPath:     relPaths.Hint,
				ContentType: "markdown",
				Content:     srcStep.HintContent,
				StorageType: "database",
				SizeBytes:   int64(len(srcStep.HintContent)),
			}
			if err := tx.Create(&file).Error; err != nil {
				return fmt.Errorf("failed to create hint ProjectFile: %w", err)
			}
			if err := tx.Model(&dbStep).Update("hint_file_id", file.ID).Error; err != nil {
				return fmt.Errorf("failed to update hint_file_id: %w", err)
			}
		}
	}

	return nil
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

// readBinaryFileBase64 reads a binary file and returns its content as base64.
// Returns empty string if the file doesn't exist or can't be read.
func readBinaryFileBase64(dirPath string, relPath string) (string, int64) {
	if relPath == "" {
		return "", 0
	}
	fullPath := filepath.Join(dirPath, relPath)
	cleanPath := filepath.Clean(fullPath)
	cleanDir := filepath.Clean(dirPath)
	if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) && cleanPath != cleanDir {
		return "", 0
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", 0
	}
	return base64.StdEncoding.EncodeToString(data), int64(len(data))
}

// imageRefRegex matches markdown image references: ![alt](path)
var imageRefRegex = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// imageExtensions lists file extensions treated as images
var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".webp": true, ".ico": true, ".bmp": true,
}

// ExtractLocalImagePaths finds all local image paths referenced in markdown.
// Skips external URLs (http://, https://) and data URIs.
func ExtractLocalImagePaths(markdown string) []string {
	matches := imageRefRegex.FindAllStringSubmatch(markdown, -1)
	seen := make(map[string]bool)
	var paths []string
	for _, m := range matches {
		path := m[1]
		// Skip external URLs and data URIs
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "data:") {
			continue
		}
		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		if !imageExtensions[ext] {
			continue
		}
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

// importScenarioImages scans all markdown content for image references,
// reads the image files from the directory, and creates ProjectFile records.
func importScenarioImages(tx *gorm.DB, scenarioID uuid.UUID, dirPath string, index *KillerCodaIndex, srcScenario *models.Scenario) error {
	// Collect (markdownContent, markdownFileDir) pairs to scan for images
	type mdSource struct {
		content string
		dir     string // directory the markdown file lives in, relative to dirPath
	}

	var sources []mdSource

	// Scenario-level markdown
	if srcScenario.IntroText != "" {
		sources = append(sources, mdSource{srcScenario.IntroText, ""})
	}
	if srcScenario.FinishText != "" {
		sources = append(sources, mdSource{srcScenario.FinishText, ""})
	}

	// Step-level markdown
	for i, step := range srcScenario.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)
		// Use KillerCoda path to determine the actual step directory
		if i < len(index.Details.Steps) && index.Details.Steps[i].Text != "" {
			stepDir = filepath.Dir(index.Details.Steps[i].Text)
		}
		if step.TextContent != "" {
			sources = append(sources, mdSource{step.TextContent, stepDir})
		}
		if step.HintContent != "" {
			sources = append(sources, mdSource{step.HintContent, stepDir})
		}
	}

	// Extract all unique image paths (resolved to scenario root)
	seen := make(map[string]bool)
	for _, src := range sources {
		localPaths := ExtractLocalImagePaths(src.content)
		for _, imgPath := range localPaths {
			// Resolve relative to the markdown file's directory
			resolved := filepath.Join(src.dir, imgPath)
			resolved = filepath.Clean(resolved)
			if !seen[resolved] {
				seen[resolved] = true
			}
		}
	}

	// Import each image (skip files over 5MB to protect database from bloat)
	const maxImageBytes int64 = 5 * 1024 * 1024
	for relPath := range seen {
		content, sizeBytes := readBinaryFileBase64(dirPath, relPath)
		if content == "" {
			continue // Image file not found in directory
		}
		if sizeBytes > maxImageBytes {
			continue // Skip oversized images
		}

		ext := strings.ToLower(filepath.Ext(relPath))
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		file := models.ProjectFile{
			Name:        filepath.Base(relPath),
			RelPath:     relPath,
			ContentType: "image",
			MimeType:    mimeType,
			Content:     content,
			StorageType: "database",
			SizeBytes:   sizeBytes,
			ScenarioID:  &scenarioID,
		}
		if err := tx.Create(&file).Error; err != nil {
			return fmt.Errorf("failed to create image ProjectFile %s: %w", relPath, err)
		}
	}

	return nil
}

