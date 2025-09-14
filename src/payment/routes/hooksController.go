// src/payment/routes/hooksController.go - Contrôleur pour gérer les hooks (admin seulement)
package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	paymentHooks "soli/formations/src/payment/hooks"

	"github.com/gin-gonic/gin"
)

type HooksController interface {
	ToggleStripeSync(ctx *gin.Context)
}

type hooksController struct {
	genericHookController controller.GenericHooksController
}

func NewHooksController() HooksController {
	return &hooksController{
		genericHookController: controller.NewGenericHooksController(),
	}
}

type Input struct {
	Enable *bool `json:"enable" binding:"required"`
}

// Toggle Stripe Sync godoc
//
//	@Summary		Activer/désactiver la synchronisation Stripe
//	@Description	Raccourci pour activer/désactiver la synchronisation avec Stripe
//	@Tags			hooks
//	@Accept			json
//	@Produce		json
//	@Param			enable	body	paymentController.Input	true	"Enable or disable"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/hooks/stripe/toggle [post]
func (hc *hooksController) ToggleStripeSync(ctx *gin.Context) {
	if !hc.genericHookController.IsAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	var input Input

	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	err := paymentHooks.EnableStripeSync(*input.Enable)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	status := "disabled"
	if *input.Enable {
		status = "enabled"
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":     "Stripe synchronization updated",
		"stripe_sync": status,
		"hook_name":   "stripe_subscription_plan_sync",
	})
}
