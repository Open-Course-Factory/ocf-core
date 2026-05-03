package impersonation_test

import (
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authModels "soli/formations/src/auth/models"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open test DB: " + err.Error())
	}

	if err := db.AutoMigrate(&authModels.ImpersonationSession{}); err != nil {
		panic("failed to migrate ImpersonationSession: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

// freshTestDB returns a clean DB for a single test by truncating the
// impersonation_sessions table. The schema is migrated once in TestMain.
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if err := sharedTestDB.Exec("DELETE FROM impersonation_sessions").Error; err != nil {
		t.Fatalf("failed to reset impersonation_sessions: %v", err)
	}
	return sharedTestDB
}
