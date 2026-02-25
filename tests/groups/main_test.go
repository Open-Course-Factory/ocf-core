package groups_tests

import (
	"os"
	"testing"

	groupModels "soli/formations/src/groups/models"
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
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	return sharedTestDB
}
