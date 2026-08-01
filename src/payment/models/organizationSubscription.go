package models

import (
	"fmt"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationSubscription represents an organization's subscription (Phase 2)
// Organizations subscribe to plans and all members inherit the features
type OrganizationSubscription struct {
	entityManagementModels.BaseModel
	OrganizationID          uuid.UUID        `gorm:"type:uuid;not null;index" json:"organization_id"` // Which organization
	SubscriptionPlanID      uuid.UUID        `gorm:"type:uuid;not null" json:"subscription_plan_id"`
	SubscriptionPlan        SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	StripeSubscriptionID    *string          `gorm:"type:varchar(100);uniqueIndex:idx_org_stripe_sub_not_null,where:stripe_subscription_id IS NOT NULL" json:"stripe_subscription_id,omitempty"` // Stripe subscription ID (nullable for incomplete subscriptions)
	StripeCustomerID        string           `gorm:"type:varchar(100);not null;index" json:"stripe_customer_id"`                                                                                 // Stripe customer (organization)
	Status                  string           `gorm:"type:varchar(50);default:'active'" json:"status"`                                                                                            // active, cancelled, past_due, unpaid, incomplete
	CurrentPeriodStart      time.Time        `json:"current_period_start"`
	CurrentPeriodEnd        time.Time        `json:"current_period_end"`
	CancelAtPeriodEnd       bool             `gorm:"default:false" json:"cancel_at_period_end"`
	// ExpiresAt is OCF's entitlement deadline: NULL means no deadline.
	//
	// Deliberately NOT CurrentPeriodEnd, which mirrors Stripe's billing window and
	// is zero on rows Stripe has not filled in — making the liveness predicate read
	// that would have silently un-entitled every such row. It is also a different
	// question: when does billing renew, versus when does access stop. One column
	// answering both is how the liveness rule fragmented in the first place.
	//
	// Set for admin assignments with a duration and for prepaid packs, which have
	// no Stripe subscription to flip a status for them (#440).
	ExpiresAt *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`

	// There is deliberately no Quantity here.
	//
	// It meant "number of seats" and was written, echoed in three DTO builders,
	// and read by no gate anywhere — an organization on a five-seat subscription
	// could add members up to MaxMembers and every one of them inherited the plan.
	// It is also unnecessary under the current model: a trainer buys the licences
	// he needs personally and distributes them as seats, where the count lives on
	// SubscriptionBatch.TotalQuantity and IS enforced; a school or OF is on a
	// bespoke plan whose terms are the plan, not a seat count (#456).
	//
	// AutoMigrate never drops columns, so `quantity` remains orphaned in the
	// database, like the plan `features` column before it.
}

func (os OrganizationSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return os.BaseModel
}

func (os OrganizationSubscription) GetReferenceObject() string {
	return "OrganizationSubscription"
}

const (
	// UniqueActiveOrgSubscriptionIndexName is the current partial unique index.
	// Exported because tests that seed legacy multi-active state must drop the
	// real index — hardcoding the name there let it silently drift out of sync
	// when the index was versioned, and the tests then failed on inserts they
	// believed were unconstrained.
	UniqueActiveOrgSubscriptionIndexName = "idx_unique_active_org_subscription_v2"

	// legacyUniqueActiveOrgSubscriptionIndexName is the pre-#439 index, whose
	// WHERE clause still counted 'trialing' as occupying the active slot.
	legacyUniqueActiveOrgSubscriptionIndexName = "idx_unique_active_org_subscription"
)

// MigrateUniqueActiveOrgSubscriptionIndex creates a partial unique index that
// enforces "at most one active OrganizationSubscription per organization" at
// the database level.
//
// This is the canonical defense against multi-pod races where two writers
// (e.g. an admin assign and a Stripe webhook firing simultaneously) both
// pass the in-process deactivate check before inserting their new rows.
// The Go-level transaction in CreateOrganizationSubscriptionAtomic prevents
// races inside a single process, but only the DB can serialize cross-pod
// concurrent inserts.
//
// We use a raw partial-index migration (rather than a GORM `uniqueIndex` tag)
// because GORM's struct-tag parser does not reliably emit partial WHERE clauses
// across dialects. The same pattern is already used by scenarios:
// MigrateUniqueActiveSessionIndex.
//
// #439: the original index spelled `status IN ('active','trialing')`. Editing
// that string in place would have been a no-op on every existing database,
// because this function returns early when the index already exists. The index
// is therefore versioned: the legacy one is dropped by name and the current one
// created fresh. MigrateTrialingStatusToActive must run FIRST, or rows still
// sitting in 'trialing' fall outside the new index and lose the uniqueness
// guarantee it exists to provide.
func MigrateUniqueActiveOrgSubscriptionIndex(db *gorm.DB) {
	// Raw DROP rather than Migrator().DropIndex: the gorm sqlite driver has
	// silently no-op'd schema drops before (see the orphan-column migration),
	// and a silent no-op here would leave the old definition in force, still
	// treating 'trialing' as occupying an organization's one active slot.
	if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, legacyUniqueActiveOrgSubscriptionIndexName)).Error; err != nil {
		fmt.Printf("MigrateUniqueActiveOrgSubscriptionIndex: failed to drop legacy index: %v\n", err)
	}

	// Idempotent: skip if the current index is already in place.
	if db.Migrator().HasIndex(&OrganizationSubscription{}, UniqueActiveOrgSubscriptionIndexName) {
		return
	}

	dialect := db.Dialector.Name()
	var sql string
	switch dialect {
	case "postgres":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX %s ON organization_subscriptions (organization_id) WHERE status = 'active' AND deleted_at IS NULL`,
			UniqueActiveOrgSubscriptionIndexName,
		)
	case "sqlite":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON organization_subscriptions (organization_id) WHERE status = 'active' AND deleted_at IS NULL`,
			UniqueActiveOrgSubscriptionIndexName,
		)
	default:
		fmt.Printf("MigrateUniqueActiveOrgSubscriptionIndex: unsupported dialect %s, skipping\n", dialect)
		return
	}

	if err := db.Exec(sql).Error; err != nil {
		fmt.Printf("MigrateUniqueActiveOrgSubscriptionIndex: failed to create index: %v\n", err)
	}
}
