package registration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// InvoiceRegistration
type InvoiceRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (i InvoiceRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "invoices",
		EntityName: "Invoice",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer toutes les factures",
			Description: "Retourne la liste de toutes les factures disponibles",
			Tags:        []string{"invoices"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer une facture",
			Description: "Retourne les détails complets d'une facture spécifique",
			Tags:        []string{"invoices"},
			Security:    true,
		},
	}
}

func (i InvoiceRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		invoice := ptr.(*models.Invoice)

		// Convert the associated UserSubscription using its registration
		userSubReg := UserSubscriptionRegistration{}
		subscriptionOutput, err := userSubReg.EntityModelToEntityOutput(&invoice.UserSubscription)
		if err != nil {
			return nil, err
		}

		return &dto.InvoiceOutput{
			ID:               invoice.ID,
			UserID:           invoice.UserID,
			UserSubscription: *subscriptionOutput.(*dto.UserSubscriptionOutput),
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
	})
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
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
