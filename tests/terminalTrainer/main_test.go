package terminalTrainer_tests

import (
	"os"
	"testing"

	configModels "soli/formations/src/configuration/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	// Initialize Casdoor SDK with a dummy config so package-level lookup
	// functions don't panic on a nil client when production code calls
	// casdoorsdk.GetUserByUserId. The HTTP call will fail gracefully and
	// the code falls back to its userID/email fallback chain — which is
	// the expected behaviour under unit tests that don't stand up a real
	// Casdoor server. Tests that need a specific Casdoor record swap the
	// per-feature seam (e.g. services.LookupCasdoorUserForOrgUsage).
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")

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
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&paymentModels.SubscriptionPlan{},
		&paymentModels.OrganizationSubscription{},
		&paymentModels.OrganizationRolePlan{},
		&paymentModels.UserSubscription{},
		&paymentModels.UsageMetrics{},
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
	sharedTestDB.Exec("DELETE FROM terminals")
	sharedTestDB.Exec("DELETE FROM user_terminal_keys")
	sharedTestDB.Exec("DELETE FROM group_members")
	sharedTestDB.Exec("DELETE FROM class_groups")
	sharedTestDB.Exec("DELETE FROM organization_members")
	sharedTestDB.Exec("DELETE FROM organization_subscriptions")
	sharedTestDB.Exec("DELETE FROM organization_role_plans")
	sharedTestDB.Exec("DELETE FROM user_subscriptions")
	sharedTestDB.Exec("DELETE FROM usage_metrics")
	sharedTestDB.Exec("DELETE FROM organizations")
	sharedTestDB.Exec("DELETE FROM subscription_plans")
	sharedTestDB.Exec("DELETE FROM features")
	return sharedTestDB
}
