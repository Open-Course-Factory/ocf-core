package services

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/utils"
)

type ScenarioSeedService struct {
	db *gorm.DB
}

func NewScenarioSeedService(db *gorm.DB) *ScenarioSeedService {
	return &ScenarioSeedService{db: db}
}

// SeedScenario creates or updates a scenario with all its steps from a
// SeedScenarioInput. Content that cannot be played is refused with a
// *ScenarioContentError before anything is written.
func (s *ScenarioSeedService) SeedScenario(input dto.SeedScenarioInput, userID string, orgID *uuid.UUID) (*models.Scenario, bool, error) {
	built, err := buildSeedScenario(input)
	if err != nil {
		return nil, false, err
	}
	if err := validateSeedContent(built, input); err != nil {
		return nil, false, err
	}
	built.CreatedByID = userID
	built.OrganizationID = orgID

	existing, err := findScenarioToReplace(s.db, built.Name, orgID)
	if err != nil {
		created, err := s.createSeededScenario(built, input)
		return created, false, err
	}
	updated, err := s.updateSeededScenario(existing, built, input)
	return updated, true, err
}

// buildSeedScenario turns the input into an unsaved scenario, with the same
// normalisation every import path applies, so validation sees what would be
// stored.
func buildSeedScenario(input dto.SeedScenarioInput) (*models.Scenario, error) {
	requiredFeatures, err := EncodeNameList(input.RequiredFeatures)
	if err != nil {
		return nil, err
	}
	buildFeatures, err := EncodeNameList(input.BuildFeatures)
	if err != nil {
		return nil, err
	}
	locales, err := EncodeNameList(input.Locales)
	if err != nil {
		return nil, err
	}

	steps := make([]models.ScenarioStep, len(input.Steps))
	for i, st := range input.Steps {
		stepType, hasFlag := models.NormalizeFlagStep(st.StepType, st.HasFlag)
		steps[i] = models.ScenarioStep{
			Order:                    i,
			Title:                    st.Title,
			StepType:                 stepType,
			ShowImmediateFeedback:    st.ShowImmediateFeedback,
			TextContent:              st.TextContent,
			HintContent:              st.HintContent,
			VerifyScript:             st.VerifyScript,
			BackgroundScript:         st.BackgroundScript,
			ForegroundScript:         st.ForegroundScript,
			IntroEffect:              st.IntroEffect,
			IntroText:                st.IntroText,
			OutroEffect:              st.OutroEffect,
			OutroText:                st.OutroText,
			BackgroundTimeoutSeconds: st.BackgroundTimeoutSeconds,
			BackgroundAsync:          st.BackgroundAsync,
			HasFlag:                  hasFlag,
			FlagPath:                 st.FlagPath,
			FlagLevel:                st.FlagLevel,
			Hints:                    BuildStepHints(st.HintContent),
			Questions:                buildSeedQuestions(st.Questions),
		}
	}

	return &models.Scenario{
		Name:                    utils.GenerateSlug(input.Title),
		Title:                   input.Title,
		Description:             input.Description,
		Difficulty:              input.Difficulty,
		EstimatedTimeMinutes:    input.EstimatedTimeMinutes,
		InstanceType:            input.InstanceType,
		Hostname:                input.Hostname,
		OsType:                  input.OsType,
		SourceType:              "seed",
		IsPublic:                input.IsPublic != nil && *input.IsPublic,
		FlagsEnabled:            input.FlagsEnabled,
		AllowedFlagPaths:        input.AllowedFlagPaths,
		RequiredFeatures:        requiredFeatures,
		BuildFeatures:           buildFeatures,
		CrashTraps:              input.CrashTraps,
		PortExposureAllowed:     input.PortExposureAllowed != nil && *input.PortExposureAllowed,
		SessionUser:             input.SessionUser,
		Objectives:              input.Objectives,
		Prerequisites:           input.Prerequisites,
		DefaultLocale:           input.DefaultLocale,
		Locales:                 locales,
		IntroText:               input.IntroText,
		FinishText:              input.FinishText,
		SetupScript:             input.SetupScript,
		CompatibleInstanceTypes: BuildCompatibleInstanceTypes(input.CompatibleInstanceTypes),
		Steps:                   steps,
	}, nil
}

func buildSeedQuestions(inputs []dto.SeedQuestionInput) []models.ScenarioStepQuestion {
	if len(inputs) == 0 {
		return nil
	}
	questions := make([]models.ScenarioStepQuestion, len(inputs))
	for i, q := range inputs {
		questions[i] = models.ScenarioStepQuestion{
			Order:         q.Order,
			QuestionText:  q.QuestionText,
			QuestionType:  q.QuestionType,
			Options:       q.Options,
			CorrectAnswer: q.CorrectAnswer,
			Explanation:   q.Explanation,
			Points:        q.Points,
		}
	}
	return questions
}

// validateSeedContent adds what only the JSON shape carries — translations and
// the lexicon — to the checks every import path shares.
func validateSeedContent(built *models.Scenario, input dto.SeedScenarioInput) error {
	problems := ScenarioContentProblems(built)

	scenarioLocales := make([]string, len(input.Translations))
	for i, t := range input.Translations {
		scenarioLocales[i] = t.Locale
	}
	problems = append(problems, translationLocaleProblems("translations", scenarioLocales)...)

	for i, st := range input.Steps {
		stepLocales := make([]string, len(st.Translations))
		for j, t := range st.Translations {
			stepLocales[j] = t.Locale
		}
		where := fmt.Sprintf("step %d (%s) translations", i+1, st.Title)
		problems = append(problems, translationLocaleProblems(where, stepLocales)...)
	}

	if err := assertResolvable(input.Lexicon); err != nil {
		problems = append(problems, err.Error())
	}
	return contentErrorOrNil(problems)
}

// translationLocaleProblems refuses what the one-translation-per-locale
// unique index would otherwise refuse later, as a database error.
func translationLocaleProblems(where string, locales []string) []string {
	var problems []string
	seen := map[string]bool{}
	for _, locale := range locales {
		switch {
		case locale == "":
			problems = append(problems, where+": a translation has no locale")
		case seen[locale]:
			problems = append(problems, fmt.Sprintf("%s: locale %q appears twice", where, locale))
		}
		seen[locale] = true
	}
	return problems
}

func (s *ScenarioSeedService) createSeededScenario(scenario *models.Scenario, input dto.SeedScenarioInput) (*models.Scenario, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(scenario).Error; err != nil {
			return fmt.Errorf("failed to create scenario: %w", err)
		}
		return replaceSeededLanguageContent(tx, scenario.ID, scenario.Steps, input)
	})
	if err != nil {
		return nil, err
	}
	return scenario, nil
}

func (s *ScenarioSeedService) updateSeededScenario(existing models.Scenario, built *models.Scenario, input dto.SeedScenarioInput) (*models.Scenario, error) {
	newSteps := built.Steps
	compatibleInstanceTypes := built.CompatibleInstanceTypes

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&existing).Updates(seedScenarioUpdates(built, input)).Error; err != nil {
			return fmt.Errorf("failed to update scenario: %w", err)
		}

		if err := tx.Where("step_id IN (?)",
			tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", existing.ID),
		).Delete(&models.ScenarioStepHint{}).Error; err != nil {
			return fmt.Errorf("failed to delete old hints: %w", err)
		}
		if err := tx.Where("step_id IN (?)",
			tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", existing.ID),
		).Delete(&models.ScenarioStepQuestion{}).Error; err != nil {
			return fmt.Errorf("failed to delete old questions: %w", err)
		}
		var previous []models.ScenarioStep
		if err := tx.Where("scenario_id = ?", existing.ID).
			Order("\"order\" ASC").Find(&previous).Error; err != nil {
			return fmt.Errorf("failed to load existing steps: %w", err)
		}
		reusable := make(map[int]models.ScenarioStep, len(previous))
		for _, step := range previous {
			reusable[step.Order] = step
		}

		keep := make(map[int]bool, len(newSteps))
		for i := range newSteps {
			keep[newSteps[i].Order] = true
		}
		for order, step := range reusable {
			if keep[order] {
				continue
			}
			if err := tx.Where("step_id = ?", step.ID).
				Delete(&models.ScenarioStepTranslation{}).Error; err != nil {
				return fmt.Errorf("failed to delete translations of a removed step: %w", err)
			}
			if err := tx.Delete(&models.ScenarioStep{}, "id = ?", step.ID).Error; err != nil {
				return fmt.Errorf("failed to delete a removed step: %w", err)
			}
			delete(reusable, order)
		}
		if err := tx.Unscoped().Where("scenario_id = ?", existing.ID).
			Delete(&models.ScenarioInstanceType{}).Error; err != nil {
			return fmt.Errorf("failed to delete old instance types: %w", err)
		}
		for i := range compatibleInstanceTypes {
			compatibleInstanceTypes[i].ScenarioID = existing.ID
			if err := tx.Create(&compatibleInstanceTypes[i]).Error; err != nil {
				return fmt.Errorf("failed to create instance type: %w", err)
			}
		}
		if err := deleteScenarioImages(tx, existing.ID); err != nil {
			return err
		}

		for i := range newSteps {
			newSteps[i].ScenarioID = existing.ID
			if kept, ok := reusable[newSteps[i].Order]; ok {
				newSteps[i].ID = kept.ID
				newSteps[i].CreatedAt = kept.CreatedAt
				if err := tx.Model(&models.ScenarioStep{}).Where("id = ?", kept.ID).
					Select("*").Omit("id", "created_at", "deleted_at", "scenario_id").
					Updates(&newSteps[i]).Error; err != nil {
					return fmt.Errorf("failed to update step: %w", err)
				}
				if err := recreateStepChildren(tx, &newSteps[i]); err != nil {
					return err
				}
				continue
			}
			if err := tx.Create(&newSteps[i]).Error; err != nil {
				return fmt.Errorf("failed to create step: %w", err)
			}
		}

		return replaceSeededLanguageContent(tx, existing.ID, newSteps, input)
	})
	if err != nil {
		return nil, err
	}

	var scenario models.Scenario
	if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).Preload("Steps.Hints", func(db *gorm.DB) *gorm.DB {
		return db.Order("level ASC")
	}).First(&scenario, "id = ?", existing.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload scenario: %w", err)
	}
	return &scenario, nil
}

// seedScenarioUpdates is the column map a re-seed writes. The fields the input
// documents as "absent keeps" are written only when present: older files and
// the challenges seeder do not send them, and a re-seed must not wipe what the
// editor set.
func seedScenarioUpdates(built *models.Scenario, input dto.SeedScenarioInput) map[string]any {
	updates := map[string]any{
		"title":                  built.Title,
		"description":            built.Description,
		"difficulty":             built.Difficulty,
		"estimated_time_minutes": built.EstimatedTimeMinutes,
		"instance_type":          built.InstanceType,
		"os_type":                built.OsType,
		"flags_enabled":          built.FlagsEnabled,
		"allowed_flag_paths":     built.AllowedFlagPaths,
		"required_features":      built.RequiredFeatures,
		"build_features":         built.BuildFeatures,
		"crash_traps":            built.CrashTraps,
		"session_user":           built.SessionUser,
		"intro_text":             built.IntroText,
		"finish_text":            built.FinishText,
		"setup_script":           built.SetupScript,
	}
	if input.Hostname != "" {
		updates["hostname"] = built.Hostname
	}
	if input.IsPublic != nil {
		updates["is_public"] = built.IsPublic
	}
	if input.PortExposureAllowed != nil {
		updates["port_exposure_allowed"] = built.PortExposureAllowed
	}
	if input.Objectives != "" {
		updates["objectives"] = built.Objectives
	}
	if input.Prerequisites != "" {
		updates["prerequisites"] = built.Prerequisites
	}
	if input.DefaultLocale != "" {
		updates["default_locale"] = built.DefaultLocale
	}
	if input.Locales != nil {
		updates["locales"] = built.Locales
	}
	return updates
}

// replaceSeededLanguageContent writes the translations and the lexicon the
// input carries. Each part the input leaves out is left as it is.
//
// steps are the saved steps, in input order, so a step's translations land on
// the step they were written for.
func replaceSeededLanguageContent(tx *gorm.DB, scenarioID uuid.UUID, steps []models.ScenarioStep, input dto.SeedScenarioInput) error {
	if input.Translations != nil {
		if err := replaceScenarioTranslations(tx, scenarioID, input.Translations); err != nil {
			return err
		}
	}
	for i, st := range input.Steps {
		if st.Translations == nil {
			continue
		}
		if err := replaceStepTranslations(tx, steps[i], st.Translations); err != nil {
			return err
		}
	}
	if input.Lexicon != nil {
		if err := ReplaceLexicon(tx, scenarioID, input.Lexicon); err != nil {
			return fmt.Errorf("failed to store the lexicon: %w", err)
		}
	}
	return nil
}

// Unscoped deletes, because a soft-deleted row still holds its place in the
// one-translation-per-locale unique index.
func replaceScenarioTranslations(tx *gorm.DB, scenarioID uuid.UUID, inputs []dto.SeedScenarioTranslationInput) error {
	if err := tx.Unscoped().Where("scenario_id = ?", scenarioID).
		Delete(&models.ScenarioTranslation{}).Error; err != nil {
		return fmt.Errorf("failed to delete scenario translations: %w", err)
	}
	for _, t := range inputs {
		row := models.ScenarioTranslation{
			ScenarioID:    scenarioID,
			Locale:        t.Locale,
			Title:         t.Title,
			Description:   t.Description,
			Objectives:    t.Objectives,
			Prerequisites: t.Prerequisites,
			IntroText:     t.IntroText,
			FinishText:    t.FinishText,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("failed to create %s scenario translation: %w", t.Locale, err)
		}
	}
	return nil
}

// replaceStepTranslations stamps each translation as written against the step
// being imported, the same stamp the translation API applies on save: an
// imported translation is as current as the text it arrived with.
func replaceStepTranslations(tx *gorm.DB, step models.ScenarioStep, inputs []dto.SeedStepTranslationInput) error {
	if err := tx.Unscoped().Where("step_id = ?", step.ID).
		Delete(&models.ScenarioStepTranslation{}).Error; err != nil {
		return fmt.Errorf("failed to delete step translations: %w", err)
	}
	hash := StepSourceHash(step)
	for _, t := range inputs {
		row := models.ScenarioStepTranslation{
			StepID:      step.ID,
			Locale:      t.Locale,
			Title:       t.Title,
			TextContent: t.TextContent,
			HintContent: t.HintContent,
			IntroText:   t.IntroText,
			OutroText:   t.OutroText,
			SourceHash:  hash,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("failed to create %s translation of step %q: %w", t.Locale, step.Title, err)
		}
	}
	return nil
}

func recreateStepChildren(tx *gorm.DB, step *models.ScenarioStep) error {
	for i := range step.Hints {
		step.Hints[i].StepID = step.ID
	}
	for i := range step.Questions {
		step.Questions[i].StepID = step.ID
	}
	if len(step.Hints) > 0 {
		if err := tx.Create(&step.Hints).Error; err != nil {
			return fmt.Errorf("failed to create step hints: %w", err)
		}
	}
	if len(step.Questions) > 0 {
		if err := tx.Create(&step.Questions).Error; err != nil {
			return fmt.Errorf("failed to create step questions: %w", err)
		}
	}
	return nil
}
