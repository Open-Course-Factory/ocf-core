// src/payment/services/stripeProductMetadata.go
//
// Pure (de)serialization helpers for Stripe Product metadata tied to
// SubscriptionPlan budget fields. Kept side-effect free so they can be
// unit-tested without mocking the Stripe SDK.
//
// Dual-mode policy (MR-CORE-7):
//   - Writes always emit BOTH the legacy `max_concurrent_terminals` key AND
//     the three new budget keys (`max_cpu`, `max_memory_mb`, `quota_model`).
//   - Reads prefer the new keys; they fall back to the legacy key only when
//     the new keys are absent or malformed.
//   - Empty/absent metadata resolves to safe defaults: QuotaModel="count",
//     MaxCPU=0, MaxMemoryMB=0, MaxConcurrentTerminals=0.
//
// The legacy `max_concurrent_terminals` key is kept for one release to give
// any external readers time to migrate; removal is tracked as a future
// cleanup.
package services

import (
	"strconv"

	"soli/formations/src/payment/models"
)

// Stripe Product metadata keys for SubscriptionPlan budget fields.
const (
	metadataKeyPlanID                 = "plan_id"
	metadataKeyMaxConcurrentTerminals = "max_concurrent_terminals" // legacy (count-mode)
	metadataKeyMaxCPU                 = "max_cpu"                  // new (budget-mode)
	metadataKeyMaxMemoryMB            = "max_memory_mb"            // new (budget-mode)
	metadataKeyQuotaModel             = "quota_model"              // new ("count" | "budget")
)

// PlanProductMetadata is the typed view of a Stripe Product's metadata map
// for the fields ocf-core cares about. It is the reverse of
// BuildPlanProductMetadata and is consumed by the import/reconcile path.
type PlanProductMetadata struct {
	MaxConcurrentTerminals int    // from legacy `max_concurrent_terminals`
	MaxCPU                 int    // from new `max_cpu`
	MaxMemoryMB            int    // from new `max_memory_mb`
	QuotaModel             string // from new `quota_model`; defaults to "count"
}

// BuildPlanProductMetadata composes the Stripe Product `metadata` map for a
// given plan. Always emits the legacy `max_concurrent_terminals` key AND the
// three new budget keys — see the package doc for the dual-mode policy.
//
// `plan_id` is included so importers can reconcile back to the DB row by ID
// (matches the historical inline behavior).
func BuildPlanProductMetadata(plan *models.SubscriptionPlan) map[string]string {
	quotaModel := plan.QuotaModel
	if quotaModel == "" {
		// In-memory zero-value plans (e.g. tests, partially-constructed
		// rows) shouldn't push an empty string into Stripe. The DB default
		// is "count", so mirror it here.
		quotaModel = "count"
	}

	return map[string]string{
		metadataKeyPlanID:                 plan.ID.String(),
		metadataKeyMaxConcurrentTerminals: strconv.Itoa(plan.MaxConcurrentTerminals),
		metadataKeyMaxCPU:                 strconv.Itoa(plan.MaxCPU),
		metadataKeyMaxMemoryMB:            strconv.Itoa(plan.MaxMemoryMB),
		metadataKeyQuotaModel:             quotaModel,
	}
}

// ParsePlanProductMetadata extracts the typed budget fields from a Stripe
// Product metadata map. New keys are preferred; absent or malformed values
// fall back to zero/"count" defaults.
//
// Note: the function intentionally does NOT fall the new-keys' values back
// to the legacy `max_concurrent_terminals` value — they describe different
// concepts (count-mode terminal cap vs. CPU/RAM budget). The legacy key is
// surfaced via MaxConcurrentTerminals so callers can still apply count-mode
// semantics when QuotaModel resolves to "count".
func ParsePlanProductMetadata(metadata map[string]string) PlanProductMetadata {
	parsed := PlanProductMetadata{
		QuotaModel: "count", // default for empty / missing quota_model
	}

	if v, ok := metadata[metadataKeyMaxCPU]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			parsed.MaxCPU = n
		}
	}
	if v, ok := metadata[metadataKeyMaxMemoryMB]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			parsed.MaxMemoryMB = n
		}
	}
	if v, ok := metadata[metadataKeyMaxConcurrentTerminals]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			parsed.MaxConcurrentTerminals = n
		}
	}
	if v, ok := metadata[metadataKeyQuotaModel]; ok && v != "" {
		parsed.QuotaModel = v
	}

	return parsed
}
