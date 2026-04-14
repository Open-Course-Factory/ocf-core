package organizations_tests

// Tests for issue #231: GET /organizations must return organizations where the
// requesting user is ANY member (owner, manager, or plain member), not only
// organizations where they are the owner_user_id.
//
// The fix relies on three pieces working together:
//   1. organizationRegistration.go registers a MembershipConfig pointing at
//      the organization_members table.
//   2. genericRepository.GetAllEntities picks up the MembershipConfig and adds
//      a GenericMembershipFilter to the query.
//   3. When an organization is created, the owner is inserted into
//      organization_members with role=owner (organizationService.go).
//
// These tests bypass the HTTP layer and call the repository directly so that
// any failure points squarely at the SQL filter logic.

import (
	"testing"
	"time"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/repositories"
	"soli/formations/src/organizations/models"
	"reflect"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupMemberVisibilityDB creates a fresh in-memory SQLite DB with the
// organizations and organization_members tables migrated.
func setupMemberVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory SQLite DB")

	err = db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
	)
	require.NoError(t, err, "failed to auto-migrate Organization / OrganizationMember")

	return db
}

// registerOrgMembershipConfig registers a minimal MembershipConfig for
// "Organization" in the global entity registration service.
// It only registers the membership config — no Casdoor/access setup needed.
func registerOrgMembershipConfig() {
	ems.GlobalEntityRegistrationService.RegisterMembershipConfig("Organization",
		&entityManagementInterfaces.MembershipConfig{
			MemberTable:      "organization_members",
			EntityIDColumn:   "organization_id",
			UserIDColumn:     "user_id",
			RoleColumn:       "role",
			IsActiveColumn:   "is_active",
			OrgAccessEnabled: false,
			FeatureProvider:  nil,
		},
	)
}

// seedMemberVisibilityData inserts:
//   - OrgOne owned by userA (with userA in organization_members as owner)
//   - userB as a plain member of OrgOne
//   - userC is not a member of any org
//
// Returns the ID of OrgOne.
func seedMemberVisibilityData(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()

	orgID := uuid.New()

	org := &models.Organization{
		Name:             "OrgOne",
		DisplayName:      "Org One",
		OwnerUserID:      "userA",
		OrganizationType: models.OrgTypeTeam,
		MaxGroups:        250,
		MaxMembers:       50,
		IsActive:         true,
	}
	org.ID = orgID
	// Omit Metadata (jsonb) and OwnerIDs (pq text[]) to avoid SQLite serialisation
	// edge-cases; they are not relevant to the membership filter logic.
	err := db.Omit("Metadata", "OwnerIDs", "Members", "Groups").Create(org).Error
	require.NoError(t, err, "failed to create OrgOne")

	// Insert userA as owner in organization_members (mirrors organizationService.go)
	memberA := &models.OrganizationMember{
		OrganizationID: orgID,
		UserID:         "userA",
		Role:           models.OrgRoleOwner,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(memberA).Error
	require.NoError(t, err, "failed to insert userA as owner member")

	// Insert userB as a plain member of OrgOne
	memberB := &models.OrganizationMember{
		OrganizationID: orgID,
		UserID:         "userB",
		Role:           models.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(memberB).Error
	require.NoError(t, err, "failed to insert userB as plain member")

	// userC has no rows in organization_members (nothing to insert)

	return orgID
}

// extractOrgs extracts the slice of models.Organization from the []any result
// returned by GetAllEntities (which wraps pages as []any{slice}).
func extractOrgs(t *testing.T, results []any) []models.Organization {
	t.Helper()

	if len(results) == 0 {
		return nil
	}

	// results[0] is a []models.Organization (or interface wrapping it)
	raw := results[0]
	rv := reflect.ValueOf(raw)

	var orgs []models.Organization
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i).Interface()
		org, ok := elem.(models.Organization)
		require.True(t, ok, "expected models.Organization in result slice, got %T", elem)
		orgs = append(orgs, org)
	}
	return orgs
}

// TestOrganization_MemberVisibility_NonOwnerMemberSeesOrg asserts that a user
// who is a plain member (role=member) of an org sees it in GetAllEntities.
func TestOrganization_MemberVisibility_NonOwnerMemberSeesOrg(t *testing.T) {
	db := setupMemberVisibilityDB(t)

	// Use a fresh registration service to avoid cross-test contamination.
	original := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	defer func() { ems.GlobalEntityRegistrationService = original }()

	registerOrgMembershipConfig()
	orgID := seedMemberVisibilityData(t, db)

	repo := repositories.NewGenericRepository(db)

	// userB is a plain member — must see OrgOne
	results, total, err := repo.GetAllEntities(
		models.Organization{},
		1, 100,
		map[string]any{"user_member_id": "userB"},
		nil,
	)
	require.NoError(t, err, "GetAllEntities returned an unexpected error for userB")
	assert.Equal(t, int64(1), total,
		"userB is a member of OrgOne — total count must be 1, got %d", total)

	orgs := extractOrgs(t, results)
	require.Len(t, orgs, 1,
		"userB should see exactly 1 organization, got %d", len(orgs))
	assert.Equal(t, orgID, orgs[0].ID,
		"userB should see OrgOne (id=%s), got id=%s", orgID, orgs[0].ID)
}

// TestOrganization_MemberVisibility_NonMemberSeesNothing asserts that a user
// who has no membership row in any org gets an empty result set.
func TestOrganization_MemberVisibility_NonMemberSeesNothing(t *testing.T) {
	db := setupMemberVisibilityDB(t)

	original := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	defer func() { ems.GlobalEntityRegistrationService = original }()

	registerOrgMembershipConfig()
	seedMemberVisibilityData(t, db)

	repo := repositories.NewGenericRepository(db)

	// userC has no membership row — must see nothing
	results, total, err := repo.GetAllEntities(
		models.Organization{},
		1, 100,
		map[string]any{"user_member_id": "userC"},
		nil,
	)
	require.NoError(t, err, "GetAllEntities returned an unexpected error for userC")
	assert.Equal(t, int64(0), total,
		"userC has no membership — total count must be 0, got %d", total)

	orgs := extractOrgs(t, results)
	assert.Empty(t, orgs, "userC should see no organizations, got %v", orgs)
}

// TestOrganization_MemberVisibility_OwnerSeesOrg is a regression guard:
// the org owner (who IS in organization_members as role=owner) must also
// see their org via the membership filter.
func TestOrganization_MemberVisibility_OwnerSeesOrg(t *testing.T) {
	db := setupMemberVisibilityDB(t)

	original := ems.GlobalEntityRegistrationService
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	defer func() { ems.GlobalEntityRegistrationService = original }()

	registerOrgMembershipConfig()
	orgID := seedMemberVisibilityData(t, db)

	repo := repositories.NewGenericRepository(db)

	// userA created OrgOne and is in organization_members as owner — must see it
	results, total, err := repo.GetAllEntities(
		models.Organization{},
		1, 100,
		map[string]any{"user_member_id": "userA"},
		nil,
	)
	require.NoError(t, err, "GetAllEntities returned an unexpected error for userA")
	assert.Equal(t, int64(1), total,
		"userA (owner) should see OrgOne — total count must be 1, got %d", total)

	orgs := extractOrgs(t, results)
	require.Len(t, orgs, 1,
		"userA should see exactly 1 organization, got %d", len(orgs))
	assert.Equal(t, orgID, orgs[0].ID,
		"userA should see OrgOne (id=%s), got id=%s", orgID, orgs[0].ID)
}
