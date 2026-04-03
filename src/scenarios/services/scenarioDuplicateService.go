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

// DuplicateScenario creates a deep copy of the source scenario including Steps, Hints,
// CompatibleInstanceTypes, and ProjectFiles. FK references (script IDs) on steps and
// scenario are remapped to the newly created ProjectFile copies.
//
// NOT duplicated: ScenarioAssignments, ScenarioSessions, Flags, StepProgress.
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
		Preload("CompatibleInstanceTypes").
		First(&source, "id = ?", sourceID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	// Load ProjectFiles for the scenario
	var sourceFiles []models.ProjectFile
	if err := s.db.Where("scenario_id = ?", sourceID).Find(&sourceFiles).Error; err != nil {
		return nil, fmt.Errorf("failed to load project files: %w", err)
	}

	// Also collect ProjectFiles referenced by FKs (scenario-level + step-level)
	// that may not have scenario_id set (non-image files)
	referencedFileIDs := collectReferencedFileIDs(&source)
	if len(referencedFileIDs) > 0 {
		var referencedFiles []models.ProjectFile
		if err := s.db.Where("id IN ?", referencedFileIDs).Find(&referencedFiles).Error; err != nil {
			return nil, fmt.Errorf("failed to load referenced project files: %w", err)
		}
		// Merge, avoiding duplicates
		existingIDs := make(map[uuid.UUID]bool)
		for _, f := range sourceFiles {
			existingIDs[f.ID] = true
		}
		for _, f := range referencedFiles {
			if !existingIDs[f.ID] {
				sourceFiles = append(sourceFiles, f)
			}
		}
	}

	var newScenario *models.Scenario
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Create new scenario (copy fields, new ID)
		var flagSecret string
		if source.FlagsEnabled {
			secretBytes := make([]byte, 32)
			if _, err := rand.Read(secretBytes); err != nil {
				return fmt.Errorf("failed to generate flag secret: %w", err)
			}
			flagSecret = hex.EncodeToString(secretBytes)
		}

		// Generate a short random suffix to ensure unique slug on repeated duplication
		suffixBytes := make([]byte, 3)
		rand.Read(suffixBytes)
		slugSuffix := hex.EncodeToString(suffixBytes) // 6 hex chars

		newScenario = &models.Scenario{
			Name:           utils.GenerateSlug(source.Title+" Copy") + "-" + slugSuffix,
			Title:          source.Title + " (Copy)",
			Description:    source.Description,
			Difficulty:     source.Difficulty,
			EstimatedTime:  source.EstimatedTime,
			InstanceType:   source.InstanceType,
			Hostname:       source.Hostname,
			OsType:         source.OsType,
			SourceType:     source.SourceType,
			GitRepository:  source.GitRepository,
			GitBranch:      source.GitBranch,
			SourcePath:     source.SourcePath,
			FlagsEnabled:     source.FlagsEnabled,
			AllowedFlagPaths: source.AllowedFlagPaths,
			FlagSecret:       flagSecret,
			GshEnabled:     source.GshEnabled,
			CrashTraps:     source.CrashTraps,
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

		// 2. Copy ProjectFiles (map old ID -> new ID)
		// NOTE: StorageRef is shallow-copied. If S3-backed storage is introduced,
		// duplication must either copy the S3 object or implement reference counting
		// to prevent a delete of one copy from breaking the other's reference.
		fileIDMap := make(map[uuid.UUID]uuid.UUID) // oldID -> newID
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
			}
			// Image files get linked to the new scenario via ScenarioID
			if srcFile.ScenarioID != nil {
				newFile.ScenarioID = &newScenario.ID
			}
			if err := tx.Create(&newFile).Error; err != nil {
				return fmt.Errorf("failed to create project file copy: %w", err)
			}
			fileIDMap[srcFile.ID] = newFile.ID
		}

		// 3. Update scenario-level FK refs to point to new ProjectFile IDs
		updates := map[string]any{}
		if source.SetupScriptID != nil {
			if newID, ok := fileIDMap[*source.SetupScriptID]; ok {
				updates["setup_script_id"] = newID
			}
		}
		if source.IntroFileID != nil {
			if newID, ok := fileIDMap[*source.IntroFileID]; ok {
				updates["intro_file_id"] = newID
			}
		}
		if source.FinishFileID != nil {
			if newID, ok := fileIDMap[*source.FinishFileID]; ok {
				updates["finish_file_id"] = newID
			}
		}
		if len(updates) > 0 {
			if err := tx.Model(newScenario).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update scenario file refs: %w", err)
			}
		}

		// 4. Copy Steps (with updated FK refs)
		for _, srcStep := range source.Steps {
			newStep := models.ScenarioStep{
				ScenarioID:       newScenario.ID,
				Order:            srcStep.Order,
				Title:            srcStep.Title,
				TextContent:      srcStep.TextContent,
				HintContent:      srcStep.HintContent,
				VerifyScript:     srcStep.VerifyScript,
				BackgroundScript: srcStep.BackgroundScript,
				ForegroundScript: srcStep.ForegroundScript,
				HasFlag:          srcStep.HasFlag,
				FlagPath:         srcStep.FlagPath,
				FlagLevel:        srcStep.FlagLevel,
			}

			// Remap step-level FK refs
			if srcStep.VerifyScriptID != nil {
				if newID, ok := fileIDMap[*srcStep.VerifyScriptID]; ok {
					newStep.VerifyScriptID = &newID
				}
			}
			if srcStep.BackgroundScriptID != nil {
				if newID, ok := fileIDMap[*srcStep.BackgroundScriptID]; ok {
					newStep.BackgroundScriptID = &newID
				}
			}
			if srcStep.ForegroundScriptID != nil {
				if newID, ok := fileIDMap[*srcStep.ForegroundScriptID]; ok {
					newStep.ForegroundScriptID = &newID
				}
			}
			if srcStep.TextFileID != nil {
				if newID, ok := fileIDMap[*srcStep.TextFileID]; ok {
					newStep.TextFileID = &newID
				}
			}
			if srcStep.HintFileID != nil {
				if newID, ok := fileIDMap[*srcStep.HintFileID]; ok {
					newStep.HintFileID = &newID
				}
			}

			if err := tx.Create(&newStep).Error; err != nil {
				return fmt.Errorf("failed to create step copy: %w", err)
			}

			// 5. Copy Hints (linked to new step ID)
			for _, srcHint := range srcStep.Hints {
				newHint := models.ScenarioStepHint{
					StepID:  newStep.ID,
					Level:   srcHint.Level,
					Content: srcHint.Content,
				}
				if err := tx.Create(&newHint).Error; err != nil {
					return fmt.Errorf("failed to create hint copy: %w", err)
				}
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
		Preload("CompatibleInstanceTypes").
		First(&result, "id = ?", newScenario.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload duplicated scenario: %w", err)
	}

	return &result, nil
}

// collectReferencedFileIDs gathers all ProjectFile IDs referenced by scenario and step FKs.
func collectReferencedFileIDs(scenario *models.Scenario) []uuid.UUID {
	var ids []uuid.UUID
	if scenario.SetupScriptID != nil {
		ids = append(ids, *scenario.SetupScriptID)
	}
	if scenario.IntroFileID != nil {
		ids = append(ids, *scenario.IntroFileID)
	}
	if scenario.FinishFileID != nil {
		ids = append(ids, *scenario.FinishFileID)
	}
	for _, step := range scenario.Steps {
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
	return ids
}
