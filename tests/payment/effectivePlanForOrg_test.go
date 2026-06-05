// tests/payment/effectivePlanForOrg_test.go
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

// createOrgWithSubscriptionAndType creates an org with a specific type and subscription.
func createOrgWithSubscriptionAndType(
	t *testing.T, db *gorm.DB,
	orgName string, userID string,
	plan *models.SubscriptionPlan,
	orgType organizationModels.OrganizationType,
) (*organizationModels.Organization, *models.OrganizationSubscription) {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: orgType,
		IsPersonal:       orgType == organizationModels.OrgTypePersonal,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	orgSub := &models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
		Quantity:           1,
	}
	err = db.Create(orgSub).Error
	assert.NoError(t, err)

	return org, orgSub
}

// createPersonalOrgWithoutSubscription creates a personal org without any subscription.
func createPersonalOrgWithoutSubscription(t *testing.T, db *gorm.DB, orgName string, userID string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypePersonal,
		IsPersonal:       true,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	return org
}

// createTeamOrgWithoutSubscription creates a team org without any subscription.
func createTeamOrgWithoutSubscription(t *testing.T, db *gorm.DB, orgName string, userID string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		IsPersonal:       false,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	return org
}

// addOrgMemberWithRole adds an active OrganizationMember with the given role to an org.
func addOrgMemberWithRole(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role organizationModels.OrganizationMemberRole) *organizationModels.OrganizationMember {
	t.Helper()
	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err := db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)
	return member
}

// createOrgRolePlan inserts an OrganizationRolePlan mapping (org+role -> plan).
func createOrgRolePlan(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string, plan *models.SubscriptionPlan) *models.OrganizationRolePlan {
	t.Helper()
	orp := &models.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		Role:               role,
		SubscriptionPlanID: plan.ID,
	}
	err := db.Create(orp).Error
	assert.NoError(t, err)
	return orp
}

// --- Role-based plan resolution tests (#357) ---

// TestGetUserEffectivePlanForOrg_RoleMapping_WinsOverOrgDefault verifies that when
// the org has a role->plan mapping for the user's role, that mapped plan is used
// instead of the org's default OrganizationSubscription plan.
func TestGetUserEffectivePlanForOrg_RoleMapping_WinsOverOrgDefault(t *testing.T) {
	db := freshTestDB(t)

	ownerUserID := "role-map-owner"
	managerUserID := "role-map-manager"

	basicPlan := createPlan(t, db, "Basic", 10, 0)
	proPlan := createPlan(t, db, "Pro", 50, 0)

	// Team org with default subscription = Basic
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "role-map-team", ownerUserID, basicPlan, organizationModels.OrgTypeTeam)

	// manager user is a member with role "manager"
	addOrgMemberWithRole(t, db, teamOrg.ID, managerUserID, "manager")

	// mapping: manager -> Pro
	createOrgRolePlan(t, db, teamOrg.ID, "manager", proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(managerUserID, &teamOrg.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, proPlan.ID, result.Plan.ID, "manager's role mapping (Pro) should win over org default (Basic)")
	assert.Equal(t, "Pro", result.Plan.Name)
}

// TestGetUserEffectivePlanForOrg_NoRoleMapping_FallsBackToOrgDefault verifies that
// when there is no mapping for the user's role, resolution falls back to the org's
// default OrganizationSubscription plan (current behavior preserved).
func TestGetUserEffectivePlanForOrg_NoRoleMapping_FallsBackToOrgDefault(t *testing.T) {
	db := freshTestDB(t)

	ownerUserID := "role-fallback-owner"
	memberUserID := "role-fallback-member"

	basicPlan := createPlan(t, db, "Basic", 10, 0)
	proPlan := createPlan(t, db, "Pro", 50, 0)

	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "role-fallback-team", ownerUserID, basicPlan, organizationModels.OrgTypeTeam)

	// plain member, no mapping for role "member"
	addOrgMemberWithRole(t, db, teamOrg.ID, memberUserID, "member")

	// mapping only for manager
	createOrgRolePlan(t, db, teamOrg.ID, "manager", proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(memberUserID, &teamOrg.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, basicPlan.ID, result.Plan.ID, "member with no role mapping should get org default (Basic)")
	assert.Equal(t, "Basic", result.Plan.Name)
}

// TestGetUserEffectivePlanForOrg_ManagerVsMember_SameOrg_Diverge verifies that within
// the SAME org, a manager and a member resolve to DIFFERENT plans driven by the
// role mapping.
func TestGetUserEffectivePlanForOrg_ManagerVsMember_SameOrg_Diverge(t *testing.T) {
	db := freshTestDB(t)

	ownerUserID := "diverge-owner"
	managerUserID := "diverge-manager"
	memberUserID := "diverge-member"

	basicPlan := createPlan(t, db, "Basic", 10, 0)
	proPlan := createPlan(t, db, "Pro", 50, 0)

	// org default = Basic
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "diverge-team", ownerUserID, basicPlan, organizationModels.OrgTypeTeam)

	addOrgMemberWithRole(t, db, teamOrg.ID, managerUserID, "manager")
	addOrgMemberWithRole(t, db, teamOrg.ID, memberUserID, "member")

	// mapping: manager -> Pro, member -> Basic
	createOrgRolePlan(t, db, teamOrg.ID, "manager", proPlan)
	createOrgRolePlan(t, db, teamOrg.ID, "member", basicPlan)

	svc := services.NewEffectivePlanService(db)

	managerResult, err := svc.GetUserEffectivePlan(managerUserID, &teamOrg.ID)
	require.NoError(t, err)
	require.NotNil(t, managerResult)
	assert.Equal(t, proPlan.ID, managerResult.Plan.ID, "manager should resolve to Pro")
	assert.Equal(t, services.PlanSourceOrganization, managerResult.Source)

	memberResult, err := svc.GetUserEffectivePlan(memberUserID, &teamOrg.ID)
	require.NoError(t, err)
	require.NotNil(t, memberResult)
	assert.Equal(t, basicPlan.ID, memberResult.Plan.ID, "member should resolve to Basic")
	assert.Equal(t, services.PlanSourceOrganization, memberResult.Source)

	assert.NotEqual(t, managerResult.Plan.ID, memberResult.Plan.ID,
		"same org should yield different plans for manager vs member")
}

// TestGetUserEffectivePlanForOrg_NoRolePlanRows_BackwardCompatible verifies that a
// team org with a default subscription and NO OrganizationRolePlan rows resolves
// every member to the org default plan exactly as before this feature.
func TestGetUserEffectivePlanForOrg_NoRolePlanRows_BackwardCompatible(t *testing.T) {
	db := freshTestDB(t)

	ownerUserID := "compat-owner"
	memberUserID := "compat-member"

	basicPlan := createPlan(t, db, "Basic", 10, 0)

	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "compat-team", ownerUserID, basicPlan, organizationModels.OrgTypeTeam)
	addOrgMemberWithRole(t, db, teamOrg.ID, memberUserID, "member")

	// No OrganizationRolePlan rows at all.

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(memberUserID, &teamOrg.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, basicPlan.ID, result.Plan.ID, "with no role-plan rows, member gets org default (Basic)")
	assert.Equal(t, "Basic", result.Plan.Name)
}

// --- GetUserEffectivePlanForOrg tests ---

func TestGetUserEffectivePlanForOrg_NilOrg_FallsBackToGlobal(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-nil-org"

	proPlan := createPlan(t, db, "Pro", 10, 5)
	createUserSubscription(t, db, userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, proPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
}

func TestGetUserEffectivePlanForOrg_PersonalOrg_ReturnsPersonalPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-personal-org"

	personalPlan := createPlan(t, db, "PersonalPlan", 5, 2)
	teamPlan := createPlan(t, db, "TeamPlan", 20, 10)

	createUserSubscription(t, db, userID, personalPlan)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "my-personal", userID)
	createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, &personalOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.Equal(t, personalPlan.ID, result.Plan.ID)
	assert.Equal(t, "PersonalPlan", result.Plan.Name)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlanForOrg_TeamOrg_ReturnsOrgPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-team-org"

	personalPlan := createPlan(t, db, "PersonalPlan", 5, 2)
	teamPlan := createPlan(t, db, "TeamPlan", 20, 10)

	createUserSubscription(t, db, userID, personalPlan)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, &teamOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, teamPlan.ID, result.Plan.ID)
	assert.Equal(t, "TeamPlan", result.Plan.Name)
	assert.Nil(t, result.UserSubscription)
	assert.NotNil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlanForOrg_PersonalOrg_NoPersonalSub_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-no-personal-sub"

	// Create a personal org but no personal subscription
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "my-personal", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, &personalOrg.ID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active personal subscription")
}

func TestGetUserEffectivePlanForOrg_TeamOrg_NoSubscription_NoPersonalSub_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-team-no-sub"

	// Create a team org without any subscription (and no personal subscription either)
	teamOrg := createTeamOrgWithoutSubscription(t, db, "my-team", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, &teamOrg.ID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no personal fallback")
}

func TestGetUserEffectivePlanForOrg_NonexistentOrg_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-nonexistent-org"

	fakeOrgID := uuid.New()

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID, &fakeOrgID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to load organization")
}

// --- Security: org membership validation tests ---

// TestGetUserEffectivePlanForOrg_NonMember_TeamOrg_ShouldRejectAccess verifies that a user
// who is NOT a member of a team org cannot access that org's subscription plan.
// BUG: The current implementation only checks that the org exists and has a subscription,
// but does NOT verify the requesting user is a member of the org. This allows any user
// to escalate their plan by passing another org's ID.
func TestGetUserEffectivePlanForOrg_NonMember_TeamOrg_ShouldRejectAccess(t *testing.T) {
	db := freshTestDB(t)

	ownerUserID := "org-owner-user"
	attackerUserID := "attacker-user"

	// Create a premium plan and attach it to a team org owned by ownerUserID
	premiumPlan := createPlan(t, db, "PremiumTeamPlan", 100, 50)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "premium-team", ownerUserID, premiumPlan, organizationModels.OrgTypeTeam)

	// attackerUserID is NOT a member of teamOrg — no OrganizationMember row exists for them.
	// They should NOT be able to access this org's plan.

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(attackerUserID, &teamOrg.ID)

	// EXPECTED: error indicating the user is not a member of the org
	// CURRENT BUG: returns the org's premium plan without any membership check
	assert.Error(t, err, "should reject access for non-member user")
	assert.Nil(t, result, "should not return a plan for non-member user")
	if err != nil {
		assert.Contains(t, err.Error(), "not a member",
			"error should indicate the user is not a member of the organization")
	}
}

// TestCheckEffectiveUsageLimitForOrg_NonMember_ShouldRejectAccess verifies that
// CheckEffectiveUsageLimit also rejects non-member access for org-scoped checks.
func TestCheckEffectiveUsageLimitForOrg_NonMember_ShouldRejectAccess(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)

	ownerUserID := "org-owner-for-limit"
	attackerUserID := "attacker-for-limit"

	// Plan with a courses cap that we can exercise without touching the
	// budget engine (the latter has its own dedicated tests).
	generousPlan := createPlan(t, db, "GenerousPlan", 50, 0)
	generousPlan.MaxCourses = 100
	require.NoError(t, db.Save(generousPlan).Error)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "generous-team", ownerUserID, generousPlan, organizationModels.OrgTypeTeam)

	// attackerUserID is NOT a member — should not be able to check limits against this org

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(attackerUserID, &teamOrg.ID, "courses_created", 1)

	assert.Error(t, err, "should reject usage limit check for non-member user")
	assert.Nil(t, check, "should not return usage limits for non-member user")
}
