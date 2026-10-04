package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

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

// withExportAssociations preloads everything an export has to read.
//
// It is one function rather than three copies of the same Preload chain
// because the three entry points had already drifted: all of them loaded
// steps, none loaded the declared images, and the omission only showed up as
// an archive that re-imported without its image requirement.
func withExportAssociations(db *gorm.DB) *gorm.DB {
	byOrder := func(db *gorm.DB) *gorm.DB { return db.Order("\"order\" ASC") }
	return db.
		Preload("Steps", byOrder).
		Preload("Steps.Questions", byOrder).
		Preload("CompatibleInstanceTypes")
}

// ExportAsJSON loads a scenario with steps and returns the export DTO
// ExportAsJSON returns the scenario in the shape import-json accepts, so that
// importing the result recreates it. Nothing secret or owner-specific is in
// it: no flag secret, no organization, no author, no visibility.
func (s *ScenarioExportService) ExportAsJSON(scenarioID uuid.UUID) (*dto.SeedScenarioInput, error) {
	var scenario models.Scenario
	if err := withExportAssociations(s.db).First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	return s.buildExportOutput(&scenario)
}

// ExportAsArchive loads a scenario with steps and returns a KillerCoda-compatible zip archive.
// Returns (zipBytes, filename, error).
func (s *ScenarioExportService) ExportAsArchive(scenarioID uuid.UUID) ([]byte, string, error) {
	var scenario models.Scenario
	if err := withExportAssociations(s.db).First(&scenario, "id = ?", scenarioID).Error; err != nil {
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
func (s *ScenarioExportService) ExportMultipleAsJSON(scenarioIDs []uuid.UUID) ([]dto.SeedScenarioInput, error) {
	var scenarios []models.Scenario
	if err := withExportAssociations(s.db).Where("id IN ?", scenarioIDs).Find(&scenarios).Error; err != nil {
		return nil, fmt.Errorf("failed to load scenarios: %w", err)
	}

	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios found for the given IDs")
	}

	outputs := make([]dto.SeedScenarioInput, 0, len(scenarios))
	for i := range scenarios {
		output, err := s.buildExportOutput(&scenarios[i])
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, *output)
	}
	return outputs, nil
}

// buildExportOutput converts a Scenario model, with its steps, questions and
// declared images loaded, into the import shape.
func (s *ScenarioExportService) buildExportOutput(scenario *models.Scenario) (*dto.SeedScenarioInput, error) {
	requiredFeatures, err := scenario.GetRequiredFeatures()
	if err != nil {
		return nil, err
	}
	buildFeatures, err := scenario.GetBuildFeatures()
	if err != nil {
		return nil, err
	}
	locales, err := scenario.GetLocales()
	if err != nil {
		return nil, err
	}
	translations, err := s.exportScenarioTranslations(scenario.ID)
	if err != nil {
		return nil, err
	}
	stepTranslations, err := s.exportStepTranslations(scenario.Steps)
	if err != nil {
		return nil, err
	}
	lexicon, err := s.exportLexicon(scenario.ID)
	if err != nil {
		return nil, err
	}

	steps := make([]dto.SeedStepInput, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		steps = append(steps, dto.SeedStepInput{
			Title:                    step.Title,
			StepType:                 step.StepType,
			ShowImmediateFeedback:    step.ShowImmediateFeedback,
			TextContent:              step.TextContent,
			HintContent:              step.HintContent,
			VerifyScript:             step.VerifyScript,
			BackgroundScript:         step.BackgroundScript,
			ForegroundScript:         step.ForegroundScript,
			IntroEffect:              step.IntroEffect,
			IntroText:                step.IntroText,
			OutroEffect:              step.OutroEffect,
			OutroText:                step.OutroText,
			BackgroundTimeoutSeconds: step.BackgroundTimeoutSeconds,
			BackgroundAsync:          step.BackgroundAsync,
			HasFlag:                  step.HasFlag,
			FlagPath:                 step.FlagPath,
			FlagLevel:                step.FlagLevel,
			Questions:                exportQuestions(step.Questions),
			Translations:             stepTranslations[step.ID],
		})
	}

	declaredImages := declaredImageNames(scenario)
	portExposureAllowed := scenario.PortExposureAllowed

	return &dto.SeedScenarioInput{
		Title:                   scenario.Title,
		Description:             scenario.Description,
		Difficulty:              scenario.Difficulty,
		EstimatedTimeMinutes:    scenario.EstimatedTimeMinutes,
		InstanceType:            scenario.InstanceType,
		Hostname:                scenario.Hostname,
		OsType:                  scenario.OsType,
		FlagsEnabled:            scenario.FlagsEnabled,
		AllowedFlagPaths:        scenario.AllowedFlagPaths,
		CrashTraps:              scenario.CrashTraps,
		PortExposureAllowed:     &portExposureAllowed,
		SessionUser:             scenario.SessionUser,
		IntroText:               scenario.IntroText,
		FinishText:              scenario.FinishText,
		SetupScript:             scenario.SetupScript,
		Objectives:              scenario.Objectives,
		Prerequisites:           scenario.Prerequisites,
		DefaultLocale:           scenario.DefaultLocale,
		Locales:                 locales,
		CompatibleInstanceTypes: declaredImages,
		RequiredFeatures:        requiredFeatures,
		BuildFeatures:           buildFeatures,
		Translations:            translations,
		Lexicon:                 lexicon,
		Steps:                   steps,
	}, nil
}

// declaredImageNames lists the scenario's compatible distributions in the
// author's order of preference.
func declaredImageNames(scenario *models.Scenario) []string {
	names := make([]string, 0, len(scenario.CompatibleInstanceTypes))
	for _, cit := range SortInstanceTypesByPriority(scenario.CompatibleInstanceTypes) {
		names = append(names, cit.InstanceType)
	}
	return names
}

func exportQuestions(questions []models.ScenarioStepQuestion) []dto.SeedQuestionInput {
	if len(questions) == 0 {
		return nil
	}
	out := make([]dto.SeedQuestionInput, 0, len(questions))
	for _, q := range questions {
		out = append(out, dto.SeedQuestionInput{
			Order:         q.Order,
			QuestionText:  q.QuestionText,
			QuestionType:  q.QuestionType,
			Options:       q.Options,
			CorrectAnswer: q.CorrectAnswer,
			Explanation:   q.Explanation,
			Points:        q.Points,
		})
	}
	return out
}

func (s *ScenarioExportService) exportScenarioTranslations(scenarioID uuid.UUID) ([]dto.SeedScenarioTranslationInput, error) {
	var rows []models.ScenarioTranslation
	if err := s.db.Where("scenario_id = ?", scenarioID).Order("locale ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load scenario translations: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]dto.SeedScenarioTranslationInput, 0, len(rows))
	for _, t := range rows {
		out = append(out, dto.SeedScenarioTranslationInput{
			Locale:        t.Locale,
			Title:         t.Title,
			Description:   t.Description,
			Objectives:    t.Objectives,
			Prerequisites: t.Prerequisites,
			IntroText:     t.IntroText,
			FinishText:    t.FinishText,
		})
	}
	return out, nil
}

// exportStepTranslations reads every step's translations in one query.
func (s *ScenarioExportService) exportStepTranslations(steps []models.ScenarioStep) (map[uuid.UUID][]dto.SeedStepTranslationInput, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	var rows []models.ScenarioStepTranslation
	if err := s.db.Where("step_id IN ?", ids).Order("locale ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load step translations: %w", err)
	}
	byStep := make(map[uuid.UUID][]dto.SeedStepTranslationInput, len(steps))
	for _, t := range rows {
		byStep[t.StepID] = append(byStep[t.StepID], dto.SeedStepTranslationInput{
			Locale:      t.Locale,
			Title:       t.Title,
			TextContent: t.TextContent,
			HintContent: t.HintContent,
			IntroText:   t.IntroText,
			OutroText:   t.OutroText,
		})
	}
	return byStep, nil
}

func (s *ScenarioExportService) exportLexicon(scenarioID uuid.UUID) ([]dto.LexiconEntryInput, error) {
	entries, names, err := loadLexicon(s.db, scenarioID)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	out := make([]dto.LexiconEntryInput, 0, len(entries))
	for _, entry := range entries {
		out = append(out, dto.LexiconEntryInput{
			Key:       entry.Key,
			ParentKey: entry.ParentKey,
			Kind:      entry.Kind,
			Names:     names[entry.ID],
		})
	}
	return out, nil
}

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
	if scenario.SetupScript != "" {
		if err := addFileToZip(w, "background.sh", []byte(scenario.SetupScript)); err != nil {
			return nil, err
		}
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
func (s *ScenarioExportService) buildKillerCodaIndex(scenario *models.Scenario) *KillerCodaIndex {
	details := KillerCodaDetails{
		Steps: make([]KillerCodaStep, 0, len(scenario.Steps)),
	}

	introFile := KillerCodaFile{}
	if scenario.IntroText != "" {
		introFile.Text = "intro.md"
	}
	if scenario.SetupScript != "" {
		introFile.Background = "background.sh"
	}
	if introFile.Text != "" || introFile.Background != "" {
		details.Intro = introFile
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

		kcStep.IntroEffect = step.IntroEffect
		kcStep.IntroText = step.IntroText
		kcStep.OutroEffect = step.OutroEffect
		kcStep.OutroText = step.OutroText

		// Set per-step flag override if different from scenario default
		hasFlag := step.HasFlag
		kcStep.HasFlag = &hasFlag
		if step.FlagPath != "" {
			kcStep.FlagPath = step.FlagPath
		}
		kcStep.BackgroundTimeoutSeconds = step.BackgroundTimeoutSeconds
		kcStep.BackgroundAsync = step.BackgroundAsync

		details.Steps = append(details.Steps, kcStep)
	}

	index := &KillerCodaIndex{
		Title:       scenario.Title,
		Description: scenario.Description,
		Difficulty:  scenario.Difficulty,
		TimeMinutes: scenario.EstimatedTimeMinutes,
		Details:     details,
		Backend:     KillerCodaBackend{ImageID: scenario.InstanceType},
	}

	// Add OCF extensions if the scenario carries any.
	//
	// The image and feature requirements belong here too: an export that drops
	// them hands back an archive that re-imports as a scenario with no image
	// requirement and no network, which then resolves onto an arbitrary
	// distribution and fails provisioning on its first apt-get. Export is how a
	// scenario moves between environments, so a lossy one is a silent
	// downgrade, not a cosmetic gap.
	declaredImages := declaredImageNames(scenario)
	requiredFeatures, featErr := scenario.GetRequiredFeatures()
	if featErr != nil {
		slog.Warn("scenario has unparseable required_features, exporting without them",
			"scenario_id", scenario.ID, "err", featErr)
		requiredFeatures = nil
	}
	buildFeatures, featErr := scenario.GetBuildFeatures()
	if featErr != nil {
		slog.Warn("scenario has unparseable build_features, exporting without them",
			"scenario_id", scenario.ID, "err", featErr)
		buildFeatures = nil
	}

	ocf := &KillerCodaOCF{
		Flags:                   scenario.FlagsEnabled,
		CrashTraps:              scenario.CrashTraps,
		CompatibleInstanceTypes: declaredImages,
		RequiredFeatures:        requiredFeatures,
		BuildFeatures:           buildFeatures,
		SessionUser:             scenario.SessionUser,
		PortExposureAllowed:     scenario.PortExposureAllowed,
	}
	// Export the hostname so an exported archive re-imports with the same
	// terminal name instead of falling back to the generated one.
	if scenario.Hostname != "" {
		hostname := scenario.Hostname
		ocf.Hostname = &hostname
	}
	if scenario.FlagsEnabled || scenario.CrashTraps || scenario.PortExposureAllowed ||
		scenario.SessionUser != nil || scenario.Hostname != "" ||
		len(declaredImages) > 0 || len(requiredFeatures) > 0 || len(buildFeatures) > 0 {
		index.Extensions = &KillerCodaExtensions{OCF: ocf}
	}

	return index
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
