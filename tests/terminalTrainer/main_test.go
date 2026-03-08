package terminalTrainer_tests

import (
	"os"
	"testing"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"

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

	// Migrate all tables needed by any terminalTrainer test
	err = db.AutoMigrate(
		&models.UserTerminalKey{},
		&models.Terminal{},
		&models.TerminalShare{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&paymentModels.SubscriptionPlan{},
		&paymentModels.OrganizationSubscription{},
		&paymentModels.UserSubscription{},
		&configModels.Feature{},
	)
	if err != nil {
		panic("failed to migrate shared test DB: " + err.Error())
	}

	sharedTestDB = db
	os.Exit(m.Run())
}

// freshTestDB returns the shared DB after cleaning all rows.
// Safe because Go tests within a package run sequentially (no t.Parallel).
func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Delete in dependency order to respect foreign keys
	sharedTestDB.Exec("DELETE FROM terminal_shares")
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	sharedTestDB.Exec("DELETE FROM features")
	return sharedTestDB
}
