// src/payment/services/stripeService.go
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/utils"
	"time"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	organizationModels "soli/formations/src/organizations/models"
	terminalRepo "soli/formations/src/terminalTrainer/repositories"

	genericService "soli/formations/src/entityManagement/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	billingPortalSession "github.com/stripe/stripe-go/v82/billingportal/session"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/paymentmethod"
	"github.com/stripe/stripe-go/v82/price"
	"github.com/stripe/stripe-go/v82/product"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
	"gorm.io/gorm"
)

// SyncSubscriptionsResult contient les résultats de la synchronisation
type SyncSubscriptionsResult struct {
	ProcessedSubscriptions int                  `json:"processed_subscriptions"`
	CreatedSubscriptions   int                  `json:"created_subscriptions"`
	UpdatedSubscriptions   int                  `json:"updated_subscriptions"`
	SkippedSubscriptions   int                  `json:"skipped_subscriptions"`
	FailedSubscriptions    []FailedSubscription `json:"failed_subscriptions"`
	CreatedDetails         []string             `json:"created_details"`
	UpdatedDetails         []string             `json:"updated_details"`
	SkippedDetails         []string             `json:"skipped_details"`
}

type SyncInvoicesResult struct {
	ProcessedInvoices int             `json:"processed_invoices"`
	CreatedInvoices   int             `json:"created_invoices"`
	UpdatedInvoices   int             `json:"updated_invoices"`
	SkippedInvoices   int             `json:"skipped_invoices"`
	FailedInvoices    []FailedInvoice `json:"failed_invoices"`
	CreatedDetails    []string        `json:"created_details"`
	UpdatedDetails    []string        `json:"updated_details"`
	SkippedDetails    []string        `json:"skipped_details"`
}

type FailedInvoice struct {
	StripeInvoiceID string `json:"stripe_invoice_id"`
	CustomerID      string `json:"customer_id"`
	Error           string `json:"error"`
}

type SyncPaymentMethodsResult struct {
	ProcessedPaymentMethods int                   `json:"processed_payment_methods"`
	CreatedPaymentMethods   int                   `json:"created_payment_methods"`
	UpdatedPaymentMethods   int                   `json:"updated_payment_methods"`
	SkippedPaymentMethods   int                   `json:"skipped_payment_methods"`
	FailedPaymentMethods    []FailedPaymentMethod `json:"failed_payment_methods"`
	CreatedDetails          []string              `json:"created_details"`
	UpdatedDetails          []string              `json:"updated_details"`
	SkippedDetails          []string              `json:"skipped_details"`
}

type FailedPaymentMethod struct {
	StripePaymentMethodID string `json:"stripe_payment_method_id"`
	CustomerID            string `json:"customer_id"`
	Error                 string `json:"error"`
}

// FailedSubscription contient les détails d'un échec de synchronisation
type FailedSubscription struct {
	StripeSubscriptionID string `json:"stripe_subscription_id"`
	UserID               string `json:"user_id,omitempty"`
	Error                string `json:"error"`
}

type StripeService interface {
	// Customer management
	CreateOrGetCustomer(userID, email, name string) (string, error)
	UpdateCustomer(customerID string, params *stripe.CustomerParams) error

	// Subscription management
	CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput, replaceSubscriptionID *uuid.UUID) (*dto.CheckoutSessionOutput, error)
	CreateBulkCheckoutSession(userID string, input dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error)
	CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error)

	// Product & Price management
	CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error
	UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error

	// Webhook handling
	ProcessWebhook(payload []byte, signature string) error
	ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error)

	// Subscription operations
	CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error
	MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error
	ReactivateSubscription(subscriptionID string) error
	UpdateSubscription(subscriptionID, newPriceID, prorationBehavior string) (*stripe.Subscription, error)

	// Subscription synchronization
	SyncExistingSubscriptions() (*SyncSubscriptionsResult, error)
	SyncUserSubscriptions(userID string) (*SyncSubscriptionsResult, error)
	SyncSubscriptionsWithMissingMetadata() (*SyncSubscriptionsResult, error)
	LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID uuid.UUID) error

	// Invoice synchronization
	SyncUserInvoices(userID string) (*SyncInvoicesResult, error)

	// Invoice cleanup
	CleanupIncompleteInvoices(input dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error)

	// Payment method synchronization
	SyncUserPaymentMethods(userID string) (*SyncPaymentMethodsResult, error)

	// Payment method operations
	AttachPaymentMethod(paymentMethodID, customerID string) error
	DetachPaymentMethod(paymentMethodID string) error
	SetDefaultPaymentMethod(customerID, paymentMethodID string) error

	// Invoice operations
	GetInvoice(invoiceID string) (*stripe.Invoice, error)
	SendInvoice(invoiceID string) error

	// Bulk license operations
	CreateSubscriptionWithQuantity(customerID string, plan *models.SubscriptionPlan, quantity int, paymentMethodID string) (*stripe.Subscription, error)
	UpdateSubscriptionQuantity(subscriptionID string, subscriptionItemID string, newQuantity int) (*stripe.Subscription, error)

	// Plan synchronization from Stripe
	ImportPlansFromStripe() (*SyncPlansResult, error)
}

// SyncPlansResult contains the results of importing plans from Stripe
type SyncPlansResult struct {
	ProcessedPlans int           `json:"processed_plans"`
	CreatedPlans   int           `json:"created_plans"`
	UpdatedPlans   int           `json:"updated_plans"`
	SkippedPlans   int           `json:"skipped_plans"`
	FailedPlans    []FailedPlan  `json:"failed_plans"`
	CreatedDetails []string      `json:"created_details"`
	UpdatedDetails []string      `json:"updated_details"`
	SkippedDetails []string      `json:"skipped_details"`
}

type FailedPlan struct {
	StripeProductID string `json:"stripe_product_id"`
	StripePriceID   string `json:"stripe_price_id"`
	Error           string `json:"error"`
}

type stripeService struct {
	db                  *gorm.DB
	subscriptionService UserSubscriptionService
	genericService      genericService.GenericService
	repository          repositories.PaymentRepository
	webhookSecret       string
}

func NewStripeService(db *gorm.DB) StripeService {
	// Initialiser Stripe
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	return &stripeService{
		db:                  db,
		subscriptionService: NewSubscriptionService(db),
		genericService:      genericService.NewGenericService(db, casdoor.Enforcer),
		repository:          repositories.NewPaymentRepository(db),
		webhookSecret:       os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
}

// CreateOrGetCustomer crée ou récupère un client Stripe
func (ss *stripeService) CreateOrGetCustomer(userID, email, name string) (string, error) {
	// Check if the user already has a Stripe customer ID from ANY subscription (active or inactive)
	// This ensures we reuse the same Stripe customer even when upgrading from free to paid
	subscriptions, err := ss.repository.GetUserSubscriptions(userID, true) // true = include inactive
	if err == nil && subscriptions != nil {
		// Find the first subscription with a StripeCustomerID
		for _, sub := range *subscriptions {
			if sub.StripeCustomerID != "" {
				utils.Debug("♻️ Reusing existing Stripe customer %s for user %s", sub.StripeCustomerID, userID)
				return sub.StripeCustomerID, nil
			}
		}
	}

	// No existing Stripe customer found - create a new one
	utils.Info("🆕 Creating new Stripe customer for user %s (%s)", userID, email)
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
		Metadata: map[string]string{
			"user_id": userID,
		},
	}

	customer, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create Stripe customer: %v", err)
	}

	utils.Info("✅ Created Stripe customer %s for user %s", customer.ID, userID)
	return customer.ID, nil
}

// UpdateCustomer met à jour un client Stripe
func (ss *stripeService) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	_, err := customer.Update(customerID, params)
	return err
}

// CreateCheckoutSession crée une session de checkout Stripe
func (ss *stripeService) CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput, replaceSubscriptionID *uuid.UUID) (*dto.CheckoutSessionOutput, error) {
	// Récupérer le plan d'abonnement
	plan, err := ss.subscriptionService.GetSubscriptionPlan(input.SubscriptionPlanID)
	if err != nil {
		return nil, fmt.Errorf("subscription plan not found: %v", err)
	}

	if !plan.IsActive {
		return nil, fmt.Errorf("subscription plan is not active")
	}

	// Vérifier que le plan a un prix Stripe configuré
	if plan.StripePriceID == nil {
		return nil, fmt.Errorf("subscription plan does not have a Stripe price configured")
	}

	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from Casdoor: %v", err)
	}

	email := user.Email
	name := user.Name

	customerID, err := ss.CreateOrGetCustomer(userID, email, name)
	if err != nil {
		return nil, err
	}

	// Update Stripe customer with latest name and address (if available)
	// This will pre-fill the checkout form
	customerUpdateParams := &stripe.CustomerParams{
		Name: stripe.String(name),
	}

	// Try to get user's default billing address
	billingAddr, err := ss.repository.GetDefaultBillingAddress(userID)
	if err == nil && billingAddr != nil {
		// User has a billing address - include it in customer update
		customerUpdateParams.Address = &stripe.AddressParams{
			Line1:      stripe.String(billingAddr.Line1),
			Line2:      stripe.String(billingAddr.Line2),
			City:       stripe.String(billingAddr.City),
			State:      stripe.String(billingAddr.State),
			PostalCode: stripe.String(billingAddr.PostalCode),
			Country:    stripe.String(billingAddr.Country),
		}
		utils.Debug("📍 Pre-filling checkout with saved address: %s, %s", billingAddr.City, billingAddr.Country)
	}

	// Update customer in Stripe to pre-fill checkout form
	if err := ss.UpdateCustomer(customerID, customerUpdateParams); err != nil {
		utils.Warn("⚠️ Failed to update Stripe customer with pre-fill data: %v", err)
		// Don't fail checkout if this fails - just won't pre-fill
	}

	// Build metadata
	metadata := map[string]string{
		"user_id":              userID,
		"subscription_plan_id": input.SubscriptionPlanID.String(),
	}

	// Add replace_subscription_id if upgrading from free plan
	if replaceSubscriptionID != nil {
		metadata["replace_subscription_id"] = replaceSubscriptionID.String()
		utils.Info("Checkout session will replace free subscription %s", replaceSubscriptionID.String())
	}

	// Paramètres de la session de checkout
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
			"sepa_debit",
		}),
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(*plan.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(input.SuccessURL),
		CancelURL:  stripe.String(input.CancelURL),
		Metadata:   metadata,
		// CRITICAL FIX: Pass metadata to the subscription that will be created
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: metadata,
		},
		BillingAddressCollection: stripe.String("required"),
		// Allow updating customer name and address during checkout
		// This also enables pre-filling from the customer object
		CustomerUpdate: &stripe.CheckoutSessionCustomerUpdateParams{
			Name:    stripe.String("auto"), // Allow name to be updated
			Address: stripe.String("auto"), // Allow address to be updated
		},
		// TaxIDCollection: &stripe.CheckoutSessionTaxIDCollectionParams{
		// 	Enabled: stripe.Bool(true),
		// },
	}

	// Ajouter un coupon si fourni
	if input.CouponCode != "" {
		params.Discounts = []*stripe.CheckoutSessionDiscountParams{
			{Coupon: stripe.String(input.CouponCode)},
		}
	}

	session, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkout session: %v", err)
	}

	return &dto.CheckoutSessionOutput{
		SessionID: session.ID,
		URL:       session.URL,
	}, nil
}

// CreateBulkCheckoutSession creates a Stripe checkout session for bulk license purchase
func (ss *stripeService) CreateBulkCheckoutSession(userID string, input dto.CreateBulkCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	// Get subscription plan
	plan, err := ss.subscriptionService.GetSubscriptionPlan(input.SubscriptionPlanID)
	if err != nil {
		return nil, fmt.Errorf("subscription plan not found: %v", err)
	}

	if !plan.IsActive {
		return nil, fmt.Errorf("subscription plan is not active")
	}

	// Verify plan has Stripe price configured
	if plan.StripePriceID == nil {
		return nil, fmt.Errorf("subscription plan does not have a Stripe price configured")
	}

	// Get user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from Casdoor: %v", err)
	}

	email := user.Email
	name := user.Name

	// Get or create Stripe customer
	customerID, err := ss.CreateOrGetCustomer(userID, email, name)
	if err != nil {
		return nil, err
	}

	// Update Stripe customer with latest information (pre-fill checkout form)
	customerUpdateParams := &stripe.CustomerParams{
		Name: stripe.String(name),
	}

	// Try to get user's default billing address
	billingAddr, err := ss.repository.GetDefaultBillingAddress(userID)
	if err == nil && billingAddr != nil {
		customerUpdateParams.Address = &stripe.AddressParams{
			Line1:      stripe.String(billingAddr.Line1),
			Line2:      stripe.String(billingAddr.Line2),
			City:       stripe.String(billingAddr.City),
			State:      stripe.String(billingAddr.State),
			PostalCode: stripe.String(billingAddr.PostalCode),
			Country:    stripe.String(billingAddr.Country),
		}
		utils.Debug("📍 Pre-filling bulk checkout with saved address: %s, %s", billingAddr.City, billingAddr.Country)
	}

	// Update customer in Stripe
	if err := ss.UpdateCustomer(customerID, customerUpdateParams); err != nil {
		utils.Warn("⚠️ Failed to update Stripe customer with pre-fill data: %v", err)
		// Don't fail checkout if this fails
	}

	// Build metadata for checkout session and subscription
	metadata := map[string]string{
		"user_id":              userID,
		"subscription_plan_id": input.SubscriptionPlanID.String(),
		"quantity":             fmt.Sprintf("%d", input.Quantity),
		"bulk_purchase":        "true",
	}

	if input.GroupID != nil {
		metadata["group_id"] = input.GroupID.String()
	}

	// Checkout session parameters
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
			"sepa_debit",
		}),
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(*plan.StripePriceID),
				Quantity: stripe.Int64(int64(input.Quantity)),
			},
		},
		SuccessURL: stripe.String(input.SuccessURL),
		CancelURL:  stripe.String(input.CancelURL),
		Metadata:   metadata,
		// CRITICAL: Pass metadata to the subscription that will be created
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: metadata,
		},
		BillingAddressCollection: stripe.String("required"),
		CustomerUpdate: &stripe.CheckoutSessionCustomerUpdateParams{
			Name:    stripe.String("auto"),
			Address: stripe.String("auto"),
		},
	}

	// Add coupon if provided
	if input.CouponCode != "" {
		params.Discounts = []*stripe.CheckoutSessionDiscountParams{
			{Coupon: stripe.String(input.CouponCode)},
		}
	}

	// Create checkout session
	checkoutSession, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create bulk checkout session: %v", err)
	}

	utils.Info("✅ Created bulk license checkout session for user %s (plan: %s, quantity: %d)",
		userID, plan.Name, input.Quantity)

	return &dto.CheckoutSessionOutput{
		SessionID: checkoutSession.ID,
		URL:       checkoutSession.URL,
	}, nil
}

// CreatePortalSession crée une session pour le portail client Stripe
func (ss *stripeService) CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error) {
	// Récupérer l'abonnement actif pour obtenir le customer ID
	subscription, err := ss.subscriptionService.GetActiveUserSubscription(userID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found: %v", err)
	}

	// Créer la session du portail client
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(subscription.StripeCustomerID),
		ReturnURL: stripe.String(input.ReturnURL),
	}

	portalSession, err := billingPortalSession.New(params)

	if err != nil {
		return nil, fmt.Errorf("failed to create portal session: %v", err)
	}

	return &dto.PortalSessionOutput{
		URL: portalSession.URL,
	}, nil
}

// CreateSubscriptionPlanInStripe crée un produit et un prix dans Stripe
func (ss *stripeService) CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	// 1. Créer le produit Stripe
	productParams := &stripe.ProductParams{
		Name:        stripe.String(plan.Name),
		Description: stripe.String(plan.Description),
		Metadata: map[string]string{
			"plan_id": plan.ID.String(),
		},
	}

	stripeProduct, err := product.New(productParams)
	if err != nil {
		return fmt.Errorf("failed to create Stripe product: %v", err)
	}

	// 2. Créer le prix Stripe
	priceParams := &stripe.PriceParams{
		Product:    stripe.String(stripeProduct.ID),
		UnitAmount: stripe.Int64(plan.PriceAmount),
		Currency:   stripe.String(plan.Currency),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String(plan.BillingInterval),
		},
		Metadata: map[string]string{
			"plan_id": plan.ID.String(),
		},
	}

	stripePrice, err := price.New(priceParams)
	if err != nil {
		return fmt.Errorf("failed to create Stripe price: %v", err)
	}

	// 3. Mettre à jour le plan avec les IDs Stripe
	plan.StripeProductID = &stripeProduct.ID
	plan.StripePriceID = &stripePrice.ID

	return ss.genericService.EditEntity(plan.ID, "SubscriptionPlan", models.SubscriptionPlan{}, plan)
}

// UpdateSubscriptionPlanInStripe met à jour un plan dans Stripe
func (ss *stripeService) UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error {
	// Mettre à jour le produit Stripe
	productParams := &stripe.ProductParams{
		Name:        stripe.String(plan.Name),
		Description: stripe.String(plan.Description),
		Active:      stripe.Bool(plan.IsActive),
	}

	_, err := product.Update(*plan.StripeProductID, productParams)
	return err
}

// ProcessWebhook traite les webhooks Stripe
func (ss *stripeService) ProcessWebhook(payload []byte, signature string) error {
	event, err := ss.ValidateWebhookSignature(payload, signature)
	if err != nil {
		return err
	}

	// Log every webhook event received for debugging
	utils.Debug("📥 Received webhook event: %s (ID: %s)", event.Type, event.ID)

	switch event.Type {
	// Subscription lifecycle events
	case "customer.subscription.created":
		utils.Debug("🆕 Processing subscription created")
		return ss.handleSubscriptionCreated(event)
	case "customer.subscription.updated":
		utils.Debug("🔄 Processing subscription updated")
		return ss.handleSubscriptionUpdated(event)
	case "customer.subscription.deleted":
		utils.Debug("❌ Processing subscription deleted")
		return ss.handleSubscriptionDeleted(event)
	case "customer.subscription.paused":
		utils.Debug("⏸️ Processing subscription paused")
		return ss.handleSubscriptionPaused(event)
	case "customer.subscription.resumed":
		utils.Debug("▶️ Processing subscription resumed")
		return ss.handleSubscriptionResumed(event)
	case "customer.subscription.trial_will_end":
		utils.Debug("⏰ Processing trial will end")
		return ss.handleTrialWillEnd(event)

	// Invoice events
	case "invoice.created":
		utils.Debug("📄 Processing invoice created")
		return ss.handleInvoiceCreated(event)
	case "invoice.finalized":
		utils.Debug("📋 Processing invoice finalized")
		return ss.handleInvoiceFinalized(event)
	case "invoice.payment_succeeded":
		utils.Debug("💰 Processing invoice payment succeeded")
		return ss.handleInvoicePaymentSucceeded(event)
	case "invoice.payment_failed":
		utils.Debug("⚠️ Processing invoice payment failed")
		return ss.handleInvoicePaymentFailed(event)

	// Payment method events
	case "payment_method.attached":
		utils.Debug("💳 Processing payment method attached")
		return ss.handlePaymentMethodAttached(event)
	case "payment_method.detached":
		utils.Debug("🗑️ Processing payment method detached")
		return ss.handlePaymentMethodDetached(event)

	// Customer events
	case "customer.updated":
		utils.Debug("👤 Processing customer updated")
		return ss.handleCustomerUpdated(event)

	// Checkout events
	case "checkout.session.completed":
		utils.Debug("✅ Processing checkout session completed")
		return ss.handleCheckoutSessionCompleted(event)

	default:
		// Événement non géré, mais pas une erreur
		utils.Debug("❓ Unhandled webhook event type: %s", event.Type)
		return nil
	}
}

// handleSubscriptionCreated traite la création d'abonnement
func (ss *stripeService) handleSubscriptionCreated(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %v", err)
	}

	// CRITICAL: Check if this is a bulk purchase before creating individual subscription
	if isBulkSubscription(&subscription) {
		utils.Info("📦 Detected bulk subscription creation: %s", subscription.ID)
		return ss.handleBulkSubscriptionCreated(&subscription)
	}

	// Phase 2: Check if this is an organization subscription
	if orgID, exists := subscription.Metadata["organization_id"]; exists {
		utils.Info("🏢 Detected organization subscription creation: %s for org %s", subscription.ID, orgID)
		return ss.handleOrganizationSubscriptionCreated(&subscription)
	}

	userID, exists := subscription.Metadata["user_id"]
	if !exists {
		return fmt.Errorf("user_id not found in subscription metadata")
	}

	// CRITICAL FIX: Determine plan ID from Stripe price, not metadata
	// Metadata can be stale if plan was changed in Stripe Dashboard
	var planID uuid.UUID
	var plan *models.SubscriptionPlan

	if len(subscription.Items.Data) > 0 {
		stripePriceID := subscription.Items.Data[0].Price.ID
		utils.Debug("🔍 Subscription created with Stripe price ID: %s", stripePriceID)

		// Try to find plan by Stripe price ID first (most reliable)
		var err error
		plan, err = ss.repository.GetSubscriptionPlanByStripePriceID(stripePriceID)
		if err == nil {
			planID = plan.ID
			utils.Debug("✅ Found plan by Stripe price: %s (%s)", plan.Name, plan.ID)
		} else {
			// Fallback to metadata if price lookup fails
			utils.Debug("⚠️ Could not find plan by price ID %s, falling back to metadata", stripePriceID)
			planIDStr, exists := subscription.Metadata["subscription_plan_id"]
			if !exists {
				return fmt.Errorf("subscription_plan_id not found in metadata and price lookup failed")
			}
			planID, err = uuid.Parse(planIDStr)
			if err != nil {
				return fmt.Errorf("invalid subscription_plan_id in metadata: %v", err)
			}
			utils.Debug("⚠️ Using plan from metadata: %s", planID)

			// Load the plan object for GORM relationship
			plan, err = ss.subscriptionService.GetSubscriptionPlan(planID)
			if err != nil {
				return fmt.Errorf("failed to load plan %s: %v", planID, err)
			}
		}
	} else {
		return fmt.Errorf("subscription has no items/price")
	}

	// Dans les nouvelles versions de Stripe, les périodes sont au niveau des items
	var currentPeriodStart, currentPeriodEnd time.Time
	if len(subscription.Items.Data) > 0 {
		// Prendre la première item pour les dates de période
		item := subscription.Items.Data[0]
		currentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		currentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
	}

	// Créer directement le modèle UserSubscription avec toutes les données Stripe
	// CRITICAL: Set BOTH the ID and the object for GORM to properly save the relationship
	userSubscription := &models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   planID,
		SubscriptionPlan:     *plan, // CRITICAL: Include the plan object
		StripeSubscriptionID: subscription.ID,
		StripeCustomerID:     subscription.Customer.ID,
		Status:               string(subscription.Status),
		CurrentPeriodStart:   currentPeriodStart,
		CurrentPeriodEnd:     currentPeriodEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
	}

	// Ajouter la date de fin d'essai si elle existe
	if subscription.TrialEnd > 0 {
		trialEnd := time.Unix(subscription.TrialEnd, 0)
		userSubscription.TrialEnd = &trialEnd
	}

	// Check if this subscription is replacing a free subscription
	// This metadata comes from the checkout session
	if replaceIDStr, hasReplaceID := subscription.Metadata["replace_subscription_id"]; hasReplaceID {
		if replaceID, err := uuid.Parse(replaceIDStr); err == nil {
			// Get the old subscription
			oldSub, err := ss.repository.GetUserSubscription(replaceID)
			if err == nil {
				// Verify it's actually a free plan before deleting
				oldPlan, err := ss.subscriptionService.GetSubscriptionPlan(oldSub.SubscriptionPlanID)
				if err == nil && oldPlan.PriceAmount == 0 {
					utils.Info("🔄 Deleting old free subscription %s (being replaced by paid subscription %s)",
						replaceID, subscription.ID)

					// Hard delete the free subscription
					if err := ss.genericService.DeleteEntity(replaceID, models.UserSubscription{}, false); err != nil {
						utils.Warn("⚠️ Failed to delete old free subscription: %v", err)
					} else {
						utils.Info("✅ Deleted old free subscription %s", replaceID)
					}
				}
			}
		}
	}

	// Créer directement dans la base via le repository
	if err := ss.repository.CreateUserSubscription(userSubscription); err != nil {
		return fmt.Errorf("failed to create subscription: %v", err)
	}

	// CRITICAL: Initialize usage metrics for the new subscription
	// This ensures limits are set correctly from the start
	utils.Debug("🔧 Initializing usage metrics for subscription %s", userSubscription.ID)
	if err := ss.subscriptionService.InitializeUsageMetrics(userID, userSubscription.ID, planID); err != nil {
		// Don't fail the subscription creation, but log the error
		utils.Debug("⚠️ Warning: Failed to initialize usage metrics for user %s: %v", userID, err)
		// The user can call sync-usage-limits endpoint to fix this
	} else {
		utils.Debug("✅ Usage metrics initialized for user %s with plan %s", userID, planID)
	}

	return nil
}

func (ss *stripeService) handleSubscriptionUpdated(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return err
	}

	// Check if this is a bulk subscription
	if isBulkSubscription(&subscription) {
		utils.Debug("📦 Detected bulk subscription update: %s", subscription.ID)
		return ss.handleBulkSubscriptionUpdated(&subscription)
	}

	// Phase 2: Check if this is an organization subscription
	if _, exists := subscription.Metadata["organization_id"]; exists {
		utils.Debug("🏢 Detected organization subscription update: %s", subscription.ID)
		return ss.handleOrganizationSubscriptionUpdated(&subscription)
	}

	// Récupérer l'abonnement existant
	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found in database: %v", err)
	}

	// Check if the plan changed by comparing Stripe price ID
	if len(subscription.Items.Data) > 0 {
		stripePriceID := subscription.Items.Data[0].Price.ID
		utils.Debug("🔍 Webhook subscription update: Stripe price ID = %s, Current plan ID = %s",
			stripePriceID, userSub.SubscriptionPlanID)

		// Find the plan that matches this Stripe price ID
		newPlan, err := ss.repository.GetSubscriptionPlanByStripePriceID(stripePriceID)
		if err != nil {
			utils.Debug("⚠️ Could not find plan with Stripe price ID %s: %v", stripePriceID, err)
			utils.Debug("💡 Tip: Make sure your subscription plan has stripe_price_id = '%s' in the database", stripePriceID)
		} else if newPlan.ID == userSub.SubscriptionPlanID {
			utils.Debug("ℹ️ Plan unchanged: %s (%s)", newPlan.Name, newPlan.ID)
		} else {
			utils.Debug("🔄 Plan change detected in webhook: %s -> %s (%s) (Stripe price: %s)",
				userSub.SubscriptionPlanID, newPlan.ID, newPlan.Name, stripePriceID)

			// Update the subscription plan ID
			userSub.SubscriptionPlanID = newPlan.ID
			userSub.SubscriptionPlan = *newPlan

			// Also update usage metric limits for the new plan
			err = ss.subscriptionService.UpdateUsageMetricLimits(userSub.UserID, newPlan.ID)
			if err != nil {
				utils.Debug("⚠️ Warning: Failed to update usage limits after plan change: %v", err)
				// Don't fail the webhook, continue with subscription update
			} else {
				utils.Debug("✅ Updated usage limits for user %s to plan %s (%s)",
					userSub.UserID, newPlan.ID, newPlan.Name)
			}
		}
	}

	// Check if subscription is being cancelled by detecting canceled_at timestamp
	// canceled_at is set when subscription is cancelled (either immediately or at period end)
	isBeingCancelled := false
	wasCancelled := userSub.CancelledAt != nil

	if subscription.CanceledAt > 0 && !wasCancelled {
		// This is a NEW cancellation (canceled_at just got set)
		isBeingCancelled = true
		utils.Info("🚫 Subscription %s is being cancelled (canceled_at: %d)", subscription.ID, subscription.CanceledAt)

		// Also log if it's immediate or at period end
		if subscription.CancelAtPeriodEnd {
			utils.Info("📅 Subscription will be cancelled at period end")
		} else {
			utils.Info("⚡ Subscription cancelled immediately")
		}
	}

	// CRITICAL: Terminate terminals if subscription is being cancelled
	if isBeingCancelled {
		utils.Info("🔌 Terminating all active terminals for user %s due to subscription cancellation", userSub.UserID)
		if err := ss.terminateUserTerminals(userSub.UserID); err != nil {
			utils.Error("Failed to terminate terminals for user %s: %v", userSub.UserID, err)
			// Don't fail webhook processing if terminal termination fails
		}
	}

	userSub.Status = string(subscription.Status)
	userSub.CurrentPeriodStart = time.Unix(subscription.Items.Data[0].CurrentPeriodStart, 0)
	userSub.CurrentPeriodEnd = time.Unix(subscription.Items.Data[0].CurrentPeriodEnd, 0)
	userSub.CancelAtPeriodEnd = subscription.CancelAtPeriodEnd

	if subscription.CanceledAt > 0 {
		cancelledAt := time.Unix(subscription.CanceledAt, 0)
		userSub.CancelledAt = &cancelledAt
	}

	return ss.repository.UpdateUserSubscription(userSub)
}

// handleSubscriptionDeleted traite la suppression d'abonnement
func (ss *stripeService) handleSubscriptionDeleted(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return err
	}

	// Check if this is a bulk subscription
	if isBulkSubscription(&subscription) {
		utils.Debug("📦 Detected bulk subscription deletion: %s", subscription.ID)
		return ss.handleBulkSubscriptionDeleted(&subscription)
	}

	// Phase 2: Check if this is an organization subscription
	if _, exists := subscription.Metadata["organization_id"]; exists {
		utils.Debug("🏢 Detected organization subscription deletion: %s", subscription.ID)
		return ss.handleOrganizationSubscriptionDeleted(&subscription)
	}

	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return err
	}

	// CRITICAL: Terminate all active terminals for this user before cancelling subscription
	utils.Info("🔌 Terminating all active terminals for user %s due to subscription cancellation", userSub.UserID)
	if err := ss.terminateUserTerminals(userSub.UserID); err != nil {
		utils.Error("Failed to terminate terminals for user %s: %v", userSub.UserID, err)
		// Don't fail subscription cancellation if terminal termination fails
	}

	userSub.Status = "cancelled"
	now := time.Now()
	userSub.CancelledAt = &now

	return ss.repository.UpdateUserSubscription(userSub)
}

// ============================================================================
// Phase 2: Organization Subscription Webhook Handlers
// ============================================================================

// handleOrganizationSubscriptionCreated processes organization subscription creation from Stripe
func (ss *stripeService) handleOrganizationSubscriptionCreated(subscription *stripe.Subscription) error {
	orgIDStr, exists := subscription.Metadata["organization_id"]
	if !exists {
		return fmt.Errorf("organization_id not found in subscription metadata")
	}

	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return fmt.Errorf("invalid organization_id in metadata: %v", err)
	}

	// Determine plan ID from Stripe price (same logic as user subscriptions)
	var planID uuid.UUID
	var plan *models.SubscriptionPlan

	if len(subscription.Items.Data) > 0 {
		stripePriceID := subscription.Items.Data[0].Price.ID
		utils.Debug("🔍 Organization subscription created with Stripe price ID: %s", stripePriceID)

		// Try to find plan by Stripe price ID
		plan, err = ss.repository.GetSubscriptionPlanByStripePriceID(stripePriceID)
		if err == nil {
			planID = plan.ID
			utils.Debug("✅ Found plan by Stripe price: %s (%s)", plan.Name, plan.ID)
		} else {
			// Fallback to metadata
			planIDStr, exists := subscription.Metadata["subscription_plan_id"]
			if !exists {
				return fmt.Errorf("subscription_plan_id not found in metadata and price lookup failed")
			}
			planID, err = uuid.Parse(planIDStr)
			if err != nil {
				return fmt.Errorf("invalid subscription_plan_id in metadata: %v", err)
			}

			// Load the plan object
			plan, err = ss.subscriptionService.GetSubscriptionPlan(planID)
			if err != nil {
				return fmt.Errorf("failed to load plan %s: %v", planID, err)
			}
		}
	} else {
		return fmt.Errorf("subscription has no items/price")
	}

	// Get period dates from subscription items
	var currentPeriodStart, currentPeriodEnd time.Time
	if len(subscription.Items.Data) > 0 {
		item := subscription.Items.Data[0]
		currentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		currentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
	}

	// Create OrganizationSubscription record
	orgSubscription := &models.OrganizationSubscription{
		OrganizationID:       orgID,
		SubscriptionPlanID:   planID,
		SubscriptionPlan:     *plan,
		StripeSubscriptionID: &subscription.ID,
		StripeCustomerID:     subscription.Customer.ID,
		Status:               string(subscription.Status),
		CurrentPeriodStart:   currentPeriodStart,
		CurrentPeriodEnd:     currentPeriodEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		Quantity:             int(subscription.Items.Data[0].Quantity),
	}

	// Add trial end if exists
	if subscription.TrialEnd > 0 {
		trialEnd := time.Unix(subscription.TrialEnd, 0)
		orgSubscription.TrialEnd = &trialEnd
	}

	// Get organization subscription repository
	orgSubRepo := repositories.NewOrganizationSubscriptionRepository(ss.db)

	// Create the organization subscription
	if err := orgSubRepo.CreateOrganizationSubscription(orgSubscription); err != nil {
		return fmt.Errorf("failed to create organization subscription: %v", err)
	}

	// Update Organization.SubscriptionPlanID
	err = ss.db.Model(&organizationModels.Organization{}).
		Where("id = ?", orgID).
		Update("subscription_plan_id", planID).Error
	if err != nil {
		utils.Warn("⚠️ Failed to update organization subscription_plan_id: %v", err)
	}

	utils.Info("✅ Created organization subscription %s for org %s with plan %s",
		orgSubscription.ID, orgID, plan.Name)

	return nil
}

// handleOrganizationSubscriptionUpdated processes organization subscription updates from Stripe
func (ss *stripeService) handleOrganizationSubscriptionUpdated(subscription *stripe.Subscription) error {
	// Get organization subscription repository
	orgSubRepo := repositories.NewOrganizationSubscriptionRepository(ss.db)

	// Find the existing subscription
	orgSub, err := orgSubRepo.GetOrganizationSubscriptionByStripeID(*&subscription.ID)
	if err != nil {
		return fmt.Errorf("organization subscription not found in database: %v", err)
	}

	// Check if the plan changed
	if len(subscription.Items.Data) > 0 {
		stripePriceID := subscription.Items.Data[0].Price.ID
		utils.Debug("🔍 Webhook org subscription update: Stripe price ID = %s", stripePriceID)

		// Check if plan changed
		currentPlan, err := ss.subscriptionService.GetSubscriptionPlan(orgSub.SubscriptionPlanID)
		if err == nil && currentPlan.StripePriceID != nil && *currentPlan.StripePriceID != stripePriceID {
			utils.Info("🔄 Organization subscription plan changed from %s to %s",
				*currentPlan.StripePriceID, stripePriceID)

			// Find the new plan
			newPlan, err := ss.repository.GetSubscriptionPlanByStripePriceID(stripePriceID)
			if err == nil {
				orgSub.SubscriptionPlanID = newPlan.ID
				orgSub.SubscriptionPlan = *newPlan

				// Update Organization.SubscriptionPlanID
				err = ss.db.Model(&organizationModels.Organization{}).
					Where("id = ?", orgSub.OrganizationID).
					Update("subscription_plan_id", newPlan.ID).Error
				if err != nil {
					utils.Warn("⚠️ Failed to update organization subscription_plan_id: %v", err)
				}

				utils.Info("✅ Updated organization %s to plan %s", orgSub.OrganizationID, newPlan.Name)
			}
		}

		// Update period dates
		item := subscription.Items.Data[0]
		orgSub.CurrentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		orgSub.CurrentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
		orgSub.Quantity = int(item.Quantity)
	}

	// Update status and other fields
	orgSub.Status = string(subscription.Status)
	orgSub.CancelAtPeriodEnd = subscription.CancelAtPeriodEnd

	// Update trial end
	if subscription.TrialEnd > 0 {
		trialEnd := time.Unix(subscription.TrialEnd, 0)
		orgSub.TrialEnd = &trialEnd
	}

	// Save updates
	if err := orgSubRepo.UpdateOrganizationSubscription(orgSub); err != nil {
		return fmt.Errorf("failed to update organization subscription: %v", err)
	}

	utils.Info("✅ Updated organization subscription %s (status: %s)", orgSub.ID, orgSub.Status)

	return nil
}

// handleOrganizationSubscriptionDeleted processes organization subscription cancellation from Stripe
func (ss *stripeService) handleOrganizationSubscriptionDeleted(subscription *stripe.Subscription) error {
	// Get organization subscription repository
	orgSubRepo := repositories.NewOrganizationSubscriptionRepository(ss.db)

	// Find the existing subscription
	orgSub, err := orgSubRepo.GetOrganizationSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return fmt.Errorf("organization subscription not found in database: %v", err)
	}

	// Update status to cancelled
	orgSub.Status = "cancelled"
	now := time.Now()
	orgSub.CancelledAt = &now

	// Save updates
	if err := orgSubRepo.UpdateOrganizationSubscription(orgSub); err != nil {
		return fmt.Errorf("failed to cancel organization subscription: %v", err)
	}

	utils.Info("✅ Cancelled organization subscription %s for org %s", orgSub.ID, orgSub.OrganizationID)

	return nil
}

// ============================================================================
// End Phase 2: Organization Subscription Webhook Handlers
// ============================================================================

// handleInvoicePaymentSucceeded traite le paiement réussi d'une facture
func (ss *stripeService) handleInvoicePaymentSucceeded(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return err
	}

	// Try to find subscription by customer ID first
	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(stripeInvoice.Customer.ID)

	// Check if this is a bulk subscription by checking if there's a batch for this subscription
	if err == nil && userSub.StripeSubscriptionID != "" {
		batchRepo := repositories.NewSubscriptionBatchRepository(ss.db)
		_, batchErr := batchRepo.GetByStripeSubscriptionID(userSub.StripeSubscriptionID)
		if batchErr == nil {
			// This is a bulk subscription - handle it separately
			utils.Debug("📦 Detected bulk subscription invoice payment: %s", stripeInvoice.ID)
			// Get full subscription details from Stripe
			sub, subErr := subscription.Get(userSub.StripeSubscriptionID, nil)
			if subErr != nil {
				return fmt.Errorf("failed to retrieve subscription %s: %v", userSub.StripeSubscriptionID, subErr)
			}
			return ss.handleBulkInvoicePaymentSucceeded(&stripeInvoice, sub)
		}
		// Not a bulk subscription, continue with normal processing
	}
	if err != nil {
		// If no active subscription found, try to find by metadata in the invoice
		utils.Debug("⚠️ No active subscription found for customer %s, skipping invoice %s", stripeInvoice.Customer.ID, stripeInvoice.ID)
		return fmt.Errorf("subscription not found for customer %s (invoice %s): %v", stripeInvoice.Customer.ID, stripeInvoice.ID, err)
	}

	// Créer ou mettre à jour la facture
	invoiceRecord := &models.Invoice{
		UserID:             userSub.UserID,
		UserSubscriptionID: userSub.ID,
		StripeInvoiceID:    stripeInvoice.ID,
		Amount:             stripeInvoice.AmountPaid,
		Currency:           string(stripeInvoice.Currency),
		Status:             string(stripeInvoice.Status),
		InvoiceNumber:      stripeInvoice.Number,
		InvoiceDate:        time.Unix(stripeInvoice.Created, 0),
		DueDate:            time.Unix(stripeInvoice.DueDate, 0),
		StripeHostedURL:    stripeInvoice.HostedInvoiceURL,
		DownloadURL:        stripeInvoice.InvoicePDF,
	}

	if stripeInvoice.StatusTransitions.PaidAt > 0 {
		paidAt := time.Unix(stripeInvoice.StatusTransitions.PaidAt, 0)
		invoiceRecord.PaidAt = &paidAt
	}

	// Vérifier si la facture existe déjà
	existingInvoice, err := ss.repository.GetInvoiceByStripeID(stripeInvoice.ID)
	if err != nil {
		// Facture n'existe pas, la créer
		utils.Debug("✅ Creating invoice %s for user %s (amount: %d %s)",
			stripeInvoice.Number, userSub.UserID, stripeInvoice.AmountPaid, stripeInvoice.Currency)
		return ss.repository.CreateInvoice(invoiceRecord)
	} else {
		// Mettre à jour la facture existante
		existingInvoice.Status = invoiceRecord.Status
		existingInvoice.PaidAt = invoiceRecord.PaidAt
		existingInvoice.DownloadURL = invoiceRecord.DownloadURL
		utils.Debug("✅ Updated invoice %s for user %s", stripeInvoice.Number, userSub.UserID)
		return ss.repository.UpdateInvoice(existingInvoice)
	}
}

// handleInvoicePaymentFailed traite l'échec de paiement d'une facture
func (ss *stripeService) handleInvoicePaymentFailed(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return err
	}

	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(stripeInvoice.Customer.ID)
	if err != nil {
		utils.Debug("⚠️ No active subscription found for customer %s (invoice %s payment failed)", stripeInvoice.Customer.ID, stripeInvoice.ID)
		return fmt.Errorf("subscription not found for customer %s: %v", stripeInvoice.Customer.ID, err)
	}

	userSub.Status = "past_due"
	utils.Debug("⚠️ Invoice %s payment failed for subscription %s - marking as past_due", stripeInvoice.ID, userSub.StripeSubscriptionID)
	return ss.repository.UpdateUserSubscription(userSub)
}

// handleCheckoutSessionCompleted traite la finalisation d'une session de checkout
func (ss *stripeService) handleCheckoutSessionCompleted(event *stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return err
	}

	userID, exists := session.Metadata["user_id"]
	if !exists {
		return fmt.Errorf("user_id not found in session metadata")
	}

	// Check if this is replacing a free subscription
	replaceSubscriptionIDStr, hasReplaceID := session.Metadata["replace_subscription_id"]
	if hasReplaceID {
		replaceSubscriptionID, err := uuid.Parse(replaceSubscriptionIDStr)
		if err == nil {
			// Get the subscription to verify it's actually free
			oldSubscription, err := ss.repository.GetUserSubscription(replaceSubscriptionID)
			if err == nil {
				oldPlan, err := ss.subscriptionService.GetSubscriptionPlan(oldSubscription.SubscriptionPlanID)
				if err == nil && oldPlan.PriceAmount == 0 {
					// Delete the old free subscription (hard delete since it has no Stripe ID)
					utils.Info("🔄 Deleting old free subscription %s for user %s (upgrading to paid)",
						replaceSubscriptionID, userID)

					err = ss.genericService.DeleteEntity(replaceSubscriptionID, models.UserSubscription{}, false)
					if err != nil {
						utils.Warn("⚠️ Failed to delete old free subscription: %v", err)
						// Don't fail the webhook - new subscription will still be created
					} else {
						utils.Info("✅ Successfully deleted old free subscription %s", replaceSubscriptionID)
					}
				} else {
					utils.Warn("⚠️ Subscription %s is not a free plan, skipping deletion", replaceSubscriptionID)
				}
			}
		}
	}

	// This guarantees metadata is available when subscription.created webhook fires
	if session.Subscription != nil && session.Subscription.ID != "" {
		planID, hasPlanID := session.Metadata["subscription_plan_id"]

		// Build metadata to pass to subscription
		subscriptionMetadata := map[string]string{
			"user_id": userID,
		}
		if hasPlanID {
			subscriptionMetadata["subscription_plan_id"] = planID
		}
		if hasReplaceID {
			subscriptionMetadata["replace_subscription_id"] = replaceSubscriptionIDStr
		}

		// CRITICAL: Copy bulk purchase metadata from session to subscription
		if bulkPurchase, hasBulkPurchase := session.Metadata["bulk_purchase"]; hasBulkPurchase {
			subscriptionMetadata["bulk_purchase"] = bulkPurchase
			utils.Debug("🔄 Copying bulk_purchase=%s to subscription", bulkPurchase)
		}
		if quantity, hasQuantity := session.Metadata["quantity"]; hasQuantity {
			subscriptionMetadata["quantity"] = quantity
			utils.Debug("🔄 Copying quantity=%s to subscription", quantity)
		}
		if groupID, hasGroupID := session.Metadata["group_id"]; hasGroupID {
			subscriptionMetadata["group_id"] = groupID
			utils.Debug("🔄 Copying group_id=%s to subscription", groupID)
		}

		// Update the subscription metadata in Stripe to ensure it's propagated
		params := &stripe.SubscriptionParams{
			Metadata: subscriptionMetadata,
		}

		_, err := subscription.Update(session.Subscription.ID, params)
		if err != nil {
			return fmt.Errorf("failed to update subscription metadata: %v", err)
		}

		utils.Debug("✅ Updated subscription %s metadata for user %s", session.Subscription.ID, userID)
	}

	// Si c'est un abonnement, il sera créé via le webhook subscription.created
	// Ici on peut juste logger ou mettre à jour des métriques
	utils.Debug("Checkout completed for user %s, subscription: %s", userID, session.Subscription.ID)

	return nil
}

// handleSubscriptionPaused traite la mise en pause d'un abonnement
func (ss *stripeService) handleSubscriptionPaused(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %v", err)
	}

	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found in database: %v", err)
	}

	userSub.Status = "paused"
	utils.Debug("⏸️ Subscription %s paused for user %s", subscription.ID, userSub.UserID)

	return ss.repository.UpdateUserSubscription(userSub)
}

// handleSubscriptionResumed traite la reprise d'un abonnement
func (ss *stripeService) handleSubscriptionResumed(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %v", err)
	}

	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found in database: %v", err)
	}

	userSub.Status = string(subscription.Status) // Should be "active"
	utils.Debug("▶️ Subscription %s resumed for user %s (status: %s)", subscription.ID, userSub.UserID, subscription.Status)

	return ss.repository.UpdateUserSubscription(userSub)
}

// handleTrialWillEnd traite l'alerte de fin d'essai
func (ss *stripeService) handleTrialWillEnd(event *stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %v", err)
	}

	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return fmt.Errorf("subscription not found in database: %v", err)
	}

	trialEndDate := time.Unix(subscription.TrialEnd, 0)
	utils.Debug("⏰ Trial will end for user %s on %s (subscription: %s)",
		userSub.UserID, trialEndDate.Format("2006-01-02"), subscription.ID)

	// TODO: Send notification email/webhook to user about trial ending
	// For now, just log it
	return nil
}

// handleInvoiceCreated traite la création d'une facture
func (ss *stripeService) handleInvoiceCreated(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %v", err)
	}

	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(stripeInvoice.Customer.ID)
	if err != nil {
		utils.Debug("⚠️ No active subscription found for customer %s (invoice %s created)",
			stripeInvoice.Customer.ID, stripeInvoice.ID)
		return nil // Not an error - might be a one-time invoice
	}

	// Check if invoice already exists
	_, err = ss.repository.GetInvoiceByStripeID(stripeInvoice.ID)
	if err == nil {
		// Invoice already exists, skip
		return nil
	}

	// Create invoice record with "draft" or "open" status
	invoiceRecord := &models.Invoice{
		UserID:             userSub.UserID,
		UserSubscriptionID: userSub.ID,
		StripeInvoiceID:    stripeInvoice.ID,
		Amount:             stripeInvoice.AmountDue,
		Currency:           string(stripeInvoice.Currency),
		Status:             string(stripeInvoice.Status),
		InvoiceNumber:      stripeInvoice.Number,
		InvoiceDate:        time.Unix(stripeInvoice.Created, 0),
		DueDate:            time.Unix(stripeInvoice.DueDate, 0),
		StripeHostedURL:    stripeInvoice.HostedInvoiceURL,
		DownloadURL:        stripeInvoice.InvoicePDF,
	}

	utils.Debug("📄 Creating invoice %s for user %s (status: %s, amount: %d %s)",
		stripeInvoice.Number, userSub.UserID, stripeInvoice.Status, stripeInvoice.AmountDue, stripeInvoice.Currency)

	return ss.repository.CreateInvoice(invoiceRecord)
}

// handleInvoiceFinalized traite la finalisation d'une facture
func (ss *stripeService) handleInvoiceFinalized(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %v", err)
	}

	// Get existing invoice
	existingInvoice, err := ss.repository.GetInvoiceByStripeID(stripeInvoice.ID)
	if err != nil {
		// Invoice doesn't exist yet, create it
		userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(stripeInvoice.Customer.ID)
		if err != nil {
			utils.Debug("⚠️ No subscription found for customer %s (invoice %s finalized)",
				stripeInvoice.Customer.ID, stripeInvoice.ID)
			return nil
		}

		invoiceRecord := &models.Invoice{
			UserID:             userSub.UserID,
			UserSubscriptionID: userSub.ID,
			StripeInvoiceID:    stripeInvoice.ID,
			Amount:             stripeInvoice.AmountDue,
			Currency:           string(stripeInvoice.Currency),
			Status:             "open",
			InvoiceNumber:      stripeInvoice.Number,
			InvoiceDate:        time.Unix(stripeInvoice.Created, 0),
			DueDate:            time.Unix(stripeInvoice.DueDate, 0),
			StripeHostedURL:    stripeInvoice.HostedInvoiceURL,
			DownloadURL:        stripeInvoice.InvoicePDF,
		}

		utils.Debug("📋 Creating finalized invoice %s for user %s", stripeInvoice.Number, userSub.UserID)
		return ss.repository.CreateInvoice(invoiceRecord)
	}

	// Update existing invoice
	existingInvoice.Status = "open"
	existingInvoice.InvoiceNumber = stripeInvoice.Number
	existingInvoice.StripeHostedURL = stripeInvoice.HostedInvoiceURL
	existingInvoice.DownloadURL = stripeInvoice.InvoicePDF

	utils.Debug("📋 Updated invoice %s to finalized (open) status", stripeInvoice.Number)
	return ss.repository.UpdateInvoice(existingInvoice)
}

// handlePaymentMethodAttached traite l'ajout d'un moyen de paiement
func (ss *stripeService) handlePaymentMethodAttached(event *stripe.Event) error {
	var pm stripe.PaymentMethod
	if err := json.Unmarshal(event.Data.Raw, &pm); err != nil {
		return fmt.Errorf("failed to unmarshal payment method: %v", err)
	}

	// Get customer to find user ID
	if pm.Customer == nil {
		return fmt.Errorf("payment method has no customer")
	}

	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(pm.Customer.ID)
	if err != nil {
		utils.Debug("⚠️ No subscription found for customer %s (payment method attached)", pm.Customer.ID)
		return nil // Not an error - customer might not have subscription yet
	}

	// Check if payment method already exists
	_, err = ss.repository.GetPaymentMethodByStripeID(pm.ID)
	if err == nil {
		// Payment method already exists
		return nil
	}

	// Create payment method record
	pmRecord := &models.PaymentMethod{
		UserID:                userSub.UserID,
		StripePaymentMethodID: pm.ID,
		Type:                  string(pm.Type),
		IsActive:              true,
		IsDefault:             false, // Will be updated by subscription update if default
	}

	// Add card details if applicable
	if pm.Card != nil {
		pmRecord.CardBrand = string(pm.Card.Brand)
		pmRecord.CardLast4 = pm.Card.Last4
		pmRecord.CardExpMonth = int(pm.Card.ExpMonth)
		pmRecord.CardExpYear = int(pm.Card.ExpYear)
	}

	utils.Debug("💳 Creating payment method %s for user %s (type: %s, card: %s)",
		pm.ID, userSub.UserID, pm.Type, pmRecord.CardLast4)

	return ss.repository.CreatePaymentMethod(pmRecord)
}

// handlePaymentMethodDetached traite la suppression d'un moyen de paiement
func (ss *stripeService) handlePaymentMethodDetached(event *stripe.Event) error {
	var pm stripe.PaymentMethod
	if err := json.Unmarshal(event.Data.Raw, &pm); err != nil {
		return fmt.Errorf("failed to unmarshal payment method: %v", err)
	}

	// Get payment method from database
	pmRecord, err := ss.repository.GetPaymentMethodByStripeID(pm.ID)
	if err != nil {
		// Payment method not in database, nothing to do
		return nil
	}

	// Mark as inactive instead of deleting (keep history)
	pmRecord.IsActive = false
	pmRecord.IsDefault = false

	utils.Debug("🗑️ Payment method %s detached for user %s", pm.ID, pmRecord.UserID)
	return ss.repository.UpdatePaymentMethod(pmRecord)
}

// handleCustomerUpdated traite la mise à jour d'un client
func (ss *stripeService) handleCustomerUpdated(event *stripe.Event) error {
	var customer stripe.Customer
	if err := json.Unmarshal(event.Data.Raw, &customer); err != nil {
		return fmt.Errorf("failed to unmarshal customer: %v", err)
	}

	// Try to find subscription for this customer
	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(customer.ID)
	if err != nil {
		// No subscription found, might be a new customer
		utils.Debug("👤 Customer %s updated but no subscription found", customer.ID)
		return nil
	}

	// Update customer ID in subscription if changed
	if userSub.StripeCustomerID != customer.ID {
		userSub.StripeCustomerID = customer.ID
		utils.Debug("👤 Updated customer ID for user %s to %s", userSub.UserID, customer.ID)
		return ss.repository.UpdateUserSubscription(userSub)
	}

	// TODO: Sync customer email/name to Casdoor if needed
	utils.Debug("👤 Customer %s updated for user %s", customer.ID, userSub.UserID)
	return nil
}

// ============================================================================
// BULK LICENSE WEBHOOK HANDLERS
// ============================================================================

// isBulkSubscription checks if a Stripe subscription is a bulk purchase
func isBulkSubscription(subscription *stripe.Subscription) bool {
	if metadata, exists := subscription.Metadata["bulk_purchase"]; exists && metadata == "true" {
		return true
	}
	return false
}

// handleBulkSubscriptionCreated handles creation of bulk subscriptions
// This creates a SubscriptionBatch and individual UserSubscription records for each license
func (ss *stripeService) handleBulkSubscriptionCreated(subscription *stripe.Subscription) error {
	userID, exists := subscription.Metadata["user_id"]
	if !exists {
		return fmt.Errorf("user_id not found in subscription metadata")
	}

	planIDStr, exists := subscription.Metadata["subscription_plan_id"]
	if !exists {
		return fmt.Errorf("subscription_plan_id not found in bulk subscription metadata")
	}

	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		return fmt.Errorf("invalid subscription_plan_id: %v", err)
	}

	// Get quantity from metadata or subscription items
	quantityStr, exists := subscription.Metadata["quantity"]
	if !exists {
		return fmt.Errorf("quantity not found in bulk subscription metadata")
	}

	quantity := 0
	if _, err := fmt.Sscanf(quantityStr, "%d", &quantity); err != nil {
		return fmt.Errorf("invalid quantity: %v", err)
	}

	// Get group_id if present
	var groupID *uuid.UUID
	if groupIDStr, exists := subscription.Metadata["group_id"]; exists {
		if parsedGroupID, err := uuid.Parse(groupIDStr); err == nil {
			groupID = &parsedGroupID
		}
	}

	// Get subscription item ID and period info
	if len(subscription.Items.Data) == 0 {
		return fmt.Errorf("subscription has no items")
	}

	item := subscription.Items.Data[0]
	stripeSubscriptionItemID := item.ID
	currentPeriodStart := time.Unix(item.CurrentPeriodStart, 0)
	currentPeriodEnd := time.Unix(item.CurrentPeriodEnd, 0)

	// Get plan for validation
	plan, err := ss.subscriptionService.GetSubscriptionPlan(planID)
	if err != nil {
		return fmt.Errorf("failed to get subscription plan: %v", err)
	}

	utils.Info("📦 Creating bulk subscription batch for user %s: %d licenses of plan %s", userID, quantity, plan.Name)

	// CRITICAL: Check if payment already succeeded (race condition)
	// If subscription status is "active", payment already succeeded before this webhook
	batchStatus := "pending_payment"
	licenseStatus := "pending_payment"
	if subscription.Status == "active" {
		batchStatus = "active"
		licenseStatus = "unassigned"
		utils.Info("💰 Payment already succeeded - creating batch as active")
	}

	// Create the batch record
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          userID,
		SubscriptionPlanID:       planID,
		GroupID:                  groupID,
		StripeSubscriptionID:     subscription.ID,
		StripeSubscriptionItemID: stripeSubscriptionItemID,
		TotalQuantity:            quantity,
		AssignedQuantity:         0,
		Status:                   batchStatus,
		CurrentPeriodStart:       currentPeriodStart,
		CurrentPeriodEnd:         currentPeriodEnd,
	}

	batchRepo := repositories.NewSubscriptionBatchRepository(ss.db)
	if err := batchRepo.Create(batch); err != nil {
		return fmt.Errorf("failed to create batch: %v", err)
	}

	utils.Info("✅ Created batch %s with %d licenses (status: %s)", batch.ID, quantity, batchStatus)

	// Create individual license records
	for i := 0; i < quantity; i++ {
		license := models.UserSubscription{
			UserID:               "",  // Unassigned
			PurchaserUserID:      &userID,
			SubscriptionBatchID:  &batch.ID,
			SubscriptionPlanID:   planID,
			StripeSubscriptionID: subscription.ID,
			StripeCustomerID:     subscription.Customer.ID,
			Status:               licenseStatus,
			CurrentPeriodStart:   currentPeriodStart,
			CurrentPeriodEnd:     currentPeriodEnd,
		}

		if err := ss.repository.CreateUserSubscription(&license); err != nil {
			utils.Error("Failed to create license %d/%d: %v", i+1, quantity, err)
		}
	}

	if batchStatus == "active" {
		utils.Info("✅ Created %d active licenses for batch %s (ready to assign)", quantity, batch.ID)
	} else {
		utils.Info("✅ Created %d licenses for batch %s (awaiting payment confirmation)", quantity, batch.ID)
	}

	return nil
}

// handleBulkSubscriptionUpdated handles quantity changes in bulk subscriptions
func (ss *stripeService) handleBulkSubscriptionUpdated(subscription *stripe.Subscription) error {
	// Get the subscription batch
	batchRepo := repositories.NewSubscriptionBatchRepository(ss.db)
	batch, err := batchRepo.GetByStripeSubscriptionID(subscription.ID)
	if err != nil {
		return fmt.Errorf("batch not found for subscription %s: %v", subscription.ID, err)
	}

	// CRITICAL: Check if subscription is being cancelled by detecting canceled_at timestamp
	isBeingCancelled := false
	wasCancelled := batch.CancelledAt != nil

	if subscription.CanceledAt > 0 && !wasCancelled {
		// This is a NEW cancellation (canceled_at just got set)
		isBeingCancelled = true
		utils.Info("🚫 Bulk subscription %s is being cancelled (canceled_at: %d)", subscription.ID, subscription.CanceledAt)
	}

	// If subscription is cancelled, terminate all terminals and cancel all licenses
	if isBeingCancelled {
		utils.Info("❌ Cancelling batch %s and all associated licenses due to subscription cancellation", batch.ID)

		// Get all licenses in the batch
		var licenses []models.UserSubscription
		ss.db.Where("subscription_batch_id = ?", batch.ID).Find(&licenses)

		now := time.Now()
		for _, license := range licenses {
			// Terminate terminals for assigned licenses
			if license.UserID != "" {
				utils.Info("🔌 Terminating all active terminals for user %s due to batch cancellation", license.UserID)
				if err := ss.terminateUserTerminals(license.UserID); err != nil {
					utils.Error("Failed to terminate terminals for user %s: %v", license.UserID, err)
					// Continue with other licenses
				}
			}

			// Mark license as cancelled
			license.Status = "cancelled"
			license.CancelledAt = &now
			ss.repository.UpdateUserSubscription(&license)
		}

		// Mark batch as cancelled
		batch.Status = "cancelled"
		batch.CancelledAt = &now
		return batchRepo.Update(batch)
	}

	// Check if quantity changed
	newQuantity := int(subscription.Items.Data[0].Quantity)
	if newQuantity == batch.TotalQuantity {
		utils.Debug("ℹ️ Batch quantity unchanged: %d", newQuantity)
		return nil // No change
	}

	utils.Info("🔄 Batch %s quantity changed from %d to %d", batch.ID, batch.TotalQuantity, newQuantity)

	difference := newQuantity - batch.TotalQuantity

	if difference > 0 {
		// Adding licenses
		for i := 0; i < difference; i++ {
			license := models.UserSubscription{
				UserID:               "",
				PurchaserUserID:      &batch.PurchaserUserID,
				SubscriptionBatchID:  &batch.ID,
				SubscriptionPlanID:   batch.SubscriptionPlanID,
				StripeSubscriptionID: batch.StripeSubscriptionID,
				StripeCustomerID:     batch.PurchaserUserID,
				Status:               "unassigned",
				CurrentPeriodStart:   batch.CurrentPeriodStart,
				CurrentPeriodEnd:     batch.CurrentPeriodEnd,
			}
			if err := ss.repository.CreateUserSubscription(&license); err != nil {
				utils.Error("Failed to create additional license: %v", err)
			}
		}
		utils.Info("✅ Added %d licenses to batch %s", difference, batch.ID)
	} else {
		// Removing licenses (only unassigned ones)
		toRemove := -difference
		var unassignedLicenses []models.UserSubscription
		ss.db.Where("subscription_batch_id = ? AND status = ?", batch.ID, "unassigned").
			Limit(toRemove).
			Find(&unassignedLicenses)

		if len(unassignedLicenses) < toRemove {
			return fmt.Errorf("cannot remove %d licenses, only %d unassigned available", toRemove, len(unassignedLicenses))
		}

		for _, license := range unassignedLicenses {
			ss.db.Delete(&license)
		}
		utils.Info("✅ Removed %d licenses from batch %s", len(unassignedLicenses), batch.ID)
	}

	// Update batch total quantity
	batch.TotalQuantity = newQuantity
	batch.CurrentPeriodStart = time.Unix(subscription.Items.Data[0].CurrentPeriodStart, 0)
	batch.CurrentPeriodEnd = time.Unix(subscription.Items.Data[0].CurrentPeriodEnd, 0)
	return batchRepo.Update(batch)
}

// handleBulkSubscriptionDeleted handles cancellation of bulk subscriptions
func (ss *stripeService) handleBulkSubscriptionDeleted(subscription *stripe.Subscription) error {
	// Get the subscription batch
	batchRepo := repositories.NewSubscriptionBatchRepository(ss.db)
	batch, err := batchRepo.GetByStripeSubscriptionID(subscription.ID)
	if err != nil {
		return fmt.Errorf("batch not found for subscription %s: %v", subscription.ID, err)
	}

	utils.Info("❌ Cancelling batch %s and all associated licenses", batch.ID)

	// Cancel all licenses in the batch
	var licenses []models.UserSubscription
	ss.db.Where("subscription_batch_id = ?", batch.ID).Find(&licenses)

	now := time.Now()
	for _, license := range licenses {
		// CRITICAL: Terminate all active terminals for assigned licenses
		if license.UserID != "" {
			utils.Info("🔌 Terminating all active terminals for user %s due to batch license cancellation", license.UserID)
			if err := ss.terminateUserTerminals(license.UserID); err != nil {
				utils.Error("Failed to terminate terminals for user %s: %v", license.UserID, err)
				// Don't fail batch cancellation if terminal termination fails
			}
		}

		license.Status = "cancelled"
		license.CancelledAt = &now
		ss.repository.UpdateUserSubscription(&license)
	}

	// Cancel the batch
	batch.Status = "cancelled"
	batch.CancelledAt = &now
	return batchRepo.Update(batch)
}

// handleBulkInvoicePaymentSucceeded activates bulk licenses on successful payment
func (ss *stripeService) handleBulkInvoicePaymentSucceeded(invoice *stripe.Invoice, subscription *stripe.Subscription) error {
	// Get the subscription batch
	batchRepo := repositories.NewSubscriptionBatchRepository(ss.db)
	batch, err := batchRepo.GetByStripeSubscriptionID(subscription.ID)
	if err != nil {
		return fmt.Errorf("batch not found for subscription %s: %v", subscription.ID, err)
	}

	utils.Info("💰 Payment succeeded for bulk subscription batch %s - activating batch and licenses", batch.ID)

	// Activate batch if it's pending payment
	if batch.Status == "pending_payment" || batch.Status != "active" {
		batch.Status = "active"
		if err := batchRepo.Update(batch); err != nil {
			return fmt.Errorf("failed to activate batch: %v", err)
		}
		utils.Info("✅ Batch %s activated", batch.ID)
	}

	// Activate all pending_payment licenses in this batch
	var licenses []models.UserSubscription
	ss.db.Where("subscription_batch_id = ? AND status = ?", batch.ID, "pending_payment").Find(&licenses)

	activatedCount := 0
	for _, license := range licenses {
		// Change status from pending_payment to unassigned (ready to be assigned)
		license.Status = "unassigned"
		if err := ss.repository.UpdateUserSubscription(&license); err != nil {
			utils.Error("Failed to activate license %s: %v", license.ID, err)
			continue
		}
		activatedCount++
	}

	utils.Info("✅ Activated %d/%d licenses in batch %s", activatedCount, len(licenses), batch.ID)

	return nil
}

// CancelSubscription annule un abonnement
func (ss *stripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	if cancelAtPeriodEnd {
		// Annulation à la fin de la période de facturation
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		_, err := subscription.Update(subscriptionID, params)
		return err
	} else {
		// Annulation immédiate
		_, err := subscription.Cancel(subscriptionID, nil)
		return err
	}
}

// MarkSubscriptionAsCancelled marque un abonnement comme annulé dans la base de données
// Also terminates all active terminals for the user
func (ss *stripeService) MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error {
	// CRITICAL: Terminate all active terminals before cancelling subscription
	utils.Info("🔌 Terminating all active terminals for user %s due to manual subscription cancellation", userSubscription.UserID)
	if err := ss.terminateUserTerminals(userSubscription.UserID); err != nil {
		utils.Error("Failed to terminate terminals for user %s: %v", userSubscription.UserID, err)
		// Don't fail subscription cancellation if terminal termination fails
	}

	userSubscription.Status = "cancelled"
	now := time.Now()
	userSubscription.CancelledAt = &now
	return ss.repository.UpdateUserSubscription(userSubscription)
}

// terminateUserTerminals stops all active terminals for a user
// This uses direct repository calls to avoid circular dependency with terminalTrainer/services
func (ss *stripeService) terminateUserTerminals(userID string) error {
	// Get terminal repository
	termRepository := terminalRepo.NewTerminalRepository(ss.db)

	// Get all active terminals for this user
	terminals, err := termRepository.GetTerminalSessionsByUserID(userID, true)
	if err != nil {
		return fmt.Errorf("failed to get user terminals: %v", err)
	}

	if terminals == nil || len(*terminals) == 0 {
		utils.Debug("No active terminals found for user %s", userID)
		return nil
	}

	utils.Info("Found %d active terminals for user %s, terminating all", len(*terminals), userID)

	// Stop each terminal directly using repository
	terminatedCount := 0
	for _, terminal := range *terminals {
		if terminal.Status == "active" {
			utils.Debug("Stopping terminal %s (session: %s) for user %s", terminal.ID, terminal.SessionID, userID)

			// Update terminal status to stopped
			terminal.Status = "stopped"
			if err := termRepository.UpdateTerminalSession(&terminal); err != nil {
				utils.Error("Failed to update terminal %s status for user %s: %v", terminal.SessionID, userID, err)
				continue
			}

			// Decrement concurrent_terminals metric (call UpdateUsageMetricValue directly)
			if err := ss.decrementConcurrentTerminals(userID); err != nil {
				utils.Warn("Failed to decrement concurrent_terminals for user %s: %v", userID, err)
			}

			terminatedCount++
			utils.Debug("Successfully stopped terminal %s for user %s", terminal.SessionID, userID)
		}
	}

	utils.Info("Successfully terminated %d/%d terminals for user %s", terminatedCount, len(*terminals), userID)
	return nil
}

// decrementConcurrentTerminals decrements the concurrent_terminals metric for a user
func (ss *stripeService) decrementConcurrentTerminals(userID string) error {
	// Get the user's current usage metric for concurrent_terminals
	var usageMetric models.UsageMetrics
	err := ss.db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&usageMetric).Error
	if err != nil {
		return fmt.Errorf("usage metric not found: %v", err)
	}

	// Decrement usage by 1 (ensure it doesn't go negative)
	if usageMetric.CurrentValue > 0 {
		usageMetric.CurrentValue -= 1
		usageMetric.LastUpdated = time.Now()
		if err := ss.db.Save(&usageMetric).Error; err != nil {
			return fmt.Errorf("failed to update usage metric: %v", err)
		}
	}

	return nil
}

// ReactivateSubscription réactive un abonnement annulé
func (ss *stripeService) ReactivateSubscription(subscriptionID string) error {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(false),
	}
	params.AddExpand("latest_invoice.payment_intent")

	_, err := subscription.Update(subscriptionID, params)
	return err
}

// UpdateSubscription updates a Stripe subscription to a new price with proration support
func (ss *stripeService) UpdateSubscription(subscriptionID, newPriceID, prorationBehavior string) (*stripe.Subscription, error) {
	// Get current subscription to find the subscription item ID
	currentSub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get current subscription: %v", err)
	}

	if len(currentSub.Items.Data) == 0 {
		return nil, fmt.Errorf("subscription has no items")
	}

	// Default proration behavior
	if prorationBehavior == "" {
		prorationBehavior = "always_invoice"
	}

	// Validate proration behavior
	validBehaviors := map[string]bool{
		"always_invoice":    true,
		"create_prorations": true,
		"none":              true,
	}
	if !validBehaviors[prorationBehavior] {
		prorationBehavior = "always_invoice"
	}

	// Update the subscription with the new price
	params := &stripe.SubscriptionParams{
		ProrationBehavior: stripe.String(prorationBehavior),
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:    stripe.String(currentSub.Items.Data[0].ID),
				Price: stripe.String(newPriceID),
			},
		},
	}
	params.AddExpand("latest_invoice")
	params.AddExpand("customer")

	updatedSub, err := subscription.Update(subscriptionID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription in Stripe: %v", err)
	}

	return updatedSub, nil
}

// AttachPaymentMethod attache un moyen de paiement à un client
func (ss *stripeService) AttachPaymentMethod(paymentMethodID, customerID string) error {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}

	_, err := paymentmethod.Attach(paymentMethodID, params)
	return err
}

// DetachPaymentMethod détache un moyen de paiement
func (ss *stripeService) DetachPaymentMethod(paymentMethodID string) error {
	_, err := paymentmethod.Detach(paymentMethodID, nil)
	return err
}

// SetDefaultPaymentMethod définit le moyen de paiement par défaut
func (ss *stripeService) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	params := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(paymentMethodID),
		},
	}

	_, err := customer.Update(customerID, params)
	return err
}

// GetInvoice récupère une facture Stripe
func (ss *stripeService) GetInvoice(invoiceID string) (*stripe.Invoice, error) {
	return invoice.Get(invoiceID, nil)
}

// SendInvoice envoie une facture par email
func (ss *stripeService) SendInvoice(invoiceID string) error {
	params := &stripe.InvoiceSendInvoiceParams{}
	_, err := invoice.SendInvoice(invoiceID, params)
	return err
}

// ValidateWebhookSignature valide la signature d'un webhook Stripe
func (ss *stripeService) ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	event, err := webhook.ConstructEvent(payload, signature, ss.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("webhook signature verification failed: %v", err)
	}
	return &event, nil
}

// SyncExistingSubscriptions synchronise tous les abonnements Stripe existants
func (ss *stripeService) SyncExistingSubscriptions() (*SyncSubscriptionsResult, error) {
	result := &SyncSubscriptionsResult{
		FailedSubscriptions: []FailedSubscription{},
		CreatedDetails:      []string{},
		UpdatedDetails:      []string{},
		SkippedDetails:      []string{},
	}

	// Récupérer tous les abonnements depuis Stripe
	params := &stripe.SubscriptionListParams{
		Status: stripe.String("all"), // Inclure tous les statuts
	}
	params.Filters.AddFilter("limit", "", "100") // Paginer par 100

	iter := subscription.List(params)
	for iter.Next() {
		sub := iter.Subscription()
		result.ProcessedSubscriptions++

		// Traiter chaque abonnement
		if err := ss.processSingleSubscription(sub, result); err != nil {
			result.FailedSubscriptions = append(result.FailedSubscriptions, FailedSubscription{
				StripeSubscriptionID: sub.ID,
				Error:                err.Error(),
			})
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate subscriptions: %v", err)
	}

	return result, nil
}

// SyncUserSubscriptions synchronise les abonnements d'un utilisateur spécifique
func (ss *stripeService) SyncUserSubscriptions(userID string) (*SyncSubscriptionsResult, error) {
	result := &SyncSubscriptionsResult{
		FailedSubscriptions: []FailedSubscription{},
		CreatedDetails:      []string{},
		UpdatedDetails:      []string{},
		SkippedDetails:      []string{},
	}

	// Récupérer tous les abonnements depuis Stripe avec le metadata user_id
	params := &stripe.SubscriptionListParams{
		Status: stripe.String("all"),
	}
	params.Filters.AddFilter("limit", "", "100")

	iter := subscription.List(params)
	for iter.Next() {
		sub := iter.Subscription()

		// Vérifier si l'abonnement appartient à cet utilisateur
		if metaUserID, exists := sub.Metadata["user_id"]; exists && metaUserID == userID {
			result.ProcessedSubscriptions++

			if err := ss.processSingleSubscription(sub, result); err != nil {
				result.FailedSubscriptions = append(result.FailedSubscriptions, FailedSubscription{
					StripeSubscriptionID: sub.ID,
					UserID:               userID,
					Error:                err.Error(),
				})
			}
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate subscriptions: %v", err)
	}

	return result, nil
}

// processSingleSubscription traite un abonnement Stripe individuel
func (ss *stripeService) processSingleSubscription(sub *stripe.Subscription, result *SyncSubscriptionsResult) error {
	// Vérifier les métadonnées requises
	userID, userExists := sub.Metadata["user_id"]
	planIDStr, planExists := sub.Metadata["subscription_plan_id"]

	if !userExists || !planExists {
		err := "missing required metadata (user_id or subscription_plan_id)"
		result.SkippedSubscriptions++
		result.SkippedDetails = append(result.SkippedDetails,
			fmt.Sprintf("Subscription %s: %s", sub.ID, err))
		return fmt.Errorf("%s", err)
	}

	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		errMsg := fmt.Sprintf("invalid subscription_plan_id format: %v", err)
		result.SkippedSubscriptions++
		result.SkippedDetails = append(result.SkippedDetails,
			fmt.Sprintf("Subscription %s: %s", sub.ID, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Vérifier si l'abonnement existe déjà dans notre base
	existingSubscription, err := ss.repository.GetUserSubscriptionByStripeID(sub.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("database error checking existing subscription: %v", err)
	}

	// Construire les dates de période
	var currentPeriodStart, currentPeriodEnd time.Time
	if len(sub.Items.Data) > 0 {
		item := sub.Items.Data[0]
		currentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		currentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
	}

	if existingSubscription != nil {
		// Abonnement existe - mettre à jour
		existingSubscription.Status = string(sub.Status)
		existingSubscription.CurrentPeriodStart = currentPeriodStart
		existingSubscription.CurrentPeriodEnd = currentPeriodEnd
		existingSubscription.CancelAtPeriodEnd = sub.CancelAtPeriodEnd

		if sub.TrialEnd > 0 {
			trialEnd := time.Unix(sub.TrialEnd, 0)
			existingSubscription.TrialEnd = &trialEnd
		}

		if err := ss.repository.UpdateUserSubscription(existingSubscription); err != nil {
			return fmt.Errorf("failed to update subscription: %v", err)
		}

		result.UpdatedSubscriptions++
		result.UpdatedDetails = append(result.UpdatedDetails,
			fmt.Sprintf("Updated subscription %s for user %s", sub.ID, userID))
	} else {
		// Abonnement n'existe pas - créer
		userSubscription := &models.UserSubscription{
			UserID:               userID,
			SubscriptionPlanID:   planID,
			StripeSubscriptionID: sub.ID,
			StripeCustomerID:     sub.Customer.ID,
			Status:               string(sub.Status),
			CurrentPeriodStart:   currentPeriodStart,
			CurrentPeriodEnd:     currentPeriodEnd,
			CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
		}

		if sub.TrialEnd > 0 {
			trialEnd := time.Unix(sub.TrialEnd, 0)
			userSubscription.TrialEnd = &trialEnd
		}

		if err := ss.repository.CreateUserSubscription(userSubscription); err != nil {
			return fmt.Errorf("failed to create subscription: %v", err)
		}

		result.CreatedSubscriptions++
		result.CreatedDetails = append(result.CreatedDetails,
			fmt.Sprintf("Created subscription %s for user %s", sub.ID, userID))
	}

	return nil
}

// SyncSubscriptionsWithMissingMetadata tente de récupérer les métadonnées manquantes
func (ss *stripeService) SyncSubscriptionsWithMissingMetadata() (*SyncSubscriptionsResult, error) {
	result := &SyncSubscriptionsResult{
		FailedSubscriptions: []FailedSubscription{},
		CreatedDetails:      []string{},
		UpdatedDetails:      []string{},
		SkippedDetails:      []string{},
	}

	// Récupérer tous les abonnements depuis Stripe
	params := &stripe.SubscriptionListParams{
		Status: stripe.String("all"),
	}
	params.Filters.AddFilter("limit", "", "100")

	iter := subscription.List(params)
	for iter.Next() {
		sub := iter.Subscription()
		result.ProcessedSubscriptions++

		// Vérifier si les métadonnées sont manquantes
		_, hasUserID := sub.Metadata["user_id"]
		_, hasPlanID := sub.Metadata["subscription_plan_id"]

		if hasUserID && hasPlanID {
			// Métadonnées présentes, passer
			result.SkippedSubscriptions++
			result.SkippedDetails = append(result.SkippedDetails,
				fmt.Sprintf("Subscription %s: already has metadata", sub.ID))
			continue
		}

		// Essayer de récupérer les métadonnées depuis les sessions de checkout
		recoveredUserID, recoveredPlanID, err := ss.recoverMetadataFromCheckoutSessions(sub)
		if err != nil {
			result.FailedSubscriptions = append(result.FailedSubscriptions, FailedSubscription{
				StripeSubscriptionID: sub.ID,
				Error:                fmt.Sprintf("failed to recover metadata: %v", err),
			})
			continue
		}

		if recoveredUserID == "" || recoveredPlanID == uuid.Nil {
			result.SkippedSubscriptions++
			result.SkippedDetails = append(result.SkippedDetails,
				fmt.Sprintf("Subscription %s: could not recover metadata", sub.ID))
			continue
		}

		// Créer l'abonnement avec les métadonnées récupérées
		err = ss.LinkSubscriptionToUser(sub.ID, recoveredUserID, recoveredPlanID)
		if err != nil {
			result.FailedSubscriptions = append(result.FailedSubscriptions, FailedSubscription{
				StripeSubscriptionID: sub.ID,
				UserID:               recoveredUserID,
				Error:                fmt.Sprintf("failed to link subscription: %v", err),
			})
		} else {
			result.CreatedSubscriptions++
			result.CreatedDetails = append(result.CreatedDetails,
				fmt.Sprintf("Linked subscription %s to user %s", sub.ID, recoveredUserID))
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate subscriptions: %v", err)
	}

	return result, nil
}

// recoverMetadataFromCheckoutSessions essaie de récupérer les métadonnées depuis les sessions de checkout
func (ss *stripeService) recoverMetadataFromCheckoutSessions(sub *stripe.Subscription) (string, uuid.UUID, error) {
	// Récupérer les sessions de checkout pour ce client
	params := &stripe.CheckoutSessionListParams{
		Customer: stripe.String(sub.Customer.ID),
	}
	params.Filters.AddFilter("limit", "", "100")

	iter := session.List(params)
	for iter.Next() {
		checkoutSession := iter.CheckoutSession()

		// Vérifier si cette session a créé notre abonnement
		if checkoutSession.Subscription != nil && checkoutSession.Subscription.ID == sub.ID {
			// Récupérer les métadonnées de la session
			userID, hasUserID := checkoutSession.Metadata["user_id"]
			planIDStr, hasPlanID := checkoutSession.Metadata["subscription_plan_id"]

			if hasUserID && hasPlanID {
				planID, err := uuid.Parse(planIDStr)
				if err != nil {
					return "", uuid.Nil, fmt.Errorf("invalid plan ID in checkout session: %v", err)
				}
				return userID, planID, nil
			}
		}
	}

	return "", uuid.Nil, fmt.Errorf("no checkout session found with metadata")
}

// LinkSubscriptionToUser lie manuellement un abonnement Stripe à un utilisateur
func (ss *stripeService) LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID uuid.UUID) error {
	// Récupérer l'abonnement depuis Stripe
	sub, err := subscription.Get(stripeSubscriptionID, nil)
	if err != nil {
		return fmt.Errorf("failed to get subscription from Stripe: %v", err)
	}

	// Vérifier si l'abonnement existe déjà dans la base
	existingSubscription, err := ss.repository.GetUserSubscriptionByStripeID(stripeSubscriptionID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("database error checking existing subscription: %v", err)
	}

	if existingSubscription != nil {
		return fmt.Errorf("subscription already exists in database")
	}

	// Construire les dates de période
	var currentPeriodStart, currentPeriodEnd time.Time
	if len(sub.Items.Data) > 0 {
		item := sub.Items.Data[0]
		currentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		currentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
	}

	// Créer l'abonnement
	userSubscription := &models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   subscriptionPlanID,
		StripeSubscriptionID: stripeSubscriptionID,
		StripeCustomerID:     sub.Customer.ID,
		Status:               string(sub.Status),
		CurrentPeriodStart:   currentPeriodStart,
		CurrentPeriodEnd:     currentPeriodEnd,
		CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
	}

	if sub.TrialEnd > 0 {
		trialEnd := time.Unix(sub.TrialEnd, 0)
		userSubscription.TrialEnd = &trialEnd
	}

	// Sauvegarder en base
	return ss.repository.CreateUserSubscription(userSubscription)
}

// SyncUserInvoices synchronise toutes les factures d'un utilisateur depuis Stripe
func (ss *stripeService) SyncUserInvoices(userID string) (*SyncInvoicesResult, error) {
	result := &SyncInvoicesResult{
		FailedInvoices: []FailedInvoice{},
		CreatedDetails: []string{},
		UpdatedDetails: []string{},
		SkippedDetails: []string{},
	}

	// Récupérer la subscription active de l'utilisateur pour obtenir le customer ID
	userSub, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for user %s: %v", userID, err)
	}

	// Check if user has a Stripe customer ID (free plans don't have one)
	if userSub.StripeCustomerID == "" {
		utils.Debug("⚠️ User %s has a free subscription with no Stripe customer ID, skipping invoice sync", userID)
		return result, nil // Return empty result, no invoices to sync
	}

	// Récupérer toutes les factures depuis Stripe pour ce customer
	params := &stripe.InvoiceListParams{
		Customer: stripe.String(userSub.StripeCustomerID),
	}
	params.Filters.AddFilter("limit", "", "100")

	iter := invoice.List(params)
	for iter.Next() {
		inv := iter.Invoice()
		result.ProcessedInvoices++

		if err := ss.processSingleInvoice(inv, userID, result); err != nil {
			result.FailedInvoices = append(result.FailedInvoices, FailedInvoice{
				StripeInvoiceID: inv.ID,
				CustomerID:      inv.Customer.ID,
				Error:           err.Error(),
			})
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate invoices: %v", err)
	}

	utils.Debug("✅ Invoice sync complete: %d processed, %d created, %d updated, %d skipped, %d failed",
		result.ProcessedInvoices, result.CreatedInvoices, result.UpdatedInvoices,
		result.SkippedInvoices, len(result.FailedInvoices))

	return result, nil
}

// processSingleInvoice traite une facture Stripe individuelle
func (ss *stripeService) processSingleInvoice(inv *stripe.Invoice, userID string, result *SyncInvoicesResult) error {
	// Trouver la subscription associée par customer ID
	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(inv.Customer.ID)
	if err != nil {
		errMsg := fmt.Sprintf("no active subscription found for customer %s", inv.Customer.ID)
		result.SkippedInvoices++
		result.SkippedDetails = append(result.SkippedDetails,
			fmt.Sprintf("Invoice %s: %s", inv.ID, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Vérifier si la facture existe déjà dans notre base
	existingInvoice, err := ss.repository.GetInvoiceByStripeID(inv.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("database error checking existing invoice: %v", err)
	}

	// Convertir les timestamps
	invoiceDate := time.Unix(inv.Created, 0)
	dueDate := time.Unix(inv.DueDate, 0)
	var paidAt *time.Time
	if inv.StatusTransitions != nil && inv.StatusTransitions.PaidAt > 0 {
		t := time.Unix(inv.StatusTransitions.PaidAt, 0)
		paidAt = &t
	}

	if existingInvoice != nil {
		// Facture existe - mettre à jour
		existingInvoice.Status = string(inv.Status)
		existingInvoice.Amount = inv.Total
		existingInvoice.Currency = string(inv.Currency)
		existingInvoice.InvoiceNumber = inv.Number
		existingInvoice.InvoiceDate = invoiceDate
		existingInvoice.DueDate = dueDate
		existingInvoice.PaidAt = paidAt
		existingInvoice.StripeHostedURL = inv.HostedInvoiceURL
		existingInvoice.DownloadURL = inv.InvoicePDF

		if err := ss.repository.UpdateInvoice(existingInvoice); err != nil {
			return fmt.Errorf("failed to update invoice: %v", err)
		}

		result.UpdatedInvoices++
		result.UpdatedDetails = append(result.UpdatedDetails,
			fmt.Sprintf("Updated invoice %s (%s) - %d %s",
				inv.ID, inv.Number, inv.Total, inv.Currency))
		utils.Debug("✅ Updated invoice %s for user %s", inv.ID, userID)
	} else {
		// Créer nouvelle facture
		newInvoice := &models.Invoice{
			UserID:             userID,
			UserSubscriptionID: userSub.ID,
			StripeInvoiceID:    inv.ID,
			Amount:             inv.Total,
			Currency:           string(inv.Currency),
			Status:             string(inv.Status),
			InvoiceNumber:      inv.Number,
			InvoiceDate:        invoiceDate,
			DueDate:            dueDate,
			PaidAt:             paidAt,
			StripeHostedURL:    inv.HostedInvoiceURL,
			DownloadURL:        inv.InvoicePDF,
		}

		if err := ss.repository.CreateInvoice(newInvoice); err != nil {
			return fmt.Errorf("failed to create invoice: %v", err)
		}

		result.CreatedInvoices++
		result.CreatedDetails = append(result.CreatedDetails,
			fmt.Sprintf("Created invoice %s (%s) - %d %s",
				inv.ID, inv.Number, inv.Total, inv.Currency))
		utils.Debug("✅ Created invoice %s for user %s", inv.ID, userID)
	}

	return nil
}

// CleanupIncompleteInvoices voids or marks as uncollectible all incomplete invoices
func (ss *stripeService) CleanupIncompleteInvoices(input dto.CleanupInvoicesInput) (*dto.CleanupInvoicesResult, error) {
	// Validate input based on mode
	if len(input.InvoiceIDs) == 0 && input.OlderThanDays == nil {
		return nil, fmt.Errorf("older_than_days is required when invoice_ids is not provided")
	}

	result := &dto.CleanupInvoicesResult{
		DryRun:         input.DryRun,
		Action:         input.Action,
		CleanedDetails: []dto.CleanedInvoiceDetail{},
		SkippedDetails: []string{},
		FailedDetails:  []dto.FailedInvoiceCleanup{},
	}

	// Calculate cutoff date (only needed for non-selective cleanup)
	var olderThanDays int
	var cutoffDate time.Time
	if input.OlderThanDays != nil {
		olderThanDays = *input.OlderThanDays
		cutoffDate = time.Now().AddDate(0, 0, -olderThanDays)
	}

	// Build a map of allowed invoice IDs for quick lookup (if selective cleanup)
	var allowedInvoices map[string]bool
	if len(input.InvoiceIDs) > 0 {
		allowedInvoices = make(map[string]bool, len(input.InvoiceIDs))
		for _, id := range input.InvoiceIDs {
			allowedInvoices[id] = true
		}
		utils.Info("🧹 Cleanup incomplete invoices: action=%s, selective=%d invoices, dry_run=%v",
			input.Action, len(input.InvoiceIDs), input.DryRun)
	} else {
		utils.Info("🧹 Cleanup incomplete invoices: action=%s, older_than=%d days, cutoff=%s, dry_run=%v",
			input.Action, olderThanDays, cutoffDate.Format("2006-01-02"), input.DryRun)
	}

	// Determine which statuses to query
	statuses := []string{"open", "draft"}
	if input.Status != "" {
		statuses = []string{input.Status}
	}

	// Fetch invoices for each status
	for _, status := range statuses {
		params := &stripe.InvoiceListParams{}
		params.Filters.AddFilter("limit", "", "100")
		params.Filters.AddFilter("status", "", status)

		iter := invoice.List(params)
		for iter.Next() {
			inv := iter.Invoice()
			result.ProcessedInvoices++

			// Calculate invoice date (needed for logging)
			invoiceDate := time.Unix(inv.Created, 0)

			// If selective cleanup, check if this invoice is in the allowed list
			if allowedInvoices != nil && !allowedInvoices[inv.ID] {
				result.SkippedInvoices++
				result.SkippedDetails = append(result.SkippedDetails,
					fmt.Sprintf("Invoice %s not in selection", inv.ID))
				continue
			}

			// Check if invoice is older than cutoff (only if not selective)
			if allowedInvoices == nil {
				if invoiceDate.After(cutoffDate) {
					result.SkippedInvoices++
					result.SkippedDetails = append(result.SkippedDetails,
						fmt.Sprintf("Invoice %s too recent (created %s)", inv.ID, invoiceDate.Format("2006-01-02")))
					continue
				}
			}

			// Skip if already uncollectible and we're trying to mark it as uncollectible again
			if inv.Status == "uncollectible" && input.Action == "uncollectible" {
				result.SkippedInvoices++
				result.SkippedDetails = append(result.SkippedDetails,
					fmt.Sprintf("Invoice %s already uncollectible", inv.ID))
				continue
			}

			// Preview mode - just log what would be done
			if input.DryRun {
				result.CleanedInvoices++
				result.TotalAmountCleaned += inv.Total
				result.CleanedDetails = append(result.CleanedDetails, dto.CleanedInvoiceDetail{
					InvoiceID:     inv.ID,
					InvoiceNumber: inv.Number,
					CustomerID:    inv.Customer.ID,
					Amount:        inv.Total,
					Currency:      string(inv.Currency),
					Status:        string(inv.Status),
					Action:        func() string {
					if input.Action == "void" && inv.Status == "draft" {
						return "would_delete"
					}
					return fmt.Sprintf("would_%s", input.Action)
				}(),
					CreatedAt:     invoiceDate.Format("2006-01-02 15:04:05"),
				})
				if result.Currency == "" {
					result.Currency = string(inv.Currency)
				}
				continue
			}

			// Actually perform the action
			var err error
			var actionTaken string

			switch input.Action {
			case "void":
				// Draft invoices must be deleted, open invoices can be voided
				if inv.Status == "draft" {
					// Delete draft invoices (they can't be voided)
					_, err = invoice.Del(inv.ID, nil)
					actionTaken = "deleted"
				} else {
					// Void open invoices (cancels them permanently)
					_, err = invoice.VoidInvoice(inv.ID, nil)
					actionTaken = "voided"
				}

			case "uncollectible":
				// Mark as uncollectible (keeps record but stops collection)
				updateParams := &stripe.InvoiceMarkUncollectibleParams{}
				updateParams.AddMetadata("marked_uncollectible_at", time.Now().Format(time.RFC3339))
				updateParams.AddMetadata("marked_uncollectible_reason", "admin_cleanup")
				_, err = invoice.MarkUncollectible(inv.ID, updateParams)
				actionTaken = "marked_uncollectible"
			}

			if err != nil {
				result.FailedInvoices++
				result.FailedDetails = append(result.FailedDetails, dto.FailedInvoiceCleanup{
					InvoiceID:  inv.ID,
					CustomerID: inv.Customer.ID,
					Error:      err.Error(),
				})
				utils.Warn("❌ Failed to %s invoice %s: %v", input.Action, inv.ID, err)
				continue
			}

			// Success
			result.CleanedInvoices++
			result.TotalAmountCleaned += inv.Total
			result.CleanedDetails = append(result.CleanedDetails, dto.CleanedInvoiceDetail{
				InvoiceID:     inv.ID,
				InvoiceNumber: inv.Number,
				CustomerID:    inv.Customer.ID,
				Amount:        inv.Total,
				Currency:      string(inv.Currency),
				Status:        string(inv.Status),
				Action:        actionTaken,
				CreatedAt:     invoiceDate.Format("2006-01-02 15:04:05"),
			})

			if result.Currency == "" {
				result.Currency = string(inv.Currency)
			}

			utils.Info("✅ %s invoice %s (%s) - %d %s",
				actionTaken, inv.ID, inv.Number, inv.Total, inv.Currency)
		}

		if err := iter.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate invoices: %v", err)
		}
	}

	utils.Info("✅ Cleanup complete: %d processed, %d cleaned, %d skipped, %d failed (dry_run=%v)",
		result.ProcessedInvoices, result.CleanedInvoices, result.SkippedInvoices, result.FailedInvoices, input.DryRun)

	return result, nil
}

// SyncUserPaymentMethods synchronise les moyens de paiement d'un utilisateur depuis Stripe
func (ss *stripeService) SyncUserPaymentMethods(userID string) (*SyncPaymentMethodsResult, error) {
	result := &SyncPaymentMethodsResult{
		FailedPaymentMethods: []FailedPaymentMethod{},
		CreatedDetails:       []string{},
		UpdatedDetails:       []string{},
		SkippedDetails:       []string{},
	}

	// Récupérer la subscription active de l'utilisateur pour obtenir le customer ID
	userSub, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for user %s: %v", userID, err)
	}

	// Check if user has a Stripe customer ID (free plans don't have one)
	if userSub.StripeCustomerID == "" {
		utils.Debug("⚠️ User %s has a free subscription with no Stripe customer ID, skipping payment methods sync", userID)
		return result, nil // Return empty result, no payment methods to sync
	}

	// Récupérer tous les moyens de paiement depuis Stripe pour ce customer
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(userSub.StripeCustomerID),
		Type:     stripe.String("card"), // Focus on cards for now
	}
	params.Filters.AddFilter("limit", "", "100")

	iter := paymentmethod.List(params)
	for iter.Next() {
		pm := iter.PaymentMethod()
		result.ProcessedPaymentMethods++

		if err := ss.processSinglePaymentMethod(pm, userID, userSub.StripeCustomerID, result); err != nil {
			result.FailedPaymentMethods = append(result.FailedPaymentMethods, FailedPaymentMethod{
				StripePaymentMethodID: pm.ID,
				CustomerID:            userSub.StripeCustomerID,
				Error:                 err.Error(),
			})
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate payment methods: %v", err)
	}

	utils.Debug("✅ Payment method sync complete: %d processed, %d created, %d updated, %d skipped, %d failed",
		result.ProcessedPaymentMethods, result.CreatedPaymentMethods, result.UpdatedPaymentMethods,
		result.SkippedPaymentMethods, len(result.FailedPaymentMethods))

	return result, nil
}

// processSinglePaymentMethod traite un moyen de paiement Stripe individuel
func (ss *stripeService) processSinglePaymentMethod(pm *stripe.PaymentMethod, userID, customerID string, result *SyncPaymentMethodsResult) error {
	// Vérifier si le moyen de paiement existe déjà dans notre base
	existingPM, err := ss.repository.GetPaymentMethodByStripeID(pm.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("database error checking existing payment method: %v", err)
	}

	// Déterminer si c'est le moyen de paiement par défaut
	// Récupérer le customer pour vérifier le default payment method
	cust, err := customer.Get(customerID, nil)
	if err != nil {
		return fmt.Errorf("failed to get customer: %v", err)
	}

	isDefault := false
	if cust.InvoiceSettings != nil && cust.InvoiceSettings.DefaultPaymentMethod != nil {
		isDefault = cust.InvoiceSettings.DefaultPaymentMethod.ID == pm.ID
	}

	if existingPM != nil {
		// Moyen de paiement existe - mettre à jour les métadonnées minimales
		existingPM.Type = string(pm.Type)
		existingPM.IsDefault = isDefault
		existingPM.IsActive = true

		if pm.Card != nil {
			existingPM.CardBrand = string(pm.Card.Brand)
			existingPM.CardLast4 = pm.Card.Last4
			existingPM.CardExpMonth = int(pm.Card.ExpMonth)
			existingPM.CardExpYear = int(pm.Card.ExpYear)
		}

		if err := ss.repository.UpdatePaymentMethod(existingPM); err != nil {
			return fmt.Errorf("failed to update payment method: %v", err)
		}

		result.UpdatedPaymentMethods++
		result.UpdatedDetails = append(result.UpdatedDetails,
			fmt.Sprintf("Updated payment method %s (%s ****%s)",
				pm.ID, existingPM.CardBrand, existingPM.CardLast4))
		utils.Debug("✅ Updated payment method %s for user %s", pm.ID, userID)
	} else {
		// Créer nouveau moyen de paiement avec métadonnées minimales
		newPM := &models.PaymentMethod{
			UserID:                userID,
			StripePaymentMethodID: pm.ID,
			Type:                  string(pm.Type),
			IsDefault:             isDefault,
			IsActive:              true,
		}

		if pm.Card != nil {
			newPM.CardBrand = string(pm.Card.Brand)
			newPM.CardLast4 = pm.Card.Last4
			newPM.CardExpMonth = int(pm.Card.ExpMonth)
			newPM.CardExpYear = int(pm.Card.ExpYear)
		}

		if err := ss.repository.CreatePaymentMethod(newPM); err != nil {
			return fmt.Errorf("failed to create payment method: %v", err)
		}

		result.CreatedPaymentMethods++
		result.CreatedDetails = append(result.CreatedDetails,
			fmt.Sprintf("Created payment method %s (%s ****%s)",
				pm.ID, newPM.CardBrand, newPM.CardLast4))
		utils.Debug("✅ Created payment method %s for user %s", pm.ID, userID)
	}

	return nil
}

// CreateSubscriptionWithQuantity creates a Stripe subscription with a specified quantity for bulk purchases
func (ss *stripeService) CreateSubscriptionWithQuantity(customerID string, plan *models.SubscriptionPlan, quantity int, paymentMethodID string) (*stripe.Subscription, error) {
	if plan.StripePriceID == nil {
		return nil, fmt.Errorf("plan does not have a Stripe price ID configured")
	}

	params := &stripe.SubscriptionParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price:    stripe.String(*plan.StripePriceID),
				Quantity: stripe.Int64(int64(quantity)),
			},
		},
		PaymentBehavior: stripe.String("default_incomplete"),
		Metadata: map[string]string{
			"plan_id":       plan.ID.String(),
			"plan_name":     plan.Name,
			"quantity":      fmt.Sprintf("%d", quantity),
			"bulk_purchase": "true",
		},
	}

	if paymentMethodID != "" {
		params.DefaultPaymentMethod = stripe.String(paymentMethodID)
	}

	if plan.TrialDays > 0 {
		params.TrialPeriodDays = stripe.Int64(int64(plan.TrialDays))
	}

	sub, err := subscription.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe subscription: %v", err)
	}

	utils.Info("✅ Created bulk Stripe subscription %s for customer %s (quantity: %d)", sub.ID, customerID, quantity)
	return sub, nil
}

// UpdateSubscriptionQuantity updates the quantity of an existing Stripe subscription
func (ss *stripeService) UpdateSubscriptionQuantity(subscriptionID string, subscriptionItemID string, newQuantity int) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:       stripe.String(subscriptionItemID),
				Quantity: stripe.Int64(int64(newQuantity)),
			},
		},
		ProrationBehavior: stripe.String("always_invoice"),
	}

	sub, err := subscription.Update(subscriptionID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update Stripe subscription quantity: %v", err)
	}

	utils.Info("✅ Updated Stripe subscription %s quantity to %d", subscriptionID, newQuantity)
	return sub, nil
}

// ImportPlansFromStripe imports subscription plans from Stripe into the database
func (ss *stripeService) ImportPlansFromStripe() (*SyncPlansResult, error) {
	result := &SyncPlansResult{
		FailedPlans:    []FailedPlan{},
		CreatedDetails: []string{},
		UpdatedDetails: []string{},
		SkippedDetails: []string{},
	}

	// Fetch all products from Stripe
	productParams := &stripe.ProductListParams{}
	productParams.Filters.AddFilter("limit", "", "100")
	productParams.Filters.AddFilter("active", "", "true")

	productIter := product.List(productParams)
	for productIter.Next() {
		prod := productIter.Product()
		result.ProcessedPlans++

		// Get all prices for this product
		priceParams := &stripe.PriceListParams{
			Product: stripe.String(prod.ID),
		}
		priceParams.Filters.AddFilter("active", "", "true")
		priceParams.Filters.AddFilter("limit", "", "100")
		// CRITICAL: Expand tiers to get volume pricing data
		priceParams.AddExpand("data.tiers")

		priceIter := price.List(priceParams)
		for priceIter.Next() {
			priceObj := priceIter.Price()

			// Debug logging for the price structure
			utils.Debug("🔍 Processing price %s for product %s", priceObj.ID, prod.Name)
			utils.Debug("  - Tiers: %v (count: %d)", priceObj.Tiers != nil, len(priceObj.Tiers))
			utils.Debug("  - TiersMode: %s", priceObj.TiersMode)
			utils.Debug("  - BillingScheme: %s", priceObj.BillingScheme)
			if priceObj.Tiers != nil {
				for i, tier := range priceObj.Tiers {
					utils.Debug("    Tier %d: UpTo=%d, UnitAmount=%d, FlatAmount=%d",
						i, tier.UpTo, tier.UnitAmount, tier.FlatAmount)
				}
			}

			// Only process recurring prices (subscriptions)
			if priceObj.Recurring == nil {
				result.SkippedPlans++
				result.SkippedDetails = append(result.SkippedDetails,
					fmt.Sprintf("Price %s (%s): not a recurring price", priceObj.ID, prod.Name))
				continue
			}

			// Check if plan already exists by Stripe price ID
			existingPlan, err := ss.repository.GetSubscriptionPlanByStripePriceID(priceObj.ID)
			if err == nil && existingPlan != nil {
				// Plan exists - update it
				existingPlan.Name = prod.Name
				existingPlan.Description = prod.Description
				existingPlan.Currency = string(priceObj.Currency)
				existingPlan.BillingInterval = string(priceObj.Recurring.Interval)
				existingPlan.IsActive = prod.Active

				// Handle tiered pricing for updates
				if priceObj.Tiers != nil && len(priceObj.Tiers) > 0 {
					existingPlan.UseTieredPricing = true
					existingPlan.PricingTiers = make([]models.PricingTier, 0, len(priceObj.Tiers))

					previousUpTo := int64(0)
					for _, tier := range priceObj.Tiers {
						pricingTier := models.PricingTier{
							MinQuantity: int(previousUpTo + 1),
							UnitAmount:  tier.UnitAmount,
						}

						if tier.UpTo == 0 {
							pricingTier.MaxQuantity = 0 // unlimited
						} else {
							pricingTier.MaxQuantity = int(tier.UpTo)
							previousUpTo = tier.UpTo
						}

						existingPlan.PricingTiers = append(existingPlan.PricingTiers, pricingTier)
					}

					// Use first tier price for display
					if len(priceObj.Tiers) > 0 {
						existingPlan.PriceAmount = priceObj.Tiers[0].UnitAmount
					}
				} else {
					existingPlan.UseTieredPricing = false
					existingPlan.PriceAmount = priceObj.UnitAmount
					existingPlan.PricingTiers = []models.PricingTier{} // Clear any old tiers
				}

				// Update in database
				if err := ss.db.Save(existingPlan).Error; err != nil {
					result.FailedPlans = append(result.FailedPlans, FailedPlan{
						StripeProductID: prod.ID,
						StripePriceID:   priceObj.ID,
						Error:           fmt.Sprintf("failed to update plan: %v", err),
					})
					continue
				}

				// Debug: verify what was saved
				utils.Debug("✅ Saved to DB: UseTieredPricing=%v, TierCount=%d",
					existingPlan.UseTieredPricing, len(existingPlan.PricingTiers))

				result.UpdatedPlans++
				priceDisplay := fmt.Sprintf("%d %s/%s", existingPlan.PriceAmount, priceObj.Currency, priceObj.Recurring.Interval)
				if existingPlan.UseTieredPricing {
					priceDisplay = fmt.Sprintf("tiered (%d tiers)", len(existingPlan.PricingTiers))
				}
				result.UpdatedDetails = append(result.UpdatedDetails,
					fmt.Sprintf("Updated plan: %s (Stripe price: %s, pricing: %s)",
						prod.Name, priceObj.ID, priceDisplay))
				utils.Debug("✅ Updated plan %s from Stripe product %s", existingPlan.Name, prod.ID)
				continue
			}

			// Plan doesn't exist - create it
			newPlan := &models.SubscriptionPlan{
				Name:               prod.Name,
				Description:        prod.Description,
				StripeProductID:    &prod.ID,
				StripePriceID:      &priceObj.ID,
				Currency:           string(priceObj.Currency),
				BillingInterval:    string(priceObj.Recurring.Interval),
				IsActive:           prod.Active,
				StripeCreated:      true,
				MaxConcurrentUsers: 1, // Default value
				MaxCourses:         -1, // Default unlimited
				MaxLabSessions:     -1, // Default unlimited
			}

			// Handle tiered pricing (volume/graduated pricing in Stripe)
			if priceObj.Tiers != nil && len(priceObj.Tiers) > 0 {
				newPlan.UseTieredPricing = true
				newPlan.PricingTiers = make([]models.PricingTier, 0, len(priceObj.Tiers))

				previousUpTo := int64(0)
				for _, tier := range priceObj.Tiers {
					pricingTier := models.PricingTier{
						MinQuantity: int(previousUpTo + 1), // Start where previous tier ended
						UnitAmount:  tier.UnitAmount,        // Price per unit in this tier
					}

					// Stripe uses 0 (or null) for "unlimited" in the last tier
					if tier.UpTo == 0 {
						pricingTier.MaxQuantity = 0 // 0 = unlimited
					} else {
						pricingTier.MaxQuantity = int(tier.UpTo)
						previousUpTo = tier.UpTo
					}

					newPlan.PricingTiers = append(newPlan.PricingTiers, pricingTier)
				}

				// For display purposes, use the first tier's unit price
				if len(priceObj.Tiers) > 0 {
					newPlan.PriceAmount = priceObj.Tiers[0].UnitAmount
				}

				utils.Debug("📊 Imported tiered pricing: %d tiers for plan %s", len(priceObj.Tiers), prod.Name)
			} else {
				// Simple flat pricing
				newPlan.UseTieredPricing = false
				newPlan.PriceAmount = priceObj.UnitAmount
			}

			// Extract metadata if available
			if trialDaysStr, ok := prod.Metadata["trial_days"]; ok {
				var trialDays int
				if _, err := fmt.Sscanf(trialDaysStr, "%d", &trialDays); err == nil {
					newPlan.TrialDays = trialDays
				}
			}

			// Create in database
			if err := ss.db.Create(newPlan).Error; err != nil {
				result.FailedPlans = append(result.FailedPlans, FailedPlan{
					StripeProductID: prod.ID,
					StripePriceID:   priceObj.ID,
					Error:           fmt.Sprintf("failed to create plan: %v", err),
				})
				continue
			}

			result.CreatedPlans++
			result.CreatedDetails = append(result.CreatedDetails,
				fmt.Sprintf("Created plan: %s (Stripe price: %s, amount: %d %s/%s)",
					prod.Name, priceObj.ID, priceObj.UnitAmount, priceObj.Currency, priceObj.Recurring.Interval))
			utils.Info("✅ Imported new plan %s from Stripe product %s", newPlan.Name, prod.ID)
		}

		if err := priceIter.Err(); err != nil {
			result.FailedPlans = append(result.FailedPlans, FailedPlan{
				StripeProductID: prod.ID,
				Error:           fmt.Sprintf("failed to iterate prices: %v", err),
			})
		}
	}

	if err := productIter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate products: %v", err)
	}

	utils.Info("📥 Import from Stripe complete: %d processed, %d created, %d updated, %d skipped, %d failed",
		result.ProcessedPlans, result.CreatedPlans, result.UpdatedPlans, result.SkippedPlans, len(result.FailedPlans))

	return result, nil
}
