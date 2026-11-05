package services

import (
	"errors"
	"fmt"

	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// SyncUserKeyBudget — see interface doc.
//
// An axis with no budget is omitted rather than cleared: tt-backend cannot
// express "no cap" on update (omitted keeps, zero is rejected), and ocf-core's
// own gate is what refuses a user who holds nothing anyway.
func (tts *terminalTrainerService) SyncUserKeyBudget(userID string) error {
	key, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Debug("SyncUserKeyBudget: user %s holds no active key, nothing to re-provision", userID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load terminal key for user %s: %w", userID, err)
	}
	if key.TerminalTrainerKeyID == 0 {
		return fmt.Errorf("terminal key of user %s carries no tt-backend id (provisioned before it was recorded); recreate the key to re-provision its budget", userID)
	}

	ceiling, err := tts.effectivePlanService.GetUserBudgetCeiling(userID)
	if err != nil {
		return fmt.Errorf("resolve budget ceiling for user %s: %w", userID, err)
	}
	maxCPUTotal, maxMemoryMBTotal := BudgetForTerminalKey(ceiling)
	if maxCPUTotal == nil && maxMemoryMBTotal == nil {
		utils.Debug("SyncUserKeyBudget: user %s holds no budget on either axis, key %d left as is", userID, key.TerminalTrainerKeyID)
		return nil
	}

	budget := map[string]any{}
	if maxCPUTotal != nil {
		budget["max_cpu_total"] = *maxCPUTotal
	}
	if maxMemoryMBTotal != nil {
		budget["max_memory_mb_total"] = *maxMemoryMBTotal
	}
	return tts.updateKeyOnTTBackend(key.TerminalTrainerKeyID, budget)
}
