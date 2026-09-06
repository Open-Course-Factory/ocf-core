package services

import (
	"errors"
	"fmt"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// SyncUserKeyBudget — see interface doc.
//
// tt-backend's update endpoint replaces name, is_active and limits_disabled
// on every call and keeps a budget field that is omitted, so the current key
// is read first and written back with only the budget changed. An axis with
// no budget is omitted rather than cleared: tt-backend cannot express
// "no cap" on update (omitted keeps, zero is rejected), and ocf-core's own
// gate is what refuses a user who holds nothing anyway.
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

	url := fmt.Sprintf("%s/%s/admin/api-keys/%d", tts.baseURL, tts.apiVersion, key.TerminalTrainerKeyID)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	var current dto.TerminalTrainerAPIKeyResponse
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &current, opts); err != nil {
		return fmt.Errorf("read tt-backend key %d: %w", key.TerminalTrainerKeyID, err)
	}

	payload := map[string]any{
		"name":            current.Data.Name,
		"is_active":       current.Data.IsActive,
		"limits_disabled": current.Data.LimitsDisabled,
	}
	if maxCPUTotal != nil {
		payload["max_cpu_total"] = *maxCPUTotal
	}
	if maxMemoryMBTotal != nil {
		payload["max_memory_mb_total"] = *maxMemoryMBTotal
	}
	if _, err := utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, payload, opts); err != nil {
		return fmt.Errorf("update tt-backend key %d budget: %w", key.TerminalTrainerKeyID, err)
	}
	return nil
}
