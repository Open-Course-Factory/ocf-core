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
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`
	Quantity                int              `gorm:"default:1" json:"quantity"` // Number of seats/licenses
}

func (os OrganizationSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return os.BaseModel
}

func (os OrganizationSubscription) GetReferenceObject() string {
	return "OrganizationSubscription"
}

// MigrateUniqueActiveOrgSubscriptionIndex creates a partial unique index that
// enforces "at most one active/trialing OrganizationSubscription per
// organization" at the database level.
//
// This is the canonical defense against multi-pod races where two writers
// (e.g. an admin assign and a Stripe webhook firing simultaneously) both
// pass the in-process deactivate check before inserting their new rows.
// The Go-level transaction in CreateOrganizationSubscriptionAtomic prevents
// races inside a single process, but only the DB can serialize cross-pod
// concurrent inserts.
//
// We use a raw partial-index migration (rather than a GORM `uniqueIndex` tag)
// because GORM's struct-tag parser does not reliably emit multi-column WHERE
// clauses across dialects (`status IN ('active','trialing') AND deleted_at
// IS NULL`). The same pattern is already used by scenarios:
// MigrateUniqueActiveSessionIndex.
func MigrateUniqueActiveOrgSubscriptionIndex(db *gorm.DB) {
	indexName := "idx_unique_active_org_subscription"

	// Idempotent: skip if the index is already in place.
	if db.Migrator().HasIndex(&OrganizationSubscription{}, indexName) {
		return
	}

	dialect := db.Dialector.Name()
	var sql string
	switch dialect {
	case "postgres":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX %s ON organization_subscriptions (organization_id) WHERE status IN ('active', 'trialing') AND deleted_at IS NULL`,
			indexName,
		)
	case "sqlite":
		sql = fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON organization_subscriptions (organization_id) WHERE status IN ('active', 'trialing') AND deleted_at IS NULL`,
			indexName,
		)
	default:
		fmt.Printf("MigrateUniqueActiveOrgSubscriptionIndex: unsupported dialect %s, skipping\n", dialect)
		return
	}

	if err := db.Exec(sql).Error; err != nil {
		fmt.Printf("MigrateUniqueActiveOrgSubscriptionIndex: failed to create index: %v\n", err)
	}
}
