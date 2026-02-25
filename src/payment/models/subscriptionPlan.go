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
	TrialDays          int      `gorm:"default:0" json:"trial_days"`
	Features           []string `mapstructure:"features" gorm:"serializer:json" json:"features"`
	MaxConcurrentUsers int      `gorm:"default:1" json:"max_concurrent_users"`
	MaxCourses         int      `gorm:"default:-1" json:"max_courses"` // -1 = illimité
	IsActive           bool     `gorm:"default:true" json:"is_active"`
	RequiredRole       string   `gorm:"type:varchar(50)" json:"required_role"`
	StripeCreated      bool     `gorm:"default:false" json:"stripe_created"`
	CreationError      *string  `gorm:"type:text" json:"creation_error,omitempty"`

	// Terminal-specific limits (new fields for terminal pricing)
	// Note: No limit on number of sessions - only concurrent and duration limits
	MaxSessionDurationMinutes  int      `gorm:"default:60" json:"max_session_duration_minutes"` // Max time per session
	MaxConcurrentTerminals     int      `gorm:"default:1" json:"max_concurrent_terminals"`      // Max terminals running at once
	AllowedMachineSizes        []string `gorm:"serializer:json" json:"allowed_machine_sizes"`   // ["XS", "S", "M", "L", "XL"]
	NetworkAccessEnabled       bool     `gorm:"default:false" json:"network_access_enabled"`    // Allow external network access
	DataPersistenceEnabled     bool     `gorm:"default:false" json:"data_persistence_enabled"`  // Allow saving data between sessions
	DataPersistenceGB          int      `gorm:"default:0" json:"data_persistence_gb"`           // Storage quota in GB
	AllowedTemplates           []string `gorm:"serializer:json" json:"allowed_templates"`       // Template IDs allowed
	AllowedBackends            []string `gorm:"serializer:json" json:"allowed_backends"`        // Backend IDs allowed (empty = all)
	DefaultBackend             string   `gorm:"type:varchar(255);default:''" json:"default_backend"` // Default backend for this plan
	CommandHistoryRetentionDays int     `gorm:"default:0" json:"command_history_retention_days" mapstructure:"command_history_retention_days"` // 0 = no recording, >0 = days to keep

	// Add-on pricing (Stripe Price IDs for metered/add-on billing)
	AddonNetworkPriceID  *string `gorm:"type:varchar(100)" json:"addon_network_price_id,omitempty"`
	AddonStoragePriceID  *string `gorm:"type:varchar(100)" json:"addon_storage_price_id,omitempty"`
	AddonTerminalPriceID *string `gorm:"type:varchar(100)" json:"addon_terminal_price_id,omitempty"`

	// Planned features (announced but not yet available)
	PlannedFeatures []string `gorm:"serializer:json" json:"planned_features"` // Features coming soon

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

func (s SubscriptionPlan) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s SubscriptionPlan) GetReferenceObject() string {
	return "SubscriptionPlan"
}
