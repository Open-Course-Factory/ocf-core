// src/payment/entityRegistration/subscriptionRegistration.go
package registration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// UserSubscriptionRegistration
type UserSubscriptionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UserSubscriptionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		subscription := ptr.(*models.UserSubscription)
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
	})
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
		EntitySubEntities: []any{
			models.SubscriptionPlan{},
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
