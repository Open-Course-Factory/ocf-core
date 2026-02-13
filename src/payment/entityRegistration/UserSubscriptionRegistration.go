package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterUserSubscription(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.UserSubscription, dto.CreateUserSubscriptionInput, dto.UpdateUserSubscriptionInput, dto.UserSubscriptionOutput](
		service,
		"UserSubscription",
		entityManagementInterfaces.TypedEntityRegistration[models.UserSubscription, dto.CreateUserSubscriptionInput, dto.UpdateUserSubscriptionInput, dto.UserSubscriptionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.UserSubscription, dto.CreateUserSubscriptionInput, dto.UpdateUserSubscriptionInput, dto.UserSubscriptionOutput]{
				ModelToDto: func(subscription *models.UserSubscription) (dto.UserSubscriptionOutput, error) {
					return dto.UserSubscriptionOutput{
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
				},
				DtoToModel: func(input dto.CreateUserSubscriptionInput) *models.UserSubscription {
					return &models.UserSubscription{
						UserID:             input.UserID,
						SubscriptionPlanID: input.SubscriptionPlanID,
						Status:             "incomplete",
					}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SubEntities: []any{models.SubscriptionPlan{}},
		},
	)
}
