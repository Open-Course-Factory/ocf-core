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

	// Payment method synchronization
	SyncUserPaymentMethods(userID string) (*SyncPaymentMethodsResult, error)

	// Payment method operations
	AttachPaymentMethod(paymentMethodID, customerID string) error
	DetachPaymentMethod(paymentMethodID string) error
	SetDefaultPaymentMethod(customerID, paymentMethodID string) error

	// Invoice operations
	GetInvoice(invoiceID string) (*stripe.Invoice, error)
	SendInvoice(invoiceID string) error
}

type stripeService struct {
	subscriptionService UserSubscriptionService
	genericService      genericService.GenericService
	repository          repositories.PaymentRepository
	webhookSecret       string
}

func NewStripeService(db *gorm.DB) StripeService {
	// Initialiser Stripe
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	return &stripeService{
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

	userSub, err := ss.repository.GetUserSubscriptionByStripeID(subscription.ID)
	if err != nil {
		return err
	}

	userSub.Status = "cancelled"
	now := time.Now()
	userSub.CancelledAt = &now

	return ss.repository.UpdateUserSubscription(userSub)
}

// handleInvoicePaymentSucceeded traite le paiement réussi d'une facture
func (ss *stripeService) handleInvoicePaymentSucceeded(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return err
	}

	// CRITICAL FIX: Find subscription by customer ID, not invoice ID
	// Invoices created during checkout might not have subscription field populated yet
	userSub, err := ss.repository.GetActiveSubscriptionByCustomerID(stripeInvoice.Customer.ID)
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
func (ss *stripeService) MarkSubscriptionAsCancelled(userSubscription *models.UserSubscription) error {
	userSubscription.Status = "cancelled"
	now := time.Now()
	userSubscription.CancelledAt = &now
	return ss.repository.UpdateUserSubscription(userSubscription)
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
