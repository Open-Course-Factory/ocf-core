package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/utils"
)

// ScenarioSeedService handles creating or updating scenarios from JSON input
type ScenarioSeedService struct {
	db *gorm.DB
}

// NewScenarioSeedService creates a new seed service
func NewScenarioSeedService(db *gorm.DB) *ScenarioSeedService {
	return &ScenarioSeedService{db: db}
}

// SeedScenario creates or updates a scenario with all its steps from a SeedScenarioInput.
// orgID is optional — set for group-level imports. userID is the creating user.
// Returns (scenario, isUpdate, error).
func (s *ScenarioSeedService) SeedScenario(input dto.SeedScenarioInput, userID string, orgID *uuid.UUID) (*models.Scenario, bool, error) {
	name := utils.GenerateSlug(input.Title)

	// Check if a scenario with this name already exists (upsert)
	// When orgID is set (group-level import), scope lookup to the same organization
	// to prevent cross-tenant overwrites. Admin imports (orgID == nil) match globally.
	var existing models.Scenario
	isUpdate := false
	query := s.db.Where("name = ?", name)
	if orgID != nil {
		query = query.Where("organization_id = ?", *orgID)
	}
	if err := query.First(&existing).Error; err == nil {
		isUpdate = true
	}

	var flagSecret string
	if input.FlagsEnabled {
		if isUpdate && existing.FlagSecret != "" {
			// Keep existing flag secret on update so active sessions remain valid
			flagSecret = existing.FlagSecret
		} else {
			secretBytes := make([]byte, 32)
			if _, err := rand.Read(secretBytes); err != nil {
				return nil, false, fmt.Errorf("failed to generate flag secret: %w", err)
			}
			flagSecret = hex.EncodeToString(secretBytes)
		}
	}

	// Build new steps
	newSteps := make([]models.ScenarioStep, len(input.Steps))
	for i, st := range input.Steps {
		newSteps[i] = models.ScenarioStep{
			Order:            i,
			Title:            st.Title,
			TextContent:      st.TextContent,
			HintContent:      st.HintContent,
			VerifyScript:     st.VerifyScript,
			BackgroundScript: st.BackgroundScript,
			ForegroundScript: st.ForegroundScript,
			HasFlag:          st.HasFlag,
			FlagPath:         st.FlagPath,
		}

		// Build progressive hints from hint content
		if st.HintContent != "" {
			parts := SplitHintContent(st.HintContent)
			hints := make([]models.ScenarioStepHint, len(parts))
			for j, part := range parts {
				hints[j] = models.ScenarioStepHint{
					Level:   j + 1,
					Content: part,
				}
			}
			newSteps[i].Hints = hints
		}
	}

	var scenario models.Scenario
	if isUpdate {
		// Update existing scenario in a transaction
		err := s.db.Transaction(func(tx *gorm.DB) error {
			// Collect old ProjectFile IDs before deleting steps
			oldFileIDs := collectProjectFileIDs(tx, existing.ID)

			if err := tx.Model(&existing).Updates(map[string]any{
				"title":          input.Title,
				"description":    input.Description,
				"difficulty":     input.Difficulty,
				"estimated_time": input.EstimatedTime,
				"instance_type":  input.InstanceType,
				"os_type":        input.OsType,
				"flags_enabled":  input.FlagsEnabled,
				"flag_secret":    flagSecret,
				"gsh_enabled":    input.GshEnabled,
				"crash_traps":    input.CrashTraps,
				"intro_text":     input.IntroText,
				"finish_text":    input.FinishText,
				"setup_script":   input.SetupScript,
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
			// Delete old ProjectFiles (orphaned from previous imports)
			if len(oldFileIDs) > 0 {
				if err := tx.Where("id IN ?", oldFileIDs).Delete(&models.ProjectFile{}).Error; err != nil {
					return fmt.Errorf("failed to delete old project files: %w", err)
				}
			}

			// Create new steps
			for i := range newSteps {
				newSteps[i].ScenarioID = existing.ID
				if err := tx.Create(&newSteps[i]).Error; err != nil {
					return fmt.Errorf("failed to create step: %w", err)
				}
			}

			// Create ProjectFiles (dual-write: content stored inline AND in ProjectFile)
			srcScenario := &models.Scenario{
				IntroText:   input.IntroText,
				FinishText:  input.FinishText,
				SetupScript: input.SetupScript,
				Steps:       newSteps,
			}
			if err := createProjectFilesForScenario(tx, &existing, srcScenario, nil); err != nil {
				return fmt.Errorf("failed to create project files: %w", err)
			}

			return nil
		})
		if err != nil {
			return nil, false, err
		}

		// Reload with steps and hints
		if err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("\"order\" ASC")
		}).Preload("Steps.Hints", func(db *gorm.DB) *gorm.DB {
			return db.Order("level ASC")
		}).First(&scenario, "id = ?", existing.ID).Error; err != nil {
			return nil, false, fmt.Errorf("failed to reload scenario: %w", err)
		}
	} else {
		// Create new scenario
		scenario = models.Scenario{
			Name:           name,
			Title:          input.Title,
			Description:    input.Description,
			Difficulty:     input.Difficulty,
			EstimatedTime:  input.EstimatedTime,
			InstanceType:   input.InstanceType,
			OsType:         input.OsType,
			SourceType:     "seed",
			FlagsEnabled:   input.FlagsEnabled,
			FlagSecret:     flagSecret,
			GshEnabled:     input.GshEnabled,
			CrashTraps:     input.CrashTraps,
			IntroText:      input.IntroText,
			FinishText:     input.FinishText,
			SetupScript:    input.SetupScript,
			CreatedByID:    userID,
			OrganizationID: orgID,
		}
		scenario.Steps = newSteps

		if err := s.db.Create(&scenario).Error; err != nil {
			return nil, false, fmt.Errorf("failed to create scenario: %w", err)
		}

		// Create ProjectFiles (dual-write)
		srcScenario := &models.Scenario{
			IntroText:   input.IntroText,
			FinishText:  input.FinishText,
			SetupScript: input.SetupScript,
			Steps:       newSteps,
		}
		if err := createProjectFilesForScenario(s.db, &scenario, srcScenario, nil); err != nil {
			return nil, false, fmt.Errorf("failed to create project files: %w", err)
		}
	}

	return &scenario, isUpdate, nil
}
