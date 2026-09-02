package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioStepTranslationStampHook records which version of a step a
// translation was written against.
//
// The hash is computed here rather than accepted from the caller, and that is
// the whole point of it. It is the only thing that can say a translation has
// fallen behind its source; a client able to set it is a client able to declare
// stale work current, and the staleness report would then confirm the claim —
// which is worse than having no report at all.
//
// It also means a translator cannot forget. Saving a translation is what marks
// it current, because saving is what stamps it.
type ScenarioStepTranslationStampHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewScenarioStepTranslationStampHook(db *gorm.DB) *ScenarioStepTranslationStampHook {
	return &ScenarioStepTranslationStampHook{db: db, enabled: true, priority: 10}
}

func (h *ScenarioStepTranslationStampHook) GetName() string       { return "scenario_step_translation_stamp" }
func (h *ScenarioStepTranslationStampHook) GetEntityName() string { return "ScenarioStepTranslation" }
func (h *ScenarioStepTranslationStampHook) IsEnabled() bool       { return h.enabled }
func (h *ScenarioStepTranslationStampHook) GetPriority() int      { return h.priority }

func (h *ScenarioStepTranslationStampHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.AfterUpdate}
}

func (h *ScenarioStepTranslationStampHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType == hooks.AfterUpdate {
		id, ok := ctx.EntityID.(uuid.UUID)
		if !ok {
			return nil
		}
		return h.stampSavedRow(id)
	}

	translation, ok := ctx.NewEntity.(*models.ScenarioStepTranslation)
	if !ok {
		// A PATCH arrives as its input DTO, not as the model — the generic
		// service passes the request body straight through as NewEntity. There
		// is nothing to stamp in place here, and the DTO deliberately carries
		// no hash field, so the update is stamped afterwards instead.
		return nil
	}

	var step models.ScenarioStep
	if err := h.db.First(&step, "id = ?", translation.StepID).Error; err != nil {
		// Refused rather than stamped with nothing. An empty hash reads as
		// "never checked" everywhere else, so letting it through would create a
		// row that is permanently, silently stale.
		return fmt.Errorf("cannot translate step %s: %w", translation.StepID, err)
	}

	translation.SourceHash = services.StepSourceHash(step)
	return nil
}

// stampSavedRow re-stamps a translation that has just been written through the
// generic update path.
//
// Saving a translation is what marks it current — that is the rule the create
// path already keeps. Update kept it only in appearance: the hook ran, found a
// DTO instead of a model, and returned, so a translation rewritten to match new
// source text stayed stamped with the old one. It then reported as stale for
// ever, and a stale language is one the launcher stops offering: re-seeding a
// scenario whose prose had changed silently took its other languages off the
// card, with nothing anywhere saying so.
func (h *ScenarioStepTranslationStampHook) stampSavedRow(id uuid.UUID) error {
	var translation models.ScenarioStepTranslation
	if err := h.db.First(&translation, "id = ?", id).Error; err != nil {
		return fmt.Errorf("cannot re-stamp translation %s: %w", id, err)
	}

	var step models.ScenarioStep
	if err := h.db.First(&step, "id = ?", translation.StepID).Error; err != nil {
		return fmt.Errorf("cannot translate step %s: %w", translation.StepID, err)
	}

	hash := services.StepSourceHash(step)
	if translation.SourceHash == hash {
		return nil
	}
	return h.db.Model(&translation).Update("source_hash", hash).Error
}

// =============================================================================
// Authorization
// =============================================================================

// Translations are Member-writable so a trainer can translate their own
// scenarios, and that is exactly why these hooks exist. An entity granting
// Member write access without a hook on every write operation is not weakly
// protected — it is unprotected, for everyone. Guarding only create would leave
// a learner able to rewrite or delete someone else's translation, which is the
// same hole wearing a different verb.
//
// Authorization is transitive and reuses CanManageScenario rather than
// restating who may edit a scenario: translation is editing.

type ScenarioStepTranslationAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	enabled      bool
	priority     int
}

func NewScenarioStepTranslationAuthorizationHook(db *gorm.DB) *ScenarioStepTranslationAuthorizationHook {
	return &ScenarioStepTranslationAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		enabled:      true,
		// Runs before the stamp hook: there is no reason to read a step for
		// someone who may not touch it.
		priority: 5,
	}
}

func (h *ScenarioStepTranslationAuthorizationHook) GetName() string {
	return "scenario_step_translation_authorization"
}
func (h *ScenarioStepTranslationAuthorizationHook) GetEntityName() string {
	return "ScenarioStepTranslation"
}
func (h *ScenarioStepTranslationAuthorizationHook) IsEnabled() bool  { return h.enabled }
func (h *ScenarioStepTranslationAuthorizationHook) GetPriority() int { return h.priority }
func (h *ScenarioStepTranslationAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *ScenarioStepTranslationAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	if ctx.IsAdmin() {
		return nil
	}

	entity := ctx.NewEntity
	if ctx.HookType == hooks.BeforeUpdate && ctx.OldEntity != nil {
		entity = ctx.OldEntity
	}
	translation, ok := entity.(*models.ScenarioStepTranslation)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioStepTranslation, got %T", entity)
	}

	var step models.ScenarioStep
	if err := h.db.First(&step, "id = ?", translation.StepID).Error; err != nil {
		return fmt.Errorf("load step %s: %w", translation.StepID, err)
	}
	return h.assertCanManage(step.ScenarioID, ctx.UserID, "translate")
}

func (h *ScenarioStepTranslationAuthorizationHook) assertCanManage(scenarioID uuid.UUID, userID, action string) error {
	scenario, err := loadScenarioByID(h.db, scenarioID)
	if err != nil {
		return err
	}
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, userID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, "scenario")
	}
	return nil
}

type ScenarioTranslationAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	enabled      bool
	priority     int
}

func NewScenarioTranslationAuthorizationHook(db *gorm.DB) *ScenarioTranslationAuthorizationHook {
	return &ScenarioTranslationAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		enabled:      true,
		priority:     5,
	}
}

func (h *ScenarioTranslationAuthorizationHook) GetName() string {
	return "scenario_translation_authorization"
}
func (h *ScenarioTranslationAuthorizationHook) GetEntityName() string { return "ScenarioTranslation" }
func (h *ScenarioTranslationAuthorizationHook) IsEnabled() bool       { return h.enabled }
func (h *ScenarioTranslationAuthorizationHook) GetPriority() int      { return h.priority }
func (h *ScenarioTranslationAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *ScenarioTranslationAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	if ctx.IsAdmin() {
		return nil
	}

	entity := ctx.NewEntity
	if ctx.HookType == hooks.BeforeUpdate && ctx.OldEntity != nil {
		entity = ctx.OldEntity
	}
	translation, ok := entity.(*models.ScenarioTranslation)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioTranslation, got %T", entity)
	}

	scenario, err := loadScenarioByID(h.db, translation.ScenarioID)
	if err != nil {
		return err
	}
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError("translate", "scenario")
	}
	return nil
}
