package paymentController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ==========================================
// Billing Address Controller
// ==========================================

type BillingAddressController interface {
	GetUserBillingAddresses(ctx *gin.Context)
	SetDefaultBillingAddress(ctx *gin.Context)
}

type billingAddressController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
	conversionService   services.ConversionService
}

func NewBillingAddressController(db *gorm.DB) BillingAddressController {
	return &billingAddressController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		conversionService:   services.NewConversionService(),
	}
}

func (bac *billingAddressController) GetUserBillingAddresses(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer depuis le service (retourne des models)
	addresses, err := bac.subscriptionService.GetUserBillingAddresses(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTO
	addressesDTO, err := bac.conversionService.BillingAddressesToDTO(addresses)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert billing addresses",
		})
		return
	}

	ctx.JSON(http.StatusOK, addressesDTO)
}

func (bac *billingAddressController) SetDefaultBillingAddress(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	addressID := ctx.Param("id")

	err := bac.subscriptionService.SetDefaultBillingAddress(userId, uuid.MustParse(addressID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Default billing address updated"})
}

func (bac *billingAddressController) DeleteEntity(ctx *gin.Context) {
	bac.GenericController.DeleteEntity(ctx, true)
}
