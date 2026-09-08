// src/payment/services/conversionService.go
package services

import (
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// ConversionService gère les conversions entre models et DTOs
// UserSubscriptionToDTO convertit un UserSubscription model vers DTO
func UserSubscriptionToDTO(subscription *models.UserSubscription) *dto.UserSubscriptionOutput {
	if subscription == nil {
		return nil
	}

	SubscriptionPlanDto := SubscriptionPlanToDTO(&subscription.SubscriptionPlan)

	output := &dto.UserSubscriptionOutput{
		ID:                   subscription.ID,
		UserID:               subscription.UserID,
		SubscriptionPlanID:   subscription.SubscriptionPlanID,
		SubscriptionPlan:     *SubscriptionPlanDto,
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		SubscriptionType:     subscription.SubscriptionType,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		CancelledAt:          subscription.CancelledAt,
		ExpiresAt:            subscription.ExpiresAt,
		CreatedAt:            subscription.CreatedAt,
		UpdatedAt:            subscription.UpdatedAt,
	}

	// If this subscription is from a bulk purchase, fetch batch owner information
	if subscription.SubscriptionBatchID != nil && subscription.PurchaserUserID != nil {
		output.SubscriptionBatchID = subscription.SubscriptionBatchID
		output.BatchOwnerID = subscription.PurchaserUserID
		output.AssignedAt = &subscription.CreatedAt // License was assigned when subscription was created

		// Fetch batch owner details from Casdoor
		populateBatchOwnerInfo(output, *subscription.PurchaserUserID)
	}

	// Admin assignment tracking
	if subscription.AssignedByUserID != nil {
		output.AssignedByUserID = subscription.AssignedByUserID
	}

	return output
}

// UserSubscriptionsToDTO convertit une liste de UserSubscription
func UserSubscriptionsToDTO(subscriptions *[]models.UserSubscription) *[]dto.UserSubscriptionOutput {
	if subscriptions == nil {
		return nil
	}

	var outputs []dto.UserSubscriptionOutput
	for _, subscription := range *subscriptions {
		output := UserSubscriptionToDTO(&subscription)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// SubscriptionPlanToDTO convertit un SubscriptionPlan model vers DTO.
//
// Delegates to SubscriptionPlanToOutput, the single producer. This function used
// to build the DTO itself and dropped six fields doing so — including the budget
// caps, whose zero value then meant "unlimited" (#454).
func SubscriptionPlanToDTO(plan *models.SubscriptionPlan) *dto.SubscriptionPlanOutput {
	if plan == nil {
		return nil
	}

	out := SubscriptionPlanToOutput(plan)
	return &out
}

// SubscriptionPlansToDTO convertit une liste de SubscriptionPlan
func SubscriptionPlansToDTO(plans *[]models.SubscriptionPlan) *[]dto.SubscriptionPlanOutput {
	if plans == nil {
		return nil
	}

	var outputs []dto.SubscriptionPlanOutput
	for _, plan := range *plans {
		output := SubscriptionPlanToDTO(&plan)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// UsageMetricsToDTO convertit des UsageMetrics model vers DTO
func UsageMetricsToDTO(metrics *models.UsageMetrics) *dto.UsageMetricsOutput {
	if metrics == nil {
		return nil
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
	}
}

// UsageMetricsListToDTO convertit une liste d'UsageMetrics
func UsageMetricsListToDTO(metricsList *[]models.UsageMetrics) *[]dto.UsageMetricsOutput {
	if metricsList == nil {
		return nil
	}

	var outputs []dto.UsageMetricsOutput
	for _, metrics := range *metricsList {
		output := UsageMetricsToDTO(&metrics)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// PaymentMethodToDTO convertit un PaymentMethod model vers DTO
func PaymentMethodToDTO(pm *models.PaymentMethod) *dto.PaymentMethodOutput {
	if pm == nil {
		return nil
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
	}
}

// PaymentMethodsToDTO convertit une liste de PaymentMethod
func PaymentMethodsToDTO(pms *[]models.PaymentMethod) *[]dto.PaymentMethodOutput {
	if pms == nil {
		return nil
	}

	var outputs []dto.PaymentMethodOutput
	for _, pm := range *pms {
		output := PaymentMethodToDTO(&pm)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// InvoiceToDTO convertit un Invoice model vers DTO
func InvoiceToDTO(invoice *models.Invoice) *dto.InvoiceOutput {
	if invoice == nil {
		return nil
	}

	subscriptionOutput := UserSubscriptionToDTO(&invoice.UserSubscription)

	return &dto.InvoiceOutput{
		ID:                         invoice.ID,
		UserID:                     invoice.UserID,
		UserSubscription:           *subscriptionOutput,
		OrganizationID:             invoice.OrganizationID,
		OrganizationSubscriptionID: invoice.OrganizationSubscriptionID,
		StripeInvoiceID:            invoice.StripeInvoiceID,
		Amount:                     invoice.Amount,
		Currency:                   invoice.Currency,
		Status:                     invoice.Status,
		InvoiceNumber:              invoice.InvoiceNumber,
		InvoiceDate:                invoice.InvoiceDate,
		DueDate:                    invoice.DueDate,
		PaidAt:                     invoice.PaidAt,
		StripeHostedURL:            invoice.StripeHostedURL,
		DownloadURL:                invoice.DownloadURL,
		CreatedAt:                  invoice.CreatedAt,
	}
}

// InvoicesToDTO convertit une liste d'Invoice
func InvoicesToDTO(invoices *[]models.Invoice) *[]dto.InvoiceOutput {
	if invoices == nil {
		return nil
	}

	var outputs []dto.InvoiceOutput
	for _, invoice := range *invoices {
		output := InvoiceToDTO(&invoice)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// OrganizationRolePlanToDTO convertit un OrganizationRolePlan model vers DTO
func OrganizationRolePlanToDTO(rolePlan *models.OrganizationRolePlan) *dto.OrganizationRolePlanOutput {
	if rolePlan == nil {
		return nil
	}

	var planOutput dto.SubscriptionPlanOutput
	converted := SubscriptionPlanToDTO(&rolePlan.SubscriptionPlan)
	if converted != nil {
		planOutput = *converted
	}

	return &dto.OrganizationRolePlanOutput{
		ID:                 rolePlan.ID,
		OrganizationID:     rolePlan.OrganizationID,
		Role:               rolePlan.Role,
		SubscriptionPlanID: rolePlan.SubscriptionPlanID,
		SubscriptionPlan:   planOutput,
		CreatedAt:          rolePlan.CreatedAt,
		UpdatedAt:          rolePlan.UpdatedAt,
	}
}

// OrganizationRolePlansToDTO convertit une liste d'OrganizationRolePlan
func OrganizationRolePlansToDTO(rolePlans []models.OrganizationRolePlan) []dto.OrganizationRolePlanOutput {
	outputs := make([]dto.OrganizationRolePlanOutput, 0, len(rolePlans))
	for i := range rolePlans {
		output := OrganizationRolePlanToDTO(&rolePlans[i])
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return outputs
}

// BillingAddressToDTO convertit un BillingAddress model vers DTO
func BillingAddressToDTO(address *models.BillingAddress) *dto.BillingAddressOutput {
	if address == nil {
		return nil
	}

	return &dto.BillingAddressOutput{
		ID:          address.ID,
		UserID:      address.UserID,
		Line1:       address.Line1,
		Line2:       address.Line2,
		City:        address.City,
		State:       address.State,
		PostalCode:  address.PostalCode,
		Country:     address.Country,
		CompanyName: address.CompanyName,
		Siret:       address.Siret,
		VatNumber:   address.VatNumber,
		IsDefault:   address.IsDefault,
		CreatedAt:   address.CreatedAt,
		UpdatedAt:   address.UpdatedAt,
	}
}

// BillingAddressesToDTO convertit une liste de BillingAddress
func BillingAddressesToDTO(addresses *[]models.BillingAddress) *[]dto.BillingAddressOutput {
	if addresses == nil {
		return nil
	}

	var outputs []dto.BillingAddressOutput
	for _, address := range *addresses {
		output := BillingAddressToDTO(&address)
		if output != nil {
			outputs = append(outputs, *output)
		}
	}

	return &outputs
}

// SubscriptionAnalyticsToDTO convertit SubscriptionAnalytics vers DTO
func SubscriptionAnalyticsToDTO(analytics *SubscriptionAnalytics) *dto.SubscriptionAnalyticsOutput {
	if analytics == nil {
		return nil
	}

	// Convertir les subscriptions récentes
	var recentSignups []dto.UserSubscriptionOutput
	for _, signup := range analytics.RecentSignups {
		output := UserSubscriptionToDTO(&signup)
		if output != nil {
			recentSignups = append(recentSignups, *output)
		}
	}

	var recentCancellations []dto.UserSubscriptionOutput
	for _, cancellation := range analytics.RecentCancellations {
		output := UserSubscriptionToDTO(&cancellation)
		if output != nil {
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

// populateBatchOwnerInfo fetches batch owner details from Casdoor and populates the DTO
func populateBatchOwnerInfo(output *dto.UserSubscriptionOutput, purchaserUserID string) {
	// Fetch user from Casdoor
	user, err := casdoorsdk.GetUserByUserId(purchaserUserID)
	if err != nil {
		// Log error but don't fail the entire conversion
		utils.Warn("Failed to fetch batch owner info from Casdoor for user %s: %v", purchaserUserID, err)
		return
	}

	if user == nil {
		utils.Warn("Batch owner user %s not found in Casdoor", purchaserUserID)
		return
	}

	// Populate batch owner information
	output.BatchOwnerName = &user.DisplayName
	output.BatchOwnerEmail = &user.Email
}
