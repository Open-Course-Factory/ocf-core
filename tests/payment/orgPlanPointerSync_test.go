// tests/payment/orgPlanPointerSync_test.go
//
// #449: organizations.subscription_plan_id is a denormalised copy of "which plan
// does this org have", and nothing kept it in step with the subscription that
// actually decides it.
//
// It was written on purchase and never cleared. marc-corp claimed Formateur
// while running Trial, because the write happened for a subscription that never
// activated. Cancelling leaves the same lie behind: the org keeps pointing at
// the plan it no longer has, and ocf-front drives a real badge off it
// (OrganizationsList.vue: `:has-subscription="!!org.subscription_plan_id"`).
//
// The active subscription is the single source of truth. The column is a cache
// of it — so it gets exactly one writer, which recomputes it from that source
// after every lifecycle change rather than being set opportunistically at call
// sites.
package payment_tests

import (
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func orgPlanPointer(t *testing.T, db *gorm.DB, orgID uuid.UUID) *uuid.UUID {
	t.Helper()
	var org organizationModels.Organization
	require.NoError(t, db.First(&org, "id = ?", orgID).Error)
	return org.SubscriptionPlanID
}

// TestOrgPlanPointer_SetWhenSubscriptionActivates — the admin-assigned path is
// the one that legitimately gives an org its own plan.
func TestOrgPlanPointer_SetWhenSubscriptionActivates(t *testing.T) {
	db := freshTestDB(t)
	userID := "school-owner"
	org := seedOrgForSubscription(t, db, userID)
	bespoke := seedPlan(t, db, "École / OF", 49900)

	_, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, bespoke.ID, userID, true)
	require.NoError(t, err)

	ptr := orgPlanPointer(t, db, org.ID)
	require.NotNil(t, ptr)
	assert.Equal(t, bespoke.ID, *ptr)
}

// TestOrgPlanPointer_ClearedOnImmediateCancel is the drift: an org that cancels
// must stop claiming the plan, or the "has subscription" badge lies forever.
func TestOrgPlanPointer_ClearedOnImmediateCancel(t *testing.T) {
	db := freshTestDB(t)
	userID := "school-owner"
	org := seedOrgForSubscription(t, db, userID)
	bespoke := seedPlan(t, db, "École / OF", 49900)

	svc := services.NewOrganizationSubscriptionService(db)
	_, err := svc.CreateOrganizationSubscription(org.ID, bespoke.ID, userID, true)
	require.NoError(t, err)
	require.NotNil(t, orgPlanPointer(t, db, org.ID), "precondition: the org holds a plan")

	require.NoError(t, svc.CancelOrganizationSubscription(org.ID, false))

	assert.Nil(t, orgPlanPointer(t, db, org.ID),
		"no active subscription means no plan — the column is a cache of that, not a memory of it")
}

// TestOrgPlanPointer_KeptWhenCancellingAtPeriodEnd — coverage runs to the end of
// the paid period, so the org still has its plan and must still say so.
func TestOrgPlanPointer_KeptWhenCancellingAtPeriodEnd(t *testing.T) {
	db := freshTestDB(t)
	userID := "school-owner"
	org := seedOrgForSubscription(t, db, userID)
	bespoke := seedPlan(t, db, "École / OF", 49900)

	svc := services.NewOrganizationSubscriptionService(db)
	_, err := svc.CreateOrganizationSubscription(org.ID, bespoke.ID, userID, true)
	require.NoError(t, err)

	require.NoError(t, svc.CancelOrganizationSubscription(org.ID, true))

	ptr := orgPlanPointer(t, db, org.ID)
	require.NotNil(t, ptr, "cancel-at-period-end keeps the subscription active until it expires")
	assert.Equal(t, bespoke.ID, *ptr)
}

// TestOrgPlanPointer_RefusedPurchaseLeavesItAlone — the marc-corp case: a plan
// that was never granted must not be recorded. Belongs here too because this is
// the invariant, not just a side effect of the refusal added in #450.
func TestOrgPlanPointer_RefusedPurchaseLeavesItAlone(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer"
	org := seedOrgForSubscription(t, db, userID)
	formateur := seedPlan(t, db, "Formateur", 1990)

	_, _ = services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, formateur.ID, userID, false)

	assert.Nil(t, orgPlanPointer(t, db, org.ID))
}
