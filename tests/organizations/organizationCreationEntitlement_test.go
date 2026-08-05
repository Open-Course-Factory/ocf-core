package organizations_tests

// Tests for #476: creating a TEAM organization is the teaching tier's capability.
//
// Trial exists to try terminals and Solo to use them alone; Formateur is the tier
// that buys the right to run classes, and an organization is the container classes
// live in. Team-org creation was nonetheless open to every authenticated Member,
// so a learner on Trial could stand up an organization they had no plan to use —
// the same shape of gap #452 closed for groups and #458 closed for the
// personal-to-team conversion.
//
// These drive OrganizationCreationEntitlementHook directly via hook.Execute(ctx):
// organizations are created through the generic entity route (there is no POST
// /organizations controller), and a Before* hook returning an error aborts that
// write. See hookRegistry.ExecuteHooks.
//
// TestOrganizationCreationEntitlement_PersonalWorkspaceBootstrapIsUntouched is the
// regression to watch: every new user gets a personal organization at signup,
// before they could possibly hold any plan. Gating that path would lock every new
// account out of the product.

import (
	"testing"
	"time"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	organizationHooks "soli/formations/src/organizations/hooks"
	organizationModels "soli/formations/src/organizations/models"
	organizationServices "soli/formations/src/organizations/services"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	orgGateTrainerID = "trainer-wanting-an-org"
	orgGateLearnerID = "learner-on-trial"
)

// orgGateDB builds a throwaway in-memory database holding the tables the
// entitlement lookup reads.
//
// Built per test rather than through a package-level TestMain: this package has
// none, and the entitlement path touches few enough tables that isolation is
// cheaper than shared-state cleanup.
func orgGateDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&paymentModels.SubscriptionPlan{},
		&paymentModels.UserSubscription{},
		&paymentModels.SubscriptionBatch{},
		&paymentModels.OrganizationSubscription{},
		// resolveForOrg consults the role-plan table before falling back to the
		// org's default subscription and treats any error other than
		// ErrRecordNotFound as fatal, so an unmigrated table here would report
		// "no plan" instead of "no role mapping".
		&paymentModels.OrganizationRolePlan{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&groupModels.ClassGroup{},
	))

	// Metadata is a jsonb column SQLite cannot write; the same callback the
	// payment suite installs keeps org writes working here.
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("org_gate_omit_metadata_for_sqlite", func(tx *gorm.DB) {
			if tx.Statement == nil || tx.Statement.Schema == nil {
				return
			}
			table := tx.Statement.Schema.Table
			if table == "organizations" || table == "organization_members" {
				tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
			}
		}))

	return db
}

// seedPersonalPlan gives userID an active personal subscription on a plan that
// does or does not grant group management — the plan half of the classroom rule.
func seedPersonalPlan(t *testing.T, db *gorm.DB, userID, planName string, groupManagement bool) {
	t.Helper()

	plan := paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   planName,
		Priority:               20,
		IsActive:               true,
		GroupManagementEnabled: groupManagement,
	}
	require.NoError(t, db.Create(&plan).Error)

	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		SubscriptionType:   "personal",
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour),
		CurrentPeriodEnd:   time.Now().Add(24 * time.Hour),
	}).Error)
}

// runOrgCreate runs the hook for a create of an organization of the given type,
// which is the shape the generic entity route hands to BeforeCreate hooks.
func runOrgCreate(
	t *testing.T,
	db *gorm.DB,
	userID string,
	orgType organizationModels.OrganizationType,
	platformRoles ...string,
) error {
	t.Helper()

	hook := organizationHooks.NewOrganizationCreationEntitlementHook(db)
	return hook.Execute(&hooks.HookContext{
		EntityName: "Organization",
		HookType:   hooks.BeforeCreate,
		NewEntity: &organizationModels.Organization{
			Name:             "acme-training",
			DisplayName:      "Acme Training",
			OrganizationType: orgType,
			IsActive:         true,
		},
		UserID:    userID,
		UserRoles: platformRoles,
	})
}

// requireRefusedWith asserts the refusal reached the client as a 403 carrying the
// machine-readable reason the frontend renders its upgrade prompt from.
func requireRefusedWith(t *testing.T, err error, wantReason string) {
	t.Helper()

	require.Error(t, err)

	var entityErr *entityErrors.EntityError
	require.ErrorAs(t, err, &entityErr,
		"the refusal must be a structured EntityError, or WrapHookError turns it into a 500")
	assert.Equal(t, 403, entityErr.HTTPStatus,
		"refusing on entitlement is 'you may not', not 'we broke'")
	assert.Equal(t, wantReason, entityErr.Details["classroom_denied_reason"],
		"the frontend keys its upgrade prompt off this code")
}

// The DtoToModel converter leaves OrganizationType empty, and the model's
// BeforeSave normalizes anything that is not "personal" to "team". So the type
// arriving at the hook on a real API create is the zero value, and it must be
// gated exactly like an explicit team org — reading it as "not a team" is how
// this gate would silently do nothing in production.
func TestOrganizationCreationEntitlement_GatesTheShapeTheApiActuallySends(t *testing.T) {
	db := orgGateDB(t)
	seedPersonalPlan(t, db, orgGateLearnerID, "Trial", false)

	err := runOrgCreate(t, db, orgGateLearnerID, "")

	requireRefusedWith(t, err, paymentServices.ClassroomDeniedPlanLacksGroupManagement)
}

func TestOrganizationCreationEntitlement_RefusesTrialPlan(t *testing.T) {
	db := orgGateDB(t)
	seedPersonalPlan(t, db, orgGateLearnerID, "Trial", false)

	err := runOrgCreate(t, db, orgGateLearnerID, organizationModels.OrgTypeTeam)

	requireRefusedWith(t, err, paymentServices.ClassroomDeniedPlanLacksGroupManagement)
}

// Solo is a bigger terminal for one person, not a teaching tier. It fails the same
// predicate as Trial, which is the point: the plan flag decides, not the price.
func TestOrganizationCreationEntitlement_RefusesSoloPlan(t *testing.T) {
	db := orgGateDB(t)
	seedPersonalPlan(t, db, orgGateLearnerID, "Solo", false)

	err := runOrgCreate(t, db, orgGateLearnerID, organizationModels.OrgTypeTeam)

	requireRefusedWith(t, err, paymentServices.ClassroomDeniedPlanLacksGroupManagement)
}

func TestOrganizationCreationEntitlement_AllowsFormateurPlan(t *testing.T) {
	db := orgGateDB(t)
	seedPersonalPlan(t, db, orgGateTrainerID, "Formateur", true)

	require.NoError(t, runOrgCreate(t, db, orgGateTrainerID, organizationModels.OrgTypeTeam),
		"Formateur is the tier that buys the capacity to create organizations")
}

// A user with no subscription row at all must be refused on a distinct code, and
// without the entitlement lookup panicking on the nil plan it resolves to.
func TestOrganizationCreationEntitlement_RefusesUserWithNoPlanAtAll(t *testing.T) {
	db := orgGateDB(t)

	err := runOrgCreate(t, db, "user-who-never-subscribed", organizationModels.OrgTypeTeam)

	requireRefusedWith(t, err, paymentServices.ClassroomDeniedNoPlan)
}

// Platform administrators operate the platform and hold no plan of their own.
func TestOrganizationCreationEntitlement_AdministratorBypassesThePlanCheck(t *testing.T) {
	db := orgGateDB(t)

	require.NoError(t,
		runOrgCreate(t, db, "platform-operator", organizationModels.OrgTypeTeam, "administrator"),
		"an administrator provisioning a customer's organization holds no subscription")
}

// A personal workspace is not a classroom container, so its creation is not a
// teaching capability and is never gated.
func TestOrganizationCreationEntitlement_DoesNotGatePersonalWorkspaces(t *testing.T) {
	db := orgGateDB(t)

	require.NoError(t,
		runOrgCreate(t, db, orgGateLearnerID, organizationModels.OrgTypePersonal),
		"a personal workspace is what a planless user is entitled to")
}

// Writes with no authenticated caller are startup seeding, migrations and imports
// that carry their own authorization. Failing closed there would break them.
func TestOrganizationCreationEntitlement_SkipsUnauthenticatedWrites(t *testing.T) {
	db := orgGateDB(t)

	require.NoError(t, runOrgCreate(t, db, "", organizationModels.OrgTypeTeam))
}

// THE REGRESSION. userService creates a personal organization for every new user
// at signup, through organizationService.CreatePersonalOrganization →
// repository.CreateOrganization, which writes with raw GORM and never reaches the
// entity-management hooks. This test walks that real path for a user holding no
// plan whatsoever: if the gate ever migrates into the service or the repository,
// signup breaks for everybody and this is what says so.
func TestOrganizationCreationEntitlement_PersonalWorkspaceBootstrapIsUntouched(t *testing.T) {
	db := orgGateDB(t)
	casdoor.Enforcer = mocks.NewMockEnforcer()

	newUserID := uuid.NewString()

	org, err := organizationServices.NewOrganizationService(db).
		CreatePersonalOrganization(newUserID, "Brand New User")

	require.NoError(t, err,
		"a user who has just signed up holds no plan and must still get their workspace")
	require.NotNil(t, org)
	assert.True(t, org.IsPersonalOrg())
	assert.Equal(t, newUserID, org.OwnerUserID)

	// Witness the row, not just the return value.
	var persisted organizationModels.Organization
	require.NoError(t, db.First(&persisted, "id = ?", org.ID).Error)
	assert.Equal(t, organizationModels.OrgTypePersonal, persisted.OrganizationType)

	var memberCount int64
	require.NoError(t, db.Model(&organizationModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", org.ID, newUserID).
		Count(&memberCount).Error)
	assert.EqualValues(t, 1, memberCount, "the new user must own their personal workspace")
}
