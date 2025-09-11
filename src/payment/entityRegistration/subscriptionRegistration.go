// src/payment/entityRegistration/subscriptionRegistration.go
package registration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// SubscriptionPlanRegistration
type SubscriptionPlanRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SubscriptionPlanRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return subscriptionPlanPtrModelToOutput(input.(*models.SubscriptionPlan))
	} else {
		return subscriptionPlanValueModelToOutput(input.(models.SubscriptionPlan))
	}
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

func subscriptionPlanValueModelToOutput(plan models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error) {
	return subscriptionPlanPtrModelToOutput(&plan)
}

func (s SubscriptionPlanRegistration) EntityInputDtoToEntityModel(input any) any {
	planInput := input.(dto.CreateSubscriptionPlanInput)
	return &models.SubscriptionPlan{
		Name:               planInput.Name,
		Description:        planInput.Description,
		PriceAmount:        planInput.PriceAmount,
		Currency:           planInput.Currency,
		BillingInterval:    planInput.BillingInterval,
		TrialDays:          planInput.TrialDays,
		Features:           planInput.Features,
		MaxConcurrentUsers: planInput.MaxConcurrentUsers,
		MaxCourses:         planInput.MaxCourses,
		MaxLabSessions:     planInput.MaxLabSessions,
		RequiredRole:       planInput.RequiredRole,
		IsActive:           true,
	}
}

func (s SubscriptionPlanRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.SubscriptionPlan{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateSubscriptionPlanInput{},
			OutputDto:      dto.SubscriptionPlanOutput{},
			InputEditDto:   dto.UpdateSubscriptionPlanInput{},
		},
	}
}

func (s SubscriptionPlanRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Seuls les admins peuvent gérer les plans d'abonnement
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")" // Pour voir les plans disponibles

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

// UserSubscriptionRegistration
type UserSubscriptionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UserSubscriptionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return userSubscriptionPtrModelToOutput(input.(*models.UserSubscription))
	} else {
		return userSubscriptionValueModelToOutput(input.(models.UserSubscription))
	}
}

func userSubscriptionPtrModelToOutput(subscription *models.UserSubscription) (*dto.UserSubscriptionOutput, error) {

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

func userSubscriptionValueModelToOutput(subscription models.UserSubscription) (*dto.UserSubscriptionOutput, error) {
	return userSubscriptionPtrModelToOutput(&subscription)
}

func (u UserSubscriptionRegistration) EntityInputDtoToEntityModel(input any) any {
	subscriptionInput := input.(dto.CreateUserSubscriptionInput)

	return &models.UserSubscription{
		UserID:             subscriptionInput.UserID,
		SubscriptionPlanID: subscriptionInput.SubscriptionPlanID,
		Status:             "incomplete", // Sera mis à jour par les webhooks
	}
}

func (u UserSubscriptionRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.UserSubscription{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: u.EntityModelToEntityOutput,
			DtoToModel: u.EntityInputDtoToEntityModel,
			DtoToMap:   u.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateUserSubscriptionInput{},
			OutputDto:      dto.UserSubscriptionOutput{},
			InputEditDto:   dto.UpdateUserSubscriptionInput{},
		},
	}
}

func (u UserSubscriptionRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Utilisateurs peuvent voir/gérer leurs propres abonnements
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

// PaymentMethodRegistration
type PaymentMethodRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (p PaymentMethodRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return paymentMethodPtrModelToOutput(input.(*models.PaymentMethod))
	} else {
		return paymentMethodValueModelToOutput(input.(models.PaymentMethod))
	}
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

func paymentMethodValueModelToOutput(pm models.PaymentMethod) (*dto.PaymentMethodOutput, error) {
	return paymentMethodPtrModelToOutput(&pm)
}

func (p PaymentMethodRegistration) EntityInputDtoToEntityModel(input any) any {
	pmInput := input.(dto.CreatePaymentMethodInput)
	return &models.PaymentMethod{
		StripePaymentMethodID: pmInput.StripePaymentMethodID,
		IsDefault:             pmInput.SetAsDefault,
		IsActive:              true,
	}
}

func (p PaymentMethodRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.PaymentMethod{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: p.EntityModelToEntityOutput,
			DtoToModel: p.EntityInputDtoToEntityModel,
			DtoToMap:   p.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreatePaymentMethodInput{},
			OutputDto:      dto.PaymentMethodOutput{},
			InputEditDto:   dto.CreatePaymentMethodInput{}, // Réutiliser le même DTO
		},
	}
}

func (p PaymentMethodRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Utilisateurs peuvent gérer leurs propres moyens de paiement
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

// InvoiceRegistration
type InvoiceRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (i InvoiceRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return invoicePtrModelToOutput(input.(*models.Invoice))
	} else {
		return invoiceValueModelToOutput(input.(models.Invoice))
	}
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

func invoiceValueModelToOutput(invoice models.Invoice) (*dto.InvoiceOutput, error) {
	return invoicePtrModelToOutput(&invoice)
}

func (i InvoiceRegistration) EntityInputDtoToEntityModel(input any) any {
	// Les factures sont généralement créées via les webhooks Stripe
	return &models.Invoice{}
}

func (i InvoiceRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Invoice{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: i.EntityModelToEntityOutput,
			DtoToModel: i.EntityInputDtoToEntityModel,
			DtoToMap:   i.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.InvoiceOutput{}, // Pas de création manuelle
			OutputDto:      dto.InvoiceOutput{},
			InputEditDto:   dto.InvoiceOutput{}, // Pas d'édition manuelle
		},
	}
}

func (i InvoiceRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Les utilisateurs peuvent seulement voir leurs factures
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

// BillingAddressRegistration
type BillingAddressRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (b BillingAddressRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return billingAddressPtrModelToOutput(input.(*models.BillingAddress))
	} else {
		return billingAddressValueModelToOutput(input.(models.BillingAddress))
	}
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

func billingAddressValueModelToOutput(address models.BillingAddress) (*dto.BillingAddressOutput, error) {
	return billingAddressPtrModelToOutput(&address)
}

func (b BillingAddressRegistration) EntityInputDtoToEntityModel(input any) any {
	addressInput := input.(dto.CreateBillingAddressInput)
	return &models.BillingAddress{
		Line1:      addressInput.Line1,
		Line2:      addressInput.Line2,
		City:       addressInput.City,
		State:      addressInput.State,
		PostalCode: addressInput.PostalCode,
		Country:    addressInput.Country,
		IsDefault:  addressInput.SetDefault,
	}
}

func (b BillingAddressRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.BillingAddress{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: b.EntityModelToEntityOutput,
			DtoToModel: b.EntityInputDtoToEntityModel,
			DtoToMap:   b.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateBillingAddressInput{},
			OutputDto:      dto.BillingAddressOutput{},
			InputEditDto:   dto.UpdateBillingAddressInput{},
		},
	}
}

func (b BillingAddressRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Utilisateurs peuvent gérer leurs propres adresses
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

// UsageMetricsRegistration
type UsageMetricsRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UsageMetricsRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return usageMetricsPtrModelToOutput(input.(*models.UsageMetrics))
	} else {
		return usageMetricsValueModelToOutput(input.(models.UsageMetrics))
	}
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

func usageMetricsValueModelToOutput(metrics models.UsageMetrics) (*dto.UsageMetricsOutput, error) {
	return usageMetricsPtrModelToOutput(&metrics)
}

func (u UsageMetricsRegistration) EntityInputDtoToEntityModel(input any) any {
	// Les métriques sont généralement créées automatiquement
	return &models.UsageMetrics{}
}

func (u UsageMetricsRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.UsageMetrics{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: u.EntityModelToEntityOutput,
			DtoToModel: u.EntityInputDtoToEntityModel,
			DtoToMap:   u.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.UsageMetricsOutput{},
			OutputDto:      dto.UsageMetricsOutput{},
			InputEditDto:   dto.UsageMetricsOutput{},
		},
	}
}

func (u UsageMetricsRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Les utilisateurs peuvent voir leurs propres métriques
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
