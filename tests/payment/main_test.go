// tests/payment/main_test.go
// Shared test infrastructure for payment tests.
// Migrates tables ONCE and reuses the DB across all tests,
// cleaning rows between tests for isolation.
package payment_tests

import (
	"os"
	"testing"

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

	// Migrate all tables needed by any payment test
	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.UsageMetrics{},
		&models.OrganizationSubscription{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
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
	sharedTestDB.Exec("DELETE FROM usage_metrics")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	return sharedTestDB
}
