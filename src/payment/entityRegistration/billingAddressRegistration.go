package registration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

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
