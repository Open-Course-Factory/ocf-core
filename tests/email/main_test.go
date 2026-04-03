package email_tests

import (
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	emailModels "soli/formations/src/email/models"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open test DB: " + err.Error())
	}
	db.AutoMigrate(&emailModels.EmailTemplate{})
	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sharedTestDB.Exec("DELETE FROM email_templates")
	return sharedTestDB
}
