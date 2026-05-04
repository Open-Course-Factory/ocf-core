package admin_test

import (
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
)

// sharedTestDB is a single in-memory SQLite DB reused across all tests in this
// package. Each test calls freshTestDB(t) to truncate state between runs.
var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open shared admin test DB: " + err.Error())
	}

	// AutoMigrate the four GORM models needed by BuildUserListings.
	// These are pure GORM models (no PostgreSQL-specific defaults), so
	// AutoMigrate works against SQLite. Test seeds will Omit the
	// jsonb Metadata and pq text[] OwnerIDs fields to avoid SQLite
	// serialisation edge-cases.
	err = db.AutoMigrate(
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	if err != nil {
		panic("failed to migrate admin test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

// freshTestDB returns the shared DB after wiping the four tables that the
// admin user listing logic depends on. Schema is migrated once in TestMain.
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	return sharedTestDB
}
