package paymentController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ==========================================
// Invoice Controller
// ==========================================

type InvoiceController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
	stripeService       services.StripeService
}

func NewInvoiceController(db *gorm.DB) *InvoiceController {
	return &InvoiceController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		stripeService:       services.NewStripeService(db),
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
func (ic *InvoiceController) GetUserInvoices(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer depuis le service (retourne des models)
	invoices, err := ic.subscriptionService.GetUserInvoices(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// Convertir vers DTO
	invoicesDTO := services.InvoicesToDTO(invoices)

	ctx.JSON(http.StatusOK, invoicesDTO)
}

// Get Organization Invoices godoc
//
//	@Summary		Récupérer les factures d'une organisation
//	@Description	Retourne les factures de l'organisation (réservé aux managers et propriétaires)
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Organization ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.InvoiceOutput
//	@Failure		400	{object}	errors.APIError	"Invalid organization ID"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/organizations/{id}/invoices [get]
func (ic *InvoiceController) GetOrganizationInvoices(ctx *gin.Context) {
	// Layer 2 (OrgRole, manager+) has already authorized the caller for this
	// organization before the handler runs, so the :id param is trusted here.
	orgID := ctx.Param("id")

	parsedID, ok := parseUUIDParam(ctx, orgID, "Invalid organization ID format")
	if !ok {
		return
	}

	invoices, err := ic.subscriptionService.GetOrganizationInvoices(parsedID)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	invoicesDTO := services.InvoicesToDTO(invoices)

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
func (ic *InvoiceController) DownloadInvoice(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	invoiceID := ctx.Param("id")

	parsedID, ok := parseUUIDParam(ctx, invoiceID, "Invalid invoice ID format")
	if !ok {
		return
	}

	// Récupérer la facture depuis le service (retourne un model)
	invoice, err := ic.subscriptionService.GetInvoiceByID(parsedID)
	if err != nil {
		errors.Respond(ctx, http.StatusNotFound, "Invoice not found")
		return
	}

	// Vérifier l'accès
	if invoice.UserID != userId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := access.IsAdmin(userRoles)

		if !isAdmin {
			errors.Respond(ctx, http.StatusForbidden, "Access denied to this invoice")
			return
		}
	}

	if invoice.DownloadURL == "" {
		errors.Respond(ctx, http.StatusNotFound, "Download URL not available")
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
func (ic *InvoiceController) SyncUserInvoices(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	result, err := ic.stripeService.SyncUserInvoices(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
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
func (ic *InvoiceController) CleanupInvoices(ctx *gin.Context) {
	// Parse request body
	var input dto.CleanupInvoicesInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		errors.Respond(ctx, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// Call service to perform cleanup
	result, err := ic.stripeService.CleanupIncompleteInvoices(input)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, result)
}
