package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Invoice Controller
type InvoiceController interface {
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)

	GetUserInvoices(ctx *gin.Context)
	DownloadInvoice(ctx *gin.Context)
}

type invoiceController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
}

func NewInvoiceController(db *gorm.DB) InvoiceController {
	return &invoiceController{
		GenericController:   controller.NewGenericController(db),
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// Get User Invoices godoc
//
//	@Summary		Récupérer les factures de l'utilisateur
//	@Description	Retourne toutes les factures de l'utilisateur connecté
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			limit	query	int	false	"Limit number of invoices"
//	@Security		Bearer
//	@Success		200	{array}		dto.InvoiceOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/invoices/user [get]
func (ic *invoiceController) GetUserInvoices(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	invoices, err := ic.subscriptionService.GetUserInvoices(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, invoices)
}

// Download Invoice godoc
//
//	@Summary		Télécharger une facture
//	@Description	Redirige vers l'URL de téléchargement de la facture
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Invoice ID"
//	@Security		Bearer
//	@Success		302	{object}	string	"Redirect to download URL"
//	@Failure		404	{object}	errors.APIError	"Invoice not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/invoices/{id}/download [get]
func (ic *invoiceController) DownloadInvoice(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	invoiceID := ctx.Param("id")

	invoice, err := ic.subscriptionService.GetInvoiceByID(uuid.MustParse(invoiceID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Invoice not found",
		})
		return
	}

	if invoice.UserID != userId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied to this invoice",
			})
			return
		}
	}

	if invoice.DownloadURL == "" {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Download URL not available",
		})
		return
	}

	// Rediriger vers l'URL de téléchargement Stripe
	ctx.Redirect(http.StatusFound, invoice.DownloadURL)
}
