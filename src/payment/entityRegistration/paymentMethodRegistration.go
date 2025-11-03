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

// PaymentMethodRegistration
type PaymentMethodRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (p PaymentMethodRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "payment-methods",
		EntityName: "PaymentMethod",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les moyens de paiement",
			Description: "Retourne la liste de tous les moyens de paiement disponibles",
			Tags:        []string{"payment-methods"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un moyen de paiement",
			Description: "Retourne les détails complets d'un moyen de paiement spécifique",
			Tags:        []string{"payment-methods"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un moyen de paiement",
			Description: "Crée un nouveau moyens de paiement",
			Tags:        []string{"payment-methods"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un moyen de paiement",
			Description: "Supprime un moyen de paiement",
			Tags:        []string{"payment-methods"},
			Security:    true,
		},
	}
}

func (p PaymentMethodRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		pm := ptr.(*models.PaymentMethod)
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
	})
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
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
