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

	compatibleInstanceTypes := BuildCompatibleInstanceTypes(input.CompatibleInstanceTypes)

	requiredFeatures, featErr := EncodeRequiredFeatures(input.RequiredFeatures)
	if featErr != nil {
		return nil, false, featErr
	}

	buildFeatures, buildFeatErr := EncodeRequiredFeatures(input.BuildFeatures)
	if buildFeatErr != nil {
		return nil, false, buildFeatErr
	}

	// Build new steps
	newSteps := make([]models.ScenarioStep, len(input.Steps))
	for i, st := range input.Steps {
		newSteps[i] = models.ScenarioStep{
			Order:                    i,
			Title:                    st.Title,
			StepType:                 ResolveStepType(st.StepType, st.HasFlag),
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
			HasFlag:                  st.HasFlag,
			FlagPath:                 st.FlagPath,
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

		// Build quiz questions; GORM cascade-creates them with the step
		if len(st.Questions) > 0 {
			questions := make([]models.ScenarioStepQuestion, len(st.Questions))
			for j, q := range st.Questions {
				questions[j] = models.ScenarioStepQuestion{
					Order:         q.Order,
					QuestionText:  q.QuestionText,
					QuestionType:  q.QuestionType,
					Options:       q.Options,
					CorrectAnswer: q.CorrectAnswer,
					Explanation:   q.Explanation,
					Points:        q.Points,
				}
			}
			newSteps[i].Questions = questions
		}
	}

	var scenario models.Scenario
	if isUpdate {
		// Update existing scenario in a transaction
		err := s.db.Transaction(func(tx *gorm.DB) error {
			// Collect old ProjectFile IDs before deleting steps
			oldFileIDs := collectProjectFileIDs(tx, existing.ID)

			if err := tx.Model(&existing).Updates(map[string]any{
				"title":              input.Title,
				"description":        input.Description,
				"difficulty":         input.Difficulty,
				"estimated_time_minutes": input.EstimatedTimeMinutes,
				"instance_type":      input.InstanceType,
				"os_type":            input.OsType,
				"is_public":          input.IsPublic,
				"flags_enabled":      input.FlagsEnabled,
				"allowed_flag_paths": input.AllowedFlagPaths,
				"flag_secret":        flagSecret,
				"required_features":  requiredFeatures,
				"build_features":     buildFeatures,
				"crash_traps":        input.CrashTraps,
				"session_user":       input.SessionUser,
				"intro_text":         input.IntroText,
				"finish_text":        input.FinishText,
				"setup_script":       input.SetupScript,
				"setup_script_id":    nil,
				"intro_file_id":      nil,
				"finish_file_id":     nil,
			}).Error; err != nil {
				return fmt.Errorf("failed to update scenario: %w", err)
			}

			// Delete old hints before steps (soft-delete won't cascade)
			if err := tx.Where("step_id IN (?)",
				tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", existing.ID),
			).Delete(&models.ScenarioStepHint{}).Error; err != nil {
				return fmt.Errorf("failed to delete old hints: %w", err)
			}
			// Delete old quiz questions before steps (soft-delete won't cascade)
			if err := tx.Where("step_id IN (?)",
				tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", existing.ID),
			).Delete(&models.ScenarioStepQuestion{}).Error; err != nil {
				return fmt.Errorf("failed to delete old questions: %w", err)
			}
			// Steps keep their identity across a re-seed, matched by order.
			//
			// Content is authored in files and pushed repeatedly; a translation
			// is written once, by hand, and may live only here. Replacing every
			// step would detach them all silently — the work still stored,
			// attached to a row nothing reads, and the scenario simply reading
			// untranslated again.
			//
			// Reusing the row also makes an edit behave the way it should: the
			// translation survives and its source hash no longer matches, so it
			// reports as stale rather than disappearing.
			var previous []models.ScenarioStep
			if err := tx.Where("scenario_id = ?", existing.ID).
				Order("\"order\" ASC").Find(&previous).Error; err != nil {
				return fmt.Errorf("failed to load existing steps: %w", err)
			}
			reusable := make(map[int]models.ScenarioStep, len(previous))
			for _, step := range previous {
				reusable[step.Order] = step
			}

			// Steps the scenario no longer has go, and their translations with
			// them: a language reporting work for a step nobody can reach is
			// worse than one honestly short.
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
			// Replace the image declaration rather than adding to it, so a
			// re-seed converges on what the scenario now says instead of
			// leaving a corrected scenario still matching its old image.
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
			// Delete old ProjectFiles (orphaned from previous imports)
			if len(oldFileIDs) > 0 {
				if err := tx.Where("id IN ?", oldFileIDs).Delete(&models.ProjectFile{}).Error; err != nil {
					return fmt.Errorf("failed to delete old project files: %w", err)
				}
			}

			// Write the steps, reusing the row that already held each order.
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
					continue
				}
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
			Name:             name,
			Title:            input.Title,
			Description:      input.Description,
			Difficulty:       input.Difficulty,
			EstimatedTimeMinutes:    input.EstimatedTimeMinutes,
			InstanceType:     input.InstanceType,
			OsType:           input.OsType,
			SourceType:       "seed",
			IsPublic:         input.IsPublic,
			FlagsEnabled:     input.FlagsEnabled,
			AllowedFlagPaths: input.AllowedFlagPaths,
			FlagSecret:       flagSecret,
			RequiredFeatures: requiredFeatures,
			BuildFeatures:    buildFeatures,
			CrashTraps:       input.CrashTraps,
			SessionUser:      input.SessionUser,
			IntroText:        input.IntroText,
			FinishText:       input.FinishText,
			SetupScript:      input.SetupScript,
			CreatedByID:      userID,
			OrganizationID:   orgID,
		}
		scenario.Steps = newSteps
		scenario.CompatibleInstanceTypes = compatibleInstanceTypes

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
