// src/payment/models/subscription.go
// ToDo: SHOULD BE SPLITTED IN MULTIPLE FILES
package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// SubscriptionPlan représente un plan d'abonnement
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
	MaxSessionDurationMinutes int      `gorm:"default:60" json:"max_session_duration_minutes"` // Max time per session
	MaxConcurrentTerminals    int      `gorm:"default:1" json:"max_concurrent_terminals"`      // Max terminals running at once
	AllowedMachineSizes       []string `gorm:"serializer:json" json:"allowed_machine_sizes"`   // ["XS", "S", "M", "L", "XL"]
	NetworkAccessEnabled      bool     `gorm:"default:false" json:"network_access_enabled"`    // Allow external network access
	DataPersistenceEnabled    bool     `gorm:"default:false" json:"data_persistence_enabled"`  // Allow saving data between sessions
	DataPersistenceGB         int      `gorm:"default:0" json:"data_persistence_gb"`           // Storage quota in GB
	AllowedTemplates          []string `gorm:"serializer:json" json:"allowed_templates"`       // Template IDs allowed
	AllowedBackends           []string `gorm:"serializer:json" json:"allowed_backends"`        // Backend IDs allowed (empty = all)
	DefaultBackend            string   `gorm:"type:varchar(255);default:''" json:"default_backend"` // Default backend for this plan
	CommandHistoryRetentionDays int      `gorm:"default:0" json:"command_history_retention_days"` // 0 = no recording, >0 = days to keep

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

// SubscriptionBatch tracks bulk license purchases (one Stripe subscription with quantity > 1)
type SubscriptionBatch struct {
	entityManagementModels.BaseModel
	PurchaserUserID          string           `gorm:"type:varchar(100);not null;index" json:"purchaser_user_id"` // Who purchased the batch
	SubscriptionPlanID       uuid.UUID        `gorm:"type:uuid;not null" json:"subscription_plan_id"`
	SubscriptionPlan         SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	GroupID                  *uuid.UUID       `gorm:"type:uuid;index" json:"group_id,omitempty"`                   // Optional: link to a group
	StripeSubscriptionID     string           `gorm:"type:varchar(100);uniqueIndex" json:"stripe_subscription_id"` // Stripe subscription with quantity
	StripeSubscriptionItemID string           `gorm:"type:varchar(100)" json:"stripe_subscription_item_id"`        // For updating quantity
	TotalQuantity            int              `gorm:"not null" json:"total_quantity"`                              // Total licenses purchased
	AssignedQuantity         int              `gorm:"default:0" json:"assigned_quantity"`                          // How many are assigned
	Status                   string           `gorm:"type:varchar(50);default:'active'" json:"status"`             // active, cancelled, expired
	CurrentPeriodStart       time.Time        `json:"current_period_start"`
	CurrentPeriodEnd         time.Time        `json:"current_period_end"`
	CancelledAt              *time.Time       `json:"cancelled_at,omitempty"`
}

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
	TrialEnd                *time.Time       `json:"trial_end,omitempty"`
	CancelAtPeriodEnd       bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`
	Quantity                int              `gorm:"default:1" json:"quantity"` // Number of seats/licenses
}

// UserSubscription représente l'abonnement d'un utilisateur
// DEPRECATED in Phase 2: New subscriptions should use OrganizationSubscription
// Kept for backward compatibility with existing user subscriptions
type UserSubscription struct {
	entityManagementModels.BaseModel
	UserID                  string           `gorm:"type:varchar(100);index" json:"user_id"`                       // Who uses it (nullable for unassigned)
	PurchaserUserID         *string          `gorm:"type:varchar(100);index" json:"purchaser_user_id,omitempty"`   // Who purchased it (null = self-purchase)
	SubscriptionBatchID     *uuid.UUID       `gorm:"type:uuid;index" json:"subscription_batch_id,omitempty"`       // Link to bulk purchase batch
	SubscriptionType        string           `gorm:"type:varchar(20);default:'personal'" json:"subscription_type"` // "personal" (self-purchased) or "assigned" (from batch)
	SubscriptionPlanID      uuid.UUID        `json:"subscription_plan_id"`
	SubscriptionPlan        SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	StripeSubscriptionID    *string          `gorm:"type:varchar(100);uniqueIndex:idx_user_stripe_sub_not_null,where:stripe_subscription_id IS NOT NULL" json:"stripe_subscription_id,omitempty"`
	StripeCustomerID        *string          `gorm:"type:varchar(100);index" json:"stripe_customer_id,omitempty"`
	Status                  string           `gorm:"type:varchar(50);default:'active'" json:"status"` // active, cancelled, past_due, unpaid, unassigned, assigned
	CurrentPeriodStart      time.Time        `json:"current_period_start"`
	CurrentPeriodEnd        time.Time        `json:"current_period_end"`
	TrialEnd                *time.Time       `json:"trial_end,omitempty"`
	CancelAtPeriodEnd       bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`
}

// Invoice représente une facture
type Invoice struct {
	entityManagementModels.BaseModel
	UserID             string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	UserSubscriptionID uuid.UUID        `gorm:"not null" json:"user_subscription_id"`
	UserSubscription   UserSubscription `gorm:"foreignKey:UserSubscriptionID" json:"user_subscription"`
	StripeInvoiceID    string           `gorm:"type:varchar(100);uniqueIndex" json:"stripe_invoice_id"`
	Amount             int64            `json:"amount"` // Montant en centimes
	Currency           string           `gorm:"type:varchar(3)" json:"currency"`
	Status             string           `gorm:"type:varchar(50)" json:"status"` // paid, open, void, uncollectible
	InvoiceNumber      string           `gorm:"type:varchar(100)" json:"invoice_number"`
	InvoiceDate        time.Time        `json:"invoice_date"`
	DueDate            time.Time        `json:"due_date"`
	PaidAt             *time.Time       `json:"paid_at,omitempty"`
	StripeHostedURL    string           `gorm:"type:varchar(500)" json:"stripe_hosted_url"`
	DownloadURL        string           `gorm:"type:varchar(500)" json:"download_url"`
}

// PaymentMethod représente un moyen de paiement
type PaymentMethod struct {
	entityManagementModels.BaseModel
	UserID                string `gorm:"type:varchar(100);not null;index" json:"user_id"`
	StripePaymentMethodID string `gorm:"type:varchar(100);uniqueIndex" json:"stripe_payment_method_id"`
	Type                  string `gorm:"type:varchar(50)" json:"type"` // card, sepa_debit, etc.
	CardBrand             string `gorm:"type:varchar(20)" json:"card_brand,omitempty"`
	CardLast4             string `gorm:"type:varchar(4)" json:"card_last4,omitempty"`
	CardExpMonth          int    `json:"card_exp_month,omitempty"`
	CardExpYear           int    `json:"card_exp_year,omitempty"`
	IsDefault             bool   `gorm:"default:false" json:"is_default"`
	IsActive              bool   `gorm:"default:true" json:"is_active"`
}

// UsageMetrics pour tracker l'utilisation
type UsageMetrics struct {
	entityManagementModels.BaseModel
	UserID         string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	SubscriptionID uuid.UUID        `gorm:"not null" json:"subscription_id"`
	Subscription   UserSubscription `gorm:"foreignKey:SubscriptionID" json:"subscription"`
	MetricType     string           `gorm:"type:varchar(50);not null" json:"metric_type"` // courses_created, storage_used
	CurrentValue   int64            `json:"current_value"`
	LimitValue     int64            `json:"limit_value"` // -1 = unlimited
	PeriodStart    time.Time        `json:"period_start"`
	PeriodEnd      time.Time        `json:"period_end"`
	LastUpdated    time.Time        `json:"last_updated"`
}

// BillingAddress pour les adresses de facturation
type BillingAddress struct {
	entityManagementModels.BaseModel
	UserID     string `gorm:"type:varchar(100);not null;index" json:"user_id"`
	Line1      string `gorm:"type:varchar(255)" json:"line1"`
	Line2      string `gorm:"type:varchar(255)" json:"line2,omitempty"`
	City       string `gorm:"type:varchar(100)" json:"city"`
	State      string `gorm:"type:varchar(100)" json:"state,omitempty"`
	PostalCode string `gorm:"type:varchar(20)" json:"postal_code"`
	Country    string `gorm:"type:varchar(2)" json:"country"` // Code ISO 2 lettres
	IsDefault  bool   `gorm:"default:false" json:"is_default"`
}

// Implémentation des interfaces pour le système générique
func (s SubscriptionPlan) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s SubscriptionPlan) GetReferenceObject() string {
	return "SubscriptionPlan"
}

func (sb SubscriptionBatch) GetBaseModel() entityManagementModels.BaseModel {
	return sb.BaseModel
}

func (sb SubscriptionBatch) GetReferenceObject() string {
	return "SubscriptionBatch"
}

func (u UserSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u UserSubscription) GetReferenceObject() string {
	return "UserSubscription"
}

func (i Invoice) GetBaseModel() entityManagementModels.BaseModel {
	return i.BaseModel
}

func (i Invoice) GetReferenceObject() string {
	return "Invoice"
}

func (p PaymentMethod) GetBaseModel() entityManagementModels.BaseModel {
	return p.BaseModel
}

func (p PaymentMethod) GetReferenceObject() string {
	return "PaymentMethod"
}

func (u UsageMetrics) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u UsageMetrics) GetReferenceObject() string {
	return "UsageMetrics"
}

func (b BillingAddress) GetBaseModel() entityManagementModels.BaseModel {
	return b.BaseModel
}

func (b BillingAddress) GetReferenceObject() string {
	return "BillingAddress"
}

// OrganizationSubscription interface implementations (Phase 2)
func (os OrganizationSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return os.BaseModel
}

func (os OrganizationSubscription) GetReferenceObject() string {
	return "OrganizationSubscription"
}
