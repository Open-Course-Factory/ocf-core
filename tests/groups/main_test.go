package groups_tests

import (
	"os"
	"testing"

	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	testTools "soli/formations/tests/testTools"

	"gorm.io/gorm"
)

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	sharedTestDB = testTools.MemoryDB(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.SubscriptionBatch{},
		&models.OrganizationSubscription{},
		&models.OrganizationRolePlan{},
		&organizationModels.Organization{},
		&organizationModels.OrganizationMember{},
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	os.Exit(m.Run())
}

func freshTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testTools.Truncate(sharedTestDB,
		"group_members",
		"class_groups",
		"subscription_batches",
		"user_subscriptions",
		"organization_subscriptions",
		"organization_role_plans",
		"organization_members",
		"organizations",
		"subscription_plans",
	)
	return sharedTestDB
}
