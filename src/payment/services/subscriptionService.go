// src/payment/services/subscriptionService.go
package services

import (
	"soli/formations/src/auth/casdoor"
	"fmt"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"

	genericService "soli/formations/src/entityManagement/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionService interface {
	// Subscription management - retourne des models
	HasActiveSubscription(userID string) (bool, error)
	GetActiveUserSubscription(userID string) (*models.UserSubscription, error)
	GetUserSubscriptionByID(id uuid.UUID) (*models.UserSubscription, error)
	CreateUserSubscription(userID string, planID uuid.UUID) (*models.UserSubscription, error)
	UpgradeUserPlan(userID string, newPlanID uuid.UUID, prorationBehavior string) (*models.UserSubscription, error)

	// Usage limits and metrics - types métiers
	CheckUsageLimit(userID, metricType string, increment int64) (*UsageLimitCheck, error)
	IncrementUsage(userID, metricType string, increment int64) error
	GetUserUsageMetrics(userID string) (*[]models.UsageMetrics, error)
	ResetMonthlyUsage(userID string) error
	UpdateUsageMetricLimits(userID string, newPlanID uuid.UUID) error
	InitializeUsageMetrics(userID string, subscriptionID uuid.UUID, planID uuid.UUID) error

	// Payment methods - retourne des models
	GetUserPaymentMethods(userID string) (*[]models.PaymentMethod, error)
	SetDefaultPaymentMethod(userID string, paymentMethodID uuid.UUID) error

	// Invoices - retourne des models
	GetUserInvoices(userID string) (*[]models.Invoice, error)
	GetInvoiceByID(id uuid.UUID) (*models.Invoice, error)

	// Analytics (admin only) - type métier
	GetSubscriptionAnalytics() (*SubscriptionAnalytics, error)

	// Role management integration
	UpdateUserRoleBasedOnSubscription(userID string) error
	GetRequiredRoleForPlan(planID uuid.UUID) (string, error)

	// Billing addresses - retourne des models
	GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error)
	SetDefaultBillingAddress(userID string, addressID uuid.UUID) error

	// Plans
	GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error)
	GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error)
}

// Types métiers pour les opérations complexes
type UsageLimitCheck struct {
	Allowed        bool
	CurrentUsage   int64
	Limit          int64
	RemainingUsage int64
	Message        string
	UserID         string
	MetricType     string
}

type SubscriptionAnalytics struct {
	TotalSubscriptions      int64
	ActiveSubscriptions     int64
	CancelledSubscriptions  int64
	TrialSubscriptions      int64
	Revenue                 int64
	MonthlyRecurringRevenue int64
	ChurnRate               float64
	ByPlan                  map[string]int
	RecentSignups           []models.UserSubscription
	RecentCancellations     []models.UserSubscription
	GeneratedAt             time.Time
}

type subscriptionService struct {
	repository     repositories.PaymentRepository
	db             *gorm.DB
	genericService genericService.GenericService
}

func NewSubscriptionService(db *gorm.DB) SubscriptionService {
	return &subscriptionService{
		genericService: genericService.NewGenericService(db, casdoor.Enforcer),
		repository:     repositories.NewPaymentRepository(db),
		db:             db,
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
func (ss *subscriptionService) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	return ss.repository.GetActiveUserSubscription(userID)
}

// GetUserSubscriptionByID récupère un abonnement par son ID
func (ss *subscriptionService) GetUserSubscriptionByID(id uuid.UUID) (*models.UserSubscription, error) {
	return ss.repository.GetUserSubscription(id)
}

// CreateUserSubscription crée un nouvel abonnement
func (ss *subscriptionService) CreateUserSubscription(userID string, planID uuid.UUID) (*models.UserSubscription, error) {

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
func (ss *subscriptionService) CheckUsageLimit(userID, metricType string, increment int64) (*UsageLimitCheck, error) {
	// Récupérer l'abonnement actif
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		// Pas d'abonnement = utilisateur gratuit avec des limites très restrictives
		return &UsageLimitCheck{
			Allowed:        false,
			CurrentUsage:   0,
			Limit:          0,
			RemainingUsage: 0,
			Message:        "No active subscription - upgrade required",
			UserID:         userID,
			MetricType:     metricType,
		}, nil
	}

	sPlan := subscription.SubscriptionPlan

	// Récupérer les métriques actuelles
	metrics, err := ss.repository.GetUserUsageMetrics(userID, metricType)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Première utilisation, créer les métriques
			var limit int64
			switch metricType {
			case "courses_created":
				limit = int64(sPlan.MaxCourses)
			case "lab_sessions":
				limit = int64(sPlan.MaxLabSessions)
			case "concurrent_users":
				limit = int64(sPlan.MaxConcurrentUsers)
			case "concurrent_terminals":
				limit = int64(sPlan.MaxConcurrentTerminals)
			default:
				limit = -1 // Illimité
			}

			// Pour concurrent_terminals, vérifier le compte réel depuis la DB
			var currentUsage int64 = 0
			if metricType == "concurrent_terminals" {
				var activeCount int64
				countErr := ss.db.Table("terminals").
					Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active").
					Count(&activeCount).Error
				if countErr == nil {
					currentUsage = activeCount
				}
			}

			return &UsageLimitCheck{
				Allowed:        limit == -1 || (currentUsage+increment) <= limit,
				CurrentUsage:   currentUsage,
				Limit:          limit,
				RemainingUsage: limit - currentUsage,
				Message:        "",
				UserID:         userID,
				MetricType:     metricType,
			}, nil
		}
		return nil, err
	}

	// Pour concurrent_terminals, recalculer la valeur en temps réel
	currentValue := metrics.CurrentValue
	if metricType == "concurrent_terminals" {
		var activeCount int64
		err := ss.db.Table("terminals").
			Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active").
			Count(&activeCount).Error
		if err == nil {
			currentValue = activeCount
		}
	}

	// Calculer si l'action est autorisée
	newUsage := currentValue + increment
	allowed := metrics.LimitValue == -1 || newUsage <= metrics.LimitValue

	var remaining int64
	if metrics.LimitValue == -1 {
		remaining = -1 // Illimité
	} else {
		remaining = metrics.LimitValue - currentValue
		if remaining < 0 {
			remaining = 0
		}
	}

	message := ""
	if !allowed {
		message = fmt.Sprintf("Usage limit exceeded. Current: %d, Limit: %d", currentValue, metrics.LimitValue)
	}

	return &UsageLimitCheck{
		Allowed:        allowed,
		CurrentUsage:   currentValue,
		Limit:          metrics.LimitValue,
		RemainingUsage: remaining,
		Message:        message,
		UserID:         userID,
		MetricType:     metricType,
	}, nil
}

// IncrementUsage incrémente l'utilisation d'une métrique
func (ss *subscriptionService) IncrementUsage(userID, metricType string, increment int64) error {
	return ss.repository.IncrementUsageMetric(userID, metricType, increment)
}

// GetUserUsageMetrics récupère toutes les métriques d'utilisation d'un utilisateur
// Pour concurrent_terminals, recalcule la valeur en temps réel depuis les terminaux actifs
func (ss *subscriptionService) GetUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	metrics, err := ss.repository.GetAllUserUsageMetrics(userID)
	if err != nil {
		return nil, err
	}

	// Recalculer concurrent_terminals en temps réel à partir des terminaux actifs
	for i := range *metrics {
		metric := &(*metrics)[i]
		if metric.MetricType == "concurrent_terminals" {
			// Compter uniquement les terminaux avec status 'active'
			var activeCount int64
			err := ss.db.Table("terminals").
				Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active").
				Count(&activeCount).Error

			if err == nil {
				// Mettre à jour la valeur avec le compte réel
				metric.CurrentValue = activeCount
			}
		}
	}

	return metrics, nil
}

// ResetMonthlyUsage remet à zéro les métriques mensuelles
func (ss *subscriptionService) ResetMonthlyUsage(userID string) error {
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	return ss.repository.ResetUsageMetrics(userID, periodStart, periodEnd)
}

// GetUserPaymentMethods récupère les moyens de paiement d'un utilisateur
func (ss *subscriptionService) GetUserPaymentMethods(userID string) (*[]models.PaymentMethod, error) {
	return ss.repository.GetUserPaymentMethods(userID, true)
}

// SetDefaultPaymentMethod définit le moyen de paiement par défaut
func (ss *subscriptionService) SetDefaultPaymentMethod(userID string, paymentMethodID uuid.UUID) error {
	return ss.repository.SetDefaultPaymentMethod(userID, paymentMethodID)
}

// GetUserInvoices récupère les factures d'un utilisateur
func (ss *subscriptionService) GetUserInvoices(userID string) (*[]models.Invoice, error) {
	return ss.repository.GetUserInvoices(userID, 50) // Limite à 50 factures
}

// GetInvoiceByID récupère une facture par son ID
func (ss *subscriptionService) GetInvoiceByID(id uuid.UUID) (*models.Invoice, error) {
	return ss.repository.GetInvoice(id)
}

// GetSubscriptionAnalytics récupère les analytics des abonnements (admin seulement)
func (ss *subscriptionService) GetSubscriptionAnalytics() (*SubscriptionAnalytics, error) {
	startDate := time.Now().AddDate(0, -12, 0) // 12 mois en arrière
	endDate := time.Now()

	repoAnalytics, err := ss.repository.GetSubscriptionAnalytics(startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Calculer le MRR (Monthly Recurring Revenue)
	var mrr int64
	plans, err := ss.repository.GetAllSubscriptionPlans(true)
	if err == nil {
		for _, plan := range *plans {
			// Compter les abonnements actifs pour ce plan
			if count, exists := repoAnalytics.ByPlan[plan.Name]; exists {
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
	if repoAnalytics.TotalSubscriptions > 0 {
		churnRate = float64(repoAnalytics.CancelledSubscriptions) / float64(repoAnalytics.TotalSubscriptions) * 100
	}

	// TODO: Récupérer les signups et cancellations récentes
	var recentSignups []models.UserSubscription
	var recentCancellations []models.UserSubscription

	return &SubscriptionAnalytics{
		TotalSubscriptions:      repoAnalytics.TotalSubscriptions,
		ActiveSubscriptions:     repoAnalytics.ActiveSubscriptions,
		CancelledSubscriptions:  repoAnalytics.CancelledSubscriptions,
		TrialSubscriptions:      repoAnalytics.TrialSubscriptions,
		Revenue:                 repoAnalytics.Revenue,
		MonthlyRecurringRevenue: mrr,
		ChurnRate:               churnRate,
		ByPlan:                  repoAnalytics.ByPlan,
		RecentSignups:           recentSignups,
		RecentCancellations:     recentCancellations,
		GeneratedAt:             repoAnalytics.GeneratedAt,
	}, nil
}

// GetRequiredRoleForPlan récupère le rôle requis pour un plan
func (ss *subscriptionService) GetRequiredRoleForPlan(planID uuid.UUID) (string, error) {
	planEntity, err := ss.genericService.GetEntity(planID, models.SubscriptionPlan{}, "SubscriptionPlan", nil)
	if err != nil {
		return "", err
	}
	plan := planEntity.(*models.SubscriptionPlan)
	return plan.RequiredRole, nil
}

// GetUserBillingAddresses récupère les adresses de facturation
func (ss *subscriptionService) GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error) {
	return ss.repository.GetUserBillingAddresses(userID)
}

// SetDefaultBillingAddress définit l'adresse de facturation par défaut
func (ss *subscriptionService) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	return ss.repository.SetDefaultBillingAddress(userID, addressID)
}

// GetSubscriptionPlan récupère un plan par son ID
func (ss *subscriptionService) GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error) {
	planEntity, err := ss.genericService.GetEntity(id, models.SubscriptionPlan{}, "SubscriptionPlan", nil)
	if err != nil {
		return nil, err
	}
	plan := planEntity.(*models.SubscriptionPlan)

	return plan, nil
}

// GetAllSubscriptionPlans récupère tous les plans
func (ss *subscriptionService) GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	return ss.repository.GetAllSubscriptionPlans(activeOnly)
}

// UpgradeUserPlan upgrades a user's subscription plan and updates all usage metric limits
func (ss *subscriptionService) UpgradeUserPlan(userID string, newPlanID uuid.UUID, prorationBehavior string) (*models.UserSubscription, error) {
	// Get the user's active subscription
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		return nil, fmt.Errorf("no active subscription found for user: %v", err)
	}

	// Get the new plan to verify it exists
	newPlan, err := ss.GetSubscriptionPlan(newPlanID)
	if err != nil {
		return nil, fmt.Errorf("invalid plan ID: %v", err)
	}

	// Verify the new plan has a Stripe price ID
	if newPlan.StripePriceID == nil {
		return nil, fmt.Errorf("new plan does not have a Stripe price configured")
	}

	// Use transaction to ensure atomicity
	err = ss.db.Transaction(func(tx *gorm.DB) error {
		// Update subscription plan ID only (don't save associations)
		if err := tx.Model(subscription).Update("subscription_plan_id", newPlanID).Error; err != nil {
			return fmt.Errorf("failed to update subscription: %v", err)
		}

		// Update all usage metric limits for this user
		err := tx.Model(&models.UsageMetrics{}).
			Where("user_id = ? AND subscription_id = ?", userID, subscription.ID).
			Updates(map[string]interface{}{
				"limit_value": gorm.Expr("CASE "+
					"WHEN metric_type = 'concurrent_terminals' THEN ? "+
					"WHEN metric_type = 'courses_created' THEN ? "+
					"WHEN metric_type = 'lab_sessions' THEN ? "+
					"ELSE limit_value END",
					newPlan.MaxConcurrentTerminals,
					newPlan.MaxCourses,
					newPlan.MaxLabSessions,
				),
			}).Error

		if err != nil {
			return fmt.Errorf("failed to update usage metrics: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Reload to get the full subscription with updated plan
	return ss.repository.GetUserSubscription(subscription.ID)
}

// UpdateUsageMetricLimits updates the limit values for all usage metrics based on a new plan
func (ss *subscriptionService) UpdateUsageMetricLimits(userID string, newPlanID uuid.UUID) error {
	// Get the new plan
	newPlan, err := ss.GetSubscriptionPlan(newPlanID)
	if err != nil {
		return fmt.Errorf("invalid plan ID: %v", err)
	}

	// Get the user's subscription
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		return fmt.Errorf("no active subscription found: %v", err)
	}

	// Delete all existing metrics for this subscription
	// This ensures we remove metrics from the old plan and add only the new plan's metrics
	err = ss.db.Where("user_id = ? AND subscription_id = ?", userID, subscription.ID).
		Delete(&models.UsageMetrics{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete old metrics: %v", err)
	}

	fmt.Printf("🗑️ Deleted old usage metrics for user %s (subscription %s)\n", userID, subscription.ID)

	// Initialize new metrics for the new plan
	err = ss.InitializeUsageMetrics(userID, subscription.ID, newPlanID)
	if err != nil {
		return fmt.Errorf("failed to initialize new metrics: %v", err)
	}

	fmt.Printf("✅ Initialized new usage metrics for plan %s (%s)\n", newPlan.ID, newPlan.Name)
	return nil
}

// InitializeUsageMetrics creates usage metric records for a new subscription
func (ss *subscriptionService) InitializeUsageMetrics(userID string, subscriptionID uuid.UUID, planID uuid.UUID) error {
	// Get the plan
	plan, err := ss.GetSubscriptionPlan(planID)
	if err != nil {
		return fmt.Errorf("invalid plan ID: %v", err)
	}

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	// Define the metrics to create
	metrics := []models.UsageMetrics{
		{
			UserID:         userID,
			SubscriptionID: subscriptionID,
			MetricType:     "concurrent_terminals",
			CurrentValue:   0,
			LimitValue:     int64(plan.MaxConcurrentTerminals),
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
		},
		{
			UserID:         userID,
			SubscriptionID: subscriptionID,
			MetricType:     "courses_created",
			CurrentValue:   0,
			LimitValue:     int64(plan.MaxCourses),
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
		},
		{
			UserID:         userID,
			SubscriptionID: subscriptionID,
			MetricType:     "lab_sessions",
			CurrentValue:   0,
			LimitValue:     int64(plan.MaxLabSessions),
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
		},
	}

	// Create each metric, or update if it already exists
	for _, metric := range metrics {
		err := ss.repository.CreateOrUpdateUsageMetrics(&metric)
		if err != nil {
			return fmt.Errorf("failed to create metric %s: %v", metric.MetricType, err)
		}
	}

	fmt.Printf("✅ Initialized %d usage metrics for user %s (subscription: %s)\n",
		len(metrics), userID, subscriptionID)

	return nil
}
