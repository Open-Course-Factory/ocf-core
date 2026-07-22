package services

import (
	"soli/formations/src/payment/models"
)

// derivePlanEntitlements projects a plan's TYPED capability fields into the
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
func derivePlanEntitlements(plan *models.SubscriptionPlan) []string {
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

// DerivePlanEntitlements is the exported accessor for derivePlanEntitlements. It
// lets cross-package feature-resolution paths (e.g. the organizations module's
// OrganizationFeatureProvider) project a plan's typed capability fields through
// the same single source of truth, keeping the projection logic single-homed.
func DerivePlanEntitlements(plan *models.SubscriptionPlan) []string {
	return derivePlanEntitlements(plan)
}
