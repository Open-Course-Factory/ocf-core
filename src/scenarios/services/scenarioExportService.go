package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/utils"
)

// ScenarioExportService handles exporting scenarios to JSON or KillerCoda archive format
type ScenarioExportService struct {
	db *gorm.DB
}

// NewScenarioExportService creates a new export service
func NewScenarioExportService(db *gorm.DB) *ScenarioExportService {
	return &ScenarioExportService{db: db}
}

// ExportAsJSON loads a scenario with steps and returns the export DTO
func (s *ScenarioExportService) ExportAsJSON(scenarioID uuid.UUID) (*dto.ScenarioExportOutput, error) {
	var scenario models.Scenario
	if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	return s.buildExportOutput(&scenario), nil
}

// ExportAsArchive loads a scenario with steps and returns a KillerCoda-compatible zip archive.
// Returns (zipBytes, filename, error).
func (s *ScenarioExportService) ExportAsArchive(scenarioID uuid.UUID) ([]byte, string, error) {
	var scenario models.Scenario
	if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, "", fmt.Errorf("scenario not found: %w", err)
	}

	zipBytes, err := s.buildArchive(&scenario)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build archive: %w", err)
	}

	filename := utils.GenerateSlug(scenario.Title) + ".zip"
	return zipBytes, filename, nil
}

// ExportMultipleAsJSON loads multiple scenarios with steps and returns export DTOs
func (s *ScenarioExportService) ExportMultipleAsJSON(scenarioIDs []uuid.UUID) ([]dto.ScenarioExportOutput, error) {
	var scenarios []models.Scenario
	if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Where("id IN ?", scenarioIDs).Find(&scenarios).Error; err != nil {
		return nil, fmt.Errorf("failed to load scenarios: %w", err)
	}

	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios found for the given IDs")
	}

	outputs := make([]dto.ScenarioExportOutput, 0, len(scenarios))
	for i := range scenarios {
		outputs = append(outputs, *s.buildExportOutput(&scenarios[i]))
	}
	return outputs, nil
}

// buildExportOutput converts a Scenario model to a ScenarioExportOutput DTO
func (s *ScenarioExportService) buildExportOutput(scenario *models.Scenario) *dto.ScenarioExportOutput {
	steps := make([]dto.ScenarioExportStepOutput, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		steps = append(steps, dto.ScenarioExportStepOutput{
			Order:            step.Order,
			Title:            step.Title,
			TextContent:      step.TextContent,
			HintContent:      step.HintContent,
			VerifyScript:     step.VerifyScript,
			BackgroundScript: step.BackgroundScript,
			ForegroundScript: step.ForegroundScript,
			HasFlag:          step.HasFlag,
			FlagPath:         step.FlagPath,
			FlagLevel:        step.FlagLevel,
		})
	}

	return &dto.ScenarioExportOutput{
		Title:         scenario.Title,
		Description:   scenario.Description,
		Difficulty:    scenario.Difficulty,
		EstimatedTime: scenario.EstimatedTime,
		InstanceType:  scenario.InstanceType,
		OsType:        scenario.OsType,
		FlagsEnabled:  scenario.FlagsEnabled,
		GshEnabled:    scenario.GshEnabled,
		CrashTraps:    scenario.CrashTraps,
		IntroText:     scenario.IntroText,
		FinishText:    scenario.FinishText,
		Steps:         steps,
	}
}

// buildArchive generates a KillerCoda-compatible zip archive from a scenario
func (s *ScenarioExportService) buildArchive(scenario *models.Scenario) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Build KillerCoda index.json
	index := s.buildKillerCodaIndex(scenario)
	indexJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index.json: %w", err)
	}

	if err := addFileToZip(w, "index.json", indexJSON); err != nil {
		return nil, err
	}

	// Write intro.md
	if scenario.IntroText != "" {
		if err := addFileToZip(w, "intro.md", []byte(scenario.IntroText)); err != nil {
			return nil, err
		}
	}

	// Write finish.md
	if scenario.FinishText != "" {
		if err := addFileToZip(w, "finish.md", []byte(scenario.FinishText)); err != nil {
			return nil, err
		}
	}

	// Write step files
	for i, step := range scenario.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)

		if step.TextContent != "" {
			if err := addFileToZip(w, stepDir+"/text.md", []byte(step.TextContent)); err != nil {
				return nil, err
			}
		}
		if step.HintContent != "" {
			if err := addFileToZip(w, stepDir+"/hint.md", []byte(step.HintContent)); err != nil {
				return nil, err
			}
		}
		if step.VerifyScript != "" {
			if err := addFileToZip(w, stepDir+"/verify.sh", []byte(step.VerifyScript)); err != nil {
				return nil, err
			}
		}
		if step.BackgroundScript != "" {
			if err := addFileToZip(w, stepDir+"/background.sh", []byte(step.BackgroundScript)); err != nil {
				return nil, err
			}
		}
		if step.ForegroundScript != "" {
			if err := addFileToZip(w, stepDir+"/foreground.sh", []byte(step.ForegroundScript)); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// buildKillerCodaIndex constructs the KillerCoda index.json structure from a scenario
func (s *ScenarioExportService) buildKillerCodaIndex(scenario *models.Scenario) *KillerCodaIndex {
	details := KillerCodaDetails{
		Steps: make([]KillerCodaStep, 0, len(scenario.Steps)),
	}

	if scenario.IntroText != "" {
		details.Intro = KillerCodaFile{Text: "intro.md"}
	}
	if scenario.FinishText != "" {
		details.Finish = KillerCodaFile{Text: "finish.md"}
	}

	for i, step := range scenario.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)
		kcStep := KillerCodaStep{
			Title: step.Title,
		}

		if step.TextContent != "" {
			kcStep.Text = stepDir + "/text.md"
		}
		if step.HintContent != "" {
			kcStep.Hint = stepDir + "/hint.md"
		}
		if step.VerifyScript != "" {
			kcStep.Verify = stepDir + "/verify.sh"
		}
		if step.BackgroundScript != "" {
			kcStep.Background = stepDir + "/background.sh"
		}
		if step.ForegroundScript != "" {
			kcStep.Foreground = stepDir + "/foreground.sh"
		}

		// Set per-step flag override if different from scenario default
		hasFlag := step.HasFlag
		kcStep.HasFlag = &hasFlag
		if step.FlagPath != "" {
			kcStep.FlagPath = step.FlagPath
		}

		details.Steps = append(details.Steps, kcStep)
	}

	index := &KillerCodaIndex{
		Title:       scenario.Title,
		Description: scenario.Description,
		Difficulty:  scenario.Difficulty,
		Time:        scenario.EstimatedTime,
		Details:     details,
		Backend:     KillerCodaBackend{ImageID: scenario.InstanceType},
	}

	// Add OCF extensions if any are enabled
	if scenario.FlagsEnabled || scenario.CrashTraps || scenario.GshEnabled {
		index.Extensions = &KillerCodaExtensions{
			OCF: &KillerCodaOCF{
				Flags:      scenario.FlagsEnabled,
				CrashTraps: scenario.CrashTraps,
				GshEnabled: scenario.GshEnabled,
			},
		}
	}

	// Add os_type to backend ImageID suffix if present (for roundtrip)
	// The importer reads from backend.imageid, so we keep them together
	if scenario.OsType != "" && !strings.Contains(scenario.InstanceType, scenario.OsType) {
		// Store os_type info — the importer doesn't read it from index.json,
		// but it's included in JSON export, so we just leave ImageID as-is
	}

	return index
}

// addFileToZip adds a file with the given content to the zip writer
func addFileToZip(w *zip.Writer, name string, content []byte) error {
	f, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("failed to create %s in zip: %w", name, err)
	}
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("failed to write %s in zip: %w", name, err)
	}
	return nil
}
