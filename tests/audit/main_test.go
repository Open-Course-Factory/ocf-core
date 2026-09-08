package audit_tests

import (
	"os"
	"testing"

	auditModels "soli/formations/src/audit/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open test DB: " + err.Error())
	}

	if err := db.AutoMigrate(&auditModels.AuditLog{}); err != nil {
		panic("failed to migrate audit_logs: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sharedTestDB.Exec("DELETE FROM audit_logs")
	return sharedTestDB
}
