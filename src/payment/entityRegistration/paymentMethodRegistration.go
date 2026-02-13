package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterPaymentMethod(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.PaymentMethod, dto.CreatePaymentMethodInput, dto.CreatePaymentMethodInput, dto.PaymentMethodOutput](
		service,
		"PaymentMethod",
		entityManagementInterfaces.TypedEntityRegistration[models.PaymentMethod, dto.CreatePaymentMethodInput, dto.CreatePaymentMethodInput, dto.PaymentMethodOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.PaymentMethod, dto.CreatePaymentMethodInput, dto.CreatePaymentMethodInput, dto.PaymentMethodOutput]{
				ModelToDto: func(pm *models.PaymentMethod) (dto.PaymentMethodOutput, error) {
					return dto.PaymentMethodOutput{
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
				},
				DtoToModel: func(input dto.CreatePaymentMethodInput) *models.PaymentMethod {
					return &models.PaymentMethod{
						StripePaymentMethodID: input.StripePaymentMethodID,
						IsDefault:             input.SetAsDefault,
						IsActive:              true,
					}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
