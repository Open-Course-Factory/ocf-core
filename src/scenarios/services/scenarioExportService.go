package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"

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
	}).Preload("Steps.Questions", func(db *gorm.DB) *gorm.DB {
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
	}).Preload("Steps.Questions", func(db *gorm.DB) *gorm.DB {
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
	}).Preload("Steps.Questions", func(db *gorm.DB) *gorm.DB {
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

// buildExportOutput converts a Scenario model to a ScenarioExportOutput DTO.
// Resolves content from ProjectFile when available, falling back to inline fields.
func (s *ScenarioExportService) buildExportOutput(scenario *models.Scenario) *dto.ScenarioExportOutput {
	introText := ResolveScriptContent(s.db, scenario.IntroFileID, scenario.IntroText)
	finishText := ResolveScriptContent(s.db, scenario.FinishFileID, scenario.FinishText)
	setupScript := ResolveScriptContent(s.db, scenario.SetupScriptID, scenario.SetupScript)

	steps := make([]dto.ScenarioExportStepOutput, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		stepType := step.StepType
		if stepType == "" {
			stepType = "terminal"
		}

		var questions []dto.ScenarioExportStepQuestionOutput
		if len(step.Questions) > 0 {
			questions = make([]dto.ScenarioExportStepQuestionOutput, 0, len(step.Questions))
			for _, q := range step.Questions {
				questions = append(questions, dto.ScenarioExportStepQuestionOutput{
					Order:         q.Order,
					QuestionText:  q.QuestionText,
					QuestionType:  q.QuestionType,
					Options:       q.Options,
					CorrectAnswer: q.CorrectAnswer,
					Explanation:   q.Explanation,
					Points:        q.Points,
				})
			}
		}

		steps = append(steps, dto.ScenarioExportStepOutput{
			Order:                 step.Order,
			Title:                 step.Title,
			StepType:              stepType,
			ShowImmediateFeedback: step.ShowImmediateFeedback,
			TextContent:           ResolveScriptContent(s.db, step.TextFileID, step.TextContent),
			HintContent:           ResolveScriptContent(s.db, step.HintFileID, step.HintContent),
			VerifyScript:          ResolveScriptContent(s.db, step.VerifyScriptID, step.VerifyScript),
			BackgroundScript:      ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript),
			ForegroundScript:      ResolveScriptContent(s.db, step.ForegroundScriptID, step.ForegroundScript),
			HasFlag:               step.HasFlag,
			FlagPath:              step.FlagPath,
			FlagLevel:             step.FlagLevel,
			Questions:             questions,
		})
	}

	return &dto.ScenarioExportOutput{
		Title:         scenario.Title,
		Description:   scenario.Description,
		Difficulty:    scenario.Difficulty,
		EstimatedTime: scenario.EstimatedTime,
		InstanceType:  scenario.InstanceType,
		OsType:        scenario.OsType,
		FlagsEnabled:     scenario.FlagsEnabled,
		AllowedFlagPaths: scenario.AllowedFlagPaths,
		GshEnabled:       scenario.GshEnabled,
		CrashTraps:    scenario.CrashTraps,
		IsPublic:      scenario.IsPublic,
		IntroText:     introText,
		FinishText:    finishText,
		SetupScript:   setupScript,
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

	// Write background.sh (scenario-level setup script)
	archiveSetupScript := ResolveScriptContent(s.db, scenario.SetupScriptID, scenario.SetupScript)
	if archiveSetupScript != "" {
		if err := addFileToZip(w, "background.sh", []byte(archiveSetupScript)); err != nil {
			return nil, err
		}
	}

	// Write intro.md (resolve from ProjectFile if available)
	introText := ResolveScriptContent(s.db, scenario.IntroFileID, scenario.IntroText)
	if introText != "" {
		if err := addFileToZip(w, "intro.md", []byte(introText)); err != nil {
			return nil, err
		}
	}

	// Write finish.md (resolve from ProjectFile if available)
	finishText := ResolveScriptContent(s.db, scenario.FinishFileID, scenario.FinishText)
	if finishText != "" {
		if err := addFileToZip(w, "finish.md", []byte(finishText)); err != nil {
			return nil, err
		}
	}

	// Write step files (resolve from ProjectFile if available)
	for i, step := range scenario.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)

		textContent := ResolveScriptContent(s.db, step.TextFileID, step.TextContent)
		if textContent != "" {
			if err := addFileToZip(w, stepDir+"/text.md", []byte(textContent)); err != nil {
				return nil, err
			}
		}
		hintContent := ResolveScriptContent(s.db, step.HintFileID, step.HintContent)
		if hintContent != "" {
			if err := addFileToZip(w, stepDir+"/hint.md", []byte(hintContent)); err != nil {
				return nil, err
			}
		}
		verifyScript := ResolveScriptContent(s.db, step.VerifyScriptID, step.VerifyScript)
		if verifyScript != "" {
			if err := addFileToZip(w, stepDir+"/verify.sh", []byte(verifyScript)); err != nil {
				return nil, err
			}
		}
		bgScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
		if bgScript != "" {
			if err := addFileToZip(w, stepDir+"/background.sh", []byte(bgScript)); err != nil {
				return nil, err
			}
		}
		fgScript := ResolveScriptContent(s.db, step.ForegroundScriptID, step.ForegroundScript)
		if fgScript != "" {
			if err := addFileToZip(w, stepDir+"/foreground.sh", []byte(fgScript)); err != nil {
				return nil, err
			}
		}

		// Write OCF-specific extension data as a sidecar file so KillerCoda
		// compatibility (index.json schema) is preserved. Only write when
		// the step carries non-default OCF data.
		if needsStepExtensions(&step) {
			sidecar := buildStepExtensions(&step)
			sidecarBytes, err := json.MarshalIndent(sidecar, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal step %d extensions.json: %w", i+1, err)
			}
			if err := addFileToZip(w, stepDir+"/extensions.json", sidecarBytes); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// buildKillerCodaIndex constructs the KillerCoda index.json structure from a scenario.
// When ProjectFile records exist with RelPath, those paths are used for round-trip fidelity.
func (s *ScenarioExportService) buildKillerCodaIndex(scenario *models.Scenario) *KillerCodaIndex {
	details := KillerCodaDetails{
		Steps: make([]KillerCodaStep, 0, len(scenario.Steps)),
	}

	introText := ResolveScriptContent(s.db, scenario.IntroFileID, scenario.IntroText)
	indexSetupScript := ResolveScriptContent(s.db, scenario.SetupScriptID, scenario.SetupScript)
	introFile := KillerCodaFile{}
	if introText != "" {
		introFile.Text = "intro.md"
	}
	if indexSetupScript != "" {
		introFile.Background = "background.sh"
	}
	if introFile.Text != "" || introFile.Background != "" {
		details.Intro = introFile
	}
	finishText := ResolveScriptContent(s.db, scenario.FinishFileID, scenario.FinishText)
	if finishText != "" {
		details.Finish = KillerCodaFile{Text: "finish.md"}
	}

	for i, step := range scenario.Steps {
		stepDir := fmt.Sprintf("step%d", i+1)
		kcStep := KillerCodaStep{
			Title: step.Title,
		}

		textContent := ResolveScriptContent(s.db, step.TextFileID, step.TextContent)
		if textContent != "" {
			kcStep.Text = resolveRelPath(s.db, step.TextFileID, stepDir+"/text.md")
		}
		hintContent := ResolveScriptContent(s.db, step.HintFileID, step.HintContent)
		if hintContent != "" {
			kcStep.Hint = resolveRelPath(s.db, step.HintFileID, stepDir+"/hint.md")
		}
		verifyScript := ResolveScriptContent(s.db, step.VerifyScriptID, step.VerifyScript)
		if verifyScript != "" {
			kcStep.Verify = resolveRelPath(s.db, step.VerifyScriptID, stepDir+"/verify.sh")
		}
		bgScript := ResolveScriptContent(s.db, step.BackgroundScriptID, step.BackgroundScript)
		if bgScript != "" {
			kcStep.Background = resolveRelPath(s.db, step.BackgroundScriptID, stepDir+"/background.sh")
		}
		fgScript := ResolveScriptContent(s.db, step.ForegroundScriptID, step.ForegroundScript)
		if fgScript != "" {
			kcStep.Foreground = resolveRelPath(s.db, step.ForegroundScriptID, stepDir+"/foreground.sh")
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

	return index
}

// resolveRelPath returns the RelPath from a ProjectFile if fileID is non-nil and the file
// has a non-empty RelPath, otherwise returns the fallback path.
func resolveRelPath(db *gorm.DB, fileID *uuid.UUID, fallback string) string {
	if fileID == nil {
		return fallback
	}
	var file models.ProjectFile
	if err := db.Select("rel_path").First(&file, "id = ?", *fileID).Error; err != nil {
		return fallback
	}
	if file.RelPath != "" {
		return file.RelPath
	}
	return fallback
}

// needsStepExtensions reports whether a step carries extension data that does not fit
// the legacy KillerCoda index.json schema (and therefore needs a sidecar file).
// The on-disk payload type (stepExtensions) is defined alongside the importer.
func needsStepExtensions(step *models.ScenarioStep) bool {
	if len(step.Questions) > 0 {
		return true
	}
	if step.ShowImmediateFeedback {
		return true
	}
	if step.StepType != "" && step.StepType != "terminal" {
		return true
	}
	return false
}

// buildStepExtensions converts a step's extension fields into the sidecar payload.
// The returned type is shared with the importer so the on-disk JSON shape is symmetric.
func buildStepExtensions(step *models.ScenarioStep) *stepExtensions {
	stepType := step.StepType
	if stepType == "" {
		stepType = "terminal"
	}

	var questions []stepExtensionsQuestion
	if len(step.Questions) > 0 {
		questions = make([]stepExtensionsQuestion, 0, len(step.Questions))
		for _, q := range step.Questions {
			questions = append(questions, stepExtensionsQuestion{
				Order:         q.Order,
				QuestionText:  q.QuestionText,
				QuestionType:  q.QuestionType,
				Options:       q.Options,
				CorrectAnswer: q.CorrectAnswer,
				Explanation:   q.Explanation,
				Points:        q.Points,
			})
		}
	}

	return &stepExtensions{
		StepType:              stepType,
		ShowImmediateFeedback: step.ShowImmediateFeedback,
		Questions:             questions,
	}
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
