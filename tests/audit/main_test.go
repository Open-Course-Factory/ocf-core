package audit_tests

import (
	"os"
	"testing"

	auditModels "soli/formations/src/audit/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	db := testTools.MemoryDB(&auditModels.AuditLog{})

	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB, "audit_logs")
	return sharedTestDB
}
