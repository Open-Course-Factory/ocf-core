package payment_tests

// #458: an organization's plan overrides its members' own, unconditionally and by
// design — a school's subscription decides for the school. That makes assigning
// the WRONG plan quietly destructive: a trainer whose organization was given a
// Solo plan could no longer create classes and silently dropped to Solo's budget,
// with nothing reporting a problem.
//
// The rule is guarded on BOTH doors. Guarding only the organization subscription
// would leave OrganizationRolePlan open — and resolveForOrg consults role mappings
// FIRST, so that door does not merely match the other one, it outranks it.

import (
	"testing"

	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentHooks "soli/formations/src/payment/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedPlanWithGroupManagement(t *testing.T, db *gorm.DB, name string, groupManagement bool) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		GroupManagementEnabled: groupManagement,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func TestValidateOrgAssignablePlan_Rule(t *testing.T) {
	cases := []struct {
		name    string
		plan    *models.SubscriptionPlan
		allowed bool
	}{
		{
			name:    "a classroom plan may govern an organization",
			plan:    &models.SubscriptionPlan{Name: "École / OF", GroupManagementEnabled: true},
			allowed: true,
		},
		{
			name:    "an individual plan may not",
			plan:    &models.SubscriptionPlan{Name: "Solo"},
			allowed: false,
		},
		{
			name:    "a missing plan may not",
			plan:    nil,
			allowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := services.ValidateOrgAssignablePlan(tc.plan)
			if tc.allowed {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

// Door one.
func TestOrgSubscription_RefusesAnIndividualPlan(t *testing.T) {
	db := freshTestDB(t)
	solo := seedPlanWithGroupManagement(t, db, "Solo", false)
	org := seedOrgOwning(t, db, "marc-formations", "trainer-marc", nil)

	_, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org, solo.ID, "trainer-marc", true)

	require.Error(t, err,
		"an individual plan assigned to an org silently downgrades every member")
	assert.Contains(t, err.Error(), "individual plan")
}

func TestOrgSubscription_AcceptsAClassroomPlan(t *testing.T) {
	db := freshTestDB(t)
	school := seedPlanWithGroupManagement(t, db, "École / OF", true)
	org := seedOrgOwning(t, db, "esitech", "school-admin", nil)

	sub, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org, school.ID, "school-admin", true)

	require.NoError(t, err)
	require.NotNil(t, sub)
}

// Door two — the one resolveForOrg consults first, and the one a guard placed
// only on the subscription path would leave wide open.
func TestOrgRolePlan_RefusesAnIndividualPlan(t *testing.T) {
	db := freshTestDB(t)
	solo := seedPlanWithGroupManagement(t, db, "Solo", false)
	orgID := uuid.New()

	hook := paymentHooks.NewOrganizationRolePlanValidationHook(db)
	err := hook.Execute(&hooks.HookContext{
		EntityName: "OrganizationRolePlan",
		HookType:   hooks.BeforeCreate,
		NewEntity: &models.OrganizationRolePlan{
			OrganizationID:     orgID,
			Role:               "member",
			SubscriptionPlanID: solo.ID,
		},
		UserID:    "platform-operator",
		UserRoles: []string{"administrator"},
	})

	require.Error(t, err,
		"mapping a role to an individual plan reproduces the same downgrade, and "+
			"role mappings take precedence over the org's subscription")
	assert.Contains(t, err.Error(), "individual plan")
}

func TestOrgRolePlan_AcceptsAClassroomPlan(t *testing.T) {
	db := freshTestDB(t)
	teaching := seedPlanWithGroupManagement(t, db, "École / OF", true)

	hook := paymentHooks.NewOrganizationRolePlanValidationHook(db)
	err := hook.Execute(&hooks.HookContext{
		EntityName: "OrganizationRolePlan",
		HookType:   hooks.BeforeCreate,
		NewEntity: &models.OrganizationRolePlan{
			OrganizationID:     uuid.New(),
			Role:               "teacher",
			SubscriptionPlanID: teaching.ID,
		},
		UserID: "platform-operator",
	})

	require.NoError(t, err)
}

// A partial update that does not touch the plan has nothing to validate, and must
// not be blocked by the guard.
func TestOrgRolePlan_UpdateWithoutAPlanChangeIsAllowed(t *testing.T) {
	db := freshTestDB(t)

	hook := paymentHooks.NewOrganizationRolePlanValidationHook(db)
	err := hook.Execute(&hooks.HookContext{
		EntityName: "OrganizationRolePlan",
		HookType:   hooks.BeforeUpdate,
		EntityID:   uuid.New(),
		NewEntity:  map[string]any{"role": "teacher"},
		UserID:     "platform-operator",
	})

	require.NoError(t, err)
}

// Moving a mapping onto an individual plan is the same mistake as creating one.
func TestOrgRolePlan_UpdateOntoAnIndividualPlanIsRefused(t *testing.T) {
	db := freshTestDB(t)
	solo := seedPlanWithGroupManagement(t, db, "Solo", false)

	hook := paymentHooks.NewOrganizationRolePlanValidationHook(db)
	err := hook.Execute(&hooks.HookContext{
		EntityName: "OrganizationRolePlan",
		HookType:   hooks.BeforeUpdate,
		EntityID:   uuid.New(),
		NewEntity:  map[string]any{"subscription_plan_id": solo.ID},
		UserID:     "platform-operator",
	})

	require.Error(t, err)
}
