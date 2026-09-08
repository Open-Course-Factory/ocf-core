package impersonation_test

import (
	"os"
	"testing"

	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(&authModels.ImpersonationSession{})
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if err := testTools.Truncate(sharedTestDB, "impersonation_sessions"); err != nil {
		t.Fatalf("failed to reset impersonation_sessions: %v", err)
	}
	return sharedTestDB
}
