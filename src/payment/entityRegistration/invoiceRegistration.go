package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

func RegisterInvoice(service *ems.EntityRegistrationService) {
	conversionService := paymentServices.NewConversionService()

	ems.RegisterTypedEntity[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput](
		service,
		"Invoice",
		entityManagementInterfaces.TypedEntityRegistration[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Invoice, dto.InvoiceOutput, dto.InvoiceOutput, dto.InvoiceOutput]{
				ModelToDto: func(invoice *models.Invoice) (dto.InvoiceOutput, error) {
					// Single source of truth: delegate to ConversionService.InvoiceToDTO so
					// the generic entity endpoint and the invoice controllers share ONE
					// Invoice→DTO mapping (organization fields included). A second local
					// mapper here would silently drift when the DTO gains fields.
					output, err := conversionService.InvoiceToDTO(invoice)
					if err != nil {
						return dto.InvoiceOutput{}, err
					}
					if output == nil {
						return dto.InvoiceOutput{}, nil
					}
					return *output, nil
				},
				DtoToModel: func(input dto.InvoiceOutput) *models.Invoice {
					// Invoices are created via Stripe webhooks
					return &models.Invoice{}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "()",
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
