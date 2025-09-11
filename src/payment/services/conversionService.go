// src/payment/services/conversionService.go
package services

import (
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// ConversionService gère les conversions entre models et DTOs
type ConversionService interface {
	// Subscription conversions
	UserSubscriptionToDTO(subscription *models.UserSubscription) (*dto.UserSubscriptionOutput, error)
	UserSubscriptionsToDTO(subscriptions *[]models.UserSubscription) (*[]dto.UserSubscriptionOutput, error)

	// Plan conversions
	SubscriptionPlanToDTO(plan *models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error)
	SubscriptionPlansToDTO(plans *[]models.SubscriptionPlan) (*[]dto.SubscriptionPlanOutput, error)

	// Usage metrics conversions
	UsageMetricsToDTO(metrics *models.UsageMetrics) (*dto.UsageMetricsOutput, error)
	UsageMetricsListToDTO(metricsList *[]models.UsageMetrics) (*[]dto.UsageMetricsOutput, error)
	UsageLimitCheckToDTO(check *UsageLimitCheck) *dto.UsageLimitCheckOutput

	// Payment method conversions
	PaymentMethodToDTO(pm *models.PaymentMethod) (*dto.PaymentMethodOutput, error)
	PaymentMethodsToDTO(pms *[]models.PaymentMethod) (*[]dto.PaymentMethodOutput, error)

	// Invoice conversions
	InvoiceToDTO(invoice *models.Invoice) (*dto.InvoiceOutput, error)
	InvoicesToDTO(invoices *[]models.Invoice) (*[]dto.InvoiceOutput, error)

	// Billing address conversions
	BillingAddressToDTO(address *models.BillingAddress) (*dto.BillingAddressOutput, error)
	BillingAddressesToDTO(addresses *[]models.BillingAddress) (*[]dto.BillingAddressOutput, error)

	// Analytics conversions
	SubscriptionAnalyticsToDTO(analytics *SubscriptionAnalytics) *dto.SubscriptionAnalyticsOutput
}

type conversionService struct{}

func NewConversionService() ConversionService {
	return &conversionService{}
}

// UserSubscriptionToDTO convertit un UserSubscription model vers DTO
func (cs *conversionService) UserSubscriptionToDTO(subscription *models.UserSubscription) (*dto.UserSubscriptionOutput, error) {
	if subscription == nil {
		return nil, nil
	}

	return &dto.UserSubscriptionOutput{
		ID:                   subscription.ID,
		UserID:               subscription.UserID,
		SubscriptionPlanID:   subscription.SubscriptionPlanID,
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

// UserSubscriptionsToDTO convertit une liste de UserSubscription
func (cs *conversionService) UserSubscriptionsToDTO(subscriptions *[]models.UserSubscription) (*[]dto.UserSubscriptionOutput, error) {
	if subscriptions == nil {
		return nil, nil
	}

	var outputs []dto.UserSubscriptionOutput
	for _, subscription := range *subscriptions {
		output, err := cs.UserSubscriptionToDTO(&subscription)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// SubscriptionPlanToDTO convertit un SubscriptionPlan model vers DTO
func (cs *conversionService) SubscriptionPlanToDTO(plan *models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error) {
	if plan == nil {
		return nil, nil
	}

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

// SubscriptionPlansToDTO convertit une liste de SubscriptionPlan
func (cs *conversionService) SubscriptionPlansToDTO(plans *[]models.SubscriptionPlan) (*[]dto.SubscriptionPlanOutput, error) {
	if plans == nil {
		return nil, nil
	}

	var outputs []dto.SubscriptionPlanOutput
	for _, plan := range *plans {
		output, err := cs.SubscriptionPlanToDTO(&plan)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// UsageMetricsToDTO convertit des UsageMetrics model vers DTO
func (cs *conversionService) UsageMetricsToDTO(metrics *models.UsageMetrics) (*dto.UsageMetricsOutput, error) {
	if metrics == nil {
		return nil, nil
	}

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

// UsageMetricsListToDTO convertit une liste d'UsageMetrics
func (cs *conversionService) UsageMetricsListToDTO(metricsList *[]models.UsageMetrics) (*[]dto.UsageMetricsOutput, error) {
	if metricsList == nil {
		return nil, nil
	}

	var outputs []dto.UsageMetricsOutput
	for _, metrics := range *metricsList {
		output, err := cs.UsageMetricsToDTO(&metrics)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// UsageLimitCheckToDTO convertit un UsageLimitCheck vers DTO
func (cs *conversionService) UsageLimitCheckToDTO(check *UsageLimitCheck) *dto.UsageLimitCheckOutput {
	if check == nil {
		return nil
	}

	return &dto.UsageLimitCheckOutput{
		Allowed:        check.Allowed,
		CurrentUsage:   check.CurrentUsage,
		Limit:          check.Limit,
		RemainingUsage: check.RemainingUsage,
		Message:        check.Message,
	}
}

// PaymentMethodToDTO convertit un PaymentMethod model vers DTO
func (cs *conversionService) PaymentMethodToDTO(pm *models.PaymentMethod) (*dto.PaymentMethodOutput, error) {
	if pm == nil {
		return nil, nil
	}

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

// PaymentMethodsToDTO convertit une liste de PaymentMethod
func (cs *conversionService) PaymentMethodsToDTO(pms *[]models.PaymentMethod) (*[]dto.PaymentMethodOutput, error) {
	if pms == nil {
		return nil, nil
	}

	var outputs []dto.PaymentMethodOutput
	for _, pm := range *pms {
		output, err := cs.PaymentMethodToDTO(&pm)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// InvoiceToDTO convertit un Invoice model vers DTO
func (cs *conversionService) InvoiceToDTO(invoice *models.Invoice) (*dto.InvoiceOutput, error) {
	if invoice == nil {
		return nil, nil
	}

	subscriptionOutput, err := cs.UserSubscriptionToDTO(&invoice.UserSubscription)
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

// InvoicesToDTO convertit une liste d'Invoice
func (cs *conversionService) InvoicesToDTO(invoices *[]models.Invoice) (*[]dto.InvoiceOutput, error) {
	if invoices == nil {
		return nil, nil
	}

	var outputs []dto.InvoiceOutput
	for _, invoice := range *invoices {
		output, err := cs.InvoiceToDTO(&invoice)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// BillingAddressToDTO convertit un BillingAddress model vers DTO
func (cs *conversionService) BillingAddressToDTO(address *models.BillingAddress) (*dto.BillingAddressOutput, error) {
	if address == nil {
		return nil, nil
	}

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

// BillingAddressesToDTO convertit une liste de BillingAddress
func (cs *conversionService) BillingAddressesToDTO(addresses *[]models.BillingAddress) (*[]dto.BillingAddressOutput, error) {
	if addresses == nil {
		return nil, nil
	}

	var outputs []dto.BillingAddressOutput
	for _, address := range *addresses {
		output, err := cs.BillingAddressToDTO(&address)
		if err != nil {
			return nil, err
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs, nil
}

// SubscriptionAnalyticsToDTO convertit SubscriptionAnalytics vers DTO
func (cs *conversionService) SubscriptionAnalyticsToDTO(analytics *SubscriptionAnalytics) *dto.SubscriptionAnalyticsOutput {
	if analytics == nil {
		return nil
	}

	// Convertir les subscriptions récentes
	var recentSignups []dto.UserSubscriptionOutput
	for _, signup := range analytics.RecentSignups {
		output, err := cs.UserSubscriptionToDTO(&signup)
		if err == nil && output != nil {
			recentSignups = append(recentSignups, *output)
		}
	}

	var recentCancellations []dto.UserSubscriptionOutput
	for _, cancellation := range analytics.RecentCancellations {
		output, err := cs.UserSubscriptionToDTO(&cancellation)
		if err == nil && output != nil {
			recentCancellations = append(recentCancellations, *output)
		}
	}

	return &dto.SubscriptionAnalyticsOutput{
		TotalSubscriptions:      analytics.TotalSubscriptions,
		ActiveSubscriptions:     analytics.ActiveSubscriptions,
		CancelledSubscriptions:  analytics.CancelledSubscriptions,
		TrialSubscriptions:      analytics.TrialSubscriptions,
		Revenue:                 analytics.Revenue,
		MonthlyRecurringRevenue: analytics.MonthlyRecurringRevenue,
		ChurnRate:               analytics.ChurnRate,
		ByPlan:                  analytics.ByPlan,
		RecentSignups:           recentSignups,
		RecentCancellations:     recentCancellations,
		GeneratedAt:             analytics.GeneratedAt,
	}
}
