package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterInvoice(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput](
		service,
		"Invoice",
		entityManagementInterfaces.TypedEntityRegistration[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput]{
				ModelToDto: func(invoice *models.Invoice) (dto.InvoiceOutput, error) {
					// Convert the associated UserSubscription using the global typed ops
					var subscriptionOutput dto.UserSubscriptionOutput
					if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("UserSubscription"); ok {
						subOutput, err := ops.ConvertModelToDto(&invoice.UserSubscription)
						if err == nil {
							subscriptionOutput = subOutput.(dto.UserSubscriptionOutput)
						}
					}

					return dto.InvoiceOutput{
						ID:               invoice.ID,
						UserID:           invoice.UserID,
						UserSubscription: subscriptionOutput,
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
				},
				DtoToModel: func(input dto.InvoiceOutput) *models.Invoice {
					// Invoices are created via Stripe webhooks
					return &models.Invoice{}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
