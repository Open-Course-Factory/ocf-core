// tests/payment/danglingPlanReference_test.go
//
// #481: a subscription pointing at a plan that no longer exists resolved to a
// BLANK plan, and the budget engine read that blank plan as unlimited.
//
// GORM's Preload honours soft deletes, so a dangling subscription -> plan
// reference leaves the zero-value struct rather than failing. EffectivePlanService
// handed that struct out as a plan; QuotaService reads MaxCPU <= 0 as "no cap on
// that axis" — a legitimate rule for unlimited plans — so a deleted plan silently
// granted every machine size, XL included.
//
// The rule these tests pin: resolution FAILS CLOSED. A plan that did not load is
// not a plan, and no caller can be handed one.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// danglingPlan creates a plan, points callers at its ID, then soft-deletes it —
// reproducing the production state of #481 exactly (the row is gone, every
// foreign key still names it).
func danglingPlan(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		PriceAmount:     1990,
		Currency:        "eur",
		IsActive:        true,
		Priority:        50,
		MaxCPU:          6000,
		MaxMemoryMB:     6144,
		BillingInterval: "month",
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Delete(plan).Error)
	return plan
}

// livePlan creates a plan that resolves normally.
func livePlan(t *testing.T, db *gorm.DB, name string, priority int) *models.SubscriptionPlan {
	t.Helper()

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		PriceAmount:     1200,
		Currency:        "eur",
		IsActive:        true,
		Priority:        priority,
		MaxCPU:          4000,
		MaxMemoryMB:     4096,
		BillingInterval: "month",
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func personalSubscriptionOn(t *testing.T, db *gorm.DB, userID string, planID uuid.UUID) {
	t.Helper()

	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: planID,
		SubscriptionType:   "personal",
		Status:             "active",
	}).Error)
}

// TestEffectivePlan_PersonalSubscriptionOnDeletedPlan_FailsClosed is the shape
// #481 was found in: 'Shared Test Org' held a subscription whose plan was gone,
// and /users/me/features answered name:"" with max_cpu:0.
func TestEffectivePlan_PersonalSubscriptionOnDeletedPlan_FailsClosed(t *testing.T) {
	db := freshTestDB(t)
	userID := "learner-karim"

	gone := danglingPlan(t, db, "Retired Plan")
	personalSubscriptionOn(t, db, userID, gone.ID)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, nil)

	require.Error(t, err, "a subscription whose plan no longer exists must not resolve")
	assert.Nil(t, result)
}

// TestEffectivePlan_DanglingPersonalPlanDoesNotHideAValidOrgPlan guards the
// other half of failing closed: refusing the dangling reference must not refuse
// the user. A plan that still resolves is still the answer.
func TestEffectivePlan_DanglingPersonalPlanDoesNotHideAValidOrgPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer-with-both"

	gone := danglingPlan(t, db, "Retired Plan")
	personalSubscriptionOn(t, db, userID, gone.ID)

	orgPlan := livePlan(t, db, "École / OF", 80)
	orgSubscriptionOn(t, db, userID, orgPlan)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, orgPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
}

// TestEffectivePlan_OrgSubscriptionOnDeletedPlan_FailsClosed pins the org
// branch. Falling back to the member's personal plan would be worse than an
// error: the organization DOES hold a subscription, so silently answering with
// someone's personal plan misattributes the budget scope on top of hiding the
// broken reference.
func TestEffectivePlan_OrgSubscriptionOnDeletedPlan_FailsClosed(t *testing.T) {
	db := freshTestDB(t)
	userID := "member-of-broken-org"

	personalSubscriptionOn(t, db, userID, livePlan(t, db, "Solo", 20).ID)

	gone := danglingPlan(t, db, "Retired Org Plan")
	org := teamOrgWithoutSubscription(t, db, "broken-corp", userID)
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: gone.ID,
		Status:             "active",
	}).Error)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, &org.ID)

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestEffectivePlan_RolePlanOnDeletedPlan_FailsClosed covers the role-plan
// branch, which builds its result from a different association and so needs the
// same guard rather than inheriting it.
func TestEffectivePlan_RolePlanOnDeletedPlan_FailsClosed(t *testing.T) {
	db := freshTestDB(t)
	userID := "member-with-role-plan"

	gone := danglingPlan(t, db, "Retired Role Plan")
	org := teamOrgWithoutSubscription(t, db, "role-corp", userID)
	require.NoError(t, db.Create(&models.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		Role:               "owner",
		SubscriptionPlanID: gone.ID,
	}).Error)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, &org.ID)

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestQuota_DanglingPlanGrantsNothing is the consequence the issue reported:
// max_cpu 0 read as unlimited, so every size — XL included — came back allowed.
func TestQuota_DanglingPlanGrantsNothing(t *testing.T) {
	db := freshTestDB(t)
	userID := "learner-on-dead-plan"

	gone := danglingPlan(t, db, "Retired Plan")
	personalSubscriptionOn(t, db, userID, gone.ID)

	check, err := services.NewEffectivePlanService(db).
		CheckEffectiveUsageLimit(userID, nil, "concurrent_terminals", 1)

	require.Error(t, err, "a dead plan must not authorise a launch")
	assert.Nil(t, check)
}

// TestReportDanglingPlanReferences_NamesEveryBrokenRow: failing closed makes the
// breakage visible to the user, not to the operator. The startup report is what
// tells an operator a row needs repairing, and which one.
func TestReportDanglingPlanReferences_NamesEveryBrokenRow(t *testing.T) {
	db := freshTestDB(t)

	gone := danglingPlan(t, db, "Retired Plan")
	live := livePlan(t, db, "Solo", 20)

	personalSubscriptionOn(t, db, "user-broken", gone.ID)
	personalSubscriptionOn(t, db, "user-fine", live.ID)

	org := teamOrgWithoutSubscription(t, db, "broken-corp", "user-broken")
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: gone.ID,
		Status:             "active",
	}).Error)
	require.NoError(t, db.Create(&models.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		Role:               "member",
		SubscriptionPlanID: gone.ID,
	}).Error)

	report := services.ReportDanglingPlanReferences(db)

	assert.Equal(t, 1, report.UserSubscriptions, "one user subscription points at the deleted plan")
	assert.Equal(t, 1, report.OrganizationSubscriptions)
	assert.Equal(t, 1, report.OrganizationRolePlans)
	assert.True(t, report.Any())
}

// TestReportDanglingPlanReferences_SilentOnAHealthyDatabase keeps the report
// from crying wolf every startup, which is how a real warning gets ignored.
func TestReportDanglingPlanReferences_SilentOnAHealthyDatabase(t *testing.T) {
	db := freshTestDB(t)

	live := livePlan(t, db, "Solo", 20)
	personalSubscriptionOn(t, db, "user-fine", live.ID)

	report := services.ReportDanglingPlanReferences(db)

	assert.False(t, report.Any())
	assert.Equal(t, 0, report.UserSubscriptions)
}
