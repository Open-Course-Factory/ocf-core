package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// SubscriptionPlan represents a subscription plan
type SubscriptionPlan struct {
	entityManagementModels.BaseModel
	Name               string   `gorm:"type:varchar(100);not null" json:"name"`
	Description        string   `gorm:"type:text" json:"description"`
	Priority           int      `gorm:"default:0" json:"priority"` // Higher number = higher tier (0=Free, 10=Basic, 20=Pro, 30=Premium, etc.)
	StripeProductID    *string  `gorm:"type:varchar(100);uniqueIndex:idx_stripe_product_not_null,where:stripe_product_id IS NOT NULL" json:"stripe_product_id"`
	StripePriceID      *string  `gorm:"type:varchar(100);uniqueIndex:idx_stripe_price_not_null,where:stripe_price_id IS NOT NULL" json:"stripe_price_id"`
	PriceAmount        int64    `json:"price_amount"` // Prix en centimes
	Currency           string   `gorm:"type:varchar(3);default:'eur'" json:"currency"`
	BillingInterval    string   `gorm:"type:varchar(20);default:'month'" json:"billing_interval"` // month, year
	// NOTE: the free-form Features []string field was removed — plan capabilities
	// are typed columns now, projected via DerivePlanEntitlements. The raw
	// `features` DB column is left orphaned (AutoMigrate never drops it); the
	// startup backfill still reads it to migrate legacy group_management.
	IsActive           bool     `gorm:"default:true" json:"is_active"`
	IsCatalog          bool     `gorm:"default:true" json:"is_catalog" mapstructure:"is_catalog"` // true = shown on pricing page, false = custom/unlisted plan
	RequiredRole       string   `gorm:"type:varchar(50)" json:"required_role"`
	StripeCreated      bool     `gorm:"default:false" json:"stripe_created"`
	CreationError      *string  `gorm:"type:text" json:"creation_error,omitempty"`

	// Terminal-specific limits (new fields for terminal pricing)
	// Note: No limit on number of sessions - only a per-session duration limit
	MaxSessionDurationMinutes int `gorm:"default:60" json:"max_session_duration_minutes"` // Max time per session

	// Budget-based quota fields. The CPU/RAM budget is the single source
	// of truth for resource caps.
	//
	// MaxCPU is expressed in millicores (mCPU): 1000 mCPU = 1 vCPU. The
	// unit matches catalog.MachineSize.CPU so size.CPU and plan.MaxCPU
	// can be summed/compared directly without any unit conversion. The
	// frontend converts mCPU to fractional vCPU for display ("5000 mCPU"
	// → "5 vCPU", "500 mCPU" → "0.5 vCPU").
	MaxCPU      int `gorm:"default:0" mapstructure:"max_cpu" json:"max_cpu"`             // Total CPU budget in mCPU (1000 = 1 vCPU); 0 = unlimited
	MaxMemoryMB int `gorm:"default:0" mapstructure:"max_memory_mb" json:"max_memory_mb"` // Total RAM budget in MiB; 0 = unlimited

	NetworkAccessEnabled       bool     `gorm:"default:false" json:"network_access_enabled"`    // Allow external network access
	DataPersistenceEnabled     bool     `gorm:"default:false" json:"data_persistence_enabled"`  // Allow saving data between sessions (also gates persistent persistence_mode — SSOT)
	SessionSupervisionEnabled  bool     `gorm:"default:false" json:"session_supervision_enabled"` // Allow trainers (group manager+) to live-supervise a learner's terminal and take the hand
	GroupManagementEnabled     bool     `gorm:"default:false" json:"group_management_enabled" mapstructure:"group_management_enabled"` // Typed entitlement: plan grants group management (replaces the legacy features[] "group_management" string)
	DataPersistenceGB          int      `gorm:"default:0" json:"data_persistence_gb"`           // Storage quota in GB

	CommandHistoryRetentionDays int     `gorm:"default:0" json:"command_history_retention_days" mapstructure:"command_history_retention_days"` // days to retain command history (minimum 1)

	// Backend routing (applies when org has no backend config)
	DefaultBackend  string   `gorm:"type:varchar(255);default:''" json:"default_backend"`
	AllowedBackends []string `gorm:"serializer:json" json:"allowed_backends"` // Empty = no restriction

	// Tiered pricing for volume discounts
	UseTieredPricing bool          `gorm:"default:false" json:"use_tiered_pricing"` // Enable volume pricing
	PricingTiers     []PricingTier `gorm:"serializer:json" json:"pricing_tiers"`    // Tier definitions
}

// PricingTier represents a volume pricing tier
type PricingTier struct {
	MinQuantity int    `json:"min_quantity"`          // Start of tier (e.g., 1, 6, 16)
	MaxQuantity int    `json:"max_quantity"`          // End of tier (0 = unlimited)
	UnitAmount  int64  `json:"unit_amount"`           // Price per license in cents
	Description string `json:"description,omitempty"` // e.g., "Great for small classes"
}

// IsFree reports whether the plan carries no recurring charge and therefore
// must not be synced to Stripe as a billable product/price.
func (s SubscriptionPlan) IsFree() bool { return s.PriceAmount <= 0 }

func (s SubscriptionPlan) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s SubscriptionPlan) GetReferenceObject() string {
	return "SubscriptionPlan"
}
