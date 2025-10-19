// src/payment/dto/subscriptionDto.go
package dto

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionPlan DTOs
type CreateSubscriptionPlanInput struct {
	Name               string   `binding:"required" json:"name" mapstructure:"name"`
	Description        string   `json:"description" mapstructure:"description"`
	PriceAmount        int64    `binding:"required" json:"price_amount" mapstructure:"price_amount"`
	Currency           string   `json:"currency" mapstructure:"currency"`
	BillingInterval    string   `binding:"required" json:"billing_interval" mapstructure:"billing_interval"`
	TrialDays          int      `json:"trial_days" mapstructure:"trial_days"`
	Features           []string `json:"features" mapstructure:"features"`
	MaxConcurrentUsers int      `json:"max_concurrent_users" mapstructure:"max_concurrent_users"`
	MaxCourses         int      `json:"max_courses" mapstructure:"max_courses"`
	MaxLabSessions     int      `json:"max_lab_sessions" mapstructure:"max_lab_sessions"`
	RequiredRole       string   `json:"required_role" mapstructure:"required_role"`
}

type UpdateSubscriptionPlanInput struct {
	Name               string   `json:"name,omitempty" mapstructure:"name"`
	Description        string   `json:"description,omitempty" mapstructure:"description"`
	IsActive           *bool    `json:"is_active,omitempty" mapstructure:"is_active"`
	Features           []string `json:"features,omitempty" mapstructure:"features"`
	MaxConcurrentUsers *int     `json:"max_concurrent_users,omitempty" mapstructure:"max_concurrent_users"`
	MaxCourses         *int     `json:"max_courses,omitempty" mapstructure:"max_courses"`
	MaxLabSessions     *int     `json:"max_lab_sessions,omitempty" mapstructure:"max_lab_sessions"`
}

type SubscriptionPlanOutput struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	StripeProductID    *string   `json:"stripe_product_id"`
	StripePriceID      *string   `json:"stripe_price_id"`
	PriceAmount        int64     `json:"price_amount"`
	Currency           string    `json:"currency"`
	BillingInterval    string    `json:"billing_interval"`
	TrialDays          int       `json:"trial_days"`
	Features           []string  `json:"features"`
	MaxConcurrentUsers int       `json:"max_concurrent_users"`
	MaxCourses         int       `json:"max_courses"`
	MaxLabSessions     int       `json:"max_lab_sessions"`
	IsActive           bool      `json:"is_active"`
	RequiredRole       string    `json:"required_role"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Terminal-specific limits (for Terminal Trainer feature)
	MaxSessionDurationMinutes int      `json:"max_session_duration_minutes"`
	MaxConcurrentTerminals    int      `json:"max_concurrent_terminals"`
	AllowedMachineSizes       []string `json:"allowed_machine_sizes"`
	NetworkAccessEnabled      bool     `json:"network_access_enabled"`
	DataPersistenceEnabled    bool     `json:"data_persistence_enabled"`
	DataPersistenceGB         int      `json:"data_persistence_gb"`
	AllowedTemplates          []string `json:"allowed_templates"`

	// Planned features (announced but not yet available)
	PlannedFeatures []string `json:"planned_features"` // Features coming soon

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
	Status            string `json:"status,omitempty" mapstructure:"status"`
	CancelAtPeriodEnd *bool  `json:"cancel_at_period_end,omitempty" mapstructure:"cancel_at_period_end"`
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
	StripeSubscriptionID string                 `json:"stripe_subscription_id"`
	StripeCustomerID     string                 `json:"stripe_customer_id"`
	Status               string                 `json:"status"`
	CurrentPeriodStart   time.Time              `json:"current_period_start"`
	CurrentPeriodEnd     time.Time              `json:"current_period_end"`
	TrialEnd             *time.Time             `json:"trial_end,omitempty"`
	CancelAtPeriodEnd    bool                   `json:"cancel_at_period_end"`
	CancelledAt          *time.Time             `json:"cancelled_at,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

// Invoice DTOs
type InvoiceOutput struct {
	ID               uuid.UUID              `json:"id"`
	UserID           string                 `json:"user_id"`
	UserSubscription UserSubscriptionOutput `json:"user_subscription"`
	StripeInvoiceID  string                 `json:"stripe_invoice_id"`
	Amount           int64                  `json:"amount"`
	Currency         string                 `json:"currency"`
	Status           string                 `json:"status"`
	InvoiceNumber    string                 `json:"invoice_number"`
	InvoiceDate      time.Time              `json:"invoice_date"`
	DueDate          time.Time              `json:"due_date"`
	PaidAt           *time.Time             `json:"paid_at,omitempty"`
	StripeHostedURL  string                 `json:"stripe_hosted_url"`
	DownloadURL      string                 `json:"download_url"`
	CreatedAt        time.Time              `json:"created_at"`
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
	SetDefault bool   `json:"set_default" mapstructure:"set_default"`
}

type UpdateBillingAddressInput struct {
	Line1      string `json:"line1,omitempty" mapstructure:"line1"`
	Line2      string `json:"line2,omitempty" mapstructure:"line2"`
	City       string `json:"city,omitempty" mapstructure:"city"`
	State      string `json:"state,omitempty" mapstructure:"state"`
	PostalCode string `json:"postal_code,omitempty" mapstructure:"postal_code"`
	Country    string `json:"country,omitempty" mapstructure:"country"`
	IsDefault  *bool  `json:"is_default,omitempty" mapstructure:"is_default"`
}

type BillingAddressOutput struct {
	ID         uuid.UUID `json:"id"`
	UserID     string    `json:"user_id"`
	Line1      string    `json:"line1"`
	Line2      string    `json:"line2,omitempty"`
	City       string    `json:"city"`
	State      string    `json:"state,omitempty"`
	PostalCode string    `json:"postal_code"`
	Country    string    `json:"country"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DTOs pour les actions Stripe
type CreateCheckoutSessionInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id"`
	SuccessURL         string    `binding:"required" json:"success_url"`
	CancelURL          string    `binding:"required" json:"cancel_url"`
	CouponCode         string    `json:"coupon_code,omitempty"`
	AllowReplace       bool      `json:"allow_replace,omitempty"` // Allow replacing free subscription with paid one
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
		Object map[string]interface{} `json:"object"`
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
}

type UsageLimitCheckOutput struct {
	Allowed        bool   `json:"allowed"`
	CurrentUsage   int64  `json:"current_usage"`
	Limit          int64  `json:"limit"`
	RemainingUsage int64  `json:"remaining_usage"`
	Message        string `json:"message,omitempty"`
}

// DTOs for bulk license purchases
type BulkPurchaseInput struct {
	SubscriptionPlanID uuid.UUID  `binding:"required" json:"subscription_plan_id" mapstructure:"subscription_plan_id"`
	Quantity           int        `binding:"required,min=1" json:"quantity" mapstructure:"quantity"`
	GroupID            *uuid.UUID `json:"group_id,omitempty" mapstructure:"group_id"` // Optional: link to group
	PaymentMethodID    string     `json:"payment_method_id,omitempty" mapstructure:"payment_method_id"`
	CouponCode         string     `json:"coupon_code,omitempty" mapstructure:"coupon_code"`
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
	NewQuantity int `binding:"required,min=1" json:"new_quantity" mapstructure:"new_quantity"`
}

// DTOs for pricing preview
type PricingPreviewInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id"`
	Quantity           int       `binding:"required,min=1" json:"quantity"`
}

type PricingBreakdown struct {
	PlanName         string     `json:"plan_name"`
	TotalQuantity    int        `json:"total_quantity"`
	TierBreakdown    []TierCost `json:"tier_breakdown"`
	TotalMonthlyCost int64      `json:"total_monthly_cost"`       // In cents
	AveragePerUnit   float64    `json:"average_per_license"`      // In currency (e.g., 8.33 for €8.33)
	Savings          int64      `json:"savings_vs_individual"`    // In cents
	Currency         string     `json:"currency"`
}

type TierCost struct {
	Range     string `json:"range"`      // e.g., "1-10"
	Quantity  int    `json:"quantity"`   // How many licenses in this tier
	UnitPrice int64  `json:"unit_price"` // Price per license in cents
	Subtotal  int64  `json:"subtotal"`   // Total for this tier in cents
}

// ==========================================
// Invoice Cleanup DTOs
// ==========================================

type CleanupInvoicesInput struct {
	Action        string   `binding:"required,oneof=void uncollectible" json:"action"`                   // "void" or "uncollectible"
	OlderThanDays *int     `binding:"omitempty,min=0" json:"older_than_days,omitempty"`                 // Cleanup invoices older than N days (optional when invoice_ids provided)
	DryRun        bool     `json:"dry_run"`                                                              // If true, only preview what would be cleaned up
	Status        string   `json:"status,omitempty" binding:"omitempty,oneof=draft open uncollectible"` // Filter by status (optional, defaults to "open,draft")
	InvoiceIDs    []string `json:"invoice_ids,omitempty"`                                                // Optional: specific invoice IDs to clean (if empty, cleans all matching)
}

type CleanupInvoicesResult struct {
	DryRun            bool                      `json:"dry_run"`
	Action            string                    `json:"action"`
	ProcessedInvoices int                       `json:"processed_invoices"`
	CleanedInvoices   int                       `json:"cleaned_invoices"`
	SkippedInvoices   int                       `json:"skipped_invoices"`
	FailedInvoices    int                       `json:"failed_invoices"`
	CleanedDetails    []CleanedInvoiceDetail    `json:"cleaned_details"`
	SkippedDetails    []string                  `json:"skipped_details"`
	FailedDetails     []FailedInvoiceCleanup    `json:"failed_details"`
	TotalAmountCleaned int64                    `json:"total_amount_cleaned"` // Total amount in cents
	Currency          string                    `json:"currency"`
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
	InvoiceID string `json:"invoice_id"`
	CustomerID string `json:"customer_id"`
	Error     string `json:"error"`
}
