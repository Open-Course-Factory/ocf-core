package paymentController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ==========================================
// Billing Address Controller
// ==========================================

type BillingAddressController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
}

func NewBillingAddressController(db *gorm.DB) *BillingAddressController {
	return &BillingAddressController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
	}
}

func (bac *BillingAddressController) GetUserBillingAddresses(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer depuis le service (retourne des models)
	addresses, err := bac.subscriptionService.GetUserBillingAddresses(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// Convertir vers DTO
	addressesDTO := services.BillingAddressesToDTO(addresses)

	ctx.JSON(http.StatusOK, addressesDTO)
}

func (bac *BillingAddressController) SetDefaultBillingAddress(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	addressID := ctx.Param("id")

	parsedID, ok := parseUUIDParam(ctx, addressID, "Invalid billing address ID format")
	if !ok {
		return
	}

	err := bac.subscriptionService.SetDefaultBillingAddress(userId, parsedID)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Default billing address updated"})
}

func (bac *BillingAddressController) DeleteEntity(ctx *gin.Context) {
	bac.GenericController.DeleteEntity(ctx, true)
}
