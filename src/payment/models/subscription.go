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
	StripeProductID    *string  `gorm:"type:varchar(100);uniqueIndex:idx_stripe_product_not_null,where:stripe_product_id IS NOT NULL" json:"stripe_product_id"`
	StripePriceID      *string  `gorm:"type:varchar(100);uniqueIndex:idx_stripe_price_not_null,where:stripe_price_id IS NOT NULL" json:"stripe_price_id"`
	PriceAmount        int64    `json:"price_amount"` // Prix en centimes
	Currency           string   `gorm:"type:varchar(3);default:'eur'" json:"currency"`
	BillingInterval    string   `gorm:"type:varchar(20);default:'month'" json:"billing_interval"` // month, year
	TrialDays          int      `gorm:"default:0" json:"trial_days"`
	Features           []string `mapstructure:"features" gorm:"serializer:json" json:"features"`
	MaxConcurrentUsers int      `gorm:"default:1" json:"max_concurrent_users"`
	MaxCourses         int      `gorm:"default:-1" json:"max_courses"` // -1 = illimité
	MaxLabSessions     int      `gorm:"default:-1" json:"max_lab_sessions"`
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

	// Add-on pricing (Stripe Price IDs for metered/add-on billing)
	AddonNetworkPriceID  *string `gorm:"type:varchar(100)" json:"addon_network_price_id,omitempty"`
	AddonStoragePriceID  *string `gorm:"type:varchar(100)" json:"addon_storage_price_id,omitempty"`
	AddonTerminalPriceID *string `gorm:"type:varchar(100)" json:"addon_terminal_price_id,omitempty"`

	// Planned features (announced but not yet available)
	PlannedFeatures []string `gorm:"serializer:json" json:"planned_features"` // Features coming soon
}

// UserSubscription représente l'abonnement d'un utilisateur
type UserSubscription struct {
	entityManagementModels.BaseModel
	UserID                  string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	SubscriptionPlanID      uuid.UUID        `json:"subscription_plan_id"`
	SubscriptionPlan        SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	StripeSubscriptionID    string           `gorm:"type:varchar(100);" json:"stripe_subscription_id"`
	StripeCustomerID        string           `gorm:"type:varchar(100);not null;index" json:"stripe_customer_id"`
	Status                  string           `gorm:"type:varchar(50);default:'active'" json:"status"` // active, cancelled, past_due, unpaid
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
	MetricType     string           `gorm:"type:varchar(50);not null" json:"metric_type"` // courses_created, lab_sessions, storage_used
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
