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
// Usage Metrics Controller
// ==========================================

type UsageMetricsController interface {
	GetUserUsageMetrics(ctx *gin.Context)
	IncrementUsageMetric(ctx *gin.Context)
	ResetUserUsage(ctx *gin.Context)
}

type usageMetricsController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
	conversionService   services.ConversionService
}

func NewUsageMetricsController(db *gorm.DB) UsageMetricsController {
	return &usageMetricsController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		conversionService:   services.NewConversionService(),
	}
}

func (umc *usageMetricsController) GetUserUsageMetrics(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Manual admin check for user targeting: this endpoint is SelfScoped (member-accessible),
	// unlike increment/reset which are AdminOnly. So we must check roles manually when
	// a user_id param targets another user's metrics.
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
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied",
			})
			return
		}
		userId = targetUserID
	}

	// Récupérer depuis le service (retourne des models)
	metrics, err := umc.subscriptionService.GetUserUsageMetrics(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTO
	metricsDTO, err := umc.conversionService.UsageMetricsListToDTO(metrics)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert usage metrics",
		})
		return
	}

	ctx.JSON(http.StatusOK, metricsDTO)
}

func (umc *usageMetricsController) IncrementUsageMetric(ctx *gin.Context) {
	// Admin-only endpoint (enforced by Layer 1 + Layer 2 middleware).
	// Optionally target a specific user via ?user_id= query param.
	userId := ctx.GetString("userId")
	if targetUserID := ctx.Query("user_id"); targetUserID != "" {
		userId = targetUserID
	}

	var input struct {
		MetricType string `json:"metric_type" binding:"required"`
		Increment  int64  `json:"increment"`
	}

	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	if input.Increment <= 0 {
		input.Increment = 1
	}

	err := umc.subscriptionService.IncrementUsage(userId, input.MetricType, input.Increment)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Usage metric incremented"})
}

func (umc *usageMetricsController) ResetUserUsage(ctx *gin.Context) {
	// Admin-only endpoint (enforced by Layer 1 + Layer 2 middleware).
	// Optionally target a specific user via ?user_id= query param.
	userId := ctx.GetString("userId")
	if targetUserID := ctx.Query("user_id"); targetUserID != "" {
		userId = targetUserID
	}

	err := umc.subscriptionService.ResetMonthlyUsage(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "User usage metrics reset"})
}
