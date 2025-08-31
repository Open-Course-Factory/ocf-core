// src/payment/services/subscriptionService.go
package services

import (
	"fmt"
	"time"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionService interface {
	// Subscription management
	HasActiveSubscription(userID string) (bool, error)
	GetActiveUserSubscription(userID string) (*dto.UserSubscriptionOutput, error)
	GetUserSubscriptionByID(id uuid.UUID) (*dto.UserSubscriptionOutput, error)
	CreateUserSubscription(userID string, planID uuid.UUID) (*dto.UserSubscriptionOutput, error)

	// Usage limits and metrics
	CheckUsageLimit(userID, metricType string, increment int64) (*dto.UsageLimitCheckOutput, error)
	IncrementUsage(userID, metricType string, increment int64) error
	GetUserUsageMetrics(userID string) (*[]dto.UsageMetricsOutput, error)
	ResetMonthlyUsage(userID string) error

	// Payment methods
	GetUserPaymentMethods(userID string) (*[]dto.PaymentMethodOutput, error)
	SetDefaultPaymentMethod(userID string, paymentMethodID uuid.UUID) error

	// Invoices
	GetUserInvoices(userID string) (*[]dto.InvoiceOutput, error)
	GetInvoiceByID(id uuid.UUID) (*dto.InvoiceOutput, error)

	// Analytics (admin only)
	GetSubscriptionAnalytics() (*dto.SubscriptionAnalyticsOutput, error)

	// Role management integration
	UpdateUserRoleBasedOnSubscription(userID string) error
	GetRequiredRoleForPlan(planID uuid.UUID) (string, error)

	// Billing addresses
	GetUserBillingAddresses(userID string) (*[]dto.BillingAddressOutput, error)
	SetDefaultBillingAddress(userID string, addressID uuid.UUID) error
}

type subscriptionService struct {
	repository repositories.PaymentRepository
	db         *gorm.DB
}

func NewSubscriptionService(db *gorm.DB) SubscriptionService {
	return &subscriptionService{
		repository: repositories.NewPaymentRepository(db),
		db:         db,
	}
}

// HasActiveSubscription vérifie si un utilisateur a un abonnement actif
func (ss *subscriptionService) HasActiveSubscription(userID string) (bool, error) {
	_, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetActiveUserSubscription récupère l'abonnement actif d'un utilisateur
func (ss *subscriptionService) GetActiveUserSubscription(userID string) (*dto.UserSubscriptionOutput, error) {
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		return nil, err
	}

	// Convertir vers DTO en utilisant la registration
	output, err := userSubscriptionPtrModelToOutput(subscription)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// GetUserSubscriptionByID récupère un abonnement par son ID
func (ss *subscriptionService) GetUserSubscriptionByID(id uuid.UUID) (*dto.UserSubscriptionOutput, error) {
	subscription, err := ss.repository.GetUserSubscription(id)
	if err != nil {
		return nil, err
	}

	output, err := userSubscriptionPtrModelToOutput(subscription)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// CreateUserSubscription crée un nouvel abonnement (utilisé par les webhooks)
func (ss *subscriptionService) CreateUserSubscription(userID string, planID uuid.UUID) (*dto.UserSubscriptionOutput, error) {
	subscription := &models.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: planID,
		Status:             "incomplete",
	}

	err := ss.repository.CreateUserSubscription(subscription)
	if err != nil {
		return nil, err
	}

	return ss.GetUserSubscriptionByID(subscription.ID)
}

// CheckUsageLimit vérifie si une action est autorisée selon les limites d'abonnement
func (ss *subscriptionService) CheckUsageLimit(userID, metricType string, increment int64) (*dto.UsageLimitCheckOutput, error) {
	// Récupérer l'abonnement actif
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		// Pas d'abonnement = utilisateur gratuit avec des limites très restrictives
		return &dto.UsageLimitCheckOutput{
			Allowed:        false,
			CurrentUsage:   0,
			Limit:          0,
			RemainingUsage: 0,
			Message:        "No active subscription - upgrade required",
		}, nil
	}

	// Récupérer les métriques actuelles
	metrics, err := ss.repository.GetUserUsageMetrics(userID, metricType)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Première utilisation, créer les métriques
			var limit int64
			switch metricType {
			case "courses_created":
				limit = int64(subscription.SubscriptionPlan.MaxCourses)
			case "lab_sessions":
				limit = int64(subscription.SubscriptionPlan.MaxLabSessions)
			case "concurrent_users":
				limit = int64(subscription.SubscriptionPlan.MaxConcurrentUsers)
			default:
				limit = -1 // Illimité
			}

			return &dto.UsageLimitCheckOutput{
				Allowed:        limit == -1 || increment <= limit,
				CurrentUsage:   0,
				Limit:          limit,
				RemainingUsage: limit,
				Message:        "",
			}, nil
		}
		return nil, err
	}

	// Calculer si l'action est autorisée
	newUsage := metrics.CurrentValue + increment
	allowed := metrics.LimitValue == -1 || newUsage <= metrics.LimitValue

	var remaining int64
	if metrics.LimitValue == -1 {
		remaining = -1 // Illimité
	} else {
		remaining = metrics.LimitValue - metrics.CurrentValue
		if remaining < 0 {
			remaining = 0
		}
	}

	message := ""
	if !allowed {
		message = fmt.Sprintf("Usage limit exceeded. Current: %d, Limit: %d", metrics.CurrentValue, metrics.LimitValue)
	}

	return &dto.UsageLimitCheckOutput{
		Allowed:        allowed,
		CurrentUsage:   metrics.CurrentValue,
		Limit:          metrics.LimitValue,
		RemainingUsage: remaining,
		Message:        message,
	}, nil
}

// IncrementUsage incrémente l'utilisation d'une métrique
func (ss *subscriptionService) IncrementUsage(userID, metricType string, increment int64) error {
	return ss.repository.IncrementUsageMetric(userID, metricType, increment)
}

// GetUserUsageMetrics récupère toutes les métriques d'utilisation d'un utilisateur
func (ss *subscriptionService) GetUserUsageMetrics(userID string) (*[]dto.UsageMetricsOutput, error) {
	metrics, err := ss.repository.GetAllUserUsageMetrics(userID)
	if err != nil {
		return nil, err
	}

	var outputs []dto.UsageMetricsOutput
	for _, metric := range *metrics {
		output, err := usageMetricsPtrModelToOutput(&metric)
		if err != nil {
			continue // Skip en cas d'erreur de conversion
		}
		outputs = append(outputs, *output)
	}

	return &outputs, nil
}

// ResetMonthlyUsage remet à zéro les métriques mensuelles
func (ss *subscriptionService) ResetMonthlyUsage(userID string) error {
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	return ss.repository.ResetUsageMetrics(userID, periodStart, periodEnd)
}

// GetUserPaymentMethods récupère les moyens de paiement d'un utilisateur
func (ss *subscriptionService) GetUserPaymentMethods(userID string) (*[]dto.PaymentMethodOutput, error) {
	paymentMethods, err := ss.repository.GetUserPaymentMethods(userID, true)
	if err != nil {
		return nil, err
	}

	var outputs []dto.PaymentMethodOutput
	for _, pm := range *paymentMethods {
		output, err := paymentMethodPtrModelToOutput(&pm)
		if err != nil {
			continue
		}
		outputs = append(outputs, *output)
	}

	return &outputs, nil
}

// SetDefaultPaymentMethod définit le moyen de paiement par défaut
func (ss *subscriptionService) SetDefaultPaymentMethod(userID string, paymentMethodID uuid.UUID) error {
	return ss.repository.SetDefaultPaymentMethod(userID, paymentMethodID)
}

// GetUserInvoices récupère les factures d'un utilisateur
func (ss *subscriptionService) GetUserInvoices(userID string) (*[]dto.InvoiceOutput, error) {
	invoices, err := ss.repository.GetUserInvoices(userID, 50) // Limite à 50 factures
	if err != nil {
		return nil, err
	}

	var outputs []dto.InvoiceOutput
	for _, invoice := range *invoices {
		output, err := invoicePtrModelToOutput(&invoice)
		if err != nil {
			continue
		}
		outputs = append(outputs, *output)
	}

	return &outputs, nil
}

// GetInvoiceByID récupère une facture par son ID
func (ss *subscriptionService) GetInvoiceByID(id uuid.UUID) (*dto.InvoiceOutput, error) {
	invoice, err := ss.repository.GetInvoice(id)
	if err != nil {
		return nil, err
	}

	return invoicePtrModelToOutput(invoice)
}

// GetSubscriptionAnalytics récupère les analytics des abonnements (admin seulement)
func (ss *subscriptionService) GetSubscriptionAnalytics() (*dto.SubscriptionAnalyticsOutput, error) {
	startDate := time.Now().AddDate(0, -12, 0) // 12 mois en arrière
	endDate := time.Now()

	analytics, err := ss.repository.GetSubscriptionAnalytics(startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Calculer le MRR (Monthly Recurring Revenue)
	var mrr int64
	plans, err := ss.repository.GetAllSubscriptionPlans(true)
	if err == nil {
		for _, plan := range *plans {
			// Compter les abonnements actifs pour ce plan
			if count, exists := analytics.ByPlan[plan.Name]; exists {
				monthlyAmount := plan.PriceAmount
				if plan.BillingInterval == "year" {
					monthlyAmount = monthlyAmount / 12
				}
				mrr += monthlyAmount * int64(count)
			}
		}
	}

	// Calculer le taux de churn (approximation simple)
	var churnRate float64
	if analytics.TotalSubscriptions > 0 {
		churnRate = float64(analytics.CancelledSubscriptions) / float64(analytics.TotalSubscriptions) * 100
	}

	return &dto.SubscriptionAnalyticsOutput{
		TotalSubscriptions:      int(analytics.TotalSubscriptions),
		ActiveSubscriptions:     int(analytics.ActiveSubscriptions),
		CancelledSubscriptions:  int(analytics.CancelledSubscriptions),
		TrialSubscriptions:      int(analytics.TrialSubscriptions),
		Revenue:                 analytics.Revenue,
		MonthlyRecurringRevenue: mrr,
		ChurnRate:               churnRate,
		ByPlan:                  analytics.ByPlan,
		GeneratedAt:             analytics.GeneratedAt,
	}, nil
}

// UpdateUserRoleBasedOnSubscription met à jour le rôle de l'utilisateur selon son abonnement
func (ss *subscriptionService) UpdateUserRoleBasedOnSubscription(userID string) error {
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		// Pas d'abonnement actif, garder le rôle de base
		return nil
	}

	requiredRole := subscription.SubscriptionPlan.RequiredRole
	if requiredRole == "" {
		return nil // Pas de rôle spécifique requis
	}

	// Ici vous devrez intégrer avec Casdoor pour mettre à jour le rôle
	// Exemple d'implémentation :
	/*
		import "soli/formations/src/auth/casdoor"

		// Supprimer les anciens rôles liés aux abonnements
		casdoor.Enforcer.RemoveGroupingPolicy(userID, "student_premium")
		casdoor.Enforcer.RemoveGroupingPolicy(userID, "supervisor_pro")
		casdoor.Enforcer.RemoveGroupingPolicy(userID, "organization")

		// Ajouter le nouveau rôle
		_, err = casdoor.Enforcer.AddGroupingPolicy(userID, requiredRole)
		if err != nil {
			return fmt.Errorf("failed to update user role: %v", err)
		}
	*/

	fmt.Printf("Should update user %s to role %s\n", userID, requiredRole)
	return nil
}

// GetRequiredRoleForPlan récupère le rôle requis pour un plan
func (ss *subscriptionService) GetRequiredRoleForPlan(planID uuid.UUID) (string, error) {
	plan, err := ss.repository.GetSubscriptionPlan(planID)
	if err != nil {
		return "", err
	}
	return plan.RequiredRole, nil
}

// GetUserBillingAddresses récupère les adresses de facturation
func (ss *subscriptionService) GetUserBillingAddresses(userID string) (*[]dto.BillingAddressOutput, error) {
	addresses, err := ss.repository.GetUserBillingAddresses(userID)
	if err != nil {
		return nil, err
	}

	var outputs []dto.BillingAddressOutput
	for _, address := range *addresses {
		output, err := billingAddressPtrModelToOutput(&address)
		if err != nil {
			continue
		}
		outputs = append(outputs, *output)
	}

	return &outputs, nil
}

// SetDefaultBillingAddress définit l'adresse de facturation par défaut
func (ss *subscriptionService) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	return ss.repository.SetDefaultBillingAddress(userID, addressID)
}

// Fonctions utilitaires pour les conversions (réutilisées depuis les registrations)
func userSubscriptionPtrModelToOutput(subscription *models.UserSubscription) (*dto.UserSubscriptionOutput, error) {
	planOutput, err := subscriptionPlanPtrModelToOutput(&subscription.SubscriptionPlan)
	if err != nil {
		return nil, err
	}

	return &dto.UserSubscriptionOutput{
		ID:                   subscription.ID,
		UserID:               subscription.UserID,
		SubscriptionPlan:     *planOutput,
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
		TrialEnd:             subscription.TrialEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		CancelledAt:          subscription.CancelledAt,
		CreatedAt:            subscription.CreatedAt,
		UpdatedAt:            subscription.UpdatedAt,
	}, nil
}

func subscriptionPlanPtrModelToOutput(plan *models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error) {
	return &dto.SubscriptionPlanOutput{
		ID:                 plan.ID,
		Name:               plan.Name,
		Description:        plan.Description,
		StripeProductID:    plan.StripeProductID,
		StripePriceID:      plan.StripePriceID,
		PriceAmount:        plan.PriceAmount,
		Currency:           plan.Currency,
		BillingInterval:    plan.BillingInterval,
		TrialDays:          plan.TrialDays,
		Features:           plan.Features,
		MaxConcurrentUsers: plan.MaxConcurrentUsers,
		MaxCourses:         plan.MaxCourses,
		MaxLabSessions:     plan.MaxLabSessions,
		IsActive:           plan.IsActive,
		RequiredRole:       plan.RequiredRole,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,
	}, nil
}

func paymentMethodPtrModelToOutput(pm *models.PaymentMethod) (*dto.PaymentMethodOutput, error) {
	return &dto.PaymentMethodOutput{
		ID:                    pm.ID,
		UserID:                pm.UserID,
		StripePaymentMethodID: pm.StripePaymentMethodID,
		Type:                  pm.Type,
		CardBrand:             pm.CardBrand,
		CardLast4:             pm.CardLast4,
		CardExpMonth:          pm.CardExpMonth,
		CardExpYear:           pm.CardExpYear,
		IsDefault:             pm.IsDefault,
		IsActive:              pm.IsActive,
		CreatedAt:             pm.CreatedAt,
	}, nil
}

func invoicePtrModelToOutput(invoice *models.Invoice) (*dto.InvoiceOutput, error) {
	subscriptionOutput, err := userSubscriptionPtrModelToOutput(&invoice.UserSubscription)
	if err != nil {
		return nil, err
	}

	return &dto.InvoiceOutput{
		ID:               invoice.ID,
		UserID:           invoice.UserID,
		UserSubscription: *subscriptionOutput,
		StripeInvoiceID:  invoice.StripeInvoiceID,
		Amount:           invoice.Amount,
		Currency:         invoice.Currency,
		Status:           invoice.Status,
		InvoiceNumber:    invoice.InvoiceNumber,
		InvoiceDate:      invoice.InvoiceDate,
		DueDate:          invoice.DueDate,
		PaidAt:           invoice.PaidAt,
		StripeHostedURL:  invoice.StripeHostedURL,
		DownloadURL:      invoice.DownloadURL,
		CreatedAt:        invoice.CreatedAt,
	}, nil
}

func usageMetricsPtrModelToOutput(metrics *models.UsageMetrics) (*dto.UsageMetricsOutput, error) {
	var usagePercent float64
	if metrics.LimitValue > 0 {
		usagePercent = (float64(metrics.CurrentValue) / float64(metrics.LimitValue)) * 100
	} else {
		usagePercent = 0 // Unlimited
	}

	return &dto.UsageMetricsOutput{
		ID:           metrics.ID,
		UserID:       metrics.UserID,
		MetricType:   metrics.MetricType,
		CurrentValue: metrics.CurrentValue,
		LimitValue:   metrics.LimitValue,
		PeriodStart:  metrics.PeriodStart,
		PeriodEnd:    metrics.PeriodEnd,
		LastUpdated:  metrics.LastUpdated,
		UsagePercent: usagePercent,
	}, nil
}

func billingAddressPtrModelToOutput(address *models.BillingAddress) (*dto.BillingAddressOutput, error) {
	return &dto.BillingAddressOutput{
		ID:         address.ID,
		UserID:     address.UserID,
		Line1:      address.Line1,
		Line2:      address.Line2,
		City:       address.City,
		State:      address.State,
		PostalCode: address.PostalCode,
		Country:    address.Country,
		IsDefault:  address.IsDefault,
		CreatedAt:  address.CreatedAt,
		UpdatedAt:  address.UpdatedAt,
	}, nil
}
