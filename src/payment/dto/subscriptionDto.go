// src/payment/dto/subscriptionDto.go
package dto

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionPlan DTOs
type CreateSubscriptionPlanInput struct {
	Name                        string   `binding:"required" json:"name" mapstructure:"name"`
	Description                 string   `json:"description" mapstructure:"description"`
	PriceAmount                 int64    `binding:"required" json:"price_amount" mapstructure:"price_amount"`
	Currency                    string   `json:"currency" mapstructure:"currency"`
	BillingInterval             string   `binding:"required" json:"billing_interval" mapstructure:"billing_interval"`
	RequiredRole                string   `json:"required_role" mapstructure:"required_role"`
	MaxSessionDurationMinutes   int      `json:"max_session_duration_minutes" mapstructure:"max_session_duration_minutes"`
	NetworkAccessEnabled        bool     `json:"network_access_enabled" mapstructure:"network_access_enabled"`
	DataPersistenceEnabled      bool     `json:"data_persistence_enabled" mapstructure:"data_persistence_enabled"`
	SessionSupervisionEnabled   bool     `json:"session_supervision_enabled" mapstructure:"session_supervision_enabled"`
	DataPersistenceGB           int      `json:"data_persistence_gb" mapstructure:"data_persistence_gb"`
	CommandHistoryRetentionDays int      `json:"command_history_retention_days" mapstructure:"command_history_retention_days"`
	DefaultBackend              string   `json:"default_backend" mapstructure:"default_backend"`
	AllowedBackends             []string `json:"allowed_backends" mapstructure:"allowed_backends"`
	Priority                    int      `json:"priority" mapstructure:"priority"`
	IsActive                    *bool    `json:"is_active" mapstructure:"is_active"`
	IsCatalog                   *bool    `json:"is_catalog" mapstructure:"is_catalog"`
	GroupManagementEnabled      bool     `json:"group_management_enabled" mapstructure:"group_management_enabled"`
	BulkPurchasable             bool     `json:"bulk_purchasable" mapstructure:"bulk_purchasable"`
	SeatUnit                    string   `json:"seat_unit" mapstructure:"seat_unit"`
	// Budget-based quota fields.
	// MaxCPU is in millicores (mCPU); 1000 mCPU = 1 vCPU. Frontends
	// convert to fractional vCPU for display.
	MaxCPU      int `json:"max_cpu" mapstructure:"max_cpu"`
	MaxMemoryMB int `json:"max_memory_mb" mapstructure:"max_memory_mb"`
}

type UpdateSubscriptionPlanInput struct {
	Name                        string   `json:"name,omitempty" mapstructure:"name"`
	Description                 string   `json:"description,omitempty" mapstructure:"description"`
	IsActive                    *bool    `json:"is_active,omitempty" mapstructure:"is_active"`
	IsCatalog                   *bool    `json:"is_catalog,omitempty" mapstructure:"is_catalog"`
	MaxSessionDurationMinutes   *int     `json:"max_session_duration_minutes,omitempty" mapstructure:"max_session_duration_minutes"`
	NetworkAccessEnabled        *bool    `json:"network_access_enabled,omitempty" mapstructure:"network_access_enabled"`
	DataPersistenceEnabled      *bool    `json:"data_persistence_enabled,omitempty" mapstructure:"data_persistence_enabled"`
	SessionSupervisionEnabled   *bool    `json:"session_supervision_enabled,omitempty" mapstructure:"session_supervision_enabled"`
	DataPersistenceGB           *int     `json:"data_persistence_gb,omitempty" mapstructure:"data_persistence_gb"`
	CommandHistoryRetentionDays *int     `json:"command_history_retention_days,omitempty" mapstructure:"command_history_retention_days"`
	DefaultBackend              string   `json:"default_backend,omitempty" mapstructure:"default_backend"`
	AllowedBackends             []string `json:"allowed_backends,omitempty" mapstructure:"allowed_backends"`
	GroupManagementEnabled      *bool    `json:"group_management_enabled,omitempty" mapstructure:"group_management_enabled"`
	BulkPurchasable             *bool    `json:"bulk_purchasable,omitempty" mapstructure:"bulk_purchasable"`
	SeatUnit                    string   `json:"seat_unit,omitempty" mapstructure:"seat_unit"`
	Priority                    *int     `json:"priority,omitempty" mapstructure:"priority"`
	// Budget-based quota fields.
	// MaxCPU is in millicores (mCPU); 1000 mCPU = 1 vCPU.
	MaxCPU      *int `json:"max_cpu,omitempty" mapstructure:"max_cpu"`
	MaxMemoryMB *int `json:"max_memory_mb,omitempty" mapstructure:"max_memory_mb"`
}

type SubscriptionPlanOutput struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	Priority           int       `json:"priority"` // Higher = better tier (0=Free, 10=Basic, 20=Pro, 30=Premium)
	StripeProductID    *string   `json:"stripe_product_id"`
	StripePriceID      *string   `json:"stripe_price_id"`
	PriceAmount        int64     `json:"price_amount"`
	Currency           string    `json:"currency"`
	BillingInterval    string    `json:"billing_interval"`
	// Features is the projection of the plan's typed capability fields (see
	// DerivePlanEntitlements), no longer a free-form list. Converters populate it.
	Features           []string  `json:"features"`
	IsActive           bool      `json:"is_active"`
	IsCatalog          bool      `json:"is_catalog"`
	// IsDefaultFree is read-only: which plan new signups receive is elected at
	// startup, not chosen per request. There is deliberately no input field for
	// it — two plans claiming the election is a worse state than none.
	IsDefaultFree      bool      `json:"is_default_free"`
	GroupManagementEnabled bool  `json:"group_management_enabled"`
	BulkPurchasable        bool  `json:"bulk_purchasable"`
	SeatUnit               string `json:"seat_unit"`
	RequiredRole       string    `json:"required_role"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Terminal-specific limits (for Terminal Trainer feature)
	MaxSessionDurationMinutes   int      `json:"max_session_duration_minutes"`
	NetworkAccessEnabled        bool     `json:"network_access_enabled"`
	DataPersistenceEnabled      bool     `json:"data_persistence_enabled"`
	SessionSupervisionEnabled   bool     `json:"session_supervision_enabled" mapstructure:"session_supervision_enabled"`
	DataPersistenceGB           int      `json:"data_persistence_gb"`
	CommandHistoryRetentionDays int      `json:"command_history_retention_days" mapstructure:"command_history_retention_days"`

	// Backend routing
	DefaultBackend  string   `json:"default_backend"`
	AllowedBackends []string `json:"allowed_backends"`

	// Budget-based quota fields.
	// MaxCPU is in millicores (mCPU); 1000 mCPU = 1 vCPU.
	MaxCPU      int `json:"max_cpu"`
	MaxMemoryMB int `json:"max_memory_mb"`

	// Tiered pricing for volume discounts
	UseTieredPricing bool          `json:"use_tiered_pricing"`
	PricingTiers     []PricingTier `json:"pricing_tiers,omitempty"`
}

// PricingTier represents a volume pricing tier
type PricingTier struct {
	MinQuantity int    `json:"min_quantity"`
	MaxQuantity int    `json:"max_quantity"` // 0 = unlimited
	UnitAmount  int64  `json:"unit_amount"`  // Price in cents
	Description string `json:"description,omitempty"`
}

// UserSubscription DTOs
type CreateUserSubscriptionInput struct {
	UserID             string    `json:"user_id"`
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id"`
	PaymentMethodID    string    `json:"payment_method_id,omitempty"` // Stripe Payment Method ID
	CouponCode         string    `json:"coupon_code,omitempty"`
}

type UpdateUserSubscriptionInput struct {
	CancelAtPeriodEnd *bool `json:"cancel_at_period_end,omitempty" mapstructure:"cancel_at_period_end"`
}

type UpgradePlanInput struct {
	NewPlanID         string `binding:"required" json:"new_plan_id"` // UUID as string
	ProrationBehavior string `json:"proration_behavior,omitempty"`   // "always_invoice", "create_prorations", "none" (default: "always_invoice")
}

type UserSubscriptionOutput struct {
	ID                   uuid.UUID              `json:"id"`
	UserID               string                 `json:"user_id"`
	SubscriptionPlanID   uuid.UUID              `json:"subscription_plan_id"`
	SubscriptionPlan     SubscriptionPlanOutput `json:"subscription_plan"`
	StripeSubscriptionID *string                `json:"stripe_subscription_id,omitempty"`
	StripeCustomerID     *string                `json:"stripe_customer_id,omitempty"`
	Status               string                 `json:"status"`
	SubscriptionType     string                 `json:"subscription_type"` // "personal" or "assigned"
	IsPrimary            bool                   `json:"is_primary"`        // True if this is the active subscription being used
	IsFallback           bool                   `json:"is_fallback"`       // True when using personal subscription as fallback for a team org without its own subscription
	CurrentPeriodStart   time.Time              `json:"current_period_start"`
	CurrentPeriodEnd     time.Time              `json:"current_period_end"`
	CancelAtPeriodEnd    bool                   `json:"cancel_at_period_end"`
	CancelledAt          *time.Time             `json:"cancelled_at,omitempty"`
	// ExpiresAt is the entitlement deadline for prepaid packs (nil for open-ended
	// subscriptions) — surfaced so screens can show when a seat stops working.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Bulk license assignment information (only present if from bulk purchase)
	SubscriptionBatchID *uuid.UUID `json:"subscription_batch_id,omitempty"`
	BatchOwnerID        *string    `json:"batch_owner_id,omitempty"`   // ID of the user who purchased the batch
	BatchOwnerName      *string    `json:"batch_owner_name,omitempty"` // Display name of batch owner
	BatchOwnerEmail     *string    `json:"batch_owner_email,omitempty"`
	AssignedAt          *time.Time `json:"assigned_at,omitempty"` // When the license was assigned
	AssignedBy          *string    `json:"assigned_by,omitempty"` // ID of user who performed the assignment (if different from batch owner)

	// Admin assignment tracking
	AssignedByUserID *string `json:"assigned_by_user_id,omitempty"` // Admin who assigned this subscription
}

// Admin subscription assignment
type AdminAssignSubscriptionInput struct {
	UserID       string    `binding:"required" json:"user_id"`
	PlanID       uuid.UUID `binding:"required" json:"plan_id"`
	// DurationDays sets the entitlement deadline (#440). Zero means NO deadline —
	// the assignment is open-ended, which is what granting a bespoke or org plan
	// means. It still defaults the displayed billing window to 365 days, as it
	// always has; the two are different questions and only the deadline is enforced.
	DurationDays int `json:"duration_days" binding:"min=0,max=3650"`
}

// Invoice DTOs
type InvoiceOutput struct {
	ID                         uuid.UUID              `json:"id"`
	UserID                     string                 `json:"user_id"`
	UserSubscription           UserSubscriptionOutput `json:"user_subscription"`
	OrganizationID             *uuid.UUID             `json:"organization_id,omitempty"`
	OrganizationSubscriptionID *uuid.UUID             `json:"organization_subscription_id,omitempty"`
	StripeInvoiceID            string                 `json:"stripe_invoice_id"`
	Amount                     int64                  `json:"amount"`
	Currency                   string                 `json:"currency"`
	Status                     string                 `json:"status"`
	InvoiceNumber              string                 `json:"invoice_number"`
	InvoiceDate                time.Time              `json:"invoice_date"`
	DueDate                    time.Time              `json:"due_date"`
	PaidAt                     *time.Time             `json:"paid_at,omitempty"`
	StripeHostedURL            string                 `json:"stripe_hosted_url"`
	DownloadURL                string                 `json:"download_url"`
	CreatedAt                  time.Time              `json:"created_at"`
}

// PaymentMethod DTOs
type CreatePaymentMethodInput struct {
	StripePaymentMethodID string `binding:"required" json:"stripe_payment_method_id"`
	SetAsDefault          bool   `json:"set_as_default"`
}

type PaymentMethodOutput struct {
	ID                    uuid.UUID `json:"id"`
	UserID                string    `json:"user_id"`
	StripePaymentMethodID string    `json:"stripe_payment_method_id"`
	Type                  string    `json:"type"`
	CardBrand             string    `json:"card_brand,omitempty"`
	CardLast4             string    `json:"card_last4,omitempty"`
	CardExpMonth          int       `json:"card_exp_month,omitempty"`
	CardExpYear           int       `json:"card_exp_year,omitempty"`
	IsDefault             bool      `json:"is_default"`
	IsActive              bool      `json:"is_active"`
	CreatedAt             time.Time `json:"created_at"`
}

// UsageMetrics DTOs
type UsageMetricsOutput struct {
	ID           uuid.UUID `json:"id"`
	UserID       string    `json:"user_id"`
	MetricType   string    `json:"metric_type"`
	CurrentValue int64     `json:"current_value"`
	LimitValue   int64     `json:"limit_value"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	LastUpdated  time.Time `json:"last_updated"`
	UsagePercent float64   `json:"usage_percent"` // Calculé
}

// BillingAddress DTOs
type CreateBillingAddressInput struct {
	Line1      string `binding:"required" json:"line1" mapstructure:"line1"`
	Line2      string `json:"line2,omitempty" mapstructure:"line2"`
	City       string `binding:"required" json:"city" mapstructure:"city"`
	State      string `json:"state,omitempty" mapstructure:"state"`
	PostalCode string `binding:"required" json:"postal_code" mapstructure:"postal_code"`
	Country    string `binding:"required" json:"country" mapstructure:"country"`
	// B2B facturation fields (issue #383): optional. Format validation lives in a
	// BeforeCreate/BeforeUpdate hook, NOT in binding tags — the generic entity path
	// binds JSON into an `any`, so the struct validator never runs (#390).
	CompanyName string `json:"company_name,omitempty" mapstructure:"company_name"`
	Siret       string `json:"siret,omitempty" mapstructure:"siret"`
	VatNumber   string `json:"vat_number,omitempty" mapstructure:"vat_number"`
	SetDefault  bool   `json:"set_default" mapstructure:"set_default"`
}

type UpdateBillingAddressInput struct {
	Line1       string  `json:"line1,omitempty" mapstructure:"line1"`
	Line2       string  `json:"line2,omitempty" mapstructure:"line2"`
	City        string  `json:"city,omitempty" mapstructure:"city"`
	State       string  `json:"state,omitempty" mapstructure:"state"`
	PostalCode  string  `json:"postal_code,omitempty" mapstructure:"postal_code"`
	Country     string  `json:"country,omitempty" mapstructure:"country"`
	CompanyName *string `json:"company_name,omitempty" mapstructure:"company_name"`
	Siret       *string `json:"siret,omitempty" mapstructure:"siret"`
	VatNumber   *string `json:"vat_number,omitempty" mapstructure:"vat_number"`
	IsDefault   *bool   `json:"is_default,omitempty" mapstructure:"is_default"`
}

type BillingAddressOutput struct {
	ID          uuid.UUID `json:"id"`
	UserID      string    `json:"user_id"`
	Line1       string    `json:"line1"`
	Line2       string    `json:"line2,omitempty"`
	City        string    `json:"city"`
	State       string    `json:"state,omitempty"`
	PostalCode  string    `json:"postal_code"`
	Country     string    `json:"country"`
	CompanyName string    `json:"company_name,omitempty"`
	Siret       string    `json:"siret,omitempty"`
	VatNumber   string    `json:"vat_number,omitempty"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DTOs pour les actions Stripe
type CreateCheckoutSessionInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id"`
	SuccessURL         string    `binding:"required" json:"success_url"`
	CancelURL          string    `binding:"required" json:"cancel_url"`
	CouponCode         string    `json:"coupon_code,omitempty"`
	AllowReplace       bool      `json:"allow_replace,omitempty"` // Allow replacing free subscription with paid one
}

// CreateBulkCheckoutSessionInput mirrors BulkPurchaseInput for the Stripe
// checkout path: Quantity is the number of LEARNERS, and the billing units are
// derived by ResolvePackTerms rather than sent by the client (#455).
type CreateBulkCheckoutSessionInput struct {
	SubscriptionPlanID uuid.UUID  `binding:"required" json:"subscription_plan_id"`
	Quantity           int        `binding:"required,min=1,max=1000" json:"quantity"`
	DurationDays       int        `json:"duration_days,omitempty"`
	SuccessURL         string     `binding:"required" json:"success_url"`
	CancelURL          string     `binding:"required" json:"cancel_url"`
	GroupID            *uuid.UUID `json:"group_id,omitempty"`
	CouponCode         string     `json:"coupon_code,omitempty"`
}

type CheckoutSessionOutput struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

type CreatePortalSessionInput struct {
	ReturnURL string `binding:"required" json:"return_url"`
}

type PortalSessionOutput struct {
	URL string `json:"url"`
}

// DTOs pour les webhooks Stripe
type StripeWebhookEvent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object map[string]any `json:"object"`
	} `json:"data"`
	LiveMode        bool   `json:"livemode"`
	PendingWebhooks int    `json:"pending_webhooks"`
	Request         string `json:"request,omitempty"`
}

// DTOs pour les rapports et analytics
type SubscriptionAnalyticsOutput struct {
	TotalSubscriptions      int64                    `json:"total_subscriptions"`
	ActiveSubscriptions     int64                    `json:"active_subscriptions"`
	CancelledSubscriptions  int64                    `json:"cancelled_subscriptions"`
	TrialSubscriptions      int64                    `json:"trial_subscriptions"`
	Revenue                 int64                    `json:"revenue"` // En centimes
	MonthlyRecurringRevenue int64                    `json:"monthly_recurring_revenue"`
	ChurnRate               float64                  `json:"churn_rate"`
	ByPlan                  map[string]int           `json:"by_plan"`
	RecentSignups           []UserSubscriptionOutput `json:"recent_signups"`
	RecentCancellations     []UserSubscriptionOutput `json:"recent_cancellations"`
	GeneratedAt             time.Time                `json:"generated_at"`
}

// DTOs pour la gestion des limites d'utilisation
type UsageLimitCheckInput struct {
	MetricType string `binding:"required" json:"metric_type"`
	Increment  int64  `json:"increment"` // Combien on veut ajouter
	// OrganizationID scopes the limit check to a specific org context. When set,
	// the gate uses THAT org's plan (matching the launcher's display path). When
	// empty, the gate falls back to the user's globally highest-priority plan —
	// reserved for callers that genuinely have no org context.
	// See issue #334 / MR !239 for the launcher-vs-gate mismatch this prevents.
	OrganizationID *string `json:"organization_id,omitempty"`
}

type UsageLimitCheckOutput struct {
	Allowed        bool   `json:"allowed"`
	CurrentUsage   int64  `json:"current_usage"`
	Limit          int64  `json:"limit"`
	RemainingUsage int64  `json:"remaining_usage"`
	Message        string `json:"message,omitempty"`
	Source         string `json:"source"` // "personal" or "organization" — indicates where the effective plan comes from
}

// DTOs for bulk license purchases
// BulkPurchaseInput describes a trainer buying licences for learners.
//
// Quantity is the number of LEARNERS to cover, in both product families. It used
// to be the number of billing units, which the frontend computed as learners ×
// days for a prepaid pack — so the backend created that many month-long seats
// (#455). What Stripe charges for is derived from these two by ResolvePackTerms,
// not sent by the client.
type BulkPurchaseInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id" mapstructure:"subscription_plan_id"`
	Quantity           int       `binding:"required,min=1,max=1000" json:"quantity" mapstructure:"quantity"`
	// DurationDays is the pack length. Required for a learner-day plan, rejected
	// for a per-seat one.
	DurationDays    int        `json:"duration_days,omitempty" mapstructure:"duration_days"`
	GroupID         *uuid.UUID `json:"group_id,omitempty" mapstructure:"group_id"` // Optional: link to group
	PaymentMethodID string     `json:"payment_method_id,omitempty" mapstructure:"payment_method_id"`
	CouponCode      string     `json:"coupon_code,omitempty" mapstructure:"coupon_code"`
}

type SubscriptionBatchOutput struct {
	ID                       uuid.UUID              `json:"id"`
	PurchaserUserID          string                 `json:"purchaser_user_id"`
	SubscriptionPlanID       uuid.UUID              `json:"subscription_plan_id"`
	SubscriptionPlan         SubscriptionPlanOutput `json:"subscription_plan"`
	GroupID                  *uuid.UUID             `json:"group_id,omitempty"`
	StripeSubscriptionID     string                 `json:"stripe_subscription_id"`
	StripeSubscriptionItemID string                 `json:"stripe_subscription_item_id"`
	TotalQuantity            int                    `json:"total_quantity"`
	AssignedQuantity         int                    `json:"assigned_quantity"`
	AvailableQuantity        int                    `json:"available_quantity"` // Calculated: total - assigned
	Status                   string                 `json:"status"`
	CurrentPeriodStart       time.Time              `json:"current_period_start"`
	CurrentPeriodEnd         time.Time              `json:"current_period_end"`
	CancelledAt              *time.Time             `json:"cancelled_at,omitempty"`
	CreatedAt                time.Time              `json:"created_at"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

type AssignLicenseInput struct {
	UserID string `binding:"required" json:"user_id" mapstructure:"user_id"`
}

type UpdateBatchQuantityInput struct {
	NewQuantity int `binding:"required,min=1,max=1000" json:"new_quantity" mapstructure:"new_quantity"`
}

// DTOs for pricing preview
type PricingPreviewInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id"`
	Quantity           int       `binding:"required,min=1" json:"quantity"`
}

type PricingBreakdown struct {
	PlanName            string     `json:"plan_name"`
	TotalQuantity       int        `json:"total_quantity"`
	TierBreakdown       []TierCost `json:"tier_breakdown"`
	TotalMonthlyCost    int64      `json:"total_monthly_cost"`    // In cents
	IndividualUnitPrice int64      `json:"individual_unit_price"` // In cents
	AveragePerUnit      float64    `json:"average_per_license"`   // In currency (e.g., 8.33 for €8.33)
	Savings             int64      `json:"savings_vs_individual"` // In cents
	Currency            string     `json:"currency"`
}

type TierCost struct {
	Range     string `json:"range"`      // e.g., "1-10"
	Quantity  int    `json:"quantity"`   // How many licenses in this tier
	UnitPrice int64  `json:"unit_price"` // Price per license in cents
	Subtotal  int64  `json:"subtotal"`   // Total for this tier in cents
}

// ProspectivePricingInput prices a tier set that has NOT been saved, so an admin
// can judge brackets before committing them. Graduated brackets are easy to get
// wrong in a way that is invisible without the resulting totals: a first bracket
// wider than a typical order gives most customers no discount at all.
type ProspectivePricingInput struct {
	// Tiers is the prospective ladder. Empty means untiered, and FlatAmount applies.
	Tiers []PricingTier `json:"tiers"`
	// FlatAmount is the per-unit price used when Tiers is empty, in cents.
	FlatAmount int64  `json:"flat_amount"`
	Currency   string `json:"currency"`
	// Quantities to price. Required: an empty list would return an empty table,
	// which reads as "these brackets cost nothing".
	Quantities []int `json:"quantities" binding:"required,min=1"`
}

// ProspectivePricingPoint is one row of the admin's preview table.
type ProspectivePricingPoint struct {
	Quantity      int        `json:"quantity"`
	Total         int64      `json:"total"`          // cents
	PerUnit       float64    `json:"per_unit"`       // currency units, e.g. 8.00
	TierBreakdown []TierCost `json:"tier_breakdown"` // how the total was reached
}

type ProspectivePricingOutput struct {
	Currency string                    `json:"currency"`
	Points   []ProspectivePricingPoint `json:"points"`
}

// PurchasableSeatPlan is a seat product offered to a trainer, carrying just
// enough to price an order. Deliberately leaner than SubscriptionPlanOutput:
// these are hidden plans, so only what the purchase screen needs travels.
type PurchasableSeatPlan struct {
	ID               uuid.UUID     `json:"id"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	Currency         string        `json:"currency"`
	BillingInterval  string        `json:"billing_interval"`
	PriceAmount      int64         `json:"price_amount"`
	UseTieredPricing bool          `json:"use_tiered_pricing"`
	PricingTiers     []PricingTier `json:"pricing_tiers"`
	// SeatUnit is always resolved, never empty: the screen must know whether a
	// unit is a seat-month or a learner-day to turn an order into a quantity.
	SeatUnit string `json:"seat_unit"`
}

// PurchasableSeatPlansOutput answers "what may this trainer buy for learners?".
//
// CanPurchase is returned alongside the list rather than left to be inferred
// from an empty one: "you are not allowed" and "there is nothing for sale" are
// different answers and the UI must say which.
type PurchasableSeatPlansOutput struct {
	CanPurchase bool                  `json:"can_purchase"`
	Reason      string                `json:"reason,omitempty"`
	Plans       []PurchasableSeatPlan `json:"plans"`
}

// SeatPricingCheckInput carries BOTH seat ladders, because the invariants that
// matter belong to the pair and neither plan can validate itself.
type SeatPricingCheckInput struct {
	MonthlyTiers []PricingTier `json:"monthly_tiers"`
	MonthlyFlat  int64         `json:"monthly_flat"` // per seat/month when untiered, cents
	PackTiers    []PricingTier `json:"pack_tiers"`
	PackFlat     int64         `json:"pack_flat"` // per learner-day when untiered, cents
	// IndividualPlanAmount is the individual plan a seat must undercut (Solo).
	// Zero disables that check.
	IndividualPlanAmount int64 `json:"individual_plan_amount"`
	SeatCounts           []int `json:"seat_counts" binding:"required,min=1"`
	// WorkingWeekDays is the run length that must stay cheaper than a month.
	// Defaults to 5.
	WorkingWeekDays int `json:"working_week_days"`
	// MaxDaysToProbe bounds the crossover search. Defaults to 31.
	MaxDaysToProbe int `json:"max_days_to_probe"`
}

// SeatPricingCheckPoint reports the two ladders side by side at one seat count.
type SeatPricingCheckPoint struct {
	Seats          int     `json:"seats"`
	MonthlyTotal   int64   `json:"monthly_total"`
	MonthlyPerSeat float64 `json:"monthly_per_seat"`
	WeekTotal      int64   `json:"week_total"`
	WeekPerSeat    float64 `json:"week_per_seat"`
	// CrossoverDay is the first day count at which the month becomes cheaper.
	// 0 means the pack never catches the month within the probed range.
	CrossoverDay int `json:"crossover_day"`
}

// SeatPricingViolation is machine-readable so the admin UI can translate it;
// Detail is an English fallback, not the display string.
type SeatPricingViolation struct {
	Code   string `json:"code"`
	Seats  int    `json:"seats,omitempty"`
	Detail string `json:"detail"`
}

type SeatPricingCheckOutput struct {
	// OK is true when no invariant was violated. Derived, but returned explicitly
	// so a caller cannot mistake "violations array absent" for "everything fine".
	OK         bool                    `json:"ok"`
	Points     []SeatPricingCheckPoint `json:"points"`
	Violations []SeatPricingViolation  `json:"violations"`
}

// ==========================================
// Invoice Cleanup DTOs
// ==========================================

type CleanupInvoicesInput struct {
	Action        string   `binding:"required,oneof=void uncollectible" json:"action"`                  // "void" or "uncollectible"
	OlderThanDays *int     `binding:"omitempty,min=0" json:"older_than_days,omitempty"`                 // Cleanup invoices older than N days (optional when invoice_ids provided)
	DryRun        bool     `json:"dry_run"`                                                             // If true, only preview what would be cleaned up
	Status        string   `json:"status,omitempty" binding:"omitempty,oneof=draft open uncollectible"` // Filter by status (optional, defaults to "open,draft")
	InvoiceIDs    []string `json:"invoice_ids,omitempty"`                                               // Optional: specific invoice IDs to clean (if empty, cleans all matching)
}

type CleanupInvoicesResult struct {
	DryRun             bool                   `json:"dry_run"`
	Action             string                 `json:"action"`
	ProcessedInvoices  int                    `json:"processed_invoices"`
	CleanedInvoices    int                    `json:"cleaned_invoices"`
	SkippedInvoices    int                    `json:"skipped_invoices"`
	FailedInvoices     int                    `json:"failed_invoices"`
	CleanedDetails     []CleanedInvoiceDetail `json:"cleaned_details"`
	SkippedDetails     []string               `json:"skipped_details"`
	FailedDetails      []FailedInvoiceCleanup `json:"failed_details"`
	TotalAmountCleaned int64                  `json:"total_amount_cleaned"` // Total amount in cents
	Currency           string                 `json:"currency"`
}

type CleanedInvoiceDetail struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceNumber string `json:"invoice_number"`
	CustomerID    string `json:"customer_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Status        string `json:"original_status"`
	Action        string `json:"action_taken"`
	CreatedAt     string `json:"created_at"`
}

type FailedInvoiceCleanup struct {
	InvoiceID  string `json:"invoice_id"`
	CustomerID string `json:"customer_id"`
	Error      string `json:"error"`
}
