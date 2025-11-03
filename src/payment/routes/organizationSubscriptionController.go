// src/payment/routes/organizationSubscriptionController.go
package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionController interface {
	// Organization subscription management
	CreateOrganizationSubscription(ctx *gin.Context)
	GetOrganizationSubscription(ctx *gin.Context)
	CancelOrganizationSubscription(ctx *gin.Context)

	// User feature access
	GetUserEffectiveFeatures(ctx *gin.Context)
	GetOrganizationFeatures(ctx *gin.Context)
	GetOrganizationUsageLimits(ctx *gin.Context)
}

type organizationSubscriptionController struct {
	db            *gorm.DB
	orgSubService services.OrganizationSubscriptionService
}

func NewOrganizationSubscriptionController(db *gorm.DB) OrganizationSubscriptionController {
	return &organizationSubscriptionController{
		db:            db,
		orgSubService: services.NewOrganizationSubscriptionService(db),
	}
}

// CreateOrganizationSubscription godoc
//
//	@Summary		Create organization subscription
//	@Description	Create a new subscription for an organization. Free plans (price=0) are activated immediately. Paid plans create an incomplete subscription that will be activated by Stripe webhook after payment.
//	@Tags			organization-subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			orgID		path	string									true	"Organization ID"
//	@Param			subscription	body	dto.CreateOrganizationSubscriptionInput	true	"Subscription details"
//	@Security		Bearer
//	@Success		200	{object}	dto.OrganizationSubscriptionOutput
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/organizations/{orgID}/subscribe [post]
func (osc *organizationSubscriptionController) CreateOrganizationSubscription(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Parse organization ID from URL
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	var input dto.CreateOrganizationSubscriptionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Verify user is owner or manager of the organization
	var org organizationModels.Organization
	if err := osc.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organization not found",
		})
		return
	}

	// Check if user has permission to manage organization subscriptions
	var member organizationModels.OrganizationMember
	if err := osc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true).First(&member).Error; err != nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a member of this organization",
		})
		return
	}

	if !member.IsOwner() && !member.IsManager() {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Only organization owners and managers can manage subscriptions",
		})
		return
	}

	// Create the subscription
	subscription, err := osc.orgSubService.CreateOrganizationSubscription(
		orgID,
		input.SubscriptionPlanID,
		userID,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create subscription: " + err.Error(),
		})
		return
	}

	// Convert to output DTO
	output := dto.OrganizationSubscriptionOutput{
		ID:                   subscription.ID,
		OrganizationID:       subscription.OrganizationID,
		SubscriptionPlanID:   subscription.SubscriptionPlanID,
		SubscriptionPlan:     convertSubscriptionPlanToOutput(&subscription.SubscriptionPlan),
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		Quantity:             subscription.Quantity,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
		TrialEnd:             subscription.TrialEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		CancelledAt:          subscription.CancelledAt,
		CreatedAt:            subscription.CreatedAt,
		UpdatedAt:            subscription.UpdatedAt,
	}

	ctx.JSON(http.StatusOK, output)
}

// GetOrganizationSubscription godoc
//
//	@Summary		Get organization subscription
//	@Description	Retrieve the active subscription for an organization
//	@Tags			organization-subscriptions
//	@Produce		json
//	@Param			orgID	path	string	true	"Organization ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.OrganizationSubscriptionOutput
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/organizations/{orgID}/subscription [get]
func (osc *organizationSubscriptionController) GetOrganizationSubscription(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Parse organization ID from URL
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Verify user is member of the organization
	var member organizationModels.OrganizationMember
	if err := osc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true).First(&member).Error; err != nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a member of this organization",
		})
		return
	}

	// Get subscription
	subscription, err := osc.orgSubService.GetOrganizationSubscription(orgID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found for this organization",
		})
		return
	}

	// Convert to output DTO
	output := dto.OrganizationSubscriptionOutput{
		ID:                   subscription.ID,
		OrganizationID:       subscription.OrganizationID,
		SubscriptionPlanID:   subscription.SubscriptionPlanID,
		SubscriptionPlan:     convertSubscriptionPlanToOutput(&subscription.SubscriptionPlan),
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		Quantity:             subscription.Quantity,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
		TrialEnd:             subscription.TrialEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		CancelledAt:          subscription.CancelledAt,
		CreatedAt:            subscription.CreatedAt,
		UpdatedAt:            subscription.UpdatedAt,
	}

	ctx.JSON(http.StatusOK, output)
}

// CancelOrganizationSubscription godoc
//
//	@Summary		Cancel organization subscription
//	@Description	Cancel an organization's subscription (either immediately or at period end)
//	@Tags			organization-subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			orgID	path	string	true	"Organization ID"
//	@Param			cancel	body	dto.UpdateOrganizationSubscriptionInput	true	"Cancel options"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/organizations/{orgID}/subscription [delete]
func (osc *organizationSubscriptionController) CancelOrganizationSubscription(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Parse organization ID from URL
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	var input dto.UpdateOrganizationSubscriptionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Verify user is owner or manager of the organization
	var member organizationModels.OrganizationMember
	if err := osc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true).First(&member).Error; err != nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a member of this organization",
		})
		return
	}

	if !member.IsOwner() && !member.IsManager() {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Only organization owners and managers can manage subscriptions",
		})
		return
	}

	// Cancel subscription
	cancelAtPeriodEnd := false
	if input.CancelAtPeriodEnd != nil {
		cancelAtPeriodEnd = *input.CancelAtPeriodEnd
	}

	err = osc.orgSubService.CancelOrganizationSubscription(orgID, cancelAtPeriodEnd)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to cancel subscription: " + err.Error(),
		})
		return
	}

	message := "Subscription cancelled successfully"
	if cancelAtPeriodEnd {
		message = "Subscription will be cancelled at the end of the current period"
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": message,
	})
}

// GetUserEffectiveFeatures godoc
//
//	@Summary		Get user's effective features
//	@Description	Get aggregated features from all organizations the user belongs to
//	@Tags			organization-subscriptions
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.UserEffectiveFeaturesOutput
//	@Failure		404	{object}	errors.APIError
//	@Router			/users/me/features [get]
func (osc *organizationSubscriptionController) GetUserEffectiveFeatures(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	features, err := osc.orgSubService.GetUserEffectiveFeatures(userID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No organization subscriptions found for user: " + err.Error(),
		})
		return
	}

	// Convert to output DTO
	output := dto.UserEffectiveFeaturesOutput{
		HighestPlan:            convertSubscriptionPlanToOutput(features.HighestPlan),
		AllFeatures:            features.AllFeatures,
		MaxConcurrentTerminals: features.MaxConcurrentTerminals,
		MaxCourses:             features.MaxCourses,
		Organizations:          make([]dto.OrganizationFeatureInfo, len(features.Organizations)),
	}

	for i, org := range features.Organizations {
		output.Organizations[i] = dto.OrganizationFeatureInfo{
			OrganizationID:   org.OrganizationID,
			OrganizationName: org.OrganizationName,
			SubscriptionPlan: convertSubscriptionPlanToOutput(&org.SubscriptionPlan),
			IsOwner:          org.IsOwner,
			IsManager:        org.IsManager,
		}
	}

	ctx.JSON(http.StatusOK, output)
}

// GetOrganizationFeatures godoc
//
//	@Summary		Get organization features
//	@Description	Get the subscription plan features for an organization
//	@Tags			organization-subscriptions
//	@Produce		json
//	@Param			orgID	path	string	true	"Organization ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SubscriptionPlanOutput
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/organizations/{orgID}/features [get]
func (osc *organizationSubscriptionController) GetOrganizationFeatures(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Parse organization ID from URL
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Verify user is member of the organization
	var member organizationModels.OrganizationMember
	if err := osc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true).First(&member).Error; err != nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a member of this organization",
		})
		return
	}

	// Get features
	plan, err := osc.orgSubService.GetOrganizationFeatures(orgID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found for this organization",
		})
		return
	}

	output := convertSubscriptionPlanToOutput(plan)
	ctx.JSON(http.StatusOK, output)
}

// GetOrganizationUsageLimits godoc
//
//	@Summary		Get organization usage limits
//	@Description	Get current usage and limits for an organization
//	@Tags			organization-subscriptions
//	@Produce		json
//	@Param			orgID	path	string	true	"Organization ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.OrganizationLimitsOutput
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/organizations/{orgID}/usage-limits [get]
func (osc *organizationSubscriptionController) GetOrganizationUsageLimits(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Parse organization ID from URL
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Verify user is member of the organization
	var member organizationModels.OrganizationMember
	if err := osc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true).First(&member).Error; err != nil {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not a member of this organization",
		})
		return
	}

	// Get usage limits
	limits, err := osc.orgSubService.GetOrganizationUsageLimits(orgID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get usage limits: " + err.Error(),
		})
		return
	}

	output := dto.OrganizationLimitsOutput{
		OrganizationID:         limits.OrganizationID,
		MaxConcurrentTerminals: limits.MaxConcurrentTerminals,
		MaxCourses:             limits.MaxCourses,
		CurrentTerminals:       limits.CurrentTerminals,
		CurrentCourses:         limits.CurrentCourses,
	}

	ctx.JSON(http.StatusOK, output)
}

// Helper function to convert subscription plan model to output DTO
func convertSubscriptionPlanToOutput(plan *models.SubscriptionPlan) dto.SubscriptionPlanOutput {
	if plan == nil {
		return dto.SubscriptionPlanOutput{}
	}

	// Convert model PricingTiers to DTO PricingTiers
	pricingTiers := make([]dto.PricingTier, len(plan.PricingTiers))
	for i, tier := range plan.PricingTiers {
		pricingTiers[i] = dto.PricingTier{
			MinQuantity: tier.MinQuantity,
			MaxQuantity: tier.MaxQuantity,
			UnitAmount:  tier.UnitAmount,
			Description: tier.Description,
		}
	}

	return dto.SubscriptionPlanOutput{
		ID:                 plan.ID,
		Name:               plan.Name,
		Description:        plan.Description,
		Priority:           plan.Priority,
		StripeProductID:    plan.StripeProductID,
		StripePriceID:      plan.StripePriceID,
		PriceAmount:        plan.PriceAmount,
		Currency:           plan.Currency,
		BillingInterval:    plan.BillingInterval,
		TrialDays:          plan.TrialDays,
		Features:           plan.Features,
		MaxConcurrentUsers: plan.MaxConcurrentUsers,
		MaxCourses:         plan.MaxCourses,
		IsActive:           plan.IsActive,
		RequiredRole:       plan.RequiredRole,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,

		// Terminal-specific limits
		MaxSessionDurationMinutes: plan.MaxSessionDurationMinutes,
		MaxConcurrentTerminals:    plan.MaxConcurrentTerminals,
		AllowedMachineSizes:       plan.AllowedMachineSizes,
		NetworkAccessEnabled:      plan.NetworkAccessEnabled,
		DataPersistenceEnabled:    plan.DataPersistenceEnabled,
		DataPersistenceGB:         plan.DataPersistenceGB,
		AllowedTemplates:          plan.AllowedTemplates,

		// Planned features
		PlannedFeatures: plan.PlannedFeatures,

		// Tiered pricing
		UseTieredPricing: plan.UseTieredPricing,
		PricingTiers:     pricingTiers,
	}
}
