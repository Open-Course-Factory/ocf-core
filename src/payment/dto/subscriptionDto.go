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

type UserSubscriptionOutput struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               string     `json:"user_id"`
	SubscriptionPlanID   uuid.UUID  `json:"subscription_plans"`
	StripeSubscriptionID string     `json:"stripe_subscription_id"`
	StripeCustomerID     string     `json:"stripe_customer_id"`
	Status               string     `json:"status"`
	CurrentPeriodStart   time.Time  `json:"current_period_start"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end"`
	TrialEnd             *time.Time `json:"trial_end,omitempty"`
	CancelAtPeriodEnd    bool       `json:"cancel_at_period_end"`
	CancelledAt          *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
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
