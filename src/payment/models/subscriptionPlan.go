package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// SubscriptionPlan represents a subscription plan
type SubscriptionPlan struct {
	entityManagementModels.BaseModel
	Name            string  `gorm:"type:varchar(100);not null" json:"name"`
	Description     string  `gorm:"type:text" json:"description"`
	Priority        int     `gorm:"default:0" json:"priority"` // Higher number = higher tier (0=Free, 10=Basic, 20=Pro, 30=Premium, etc.)
	StripeProductID *string `gorm:"type:varchar(100);uniqueIndex:idx_stripe_product_not_null,where:stripe_product_id IS NOT NULL" json:"stripe_product_id"`
	StripePriceID   *string `gorm:"type:varchar(100);uniqueIndex:idx_stripe_price_not_null,where:stripe_price_id IS NOT NULL" json:"stripe_price_id"`
	PriceAmount     int64   `json:"price_amount"` // Prix en centimes
	Currency        string  `gorm:"type:varchar(3);default:'eur'" json:"currency"`
	BillingInterval string  `gorm:"type:varchar(20);default:'month'" json:"billing_interval"` // month, year
	// TaxBehavior says whether PriceAmount already contains VAT ("inclusive")
	// or has it added at checkout ("exclusive"). It travels with the amount
	// because the two are one statement: 11.90 means different money under each
	// reading, and Stripe accepts the answer once per price and never again.
	//
	// Empty means "exclusive", which is what every price created before this
	// field existed carries. It is not a default anyone should rely on — the
	// catalogue states it per plan — but an inferred value here would resolve to
	// inclusive for EUR, silently turning an announced amount into the gross.
	TaxBehavior string `gorm:"type:varchar(10)" json:"tax_behavior,omitempty"`
	// NOTE: the free-form Features []string field was removed — plan capabilities
	// are typed columns now, projected via DerivePlanEntitlements. The raw
	// `features` DB column is left orphaned (AutoMigrate never drops it); the
	// startup backfill still reads it to migrate legacy group_management.
	IsActive      bool    `gorm:"default:true" json:"is_active"`
	IsCatalog     bool    `gorm:"default:true" json:"is_catalog" mapstructure:"is_catalog"` // true = shown on pricing page, false = custom/unlisted plan
	RequiredRole  string  `gorm:"type:varchar(50)" json:"required_role"`
	StripeCreated bool    `gorm:"default:false" json:"stripe_created"`
	CreationError *string `gorm:"type:text" json:"creation_error,omitempty"`

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

	NetworkAccessEnabled      bool `gorm:"default:false" json:"network_access_enabled"`                                           // Allow external network access
	DataPersistenceEnabled    bool `gorm:"default:false" json:"data_persistence_enabled"`                                         // Allow saving data between sessions (also gates persistent persistence_mode — SSOT)
	SessionSupervisionEnabled bool `gorm:"default:false" json:"session_supervision_enabled"`                                      // Allow trainers (group manager+) to live-supervise a learner's terminal and take the hand
	GroupManagementEnabled    bool `gorm:"default:false" json:"group_management_enabled" mapstructure:"group_management_enabled"` // Typed entitlement: plan grants group management (replaces the legacy features[] "group_management" string)

	// IsDefaultFree marks the ONE plan new signups are given automatically.
	//
	// It exists because that plan used to be selected by its name, "Trial" — so
	// giving it the customer-facing name the offer actually uses would have broken
	// signup auto-assignment and then silently spawned a duplicate free plan, the
	// seed recreating what the lookup could no longer find. A commercial name is
	// not an identifier: it changes when marketing changes.
	//
	// Exactly one row carries it; MarkDefaultFreePlan enforces that at startup.
	IsDefaultFree bool `gorm:"default:false" json:"is_default_free" mapstructure:"is_default_free"`

	// BulkPurchasable marks a plan as sellable in bulk — i.e. it is a seat
	// product a trainer buys on behalf of learners.
	//
	// It exists because bulk-sellability and public visibility are different
	// questions that were previously answered by one flag: the gate required
	// IsCatalog, so a learner seat could not be both sellable and hidden from
	// the pricing page. It is also distinct from GroupManagementEnabled, which
	// is now checked on the PURCHASER's plan — requiring it on the plan being
	// sold meant students inherited group management and could buy seats
	// themselves.
	//
	// Default false: a plan is not a seat product unless someone says so. Note
	// the default must stay false — GORM omits zero-value bools on Create, so a
	// `default:true` bool cannot be set to false through the entity API.
	BulkPurchasable bool `gorm:"default:false" json:"bulk_purchasable" mapstructure:"bulk_purchasable"`

	// SeatUnit says what ONE purchased unit buys, and exists because nothing else
	// distinguishes the two seat products: both are billing_interval=month, and
	// only the meaning of a unit differs — a seat for a month, or one learner for
	// one day. Without it a purchase screen cannot turn "12 learners for 3 days"
	// into a quantity, nor compare the two products against each other.
	//
	// Empty means seat_month; resolve it through EffectiveSeatUnit rather than
	// testing for "" at the call site, so the fallback lives in one place.
	SeatUnit          string `gorm:"type:varchar(20);default:''" json:"seat_unit" mapstructure:"seat_unit"`
	DataPersistenceGB int    `gorm:"default:0" json:"data_persistence_gb"` // Storage quota in GB

	CommandHistoryRetentionDays int `gorm:"default:0" json:"command_history_retention_days" mapstructure:"command_history_retention_days"` // days to retain command history (minimum 1)

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

// Seat units. One purchased unit of a seat product buys either a seat for a
// billing period, or one learner for one day — the prepaid pack's quantity is
// learners x days, which is exactly why it needs a unit of its own.
const (
	SeatUnitSeatMonth  = "seat_month"
	SeatUnitLearnerDay = "learner_day"
)

// EffectiveSeatUnit resolves the unit, defaulting an unset value to seat_month.
// Every seat plan that predates this column is per-seat, so that is the safe
// reading rather than a guess.
func (s SubscriptionPlan) EffectiveSeatUnit() string {
	if s.SeatUnit == SeatUnitLearnerDay {
		return SeatUnitLearnerDay
	}
	return SeatUnitSeatMonth
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
