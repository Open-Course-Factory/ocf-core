package scenarioHooks

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	scenarioServices "soli/formations/src/scenarios/services"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// BackendPolicyFor is the terminal service's answer to "which backends may
// this context use".
type BackendPolicyFor func(orgID *uuid.UUID, plan *paymentModels.SubscriptionPlan) (ttServices.BackendPolicy, error)

// ScenarioAssignmentBackendHook refuses a backend the assignment's
// organization does not allow, when the assignment is saved rather than when
// every learner's launch would be refused. A class's server is a class-wide
// choice, so it is checked against the organization's servers, not the
// teacher's own plan; each launch is still checked against the learner's.
type ScenarioAssignmentBackendHook struct {
	db        *gorm.DB
	policyFor BackendPolicyFor
	hooks.BaseHook
}

func NewScenarioAssignmentBackendHook(db *gorm.DB, policyFor BackendPolicyFor) hooks.Hook {
	return &ScenarioAssignmentBackendHook{
		db:        db,
		policyFor: policyFor,
		BaseHook: hooks.BaseHook{
			Name:       "scenario_assignment_backend",
			EntityName: "ScenarioAssignment",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20,
		},
	}
}

func (h *ScenarioAssignmentBackendHook) Execute(ctx *hooks.HookContext) error {
	assignment, backend, err := assignmentAndBackend(ctx)
	if err != nil || backend == "" {
		return err
	}

	orgID, err := h.organizationOf(assignment)
	if err != nil {
		return err
	}
	policy, err := h.policyFor(orgID, nil)
	if err != nil {
		return err
	}
	if _, err := policy.Pick(backend); err != nil {
		refusal := entityErrors.NewValidationError("backend", err.Error())
		refusal.Err = err
		return refusal
	}
	return nil
}

// assignmentAndBackend returns the assignment being saved and the backend it
// asks for; "" when none is set or an update leaves it alone.
func assignmentAndBackend(ctx *hooks.HookContext) (*models.ScenarioAssignment, string, error) {
	if ctx.HookType == hooks.BeforeCreate {
		a, ok := ctx.NewEntity.(*models.ScenarioAssignment)
		if !ok {
			return nil, "", fmt.Errorf("expected *models.ScenarioAssignment, got %T", ctx.NewEntity)
		}
		return a, a.Backend, nil
	}
	a, ok := ctx.OldEntity.(*models.ScenarioAssignment)
	if !ok {
		return nil, "", fmt.Errorf("expected *models.ScenarioAssignment in OldEntity, got %T", ctx.OldEntity)
	}
	updates, _ := ctx.NewEntity.(map[string]any)
	backend, _ := updates["backend"].(string)
	return a, backend, nil
}

// organizationOf is the organization whose backends the assignment's runs
// use: its own for an org assignment, the class's run organization for a
// class one — the organization a bulk start of it files the runs under.
func (h *ScenarioAssignmentBackendHook) organizationOf(a *models.ScenarioAssignment) (*uuid.UUID, error) {
	if a.Scope != "group" || a.GroupID == nil {
		return a.OrganizationID, nil
	}
	var group groupModels.ClassGroup
	if err := h.db.Select("organization_id").First(&group, "id = ?", *a.GroupID).Error; err != nil {
		return nil, fmt.Errorf("failed to load the assignment's class: %w", err)
	}
	var scenario models.Scenario
	if err := h.db.Select("organization_id").First(&scenario, "id = ?", a.ScenarioID).Error; err != nil {
		return nil, fmt.Errorf("failed to load the assigned scenario: %w", err)
	}
	return scenarioServices.ClassRunOrganization(group.OrganizationID, &scenario), nil
}
