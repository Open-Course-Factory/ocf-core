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
