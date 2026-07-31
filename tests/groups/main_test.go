package groups_tests

import (
	"os"
	"testing"

	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to open shared test DB: " + err.Error())
	}

	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.SubscriptionBatch{},
		&models.OrganizationSubscription{},
		// resolveForOrg consults the role-plan table before falling back to the
		// org's default subscription, and treats any error other than
		// ErrRecordNotFound as fatal — so an unmigrated table here reports "no
		// plan" rather than "no role mapping".
		&models.OrganizationRolePlan{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	if err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM subscription_batches")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_role_plans")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	return sharedTestDB
}
