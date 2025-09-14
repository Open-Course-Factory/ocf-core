package controller

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/entityManagement/hooks"

	"github.com/gin-gonic/gin"
)

type GenericHooksController interface {
	ListHooks(ctx *gin.Context)
	EnableHook(ctx *gin.Context)
	DisableHook(ctx *gin.Context)
	IsAdmin(ctx *gin.Context) bool
}

type genericHooksController struct{}

func NewGenericHooksController() GenericHooksController {
	return &genericHooksController{}
}

// List Hooks godoc
//
//	@Summary		Lister tous les hooks
//	@Description	Retourne la liste de tous les hooks enregistrés (admin seulement)
//	@Tags			hooks
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	map[string]interface{}
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/hooks [get]
func (hc *genericHooksController) ListHooks(ctx *gin.Context) {
	// Vérifier les permissions admin
	if !hc.IsAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	// Pour l'instant, retourner des infos basiques
	// Dans une vraie implémentation, on ajouterait une méthode GetAllHooks au registre
	hooksInfo := map[string]interface{}{
		"registered_hooks": []map[string]interface{}{
			{
				"name":        "stripe_subscription_plan_sync",
				"entity":      "SubscriptionPlan",
				"types":       []string{"after_create", "after_update", "after_delete"},
				"enabled":     true,
				"description": "Synchronizes SubscriptionPlan with Stripe",
			},
			{
				"name":        "usage_metrics_notifications",
				"entity":      "UsageMetrics",
				"types":       []string{"after_create", "after_update"},
				"enabled":     true,
				"description": "Sends notifications when usage limits are reached",
			},
		},
		"total_hooks": 2,
	}

	ctx.JSON(http.StatusOK, hooksInfo)
}

// Enable Hook godoc
//
//	@Summary		Activer un hook
//	@Description	Active un hook spécifique (admin seulement)
//	@Tags			hooks
//	@Accept			json
//	@Produce		json
//	@Param			hook_name	path	string	true	"Hook name"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/hooks/{hook_name}/enable [post]
func (hc *genericHooksController) EnableHook(ctx *gin.Context) {
	if !hc.IsAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	hookName := ctx.Param("hook_name")
	err := hooks.GlobalHookRegistry.EnableHook(hookName, true)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Hook enabled successfully",
		"hook":    hookName,
		"status":  "enabled",
	})
}

// Disable Hook godoc
//
//	@Summary		Désactiver un hook
//	@Description	Désactive un hook spécifique (admin seulement)
//	@Tags			hooks
//	@Accept			json
//	@Produce		json
//	@Param			hook_name	path	string	true	"Hook name"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/hooks/{hook_name}/disable [post]
func (hc *genericHooksController) DisableHook(ctx *gin.Context) {
	if !hc.IsAdmin(ctx) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Admin access required",
		})
		return
	}

	hookName := ctx.Param("hook_name")
	err := hooks.GlobalHookRegistry.EnableHook(hookName, false)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Hook disabled successfully",
		"hook":    hookName,
		"status":  "disabled",
	})
}

func (hc *genericHooksController) IsAdmin(ctx *gin.Context) bool {
	userRoles := ctx.GetStringSlice("userRoles")
	for _, role := range userRoles {
		if role == "administrator" {
			return true
		}
	}
	return false
}
