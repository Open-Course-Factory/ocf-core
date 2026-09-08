package services

import (
	"soli/formations/src/payment/models"
)

// DerivePlanEntitlements projects a plan's TYPED capability fields into the
// canonical entitlement-string set consumed by the feature endpoints. It is the
// single source of truth replacing the legacy free-form plan.Features array on
// the entitlement-resolution paths.
//
// Emitted entitlement registry (the ONLY strings this projection produces):
//   - "group_management" + "multiple_groups"  ← GroupManagementEnabled
//   - "network_access"                        ← NetworkAccessEnabled
//   - "data_persistence"                      ← DataPersistenceEnabled
//   - "command_history"                       ← CommandHistoryRetentionDays > 0
//   - "session_supervision"                   ← SessionSupervisionEnabled
//
// Deliberately NOT emitted: api_access, advanced_terminals. A nil plan or a
// zero-valued plan yields an empty slice.
func DerivePlanEntitlements(plan *models.SubscriptionPlan) []string {
	entitlements := []string{}
	if plan == nil {
		return entitlements
	}
	if plan.GroupManagementEnabled {
		entitlements = append(entitlements, "group_management", "multiple_groups")
	}
	if plan.NetworkAccessEnabled {
		entitlements = append(entitlements, "network_access")
	}
	if plan.DataPersistenceEnabled {
		entitlements = append(entitlements, "data_persistence")
	}
	if plan.CommandHistoryRetentionDays > 0 {
		entitlements = append(entitlements, "command_history")
	}
	if plan.SessionSupervisionEnabled {
		entitlements = append(entitlements, "session_supervision")
	}
	return entitlements
}
