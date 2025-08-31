package paymentController

import (
	"net/http"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Contrôleur pour les adresses de facturation
type BillingAddressController interface {
	AddEntity(ctx *gin.Context)
	EditEntity(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)

	GetUserBillingAddresses(ctx *gin.Context)
	SetDefaultBillingAddress(ctx *gin.Context)
}

type billingAddressController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
}

func NewBillingAddressController(db *gorm.DB) BillingAddressController {
	return &billingAddressController{
		GenericController:   controller.NewGenericController(db),
		subscriptionService: services.NewSubscriptionService(db),
	}
}

func (bac *billingAddressController) GetUserBillingAddresses(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	addresses, err := bac.subscriptionService.GetUserBillingAddresses(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, addresses)
}

func (bac *billingAddressController) SetDefaultBillingAddress(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	addressID := ctx.Param("id")

	err := bac.subscriptionService.SetDefaultBillingAddress(userId, uuid.MustParse(addressID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Default billing address updated"})
}

func (bac *billingAddressController) DeleteEntity(ctx *gin.Context) {
	bac.GenericController.DeleteEntity(ctx, true)
}
