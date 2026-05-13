package services

import (
	"errors"
	"fmt"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
)

// PersistenceMode constants forwarded to tt-backend.
const (
	PersistenceModeEphemeral  = "ephemeral"
	PersistenceModePersistent = "persistent"
)

// ErrPersistenceForbidden is returned when a user requests a persistent
// session on a plan that does not permit persistence. The controller maps
// this (via the "plan_disabled" / "not allowed" pattern) to HTTP 403.
var ErrPersistenceForbidden = errors.New("persistence not available on your plan")

// resolvePersistenceMode normalises the requested persistence_mode against
// the user's plan.
//
//   - empty string  → defaults to ephemeral
//   - "ephemeral"   → always allowed
//   - "persistent"  → only allowed when plan.DataPersistenceEnabled is true,
//                     otherwise returns ErrPersistenceForbidden (hard 403,
//                     no silent downgrade).
//
// SSOT: DataPersistenceEnabled is the single source of truth for "this plan
// permits persistent storage / persistent sessions". It used to coexist with
// a duplicate PersistentSessionsEnabled field; the two drifted (launcher read
// one, gate read the other → user-visible bug) so they were collapsed here.
//
// Any other value is rejected as invalid input.
func resolvePersistenceMode(requested string, plan *paymentModels.SubscriptionPlan) (string, error) {
	switch requested {
	case "":
		return PersistenceModeEphemeral, nil
	case PersistenceModeEphemeral:
		return PersistenceModeEphemeral, nil
	case PersistenceModePersistent:
		if plan == nil || !plan.DataPersistenceEnabled {
			// Wrap the sentinel and include "plan_disabled" so the existing
			// controller error mapping returns 403 instead of 500.
			return "", fmt.Errorf("persistence_mode is not allowed: plan_disabled: %w", ErrPersistenceForbidden)
		}
		return PersistenceModePersistent, nil
	default:
		return "", fmt.Errorf("invalid persistence_mode %q: must be 'ephemeral' or 'persistent'", requested)
	}
}

// ScenarioForcesEphemeral reports whether a scenario must run in ephemeral
// mode regardless of the user's request. Currently any scenario with the
// crash_traps mechanic forces ephemeral, because the trap design relies on
// container destruction (persistence would defeat it). Exported so callers
// (e.g. scenario controller, teacher dashboard) can apply the override
// uniformly before invoking StartComposedSession.
func ScenarioForcesEphemeral(crashTraps bool) bool {
	return crashTraps
}

// computeIdleWindowSeconds returns the org-level idle window override for the
// given persistence mode, or nil if the org has no override configured.
// nil means "tt-backend falls back to its global default".
func computeIdleWindowSeconds(org *orgModels.Organization, mode string) *int {
	if org == nil {
		return nil
	}
	switch mode {
	case PersistenceModePersistent:
		return org.IdleWindowPersistentSeconds
	default:
		// "ephemeral" or empty (callers should pass the resolved mode, but be
		// defensive against an empty value).
		return org.IdleWindowEphemeralSeconds
	}
}
