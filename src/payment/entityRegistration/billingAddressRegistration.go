package registration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// BillingAddressRegistration
type BillingAddressRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s BillingAddressRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
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
	}
}

func (b BillingAddressRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		address := ptr.(*models.BillingAddress)
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
	})
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
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
