// src/payment/services/stripeProductMetadata.go
//
// Pure (de)serialization helpers for Stripe Product metadata tied to
// SubscriptionPlan budget fields. Kept side-effect free so they can be
// unit-tested without mocking the Stripe SDK.
//
// The Stripe Product metadata mirrors only what the budget engine cares
// about: the plan ID (for reconciliation) plus MaxCPU / MaxMemoryMB.
package services

import (
	"strconv"

	"soli/formations/src/payment/models"
)

// Stripe Product metadata keys for SubscriptionPlan budget fields.
const (
	metadataKeyPlanID      = "plan_id"
	metadataKeyMaxCPU      = "max_cpu"
	metadataKeyMaxMemoryMB = "max_memory_mb"
	// metadataKeyManagedBy stamps provable OCF ownership on products we create,
	// so mirror reconciliation can archive an orphan (a product whose plan_id no
	// longer resolves to a local row) without mistaking a foreign product for ours.
	metadataKeyManagedBy = "managed_by"
)

// metadataValueManagedByOCF is the ownership marker value written into every
// Stripe product OCF pushes. Mirror archival requires this marker (or a matching
// local plan row) as proof of ownership.
const metadataValueManagedByOCF = "ocf"

// PlanProductMetadata is the typed view of a Stripe Product's metadata map
// for the fields ocf-core cares about. It is the reverse of
// BuildPlanProductMetadata and is consumed by the import/reconcile path.
type PlanProductMetadata struct {
	MaxCPU      int
	MaxMemoryMB int
}

// BuildPlanProductMetadata composes the Stripe Product `metadata` map for a
// given plan.
//
// `plan_id` is included so importers can reconcile back to the DB row by ID
// (matches the historical inline behaviour).
func BuildPlanProductMetadata(plan *models.SubscriptionPlan) map[string]string {
	return map[string]string{
		metadataKeyPlanID:      plan.ID.String(),
		metadataKeyMaxCPU:      strconv.Itoa(plan.MaxCPU),
		metadataKeyMaxMemoryMB: strconv.Itoa(plan.MaxMemoryMB),
		metadataKeyManagedBy:   metadataValueManagedByOCF,
	}
}

// ParsePlanProductMetadata extracts the typed budget fields from a Stripe
// Product metadata map. Absent or malformed values fall back to zero
// (which means "unlimited" in the budget engine).
func ParsePlanProductMetadata(metadata map[string]string) PlanProductMetadata {
	parsed := PlanProductMetadata{}

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

	return parsed
}
