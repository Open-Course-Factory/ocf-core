package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// planWithBudget creates an active plan carrying explicit CPU/RAM budgets.
// Distinct from livePlan (fixed 4000/4096) because these tests turn on the
// exact numbers.
func planWithBudget(t *testing.T, db *gorm.DB, name string, priority, maxCPU, maxMemMB int) *models.SubscriptionPlan {
	t.Helper()

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		PriceAmount:     1000,
		Currency:        "eur",
		IsActive:        true,
		Priority:        priority,
		MaxCPU:          maxCPU,
		MaxMemoryMB:     maxMemMB,
		BillingInterval: "month",
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// addMemberWithRole joins a user to an org under a specific role. The shared
// teamOrgWithoutSubscription helper always creates an owner, and the whole
// point of these tests is the non-owner path.
func addMemberWithRole(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID, role string) {
	t.Helper()

	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         userID,
		Role:           organizationModels.OrganizationMemberRole(role),
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
}

func rolePlanOn(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string, planID uuid.UUID) {
	t.Helper()

	require.NoError(t, db.Create(&models.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		Role:               role,
		SubscriptionPlanID: planID,
	}).Error)
}

// TestUserBudgetCeiling_RolePlanCapsTheLearner is the case that matters: the
// org holds a large pool plan, but the learner's role maps to a small plan.
// The ceiling pushed to their terminal key must be the ROLE plan's, otherwise
// a single learner can consume the whole class budget — the failure mode seen
// in production on 2026-09-04.
func TestUserBudgetCeiling_RolePlanCapsTheLearner(t *testing.T) {
	db := freshTestDB(t)
	teacherID := "teacher-1"
	learnerID := "learner-1"

	orgPlan := planWithBudget(t, db, "Formateur classe", 40, 24000, 12288)
	studentPlan := planWithBudget(t, db, "Siege eleve", 10, 1000, 512)

	org := orgSubscriptionOn(t, db, teacherID, orgPlan)
	addMemberWithRole(t, db, org.ID, learnerID, "member")
	rolePlanOn(t, db, org.ID, "member", studentPlan.ID)

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling(learnerID)

	require.NoError(t, err)
	assert.Equal(t, 1000, ceiling.MaxCPU, "learner must be capped by the role plan, not the org pool")
	assert.Equal(t, 512, ceiling.MaxMemoryMB)
}

// The owner has no role mapping, so they fall through to the org's own plan.
func TestUserBudgetCeiling_FallsBackToOrgPlanWithoutRoleMapping(t *testing.T) {
	db := freshTestDB(t)
	teacherID := "teacher-2"

	orgPlan := planWithBudget(t, db, "Formateur", 40, 24000, 12288)
	studentPlan := planWithBudget(t, db, "Siege eleve", 10, 1000, 512)

	org := orgSubscriptionOn(t, db, teacherID, orgPlan)
	// Mapping exists for 'member' only — the owner must not inherit it.
	rolePlanOn(t, db, org.ID, "member", studentPlan.ID)

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling(teacherID)

	require.NoError(t, err)
	assert.Equal(t, 24000, ceiling.MaxCPU)
	assert.Equal(t, 12288, ceiling.MaxMemoryMB)
}

// A user in several orgs keeps the most generous entitlement: the terminal key
// is global to the user, so capping it at the smallest context would wrongly
// block them everywhere else.
func TestUserBudgetCeiling_TakesTheMostGenerousContext(t *testing.T) {
	db := freshTestDB(t)
	userID := "multi-org-user"

	smallPlan := planWithBudget(t, db, "Small", 10, 2000, 1024)
	bigPlan := planWithBudget(t, db, "Big", 40, 8000, 4096)

	orgA := orgSubscriptionOn(t, db, "owner-a", smallPlan)
	addMemberWithRole(t, db, orgA.ID, userID, "member")

	orgB := orgSubscriptionOn(t, db, "owner-b", bigPlan)
	addMemberWithRole(t, db, orgB.ID, userID, "member")

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling(userID)

	require.NoError(t, err)
	assert.Equal(t, 8000, ceiling.MaxCPU)
	assert.Equal(t, 4096, ceiling.MaxMemoryMB)
}

// A zero budget no longer dominates. It used to mean "unlimited" and win over
// any finite value, so one misconfigured plan silently lifted the ceiling in
// every other context the user acted in. Zero now means no capacity, so the
// larger real budget wins.
func TestUserBudgetCeiling_ZeroDoesNotDominate(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-with-zero-plan"

	finite := planWithBudget(t, db, "Finite", 10, 2000, 1024)
	zero := planWithBudget(t, db, "Zero budget", 40, 0, 0)

	orgA := orgSubscriptionOn(t, db, "owner-f", finite)
	addMemberWithRole(t, db, orgA.ID, userID, "member")

	orgB := orgSubscriptionOn(t, db, "owner-z", zero)
	addMemberWithRole(t, db, orgB.ID, userID, "member")

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling(userID)

	require.NoError(t, err)
	assert.Equal(t, 2000, ceiling.MaxCPU, "the real budget must win over a zero one")
	assert.Equal(t, 1024, ceiling.MaxMemoryMB)
}

// A personal subscription counts as one of the contexts.
func TestUserBudgetCeiling_IncludesPersonalSubscription(t *testing.T) {
	db := freshTestDB(t)
	userID := "solo-user"

	personal := planWithBudget(t, db, "Solo", 30, 6000, 6144)
	personalSubscriptionOn(t, db, userID, personal.ID)

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling(userID)

	require.NoError(t, err)
	assert.Equal(t, 6000, ceiling.MaxCPU)
	assert.Equal(t, 6144, ceiling.MaxMemoryMB)
}

// No entitlement anywhere is not an error — it is a user who may not spawn
// anything, and the caller must be able to tell that apart from "unlimited".
func TestUserBudgetCeiling_NoSubscriptionGrantsNothing(t *testing.T) {
	db := freshTestDB(t)

	ceiling, err := services.NewEffectivePlanService(db).GetUserBudgetCeiling("nobody")

	require.NoError(t, err)
	assert.False(t, ceiling.HasEntitlement, "a user with no plan anywhere must not read as unlimited")
}
