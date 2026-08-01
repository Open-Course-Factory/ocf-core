package groups_tests

// Tests for #452: the rules deciding whether a group may live in the organization
// it names existed only in groupService.CreateGroup, which nothing called. Every
// group was in fact created through the generic entity route, whose hooks only set
// the owner — so the API accepted any organization id a caller sent.
//
// These drive GroupPlacementValidationHook directly via hook.Execute(ctx), which is
// the reject signal the generic write path surfaces to the caller (before-hooks
// fail fast and abort the write; see hookRegistry.ExecuteHooks).
//
// The case that opened the issue is TestGroupPlacement_RejectsPersonalOrganization;
// the one that made it a security bug rather than a UX wart is
// TestGroupPlacement_RejectsNonMemberOfTargetOrganization.

import (
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	placementTrainerID = "trainer-account"
	placementStranger  = "unrelated-account"
)

// seedClassroomPlan gives userID a personal subscription whose plan grants (or
// does not grant) group management.
func seedClassroomPlan(t *testing.T, db *gorm.DB, userID string, groupManagement bool) {
	t.Helper()

	plan := paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "test-plan",
		Priority:               20,
		IsActive:               true,
		GroupManagementEnabled: groupManagement,
	}
	require.NoError(t, db.Create(&plan).Error)

	sub := paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		SubscriptionType:   "personal",
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour),
		CurrentPeriodEnd:   time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(&sub).Error)
}

// seedOrg creates an organization of the given type and optionally enrols userID
// with a role. An empty role enrols nobody.
func seedOrg(
	t *testing.T,
	db *gorm.DB,
	orgType organizationModels.OrganizationType,
	maxGroups int,
	userID string,
	role organizationModels.OrganizationMemberRole,
) uuid.UUID {
	t.Helper()

	org := organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             "org-" + uuid.NewString()[:8],
		DisplayName:      "Test Org",
		OwnerUserID:      userID,
		OrganizationType: orgType,
		MaxGroups:        maxGroups,
		MaxMembers:       100,
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)

	if role != "" {
		member := organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			UserID:         userID,
			Role:           role,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}
		require.NoError(t, db.Omit("Metadata").Create(&member).Error)
	}

	return org.ID
}

// seedSchool creates a team org that OWNS a classroom-granting plan, which is the
// school / OF shape: every member inherits it, so the plan gate passes for
// students as well as staff and only the role separates them.
func seedSchool(t *testing.T, db *gorm.DB, ownerID string) uuid.UUID {
	t.Helper()

	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, ownerID, organizationModels.OrgRoleOwner)

	plan := paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Ecole / OF",
		Priority:               30,
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(&plan).Error)

	require.NoError(t, db.Create(&paymentModels.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_school",
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour),
		CurrentPeriodEnd:   time.Now().Add(24 * time.Hour),
	}).Error)

	return orgID
}

// enrol adds a member to an existing organization with the given role.
func enrol(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string, role organizationModels.OrganizationMemberRole) {
	t.Helper()

	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
}

// runPlacementCreate runs the hook for a create of a group placed in orgID.
func runPlacementCreate(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, platformRoles ...string) error {
	t.Helper()

	hook := groupHooks.NewGroupPlacementValidationHook(db)
	return hook.Execute(&hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeCreate,
		NewEntity: &groupModels.ClassGroup{
			Name:           "class-1",
			DisplayName:    "Class 1",
			OrganizationID: orgID,
		},
		UserID:    userID,
		UserRoles: platformRoles,
	})
}

func TestGroupPlacement_AllowsManagerWithClassroomPlan(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleOwner)

	require.NoError(t, runPlacementCreate(t, db, placementTrainerID, &orgID),
		"a trainer who manages the org and holds a classroom plan must be able to create a class")
}

// The security case: before #452 this succeeded, letting any authenticated Member
// plant a group inside an organization they have nothing to do with.
func TestGroupPlacement_RejectsNonMemberOfTargetOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementStranger, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementCreate(t, db, placementStranger, &orgID)

	require.Error(t, err, "a non-member must not be able to create a group in someone else's organization")
	require.Contains(t, err.Error(), "not a member")
}

func TestGroupPlacement_RejectsPlainMemberOfOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleMember)

	err := runPlacementCreate(t, db, placementTrainerID, &orgID)

	require.Error(t, err, "a plain member must not create groups even with a classroom plan")
	require.Contains(t, err.Error(), "teachers and managers")
}

// --- the school case (#460) -------------------------------------------------
//
// In a school every member inherits the org's plan, so the plan gate passes for
// students too. The ROLE is what separates staff from students, and these two
// tests are the pair that proves it: same org, same plan, different rank.

func TestGroupPlacement_TeacherInASchoolMayCreateClasses(t *testing.T) {
	db := freshTestDB(t)
	orgID := seedSchool(t, db, placementTrainerID)
	enrol(t, db, orgID, "teacher-marie", organizationModels.OrganizationMemberRole(access.RoleTeacher))

	require.NoError(t, runPlacementCreate(t, db, "teacher-marie", &orgID),
		"a teacher holds the classroom threshold without organization administration")
}

func TestGroupPlacement_StudentInASchoolMayNotCreateClasses(t *testing.T) {
	db := freshTestDB(t)
	orgID := seedSchool(t, db, placementTrainerID)
	enrol(t, db, orgID, "student-karim", organizationModels.OrgRoleMember)

	err := runPlacementCreate(t, db, "student-karim", &orgID)

	require.Error(t, err,
		"a student inherits the school's plan, so only the role can refuse them")
	require.Contains(t, err.Error(), "teachers and managers")
}

// The case that opened the issue: a personal organization is advertised as
// "1 member only, collaboration not available", so it must not hold classes.
func TestGroupPlacement_RejectsPersonalOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypePersonal, -1, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementCreate(t, db, placementTrainerID, &orgID)

	require.Error(t, err, "a personal organization must not hold groups")
	require.Contains(t, err.Error(), "personal organization")
}

func TestGroupPlacement_RejectsWhenPlanLacksGroupManagement(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, false)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementCreate(t, db, placementTrainerID, &orgID)

	require.Error(t, err, "managing the org is not enough — the plan must grant classrooms")
	require.Contains(t, err.Error(), "subscription plan")
}

func TestGroupPlacement_RejectsWhenOrganizationIsAtItsGroupLimit(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 1, placementTrainerID, organizationModels.OrgRoleOwner)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.ClassGroup{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		Name:           "existing",
		DisplayName:    "Existing",
		OwnerUserID:    placementTrainerID,
		OrganizationID: &orgID,
		IsActive:       true,
	}).Error)

	err := runPlacementCreate(t, db, placementTrainerID, &orgID)

	require.Error(t, err, "MaxGroups must actually bound the number of groups")
	require.Contains(t, err.Error(), "group limit")
}

// MaxGroups <= 0 means unlimited, which is how personal orgs were configured. The
// limit check must not turn that into "zero groups allowed".
func TestGroupPlacement_TreatsNonPositiveGroupLimitAsUnlimited(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 0, placementTrainerID, organizationModels.OrgRoleOwner)

	require.NoError(t, runPlacementCreate(t, db, placementTrainerID, &orgID))
}

func TestGroupPlacement_RejectsMissingOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)

	err := runPlacementCreate(t, db, placementTrainerID, nil)

	require.Error(t, err, "a group with no organization must be refused, not silently orphaned")
	require.Contains(t, err.Error(), "must belong to an organization")
}

func TestGroupPlacement_RejectsUnknownOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	unknown := uuid.New()

	err := runPlacementCreate(t, db, placementTrainerID, &unknown)

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// Administrators bypass the permission checks — they operate the platform and are
// not members of their customers' organizations.
func TestGroupPlacement_AdminBypassesMembershipAndPlan(t *testing.T) {
	db := freshTestDB(t)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleOwner)

	require.NoError(t, runPlacementCreate(t, db, "platform-operator", &orgID, "administrator"),
		"an administrator holds no membership and no plan, and must still be able to act")
}

// ...but an administrator does not get to break the data model. Personal orgs
// holding groups is an invariant, not a privilege.
func TestGroupPlacement_AdminDoesNotBypassPersonalOrganization(t *testing.T) {
	db := freshTestDB(t)
	orgID := seedOrg(t, db, organizationModels.OrgTypePersonal, -1, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementCreate(t, db, "platform-operator", &orgID, "administrator")

	require.Error(t, err, "the personal-org invariant holds for administrators too")
	require.Contains(t, err.Error(), "personal organization")
}

// Writes with no authenticated caller are startup seeding, migrations and imports
// that carry their own authorization. Failing closed there would break them.
func TestGroupPlacement_SkipsUnauthenticatedWrites(t *testing.T) {
	db := freshTestDB(t)
	orgID := seedOrg(t, db, organizationModels.OrgTypePersonal, -1, placementTrainerID, organizationModels.OrgRoleOwner)

	require.NoError(t, runPlacementCreate(t, db, "", &orgID))
}

// --- update path -----------------------------------------------------------
//
// BeforeUpdate carries the DtoToMap map of column updates, not a *ClassGroup.
// Reading only the create shape would error on every group edit; reading only the
// update shape would leave creation unguarded.

func runPlacementUpdate(t *testing.T, db *gorm.DB, userID string, groupID uuid.UUID, updates map[string]any) error {
	t.Helper()

	hook := groupHooks.NewGroupPlacementValidationHook(db)
	return hook.Execute(&hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeUpdate,
		EntityID:   groupID,
		NewEntity:  updates,
		UserID:     userID,
	})
}

// Renaming a class is not a placement decision. Re-gating it on entitlement would
// freeze existing classes the moment a plan lapsed, rather than merely stopping
// new ones.
func TestGroupPlacementUpdate_IgnoresUpdatesThatDoNotMoveTheGroup(t *testing.T) {
	db := freshTestDB(t)
	groupID := uuid.New()

	require.NoError(t, runPlacementUpdate(t, db, placementTrainerID, groupID, map[string]any{
		"display_name": "Renamed",
	}))
}

// A PATCH must not be a way around the create-time rules.
func TestGroupPlacementUpdate_RejectsMoveIntoPersonalOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	personalOrgID := seedOrg(t, db, organizationModels.OrgTypePersonal, -1, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementUpdate(t, db, placementTrainerID, uuid.New(), map[string]any{
		"organization_id": personalOrgID,
	})

	require.Error(t, err, "moving a group into a personal org must be refused like creating one there")
	require.Contains(t, err.Error(), "personal organization")
}

func TestGroupPlacementUpdate_RejectsMoveIntoForeignOrganization(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementStranger, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 250, placementTrainerID, organizationModels.OrgRoleOwner)

	err := runPlacementUpdate(t, db, placementStranger, uuid.New(), map[string]any{
		"organization_id": orgID.String(),
	})

	require.Error(t, err, "organization_id arriving as a string must be validated, not waved through")
	require.Contains(t, err.Error(), "not a member")
}

// A group already inside the organization must not count against the limit when
// an unrelated field is patched alongside organization_id.
func TestGroupPlacementUpdate_DoesNotCountTheGroupAgainstItsOwnLimit(t *testing.T) {
	db := freshTestDB(t)
	seedClassroomPlan(t, db, placementTrainerID, true)
	orgID := seedOrg(t, db, organizationModels.OrgTypeTeam, 1, placementTrainerID, organizationModels.OrgRoleOwner)

	groupID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&groupModels.ClassGroup{
		BaseModel:      entityManagementModels.BaseModel{ID: groupID},
		Name:           "only-one",
		DisplayName:    "Only One",
		OwnerUserID:    placementTrainerID,
		OrganizationID: &orgID,
		IsActive:       true,
	}).Error)

	require.NoError(t, runPlacementUpdate(t, db, placementTrainerID, groupID, map[string]any{
		"organization_id": orgID,
	}), "re-stating the group's current organization must not trip the limit it already occupies")
}
