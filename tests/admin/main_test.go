package admin_test

import (
	"os"
	"testing"

	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	testTools "soli/formations/tests/testTools"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB, "group_members", "class_groups", "organization_members", "organizations")
	return sharedTestDB
}
