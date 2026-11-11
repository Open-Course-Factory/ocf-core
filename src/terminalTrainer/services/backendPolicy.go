package services

import (
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
)

// ErrBackendNotAllowed marks a request for a backend outside the context's
// policy. Its message says "not allowed", which the HTTP layer answers with 403.
var ErrBackendNotAllowed = errors.New("backend not allowed")

// BackendPolicy is the one answer to "which backends may this context use".
// Starting a terminal, listing backends for the picker and resolving a
// scenario all read it, so they cannot disagree about it.
type BackendPolicy struct {
	// Allowed holds the usable backend IDs, Default first.
	Allowed []string
	Default string
}

// NewBackendPolicy builds the policy from the first source that says anything:
// the organization's settings, then the plan's, then the system default alone.
// The default is always one of the allowed backends.
func NewBackendPolicy(org *orgModels.Organization, plan *paymentModels.SubscriptionPlan, systemDefault string) BackendPolicy {
	var allowed []string
	var preferred string
	switch {
	case org != nil && (len(org.AllowedBackends) > 0 || org.DefaultBackend != ""):
		allowed, preferred = org.AllowedBackends, org.DefaultBackend
	case plan != nil && (len(plan.AllowedBackends) > 0 || plan.DefaultBackend != ""):
		allowed, preferred = plan.AllowedBackends, plan.DefaultBackend
	default:
		return BackendPolicy{Allowed: []string{systemDefault}, Default: systemDefault}
	}
	if len(allowed) == 0 {
		allowed = []string{preferred}
	}

	def := allowed[0]
	if slices.Contains(allowed, preferred) {
		def = preferred
	} else if slices.Contains(allowed, systemDefault) {
		def = systemDefault
	}

	ordered := []string{def}
	for _, b := range allowed {
		if !slices.Contains(ordered, b) {
			ordered = append(ordered, b)
		}
	}
	return BackendPolicy{Allowed: ordered, Default: def}
}

// Pick returns the backend to start on: the default when none was requested,
// the requested one when allowed. An unlisted request is refused, never
// swapped for another host.
func (p BackendPolicy) Pick(requested string) (string, error) {
	if requested == "" {
		return p.Default, nil
	}
	if slices.Contains(p.Allowed, requested) {
		return requested, nil
	}
	return "", fmt.Errorf("%w: '%s' is not allowed here (allowed: %v)", ErrBackendNotAllowed, requested, p.Allowed)
}

// Candidates returns the backends to try in order: every allowed one when
// none was requested, only the requested one otherwise.
func (p BackendPolicy) Candidates(requested string) ([]string, error) {
	if requested == "" {
		return p.Allowed, nil
	}
	picked, err := p.Pick(requested)
	if err != nil {
		return nil, err
	}
	return []string{picked}, nil
}

// SystemDefaultOf returns the backend tt-backend marks as default, else the
// first one it lists, else "" (tt-backend then uses its own default).
func SystemDefaultOf(backends []dto.BackendInfo) string {
	for _, b := range backends {
		if b.IsDefault {
			return b.ID
		}
	}
	if len(backends) > 0 {
		return backends[0].ID
	}
	return ""
}

// loadBackendPolicy builds the policy for an organization context; orgID and
// plan may each be nil.
func loadBackendPolicy(db *gorm.DB, proxy *terminalProxyClient, orgID *uuid.UUID, plan *paymentModels.SubscriptionPlan) (BackendPolicy, error) {
	var org *orgModels.Organization
	if orgID != nil {
		org = &orgModels.Organization{}
		if err := db.First(org, "id = ?", *orgID).Error; err != nil {
			return BackendPolicy{}, fmt.Errorf("failed to get organization: %w", err)
		}
	}
	return NewBackendPolicy(org, plan, proxy.getSystemDefault()), nil
}
