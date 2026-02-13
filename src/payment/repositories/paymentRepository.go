// src/payment/repositories/paymentRepository.go
package repositories

import (
	"errors"
	"fmt"
	"soli/formations/src/payment/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentRepository interface {
	// SubscriptionPlan operations
	GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error)
	GetSubscriptionPlanByStripePriceID(stripePriceID string) (*models.SubscriptionPlan, error)

	// UserSubscription operations
	CreateUserSubscription(subscription *models.UserSubscription) error
	GetUserSubscription(id uuid.UUID) (*models.UserSubscription, error)
	GetUserSubscriptionByStripeID(stripeSubscriptionID string) (*models.UserSubscription, error)
	GetActiveUserSubscription(userID string) (*models.UserSubscription, error)
	GetAllActiveUserSubscriptions(userID string) ([]models.UserSubscription, error)
	GetPrimaryUserSubscription(userID string) (*models.UserSubscription, error)
	GetActiveSubscriptionByCustomerID(customerID string) (*models.UserSubscription, error)
	GetUserSubscriptions(userID string, includeInactive bool) (*[]models.UserSubscription, error)
	UpdateUserSubscription(subscription *models.UserSubscription) error

	// Invoice operations
	CreateInvoice(invoice *models.Invoice) error
	GetInvoice(id uuid.UUID) (*models.Invoice, error)
	GetInvoiceByStripeID(stripeInvoiceID string) (*models.Invoice, error)
	GetUserInvoices(userID string, limit int) (*[]models.Invoice, error)
	UpdateInvoice(invoice *models.Invoice) error

	// PaymentMethod operations
	CreatePaymentMethod(pm *models.PaymentMethod) error
	GetPaymentMethod(id uuid.UUID) (*models.PaymentMethod, error)
	GetPaymentMethodByStripeID(stripePaymentMethodID string) (*models.PaymentMethod, error)
	GetUserPaymentMethods(userID string, activeOnly bool) (*[]models.PaymentMethod, error)
	UpdatePaymentMethod(pm *models.PaymentMethod) error
	DeletePaymentMethod(id uuid.UUID) error
	SetDefaultPaymentMethod(userID string, pmID uuid.UUID) error

	// BillingAddress operations
	CreateBillingAddress(address *models.BillingAddress) error
	GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error)
	GetDefaultBillingAddress(userID string) (*models.BillingAddress, error)
	UpdateBillingAddress(address *models.BillingAddress) error
	DeleteBillingAddress(id uuid.UUID) error
	SetDefaultBillingAddress(userID string, addressID uuid.UUID) error

	// UsageMetrics operations
	CreateOrUpdateUsageMetrics(metrics *models.UsageMetrics) error
	GetUserUsageMetrics(userID string, metricType string) (*models.UsageMetrics, error)
	GetAllUserUsageMetrics(userID string) (*[]models.UsageMetrics, error)
	IncrementUsageMetric(userID, metricType string, increment int64) error
	ResetUsageMetrics(userID string, periodStart, periodEnd time.Time) error

	// Analytics and reporting
	GetSubscriptionAnalytics(startDate, endDate time.Time) (*SubscriptionAnalytics, error)
	GetRevenueByPeriod(startDate, endDate time.Time, interval string) (*[]RevenueByPeriod, error)

	// Cleanup operations
	CleanupExpiredSubscriptions() error
	ArchiveOldInvoices(daysOld int) error
}

type paymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{
		db: db,
	}
}

func (r *paymentRepository) GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	var plans []models.SubscriptionPlan
	query := r.db.Model(&models.SubscriptionPlan{})

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	err := query.Find(&plans).Error
	if err != nil {
		return nil, err
	}
	return &plans, nil
}

func (r *paymentRepository) GetSubscriptionPlanByStripePriceID(stripePriceID string) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := r.db.Where("stripe_price_id = ?", stripePriceID).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// UserSubscription operations
func (r *paymentRepository) CreateUserSubscription(subscription *models.UserSubscription) error {
	return r.db.Create(subscription).Error
}

func (r *paymentRepository) GetUserSubscription(id uuid.UUID) (*models.UserSubscription, error) {
	var subscription models.UserSubscription
	err := r.db.Preload("SubscriptionPlan").Where("id = ?", id).First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

func (r *paymentRepository) GetUserSubscriptionByStripeID(stripeSubscriptionID string) (*models.UserSubscription, error) {
	var subscription models.UserSubscription
	err := r.db.Preload("SubscriptionPlan").
		Where("stripe_subscription_id = ?", stripeSubscriptionID).
		First(&subscription).Error
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetActiveUserSubscription returns the newest active subscription (legacy method for backwards compatibility)
// NOTE: For stacked subscriptions, use GetPrimaryUserSubscription instead
func (r *paymentRepository) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	var subscription models.UserSubscription
	err := r.db.
		Preload("SubscriptionPlan").
		Where("user_id = ? AND status IN (?)", userID, []string{"active", "trialing"}).
		Order("created_at DESC"). // Always return the newest subscription if multiple exist
		First(&subscription).Error
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

// GetAllActiveUserSubscriptions returns ALL active subscriptions for a user (personal + assigned)
func (r *paymentRepository) GetAllActiveUserSubscriptions(userID string) ([]models.UserSubscription, error) {
	var subscriptions []models.UserSubscription
	err := r.db.
		Preload("SubscriptionPlan").
		Where("user_id = ? AND status IN (?)", userID, []string{"active", "trialing"}).
		Order("created_at DESC").
		Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}

	return subscriptions, nil
}

// GetPrimaryUserSubscription returns the highest-priority active subscription
// Priority based on SubscriptionPlan.Priority field (higher number = higher tier)
func (r *paymentRepository) GetPrimaryUserSubscription(userID string) (*models.UserSubscription, error) {
	subscriptions, err := r.GetAllActiveUserSubscriptions(userID)
	if err != nil {
		return nil, err
	}

	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("no active subscription found")
	}

	// If only one subscription, return it
	if len(subscriptions) == 1 {
		return &subscriptions[0], nil
	}

	// Multiple subscriptions: return the one with highest plan priority
	var primary *models.UserSubscription
	highestPriority := -1

	for i := range subscriptions {
		sub := &subscriptions[i]
		planPriority := sub.SubscriptionPlan.Priority

		if planPriority > highestPriority {
			highestPriority = planPriority
			primary = sub
		}
	}

	if primary != nil {
		return primary, nil
	}

	// Fallback: return newest
	return &subscriptions[0], nil
}

func (r *paymentRepository) GetActiveSubscriptionByCustomerID(customerID string) (*models.UserSubscription, error) {
	var subscription models.UserSubscription
	err := r.db.
		Preload("SubscriptionPlan").
		Where("stripe_customer_id = ? AND status IN (?)", customerID, []string{"active", "trialing"}).
		First(&subscription).Error
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (r *paymentRepository) GetUserSubscriptions(userID string, includeInactive bool) (*[]models.UserSubscription, error) {
	var subscriptions []models.UserSubscription
	query := r.db.Preload("SubscriptionPlan").Where("user_id = ?", userID)

	if !includeInactive {
		query = query.Where("status IN (?)", []string{"active", "trialing", "past_due"})
	}

	err := query.Order("created_at DESC").Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	return &subscriptions, nil
}

func (r *paymentRepository) UpdateUserSubscription(subscription *models.UserSubscription) error {
	return r.db.Save(subscription).Error
}

// Invoice operations
func (r *paymentRepository) CreateInvoice(invoice *models.Invoice) error {
	return r.db.Create(invoice).Error
}

func (r *paymentRepository) GetInvoice(id uuid.UUID) (*models.Invoice, error) {
	var invoice models.Invoice
	err := r.db.Preload("UserSubscription").
		Where("id = ?", id).First(&invoice).Error
	if err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (r *paymentRepository) GetInvoiceByStripeID(stripeInvoiceID string) (*models.Invoice, error) {
	var invoice models.Invoice
	err := r.db.Preload("UserSubscription").
		Where("stripe_invoice_id = ?", stripeInvoiceID).
		First(&invoice).Error
	if err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (r *paymentRepository) GetUserInvoices(userID string, limit int) (*[]models.Invoice, error) {
	var invoices []models.Invoice
	query := r.db.Preload("UserSubscription").
		Where("user_id = ?", userID).
		Order("invoice_date DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&invoices).Error
	if err != nil {
		return nil, err
	}
	return &invoices, nil
}

func (r *paymentRepository) UpdateInvoice(invoice *models.Invoice) error {
	return r.db.Save(invoice).Error
}

// PaymentMethod operations
func (r *paymentRepository) CreatePaymentMethod(pm *models.PaymentMethod) error {
	return r.db.Create(pm).Error
}

func (r *paymentRepository) GetPaymentMethod(id uuid.UUID) (*models.PaymentMethod, error) {
	var pm models.PaymentMethod
	err := r.db.Where("id = ?", id).First(&pm).Error
	if err != nil {
		return nil, err
	}
	return &pm, nil
}

func (r *paymentRepository) GetPaymentMethodByStripeID(stripePaymentMethodID string) (*models.PaymentMethod, error) {
	var pm models.PaymentMethod
	err := r.db.Where("stripe_payment_method_id = ?", stripePaymentMethodID).First(&pm).Error
	if err != nil {
		return nil, err
	}
	return &pm, nil
}

func (r *paymentRepository) GetUserPaymentMethods(userID string, activeOnly bool) (*[]models.PaymentMethod, error) {
	var paymentMethods []models.PaymentMethod
	query := r.db.Where("user_id = ?", userID)

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	err := query.Order("is_default DESC, created_at DESC").Find(&paymentMethods).Error
	if err != nil {
		return nil, err
	}
	return &paymentMethods, nil
}

func (r *paymentRepository) UpdatePaymentMethod(pm *models.PaymentMethod) error {
	return r.db.Save(pm).Error
}

func (r *paymentRepository) DeletePaymentMethod(id uuid.UUID) error {
	return r.db.Delete(&models.PaymentMethod{}, id).Error
}

func (r *paymentRepository) SetDefaultPaymentMethod(userID string, pmID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Désactiver tous les moyens de paiement par défaut pour cet utilisateur
		if err := tx.Model(&models.PaymentMethod{}).
			Where("user_id = ?", userID).
			Update("is_default", false).Error; err != nil {
			return err
		}

		// Activer le nouveau moyen de paiement par défaut
		return tx.Model(&models.PaymentMethod{}).
			Where("id = ? AND user_id = ?", pmID, userID).
			Update("is_default", true).Error
	})
}

// BillingAddress operations
func (r *paymentRepository) CreateBillingAddress(address *models.BillingAddress) error {
	return r.db.Create(address).Error
}

func (r *paymentRepository) GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error) {
	var addresses []models.BillingAddress
	err := r.db.Where("user_id = ?", userID).
		Order("is_default DESC, created_at DESC").
		Find(&addresses).Error
	if err != nil {
		return nil, err
	}
	return &addresses, nil
}

func (r *paymentRepository) GetDefaultBillingAddress(userID string) (*models.BillingAddress, error) {
	var address models.BillingAddress
	err := r.db.Where("user_id = ? AND is_default = ?", userID, true).
		First(&address).Error
	if err != nil {
		return nil, err
	}
	return &address, nil
}

func (r *paymentRepository) UpdateBillingAddress(address *models.BillingAddress) error {
	return r.db.Save(address).Error
}

func (r *paymentRepository) DeleteBillingAddress(id uuid.UUID) error {
	return r.db.Delete(&models.BillingAddress{}, id).Error
}

func (r *paymentRepository) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Désactiver toutes les adresses par défaut
		if err := tx.Model(&models.BillingAddress{}).
			Where("user_id = ?", userID).
			Update("is_default", false).Error; err != nil {
			return err
		}

		// Activer la nouvelle adresse par défaut
		return tx.Model(&models.BillingAddress{}).
			Where("id = ? AND user_id = ?", addressID, userID).
			Update("is_default", true).Error
	})
}

// UsageMetrics operations
func (r *paymentRepository) CreateOrUpdateUsageMetrics(metrics *models.UsageMetrics) error {
	// Chercher si une métrique existe déjà pour cette période
	var existing models.UsageMetrics
	err := r.db.Where("user_id = ? AND subscription_id = ? AND metric_type = ? AND period_start = ? AND period_end = ?",
		metrics.UserID, metrics.SubscriptionID, metrics.MetricType, metrics.PeriodStart, metrics.PeriodEnd).
		First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Créer nouvelle métrique
		return r.db.Create(metrics).Error
	} else if err != nil {
		return err
	}

	// Mettre à jour la métrique existante
	existing.CurrentValue = metrics.CurrentValue
	existing.LimitValue = metrics.LimitValue
	existing.LastUpdated = time.Now()
	return r.db.Save(&existing).Error
}

func (r *paymentRepository) GetUserUsageMetrics(userID string, metricType string) (*models.UsageMetrics, error) {
	var metrics models.UsageMetrics
	err := r.db.Where("user_id = ? AND metric_type = ?", userID, metricType).
		Where("period_end > ?", time.Now()). // Période actuelle
		First(&metrics).Error
	if err != nil {
		return nil, err
	}
	return &metrics, nil
}

func (r *paymentRepository) GetAllUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	var metrics []models.UsageMetrics
	err := r.db.Where("user_id = ?", userID).
		Where("period_end > ?", time.Now()). // Période actuelle seulement
		Find(&metrics).Error
	if err != nil {
		return nil, err
	}
	return &metrics, nil
}

func (r *paymentRepository) IncrementUsageMetric(userID, metricType string, increment int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var metrics models.UsageMetrics
		err := tx.Where("user_id = ? AND metric_type = ?", userID, metricType).
			Where("period_end > ?", time.Now()).
			First(&metrics).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Récupérer l'abonnement pour définir la limite
			subscription, err := r.GetActiveUserSubscription(userID)
			if err != nil {
				return fmt.Errorf("no active subscription found: %w", err)
			}

			// Définir la limite selon le type de métrique et le plan
			var limit int64 = -1 // Par défaut illimité
			switch metricType {
			case "courses_created":
				limit = int64(subscription.SubscriptionPlan.MaxCourses)
			case "concurrent_terminals":
				limit = int64(subscription.SubscriptionPlan.MaxConcurrentTerminals)
			}

			// Créer nouvelle métrique
			now := time.Now()
			periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

			metrics = models.UsageMetrics{
				UserID:         userID,
				SubscriptionID: subscription.ID,
				MetricType:     metricType,
				CurrentValue:   increment,
				LimitValue:     limit,
				PeriodStart:    periodStart,
				PeriodEnd:      periodEnd,
				LastUpdated:    now,
			}
			return tx.Create(&metrics).Error
		} else if err != nil {
			return err
		}

		// Incrémenter la valeur existante
		metrics.CurrentValue += increment
		metrics.LastUpdated = time.Now()
		return tx.Save(&metrics).Error
	})
}

func (r *paymentRepository) ResetUsageMetrics(userID string, periodStart, periodEnd time.Time) error {
	return r.db.Model(&models.UsageMetrics{}).
		Where("user_id = ? AND period_start = ? AND period_end = ?", userID, periodStart, periodEnd).
		Update("current_value", 0).Error
}

// Analytics operations
func (r *paymentRepository) GetSubscriptionAnalytics(startDate, endDate time.Time) (*SubscriptionAnalytics, error) {
	analytics := &SubscriptionAnalytics{
		GeneratedAt: time.Now(),
		ByPlan:      make(map[string]int),
	}

	// Total subscriptions
	r.db.Model(&models.UserSubscription{}).
		Where("created_at BETWEEN ? AND ?", startDate, endDate).
		Count(&analytics.TotalSubscriptions)

	// Active subscriptions
	r.db.Model(&models.UserSubscription{}).
		Where("status IN (?)", []string{"active", "trialing"}).
		Count(&analytics.ActiveSubscriptions)

	// Cancelled subscriptions
	r.db.Model(&models.UserSubscription{}).
		Where("status = ?", "cancelled").
		Where("cancelled_at BETWEEN ? AND ?", startDate, endDate).
		Count(&analytics.CancelledSubscriptions)

	// Trial subscriptions
	r.db.Model(&models.UserSubscription{}).
		Where("status = ?", "trialing").
		Count(&analytics.TrialSubscriptions)

	// Revenue calculation
	var totalRevenue struct {
		Amount int64
	}
	r.db.Model(&models.Invoice{}).
		Select("SUM(amount) as amount").
		Where("status = ?", "paid").
		Where("paid_at BETWEEN ? AND ?", startDate, endDate).
		Scan(&totalRevenue)
	analytics.Revenue = totalRevenue.Amount

	// Subscriptions by plan
	var planCounts []struct {
		PlanName string `json:"plan_name"`
		Count    int    `json:"count"`
	}
	r.db.Model(&models.UserSubscription{}).
		Select("subscription_plans.name as plan_name, COUNT(*) as count").
		Joins("JOIN subscription_plans ON user_subscription.subscription_plan_id = subscription_plans.id").
		Where("user_subscription.status IN (?)", []string{"active", "trialing"}).
		Group("subscription_plans.name").
		Scan(&planCounts)

	for _, pc := range planCounts {
		analytics.ByPlan[pc.PlanName] = pc.Count
	}

	return analytics, nil
}

func (r *paymentRepository) GetRevenueByPeriod(startDate, endDate time.Time, interval string) (*[]RevenueByPeriod, error) {
	var revenues []RevenueByPeriod

	var groupBy string
	switch interval {
	case "day":
		groupBy = "DATE(paid_at)"
	case "week":
		groupBy = "DATE_TRUNC('week', paid_at)"
	case "month":
		groupBy = "DATE_TRUNC('month', paid_at)"
	default:
		return nil, fmt.Errorf("unsupported interval: %s", interval)
	}

	err := r.db.Model(&models.Invoice{}).
		Select(fmt.Sprintf("%s as period, SUM(amount) as revenue", groupBy)).
		Where("status = ? AND paid_at BETWEEN ? AND ?", "paid", startDate, endDate).
		Group(groupBy).
		Order("period").
		Scan(&revenues).Error

	if err != nil {
		return nil, err
	}
	return &revenues, nil
}

// Cleanup operations
func (r *paymentRepository) CleanupExpiredSubscriptions() error {
	cutoffTime := time.Now().AddDate(0, 0, -30) // 30 jours

	return r.db.Model(&models.UserSubscription{}).
		Where("status IN (?)", []string{"cancelled", "incomplete_expired"}).
		Where("cancelled_at < ?", cutoffTime).
		Update("deleted_at", time.Now()).Error
}

func (r *paymentRepository) ArchiveOldInvoices(daysOld int) error {
	cutoffTime := time.Now().AddDate(0, 0, -daysOld)

	// Marquer les anciennes factures payées comme archivées (ou les déplacer vers une table d'archive)
	return r.db.Model(&models.Invoice{}).
		Where("status = ? AND paid_at < ?", "paid", cutoffTime).
		Update("updated_at", time.Now()).Error // Placeholder, implémenter l'archivage selon vos besoins
}

// Utility types for analytics
type SubscriptionAnalytics struct {
	TotalSubscriptions     int64
	ActiveSubscriptions    int64
	CancelledSubscriptions int64
	TrialSubscriptions     int64
	Revenue                int64
	ByPlan                 map[string]int
	GeneratedAt            time.Time
}

type RevenueByPeriod struct {
	Period  time.Time `json:"period"`
	Revenue int64     `json:"revenue"`
}
