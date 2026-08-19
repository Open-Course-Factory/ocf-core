package services

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"soli/formations/src/payment/models"
)

// catalogue.json is the single definition of what OCF sells.
//
// It is a data file rather than Go literals because it has two consumers that
// cannot share code: the startup seed here, and scripts/bootstrap_catalogue.py,
// which creates the same plans in an environment where the seed deliberately
// does not run (production). Expressing the offer twice — once per language — is
// exactly the duplication that lets a price drift on one side and not the other.
//
//go:embed catalogue.json
var catalogueJSON []byte

// DecidedCatalogue returns the plans of the public offer, in the order they
// should be created.
//
// Returns a copy each call: the caller writes to these structs (GORM stamps IDs
// and applied defaults on Create), and a shared slice would leak one run's IDs
// into the next.
func DecidedCatalogue() ([]models.SubscriptionPlan, error) {
	var plans []models.SubscriptionPlan
	if err := json.Unmarshal(catalogueJSON, &plans); err != nil {
		return nil, fmt.Errorf("catalogue.json does not describe a valid plan list: %w", err)
	}
	return plans, nil
}

// FreePlanTemplate returns the plan new signups receive.
//
// The free tier is part of the same catalogue — it is an offer, not an
// implementation detail — but it is created and re-synced by a different routine
// than the paid plans, so it needs to be addressable on its own.
func FreePlanTemplate() (models.SubscriptionPlan, error) {
	plans, err := DecidedCatalogue()
	if err != nil {
		return models.SubscriptionPlan{}, err
	}
	for _, plan := range plans {
		if plan.IsDefaultFree {
			return plan, nil
		}
	}
	return models.SubscriptionPlan{}, fmt.Errorf("no plan in catalogue.json is marked is_default_free")
}

// PaidCatalogue returns every plan that is not the free tier — what the seed
// creates and what the bootstrap script pushes to a fresh environment.
func PaidCatalogue() ([]models.SubscriptionPlan, error) {
	plans, err := DecidedCatalogue()
	if err != nil {
		return nil, err
	}
	paid := make([]models.SubscriptionPlan, 0, len(plans))
	for _, plan := range plans {
		if plan.IsDefaultFree {
			continue
		}
		paid = append(paid, plan)
	}
	return paid, nil
}
