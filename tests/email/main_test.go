package email_tests

import (
	"os"
	"testing"

	"gorm.io/gorm"

	emailModels "soli/formations/src/email/models"
	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(&emailModels.EmailTemplate{})
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB, "email_templates")
	return sharedTestDB
}
