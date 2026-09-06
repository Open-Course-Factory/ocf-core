package terminalHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KeyBudgetSyncEntities are the entities whose writes can move a user's
// budget ceiling, in the words of GetUserBudgetCeiling: the personal
// subscription, the organization's role plans, and the memberships that
// connect a user to an organization's plan.
var KeyBudgetSyncEntities = []string{"UserSubscription", "OrganizationRolePlan", "OrganizationMember"}

// keyBudgetSyncer is the one thing this hook needs from the terminal service.
type keyBudgetSyncer interface {
	SyncUserKeyBudget(userID string) error
}

// TerminalKeyBudgetSyncHook re-provisions the tt-backend key budget of every
// user whose ceiling an entity write may have moved. It runs After* and its
// failures are recorded, not fatal: the key stays usable with its previous
// budget, and ocf-core's own gate is authoritative.
type TerminalKeyBudgetSyncHook struct {
	db         *gorm.DB
	syncer     keyBudgetSyncer
	entityName string
}

func NewTerminalKeyBudgetSyncHook(db *gorm.DB, syncer keyBudgetSyncer, entityName string) hooks.Hook {
	return &TerminalKeyBudgetSyncHook{db: db, syncer: syncer, entityName: entityName}
}

func (h *TerminalKeyBudgetSyncHook) GetName() string {
	return "terminal_key_budget_sync_" + h.entityName
}
func (h *TerminalKeyBudgetSyncHook) GetEntityName() string { return h.entityName }
func (h *TerminalKeyBudgetSyncHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate, hooks.AfterUpdate, hooks.AfterDelete}
}
func (h *TerminalKeyBudgetSyncHook) IsEnabled() bool  { return true }
func (h *TerminalKeyBudgetSyncHook) GetPriority() int { return 100 }

func (h *TerminalKeyBudgetSyncHook) Execute(ctx *hooks.HookContext) error {
	userIDs, err := h.affectedUserIDs(ctx.NewEntity)
	if err != nil {
		return err
	}
	var failed []string
	for _, userID := range userIDs {
		if syncErr := h.syncer.SyncUserKeyBudget(userID); syncErr != nil {
			utils.Warn("terminal key budget sync after %s %s: user %s: %v", ctx.EntityName, ctx.HookType, userID, syncErr)
			failed = append(failed, userID)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("terminal key budget sync failed for %d user(s): %v", len(failed), failed)
	}
	return nil
}

// affectedUserIDs names whose ceiling the written entity can move. A role
// plan reaches every active member of its organization rather than only the
// mapped role: the role is what may have changed, and a superset costs one
// idempotent sync per member.
func (h *TerminalKeyBudgetSyncHook) affectedUserIDs(entity any) ([]string, error) {
	switch e := entity.(type) {
	case *paymentModels.UserSubscription:
		return []string{e.UserID}, nil
	case *orgModels.OrganizationMember:
		return []string{e.UserID}, nil
	case *paymentModels.OrganizationRolePlan:
		return h.activeMemberIDs(e.OrganizationID)
	default:
		return nil, fmt.Errorf("terminal_key_budget_sync: unexpected entity %T for %s", entity, h.entityName)
	}
}

func (h *TerminalKeyBudgetSyncHook) activeMemberIDs(orgID uuid.UUID) ([]string, error) {
	var userIDs []string
	err := h.db.Model(&orgModels.OrganizationMember{}).
		Where("organization_id = ? AND is_active = ?", orgID, true).
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("terminal_key_budget_sync: list members of organization %s: %w", orgID.String(), err)
	}
	return userIDs, nil
}

var _ hooks.Hook = (*TerminalKeyBudgetSyncHook)(nil)
