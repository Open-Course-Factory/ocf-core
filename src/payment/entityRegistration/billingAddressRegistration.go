package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterBillingAddress(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.BillingAddress, dto.CreateBillingAddressInput, dto.UpdateBillingAddressInput, dto.BillingAddressOutput](
		service,
		"BillingAddress",
		entityManagementInterfaces.TypedEntityRegistration[models.BillingAddress, dto.CreateBillingAddressInput, dto.UpdateBillingAddressInput, dto.BillingAddressOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.BillingAddress, dto.CreateBillingAddressInput, dto.UpdateBillingAddressInput, dto.BillingAddressOutput]{
				ModelToDto: func(address *models.BillingAddress) (dto.BillingAddressOutput, error) {
					return dto.BillingAddressOutput{
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
				},
				DtoToModel: func(input dto.CreateBillingAddressInput) *models.BillingAddress {
					return &models.BillingAddress{
						Line1:      input.Line1,
						Line2:      input.Line2,
						City:       input.City,
						State:      input.State,
						PostalCode: input.PostalCode,
						Country:    input.Country,
						IsDefault:  input.SetDefault,
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
				Tag:        "billing-addresses",
				EntityName: "BillingAddress",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer toutes les adresses de facturation",
					Description: "Retourne la liste de toutes les adresses de facturation disponibles",
					Tags:        []string{"billing-addresses"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer une adresse de facturation",
					Description: "Retourne les détails complets d'une adresse de facturation spécifique",
					Tags:        []string{"billing-addresses"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Créer une adresse de facturation",
					Description: "Crée une nouvelle adresse de facturation",
					Tags:        []string{"billing-addresses"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Mettre à jour une adresse de facturation",
					Description: "Modifie une adresse de facturation existante",
					Tags:        []string{"billing-addresses"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Supprimer une adresse de facturation",
					Description: "Supprime une adresse de facturation",
					Tags:        []string{"billing-addresses"},
					Security:    true,
				},
			},
		},
	)
}
