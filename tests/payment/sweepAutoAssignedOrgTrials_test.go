// tests/payment/sweepAutoAssignedOrgTrials_test.go
//
// #448 removed the auto-assignment of a free Trial to team organizations, but
// that only helps orgs created afterwards. Every org created before it still
// holds one, and a Trial outranks its owner's paid personal plan — so until this
// sweep runs, trainers stay locked out of the classroom features they bought.
//
// It runs at startup alongside the other backfills rather than being a manual
// post-deploy step, because a manual step that must happen exactly once, in
// every environment, is a step that gets forgotten.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedOrgWithSubscription wires an org, a plan and an active subscription plus
// the denormalised pointer — the exact shape the auto-assignment left behind.
func seedOrgWithSubscription(
	t *testing.T, db *gorm.DB, orgType organizationModels.OrganizationType,
	plan *models.SubscriptionPlan,
) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		Name:               "org-" + uuid.New().String()[:8],
		DisplayName:        "Org",
		OwnerUserID:        "owner-" + uuid.New().String()[:8],
		IsActive:           true,
		OrganizationType:   orgType,
		IsPersonal:         orgType == organizationModels.OrgTypePersonal,
		SubscriptionPlanID: &plan.ID,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)

	return org
}

func subscriptionStatus(t *testing.T, db *gorm.DB, orgID uuid.UUID) string {
	t.Helper()
	var sub models.OrganizationSubscription
	require.NoError(t, db.Where("organization_id = ?", orgID).First(&sub).Error)
	return sub.Status
}

// TestSweep_CancelsTeamOrgTrial is the migration: marc-corp's exact state.
func TestSweep_CancelsTeamOrgTrial(t *testing.T) {
	db := freshTestDB(t)
	trial := seedPlan(t, db, "Trial", 0)
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, trial)

	initialization.SweepAutoAssignedOrgTrials(db)

	assert.Equal(t, "cancelled", subscriptionStatus(t, db, org.ID),
		"an auto-assigned Trial outranks the owner's paid plan and must go")
	assert.Nil(t, orgPlanPointer(t, db, org.ID),
		"the pointer must follow the subscription, or the org still advertises the plan")
}

// TestSweep_LeavesBespokeOrgPlansAlone — a structure's admin-assigned plan is a
// real commercial arrangement. Sweeping it would cancel a paying customer.
func TestSweep_LeavesBespokeOrgPlansAlone(t *testing.T) {
	db := freshTestDB(t)
	_ = seedPlan(t, db, "Trial", 0)
	bespoke := seedPlan(t, db, "École / OF", 49900)
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, bespoke)

	initialization.SweepAutoAssignedOrgTrials(db)

	assert.Equal(t, "active", subscriptionStatus(t, db, org.ID))
	require.NotNil(t, orgPlanPointer(t, db, org.ID))
	assert.Equal(t, bespoke.ID, *orgPlanPointer(t, db, org.ID))
}

// TestSweep_IsIdempotent — it runs at every startup, so a second pass must be a
// no-op rather than touching rows again.
func TestSweep_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)
	trial := seedPlan(t, db, "Trial", 0)
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, trial)

	initialization.SweepAutoAssignedOrgTrials(db)
	var first models.OrganizationSubscription
	require.NoError(t, db.Where("organization_id = ?", org.ID).First(&first).Error)

	initialization.SweepAutoAssignedOrgTrials(db)
	var second models.OrganizationSubscription
	require.NoError(t, db.Where("organization_id = ?", org.ID).First(&second).Error)

	assert.Equal(t, "cancelled", second.Status)
	assert.Equal(t, first.CancelledAt.Unix(), second.CancelledAt.Unix(),
		"a second pass must not re-stamp cancelled_at — the WHERE should match nothing")
}

// TestSweep_SurvivesAnEmptyCatalog — the empty-input case. No free plan means
// nothing to sweep, not a panic and not a wildcard UPDATE.
func TestSweep_SurvivesAnEmptyCatalog(t *testing.T) {
	db := freshTestDB(t)
	paid := seedPlan(t, db, "Formateur", 1990)
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, paid)

	initialization.SweepAutoAssignedOrgTrials(db)

	assert.Equal(t, "active", subscriptionStatus(t, db, org.ID),
		"with no free plan in the catalog the sweep must touch nothing at all")
}
