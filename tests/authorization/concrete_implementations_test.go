package authorization_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
)

// ============================================================================
// Test Models (SQLite-compatible, no jsonb)
// ============================================================================

// TestEntity is a simple entity used to test GormEntityLoader.
// It lives only in test memory — no production table mapping.
type TestEntity struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID string    `gorm:"type:varchar(255)"`
	Name   string
}

func (TestEntity) TableName() string {
	return "test_entities"
}

// testGroupMember is a SQLite-compatible version of groups.GroupMember.
// The real model uses jsonb Metadata which is unsupported by SQLite.
type testGroupMember struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	GroupID  uuid.UUID `gorm:"type:uuid;not null"`
	UserID   string    `gorm:"type:varchar(255);not null"`
	Role     string    `gorm:"type:varchar(50);default:'member'"`
	JoinedAt time.Time `gorm:"not null"`
	IsActive bool      `gorm:"default:true"`
}

func (testGroupMember) TableName() string {
	return "group_members"
}

// testOrgMember is a SQLite-compatible version of organizations.OrganizationMember.
type testOrgMember struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	UserID         string    `gorm:"type:varchar(255);not null"`
	Role           string    `gorm:"type:varchar(50);default:'member'"`
	JoinedAt       time.Time `gorm:"not null"`
	IsActive       bool      `gorm:"default:true"`
}

func (testOrgMember) TableName() string {
	return "organization_members"
}

// ============================================================================
// Helpers
// ============================================================================

// setupEntityLoaderDB creates an in-memory SQLite DB with the test_entities table.
func setupEntityLoaderDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&TestEntity{})
	require.NoError(t, err)
	return db
}

// setupMembershipDB creates an in-memory SQLite DB with group_members and
// organization_members tables (simplified, SQLite-compatible schemas).
func setupMembershipDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&testGroupMember{}, &testOrgMember{})
	require.NoError(t, err)
	return db
}

// ============================================================================
// GormEntityLoader Tests
// ============================================================================

func TestGormEntityLoader_GetOwnerField_Found(t *testing.T) {
	db := setupEntityLoaderDB(t)

	entityID, err := uuid.NewV7()
	require.NoError(t, err)

	entity := TestEntity{
		ID:     entityID,
		UserID: "user-abc-123",
		Name:   "Test Item",
	}
	err = db.Create(&entity).Error
	require.NoError(t, err)

	// The GormEntityLoader must be able to retrieve a specific field value
	// from any entity table given (entityName, entityID, fieldName).
	loader := access.NewGormEntityLoader(db)

	value, err := loader.GetOwnerField("test_entities", entityID.String(), "user_id")
	assert.NoError(t, err)
	assert.Equal(t, "user-abc-123", value)
}

func TestGormEntityLoader_GetOwnerField_NotFound(t *testing.T) {
	db := setupEntityLoaderDB(t)

	nonExistentID, err := uuid.NewV7()
	require.NoError(t, err)

	loader := access.NewGormEntityLoader(db)

	// Looking up a non-existent entity should return an error.
	_, err = loader.GetOwnerField("test_entities", nonExistentID.String(), "user_id")
	assert.Error(t, err, "Should return error when entity not found")
}

func TestGormEntityLoader_GetOwnerField_EmptyID(t *testing.T) {
	db := setupEntityLoaderDB(t)

	loader := access.NewGormEntityLoader(db)

	// Empty ID is invalid and should return an error immediately.
	_, err := loader.GetOwnerField("test_entities", "", "user_id")
	assert.Error(t, err, "Should return error for empty entity ID")
}

// ============================================================================
// GormMembershipChecker Tests
// ============================================================================

func TestGormMembershipChecker_CheckGroupRole_OwnerMeetsManager(t *testing.T) {
	db := setupMembershipDB(t)

	groupID, _ := uuid.NewV7()
	memberID, _ := uuid.NewV7()

	member := testGroupMember{
		ID:       memberID,
		GroupID:  groupID,
		UserID:   "user-owner-001",
		Role:     "owner",
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err := db.Create(&member).Error
	require.NoError(t, err)

	checker := access.NewGormMembershipChecker(db)

	// Owner (priority 100) should meet minimum role "manager" (priority 50).
	allowed, err := checker.CheckGroupRole(groupID.String(), "user-owner-001", "manager")
	assert.NoError(t, err)
	assert.True(t, allowed, "Owner should meet the minimum role of manager")
}

func TestGormMembershipChecker_CheckGroupRole_MemberFailsManager(t *testing.T) {
	db := setupMembershipDB(t)

	groupID, _ := uuid.NewV7()
	memberID, _ := uuid.NewV7()

	member := testGroupMember{
		ID:       memberID,
		GroupID:  groupID,
		UserID:   "user-member-001",
		Role:     "member",
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err := db.Create(&member).Error
	require.NoError(t, err)

	checker := access.NewGormMembershipChecker(db)

	// Member (priority 10) should NOT meet minimum role "manager" (priority 50).
	allowed, err := checker.CheckGroupRole(groupID.String(), "user-member-001", "manager")
	assert.NoError(t, err)
	assert.False(t, allowed, "Member should NOT meet the minimum role of manager")
}

func TestGormMembershipChecker_CheckGroupRole_NotAMember(t *testing.T) {
	db := setupMembershipDB(t)

	groupID, _ := uuid.NewV7()

	checker := access.NewGormMembershipChecker(db)

	// User is not in the group at all — should return false (not an error).
	allowed, err := checker.CheckGroupRole(groupID.String(), "user-nonexistent", "member")
	assert.NoError(t, err)
	assert.False(t, allowed, "Non-member should not pass any role check")
}

func TestGormMembershipChecker_CheckOrgRole_ManagerMeetsManager(t *testing.T) {
	db := setupMembershipDB(t)

	orgID, _ := uuid.NewV7()
	memberID, _ := uuid.NewV7()

	member := testOrgMember{
		ID:             memberID,
		OrganizationID: orgID,
		UserID:         "user-manager-001",
		Role:           "manager",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err := db.Create(&member).Error
	require.NoError(t, err)

	checker := access.NewGormMembershipChecker(db)

	// Manager (priority 50) should meet minimum role "manager" (priority 50).
	allowed, err := checker.CheckOrgRole(orgID.String(), "user-manager-001", "manager")
	assert.NoError(t, err)
	assert.True(t, allowed, "Manager should meet the minimum role of manager")
}

func TestGormMembershipChecker_CheckOrgRole_MemberFailsManager(t *testing.T) {
	db := setupMembershipDB(t)

	orgID, _ := uuid.NewV7()
	memberID, _ := uuid.NewV7()

	member := testOrgMember{
		ID:             memberID,
		OrganizationID: orgID,
		UserID:         "user-member-002",
		Role:           "member",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err := db.Create(&member).Error
	require.NoError(t, err)

	checker := access.NewGormMembershipChecker(db)

	// Member (priority 10) should NOT meet minimum role "manager" (priority 50).
	allowed, err := checker.CheckOrgRole(orgID.String(), "user-member-002", "manager")
	assert.NoError(t, err)
	assert.False(t, allowed, "Member should NOT meet the minimum role of manager")
}
