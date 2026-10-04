package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/utils"
)

// ScenarioDuplicateService handles deep-copying a scenario with all relations
type ScenarioDuplicateService struct {
	db *gorm.DB
}

// NewScenarioDuplicateService creates a new duplicate service
func NewScenarioDuplicateService(db *gorm.DB) *ScenarioDuplicateService {
	return &ScenarioDuplicateService{db: db}
}

// DuplicateScenario creates a deep copy of the source scenario including Steps,
// Hints, quiz Questions, CompatibleInstanceTypes, and the ProjectFiles linked
// to it (its images).
//
// NOT duplicated: ScenarioAssignments, ScenarioSessions, Flags, StepProgress.
//
// Step fields are copied field by field, which makes an omission silent — the
// copy saves cleanly and nothing fails. Adding a column to ScenarioStep means
// adding it to copyStepInto too; TestDuplicateScenario_StepsAreFieldCompleteAgainstSource
// compares every field by reflection so a forgotten one fails a test instead of
// shipping.
func (s *ScenarioDuplicateService) DuplicateScenario(sourceID uuid.UUID, userID string, orgID *uuid.UUID) (*models.Scenario, error) {
	// Load source scenario with all relations
	var source models.Scenario
	if err := s.db.
		Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).
		Preload("Steps.Hints", func(db *gorm.DB) *gorm.DB {
			return db.Order("level ASC")
		}).
		Preload("Steps.Questions", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).
		Preload("CompatibleInstanceTypes").
		First(&source, "id = ?", sourceID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	var sourceFiles []models.ProjectFile
	if err := s.db.Where("scenario_id = ?", sourceID).Find(&sourceFiles).Error; err != nil {
		return nil, fmt.Errorf("failed to load project files: %w", err)
	}

	var newScenario *models.Scenario
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Create new scenario (copy fields, new ID)
		// Generate a short random suffix to ensure unique slug on repeated duplication
		suffixBytes := make([]byte, 3)
		rand.Read(suffixBytes)
		slugSuffix := hex.EncodeToString(suffixBytes) // 6 hex chars

		newScenario = &models.Scenario{
			Name:           utils.GenerateSlug(source.Title+" Copy") + "-" + slugSuffix,
			Title:          source.Title + " (Copy)",
			Description:    source.Description,
			Difficulty:     source.Difficulty,
			EstimatedTimeMinutes:  source.EstimatedTimeMinutes,
			InstanceType:   source.InstanceType,
			Hostname:       source.Hostname,
			OsType:         source.OsType,
			SourceType:     source.SourceType,
			GitRepository:  source.GitRepository,
			GitBranch:      source.GitBranch,
			SourcePath:     source.SourcePath,
			FlagsEnabled:     source.FlagsEnabled,
			AllowedFlagPaths: source.AllowedFlagPaths,
			// No FlagSecret: the copy must not share the source's flags; it
			// gets its own at its first run (ensureFlagSecret).
			CrashTraps:     source.CrashTraps,
			PortExposureAllowed: source.PortExposureAllowed,
			Objectives:     source.Objectives,
			Prerequisites:  source.Prerequisites,
			IntroText:      source.IntroText,
			FinishText:     source.FinishText,
			SetupScript:    source.SetupScript,
			CreatedByID:    userID,
			OrganizationID: orgID,
			IsPublic:       source.IsPublic,
		}

		if err := tx.Create(newScenario).Error; err != nil {
			return fmt.Errorf("failed to create duplicate scenario: %w", err)
		}

		// 2. Copy ProjectFiles
		// NOTE: StorageRef is shallow-copied. If S3-backed storage is introduced,
		// duplication must either copy the S3 object or implement reference counting
		// to prevent a delete of one copy from breaking the other's reference.
		for _, srcFile := range sourceFiles {
			newFile := models.ProjectFile{
				Name:        srcFile.Name,
				RelPath:     srcFile.RelPath,
				ContentType: srcFile.ContentType,
				MimeType:    srcFile.MimeType,
				Content:     srcFile.Content,
				StorageType: srcFile.StorageType,
				StorageRef:  srcFile.StorageRef,
				SizeBytes:   srcFile.SizeBytes,
				ScenarioID:  &newScenario.ID,
			}
			if err := tx.Create(&newFile).Error; err != nil {
				return fmt.Errorf("failed to create project file copy: %w", err)
			}
		}

		// 3. Copy Steps, with their hints and quiz questions
		for _, srcStep := range source.Steps {
			if _, err := copyStepInto(tx, srcStep, newScenario.ID, srcStep.Order); err != nil {
				return err
			}
		}

		// 6. Copy CompatibleInstanceTypes
		for _, srcIT := range source.CompatibleInstanceTypes {
			newIT := models.ScenarioInstanceType{
				ScenarioID:   newScenario.ID,
				InstanceType: srcIT.InstanceType,
				OsType:       srcIT.OsType,
				Priority:     srcIT.Priority,
			}
			if err := tx.Create(&newIT).Error; err != nil {
				return fmt.Errorf("failed to create instance type copy: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Reload the new scenario with all relations
	var result models.Scenario
	if err := s.db.
		Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).
		Preload("Steps.Hints", func(db *gorm.DB) *gorm.DB {
			return db.Order("level ASC")
		}).
		Preload("Steps.Questions", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).
		Preload("CompatibleInstanceTypes").
		First(&result, "id = ?", newScenario.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload duplicated scenario: %w", err)
	}

	return &result, nil
}

// copyStepInto writes a copy of src, with its hint rows and quiz questions
// (src must have them loaded), as the step at order in scenarioID. Shared by
// DuplicateScenario and CopySteps, so one field list serves both.
func copyStepInto(tx *gorm.DB, src models.ScenarioStep, scenarioID uuid.UUID, order int) (models.ScenarioStep, error) {
	step := models.ScenarioStep{
		ScenarioID:               scenarioID,
		Order:                    order,
		Title:                    src.Title,
		StepType:                 src.StepType,
		ShowImmediateFeedback:    src.ShowImmediateFeedback,
		TextContent:              src.TextContent,
		HintContent:              src.HintContent,
		VerifyScript:             src.VerifyScript,
		BackgroundScript:         src.BackgroundScript,
		ForegroundScript:         src.ForegroundScript,
		BackgroundTimeoutSeconds: src.BackgroundTimeoutSeconds,
		BackgroundAsync:          src.BackgroundAsync,
		IntroEffect:              src.IntroEffect,
		IntroText:                src.IntroText,
		OutroEffect:              src.OutroEffect,
		OutroText:                src.OutroText,
		HasFlag:                  src.HasFlag,
		FlagPath:                 src.FlagPath,
		FlagLevel:                src.FlagLevel,
	}
	if err := tx.Create(&step).Error; err != nil {
		return step, fmt.Errorf("failed to create step copy: %w", err)
	}

	for _, srcHint := range src.Hints {
		hint := models.ScenarioStepHint{StepID: step.ID, Level: srcHint.Level, Content: srcHint.Content}
		if err := tx.Create(&hint).Error; err != nil {
			return step, fmt.Errorf("failed to create hint copy: %w", err)
		}
	}

	// Without its questions a copied quiz step renders as an exam with nothing in it.
	for _, srcQuestion := range src.Questions {
		question := models.ScenarioStepQuestion{
			StepID:        step.ID,
			Order:         srcQuestion.Order,
			QuestionText:  srcQuestion.QuestionText,
			QuestionType:  srcQuestion.QuestionType,
			Options:       srcQuestion.Options,
			CorrectAnswer: srcQuestion.CorrectAnswer,
			Explanation:   srcQuestion.Explanation,
			Points:        srcQuestion.Points,
		}
		if err := tx.Create(&question).Error; err != nil {
			return step, fmt.Errorf("failed to create question copy: %w", err)
		}
	}
	return step, nil
}
