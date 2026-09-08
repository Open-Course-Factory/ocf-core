package worker_tests

import (
	"os"
	"testing"

	"gorm.io/gorm"

	courseModels "soli/formations/src/courses/models"
	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(&courseModels.Generation{})
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB, "generations")
	return sharedTestDB
}
