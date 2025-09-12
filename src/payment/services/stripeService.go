// src/payment/services/stripeService.go
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"

	genericService "soli/formations/src/entityManagement/services"

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
	"gorm.io/gorm"
)

type StripeService interface {
	// Customer management
	CreateOrGetCustomer(userID, email, name string) (string, error)
	UpdateCustomer(customerID string, params *stripe.CustomerParams) error

	// Subscription management
	CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput) (*dto.CheckoutSessionOutput, error)
	CreatePortalSession(userID string, input dto.CreatePortalSessionInput) (*dto.PortalSessionOutput, error)

	// Product & Price management
	CreateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error
	UpdateSubscriptionPlanInStripe(plan *models.SubscriptionPlan) error

	// Webhook handling
	ProcessWebhook(payload []byte, signature string) error

	// Subscription operations
	CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error
	ReactivateSubscription(subscriptionID string) error

	// Payment method operations
	AttachPaymentMethod(paymentMethodID, customerID string) error
	DetachPaymentMethod(paymentMethodID string) error
	SetDefaultPaymentMethod(customerID, paymentMethodID string) error

	// Invoice operations
	GetInvoice(invoiceID string) (*stripe.Invoice, error)
	SendInvoice(invoiceID string) error

	// Utilities
	ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error)
}

type stripeService struct {
	subscriptionService SubscriptionService
	genericService      genericService.GenericService
	repository          repositories.PaymentRepository
	webhookSecret       string
}

func NewStripeService(db *gorm.DB) StripeService {
	// Initialiser Stripe
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	return &stripeService{
		subscriptionService: NewSubscriptionService(db),
		repository:          repositories.NewPaymentRepository(db),
		webhookSecret:       os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
}

// CreateOrGetCustomer crée ou récupère un client Stripe
func (ss *stripeService) CreateOrGetCustomer(userID, email, name string) (string, error) {
	// Vérifier si le client existe déjà en base
	subscription, err := ss.subscriptionService.GetActiveUserSubscription(userID)
	if err == nil && subscription.StripeCustomerID != "" {
		return subscription.StripeCustomerID, nil
	}

	// Créer un nouveau client Stripe
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

	return customer.ID, nil
}

// UpdateCustomer met à jour un client Stripe
func (ss *stripeService) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	_, err := customer.Update(customerID, params)
	return err
}

// CreateCheckoutSession crée une session de checkout Stripe
func (ss *stripeService) CreateCheckoutSession(userID string, input dto.CreateCheckoutSessionInput) (*dto.CheckoutSessionOutput, error) {
	// Récupérer le plan d'abonnement
	plan, err := ss.subscriptionService.GetSubscriptionPlan(input.SubscriptionPlanID)
	if err != nil {
		return nil, fmt.Errorf("subscription plan not found: %v", err)
	}

	if !plan.IsActive {
		return nil, fmt.Errorf("subscription plan is not active")
	}

	// Récupérer les infos utilisateur (via Casdoor)
	// Note: Vous devrez adapter cette partie selon votre système d'auth
	customerID, err := ss.CreateOrGetCustomer(userID, "user@example.com", "User Name")
	if err != nil {
		return nil, err
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
		Metadata: map[string]string{
			"user_id":              userID,
			"subscription_plan_id": input.SubscriptionPlanID.String(),
		},
		BillingAddressCollection: stripe.String("required"),
		TaxIDCollection: &stripe.CheckoutSessionTaxIDCollectionParams{
			Enabled: stripe.Bool(true),
		},
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

	portalSession, errPortalSession := billingPortalSession.New(params)

	if errPortalSession != nil {
		return nil, err
	}

	return &dto.PortalSessionOutput{
		URL: portalSession.ReturnURL,
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

	switch event.Type {
	case "customer.subscription.created":
		return ss.handleSubscriptionCreated(event)
	case "customer.subscription.updated":
		return ss.handleSubscriptionUpdated(event)
	case "customer.subscription.deleted":
		return ss.handleSubscriptionDeleted(event)
	case "invoice.payment_succeeded":
		return ss.handleInvoicePaymentSucceeded(event)
	case "invoice.payment_failed":
		return ss.handleInvoicePaymentFailed(event)
	case "checkout.session.completed":
		return ss.handleCheckoutSessionCompleted(event)
	default:
		// Événement non géré, mais pas une erreur
		fmt.Printf("Unhandled webhook event type: %s\n", event.Type)
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

	planIDStr, exists := subscription.Metadata["subscription_plan_id"]
	if !exists {
		return fmt.Errorf("subscription_plan_id not found in metadata")
	}

	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		return fmt.Errorf("invalid subscription_plan_id format: %v", err)
	}

	userSubscription := &models.UserSubscription{
		UserID:               userID,
		SubscriptionPlanID:   planID,
		StripeSubscriptionID: subscription.ID,
		StripeCustomerID:     subscription.Customer.ID,
		Status:               string(subscription.Status),
		CurrentPeriodStart:   time.Unix(subscription.Items.Data[0].CurrentPeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(subscription.Items.Data[0].CurrentPeriodEnd, 0),
	}

	if subscription.TrialEnd > 0 {
		trialEnd := time.Unix(subscription.TrialEnd, 0)
		userSubscription.TrialEnd = &trialEnd
	}

	return ss.repository.CreateUserSubscription(userSubscription)
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

	// Récupérer l'abonnement associé
	userSub, err := ss.repository.GetUserSubscriptionByStripeID(stripeInvoice.ID)
	if err != nil {
		return fmt.Errorf("subscription not found for invoice: %v", err)
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
		return ss.repository.CreateInvoice(invoiceRecord)
	} else {
		// Mettre à jour la facture existante
		existingInvoice.Status = invoiceRecord.Status
		existingInvoice.PaidAt = invoiceRecord.PaidAt
		existingInvoice.DownloadURL = invoiceRecord.DownloadURL
		return ss.repository.UpdateInvoice(existingInvoice)
	}
}

// handleInvoicePaymentFailed traite l'échec de paiement d'une facture
func (ss *stripeService) handleInvoicePaymentFailed(event *stripe.Event) error {
	var stripeInvoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &stripeInvoice); err != nil {
		return err
	}

	// Mettre à jour le statut de l'abonnement
	userSub, err := ss.repository.GetUserSubscriptionByStripeID(stripeInvoice.ID)
	if err != nil {
		return err
	}

	userSub.Status = "past_due"
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

	// Si c'est un abonnement, il sera créé via le webhook subscription.created
	// Ici on peut juste logger ou mettre à jour des métriques
	fmt.Printf("Checkout completed for user %s, subscription: %s\n", userID, session.Subscription.ID)

	return nil
}

// CancelSubscription annule un abonnement
func (ss *stripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(cancelAtPeriodEnd),
	}

	if !cancelAtPeriodEnd {
		params.CancellationDetails = &stripe.SubscriptionCancellationDetailsParams{
			Comment: stripe.String("Cancelled by user"),
		}
	}

	_, err := subscription.Update(subscriptionID, params)
	return err
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
	event, err := stripe.ConstructEvent(payload, signature, ss.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("webhook signature verification failed: %v", err)
	}
	return &event, nil
}
