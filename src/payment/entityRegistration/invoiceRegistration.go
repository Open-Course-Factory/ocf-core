package registration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

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
