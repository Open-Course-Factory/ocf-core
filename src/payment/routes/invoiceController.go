package paymentController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ==========================================
// Invoice Controller
// ==========================================

type InvoiceController interface {
	GetUserInvoices(ctx *gin.Context)
	DownloadInvoice(ctx *gin.Context)
	SyncUserInvoices(ctx *gin.Context)
	CleanupInvoices(ctx *gin.Context)
}

type invoiceController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
	stripeService       services.StripeService
	conversionService   services.ConversionService
}

func NewInvoiceController(db *gorm.DB) InvoiceController {
	return &invoiceController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		stripeService:       services.NewStripeService(db),
		conversionService:   services.NewConversionService(),
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

	// Récupérer depuis le service (retourne des models)
	invoices, err := ic.subscriptionService.GetUserInvoices(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTO
	invoicesDTO, err := ic.conversionService.InvoicesToDTO(invoices)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert invoices",
		})
		return
	}

	ctx.JSON(http.StatusOK, invoicesDTO)
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

	// Récupérer la facture depuis le service (retourne un model)
	invoice, err := ic.subscriptionService.GetInvoiceByID(uuid.MustParse(invoiceID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Invoice not found",
		})
		return
	}

	// Vérifier l'accès
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

// Sync User Invoices godoc
//
//	@Summary		Synchroniser les factures de l'utilisateur depuis Stripe
//	@Description	Récupère toutes les factures de Stripe et les synchronise dans la base de données locale
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	services.SyncInvoicesResult
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/invoices/sync [post]
func (ic *invoiceController) SyncUserInvoices(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	result, err := ic.stripeService.SyncUserInvoices(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// Cleanup Invoices godoc
//
//	@Summary		Nettoyer les factures incomplètes (Admin uniquement)
//	@Description	Annule ou marque comme non-recouvrable les factures incomplètes anciennes. Supporte le mode dry-run pour prévisualiser les changements.
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.CleanupInvoicesInput	true	"Cleanup configuration"
//	@Security		Bearer
//	@Success		200	{object}	dto.CleanupInvoicesResult	"Cleanup results"
//	@Failure		400	{object}	errors.APIError				"Invalid request"
//	@Failure		403	{object}	errors.APIError				"Access denied (admin only)"
//	@Failure		500	{object}	errors.APIError				"Internal server error"
//	@Router			/invoices/admin/cleanup [post]
func (ic *invoiceController) CleanupInvoices(ctx *gin.Context) {
	// Check if user has administrator role
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
			ErrorMessage: "Administrator role required to cleanup invoices",
		})
		return
	}

	// Parse request body
	var input dto.CleanupInvoicesInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Call service to perform cleanup
	result, err := ic.stripeService.CleanupIncompleteInvoices(input)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}
