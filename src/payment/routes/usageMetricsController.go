package paymentController

import (
	"net/http"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Contrôleur pour les métriques d'utilisation
type UsageMetricsController interface {
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)
	EditEntity(ctx *gin.Context)

	GetUserUsageMetrics(ctx *gin.Context)
	IncrementUsageMetric(ctx *gin.Context)
	ResetUserUsage(ctx *gin.Context)
}

type usageMetricsController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
}

func NewUsageMetricsController(db *gorm.DB) UsageMetricsController {
	return &usageMetricsController{
		GenericController:   controller.NewGenericController(db),
		subscriptionService: services.NewSubscriptionService(db),
	}
}

func (umc *usageMetricsController) GetUserUsageMetrics(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Pour les admins, permettre de voir les métriques d'autres utilisateurs
	targetUserID := ctx.Query("user_id")
	if targetUserID != "" {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
		userId = targetUserID
	}

	metrics, err := umc.subscriptionService.GetUserUsageMetrics(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, metrics)
}

func (umc *usageMetricsController) IncrementUsageMetric(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input struct {
		MetricType string `json:"metric_type" binding:"required"`
		Increment  int64  `json:"increment"`
	}

	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Increment <= 0 {
		input.Increment = 1
	}

	err := umc.subscriptionService.IncrementUsage(userId, input.MetricType, input.Increment)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Usage metric incremented"})
}

func (umc *usageMetricsController) ResetUserUsage(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Seuls les admins peuvent reset les métriques d'autres utilisateurs
	targetUserID := ctx.Query("user_id")
	if targetUserID != "" {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
		userId = targetUserID
	}

	err := umc.subscriptionService.ResetMonthlyUsage(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "User usage metrics reset"})
}
