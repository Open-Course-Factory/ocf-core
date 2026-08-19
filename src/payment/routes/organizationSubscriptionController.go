// src/payment/routes/organizationSubscriptionController.go
package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationSubscriptionController interface {
	// Organization subscription management
	CreateOrganizationSubscription(ctx *gin.Context)
	GetOrganizationSubscription(ctx *gin.Context)
	CancelOrganizationSubscription(ctx *gin.Context)

	// Admin bulk access
	GetAllOrganizationSubscriptions(ctx *gin.Context)

	// User feature access
	GetUserEffectiveFeatures(ctx *gin.Context)
	GetOrganizationFeatures(ctx *gin.Context)
	GetOrganizationUsageLimits(ctx *gin.Context)
}

type organizationSubscriptionController struct {
	db                   *gorm.DB
	orgSubService        services.OrganizationSubscriptionService
	effectivePlanService services.EffectivePlanService
}

func NewOrganizationSubscriptionController(db *gorm.DB) OrganizationSubscriptionController {
	return &organizationSubscriptionController{
		db:                   db,
		orgSubService:        services.NewOrganizationSubscriptionService(db),
		effectivePlanService: services.NewEffectivePlanService(db),
	}
}

// isAdmin checks if the current user has the administrator role
func isAdmin(ctx *gin.Context) bool {
	userRoles := ctx.GetStringSlice("userRoles")
	for _, role := range userRoles {
		if role == "administrator" {
			return true
		}
	}
	return false
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

	// Verify organization exists
	var org organizationModels.Organization
	if err := osc.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organization not found",
		})
		return
	}

	// Create the subscription
	subscription, err := osc.orgSubService.CreateOrganizationSubscription(
		orgID,
		input.SubscriptionPlanID,
		userID,
		isAdmin(ctx),
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
		SubscriptionPlan:     EmbeddedPlanOutput(&subscription.SubscriptionPlan),
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
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
		SubscriptionPlan:     EmbeddedPlanOutput(&subscription.SubscriptionPlan),
		StripeSubscriptionID: subscription.StripeSubscriptionID,
		StripeCustomerID:     subscription.StripeCustomerID,
		Status:               subscription.Status,
		CurrentPeriodStart:   subscription.CurrentPeriodStart,
		CurrentPeriodEnd:     subscription.CurrentPeriodEnd,
		CancelAtPeriodEnd:    subscription.CancelAtPeriodEnd,
		CancelledAt:          subscription.CancelledAt,
		CreatedAt:            subscription.CreatedAt,
		UpdatedAt:            subscription.UpdatedAt,
	}

	ctx.JSON(http.StatusOK, output)
}

// GetAllOrganizationSubscriptions godoc
//
//	@Summary		Get all active organization subscriptions
//	@Description	Admin endpoint to retrieve all active/trialing organization subscriptions
//	@Tags			organization-subscriptions
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	map[string][]dto.OrganizationSubscriptionOutput
//	@Failure		403	{object}	errors.APIError
//	@Router			/admin/organizations/subscriptions [get]
func (osc *organizationSubscriptionController) GetAllOrganizationSubscriptions(ctx *gin.Context) {
	subscriptions, err := osc.orgSubService.GetAllActiveOrganizationSubscriptions()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve subscriptions: " + err.Error(),
		})
		return
	}

	outputs := make([]dto.OrganizationSubscriptionOutput, len(subscriptions))
	for i, sub := range subscriptions {
		outputs[i] = dto.OrganizationSubscriptionOutput{
			ID:                   sub.ID,
			OrganizationID:       sub.OrganizationID,
			SubscriptionPlanID:   sub.SubscriptionPlanID,
			SubscriptionPlan:     EmbeddedPlanOutput(&sub.SubscriptionPlan),
			StripeSubscriptionID: sub.StripeSubscriptionID,
			StripeCustomerID:     sub.StripeCustomerID,
			Status:               sub.Status,
			CurrentPeriodStart:   sub.CurrentPeriodStart,
			CurrentPeriodEnd:     sub.CurrentPeriodEnd,
			CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
			CancelledAt:          sub.CancelledAt,
			CreatedAt:            sub.CreatedAt,
			UpdatedAt:            sub.UpdatedAt,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"data": outputs})
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

	// Check for optional organization_id query param for org-context-aware resolution
	if orgIDStr := ctx.Query("organization_id"); orgIDStr != "" {
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid organization_id format",
			})
			return
		}

		// Resolve features for this specific org context
		result, err := osc.effectivePlanService.GetUserEffectivePlan(userID, &orgID)
		if err != nil {
			utils.Warn("Failed to get effective plan for user %s org %s: %v", userID, orgID.String(), err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "No subscription found for this organization context",
			})
			return
		}

		effectivePlan := convertSubscriptionPlanToOutput(result.Plan)
		// Reuses the plan just resolved rather than resolving again: a second
		// resolution can legitimately return a different plan, and the verdict would
		// then not describe the plan reported beside it. The organization is passed
		// too, because in an org context the plan is only half the rule — a personal
		// organization runs no classes whatever plan its owner bought (#475).
		verdict := osc.effectivePlanService.ClassroomEntitlementInOrg(userID, orgID, result.Plan)
		output := dto.UserEffectiveFeaturesOutput{
			UserID:                  userID,
			EffectiveFeatures:       effectivePlan,
			SourceOrganizations:     nil, // Single org context, no aggregation
			HasPersonalSubscription: result.Source == services.PlanSourcePersonal,
			CanRunClassrooms:        verdict.Allowed,
			ClassroomDeniedReason:   verdict.Reason,
		}
		ctx.JSON(http.StatusOK, output)
		return
	}

	// No org context — return aggregated features from all organizations (backward compat)
	features, err := osc.orgSubService.GetUserEffectiveFeatures(userID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No organization subscriptions found for user: " + err.Error(),
		})
		return
	}

	// Build effective_features: the highest plan, with the union of boolean features
	// from all plans. Numeric limits come from HighestPlan only — no max-aggregation,
	// so the response is internally consistent (machine sizes, terminal cap, etc. all
	// originate from the same plan).
	// HighestPlan can legitimately be nil — every contributing subscription may
	// point at a deleted plan. Converting nil produced a zero-value plan that
	// looked real to the frontend, whose gray-out logic then hid everything (#451).
	if features.HighestPlan == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No plan applies to this user",
		})
		return
	}
	effectivePlan := convertSubscriptionPlanToOutput(features.HighestPlan)
	effectivePlan.Features = features.AllFeatures

	// Build source_organizations
	sourceOrgs := make([]dto.OrganizationFeatureSourceInfo, len(features.Organizations))
	for i, org := range features.Organizations {
		role := "member"
		if org.IsOwner {
			role = "owner"
		} else if org.IsManager {
			role = "manager"
		}
		sourceOrgs[i] = dto.OrganizationFeatureSourceInfo{
			OrganizationID:       org.OrganizationID,
			OrganizationName:     org.OrganizationName,
			Role:                 role,
			ContributingFeatures: services.DerivePlanEntitlements(&org.SubscriptionPlan),
		}
	}

	// The verdict comes from HighestPlan, NOT from the union in
	// effectivePlan.Features. The union answers "is this available to them
	// somewhere", which is the gray-out question; entitlement is a property of the
	// one plan that actually applies.
	verdict := services.ClassroomEntitlementFor(features.HighestPlan)

	output := dto.UserEffectiveFeaturesOutput{
		UserID:                  userID,
		EffectiveFeatures:       effectivePlan,
		SourceOrganizations:     sourceOrgs,
		HasPersonalSubscription: features.HasPersonalSubscription,
		CanRunClassrooms:        verdict.Allowed,
		ClassroomDeniedReason:   verdict.Reason,
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

	// Scoped to the caller: a team org has no plan of its own, so "what can be
	// done here" is answered by the acting member's entitlement (#451).
	plan, err := osc.orgSubService.GetOrganizationFeatures(orgID, ctx.GetString("userId"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No plan applies for this user in this organization",
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
		OrganizationID:   limits.OrganizationID,
		CurrentTerminals: limits.CurrentTerminals,
		CurrentCourses:   limits.CurrentCourses,
	}

	ctx.JSON(http.StatusOK, output)
}

// EmbeddedPlanOutput converts a plan association into an optional output.
//
// It returns nil when the association never loaded. GORM's Preload honours soft
// deletes, so a subscription pointing at a deleted plan keeps the zero-value
// struct — and converting that yields {name: "", currency: "", price_amount: 0},
// which is indistinguishable from a real free plan and blew up the admin
// organizations panel when "" reached Intl.NumberFormat.
//
// Absence is detected on the ID, not the amount: a 0 EUR plan is a real plan.
func EmbeddedPlanOutput(plan *models.SubscriptionPlan) *dto.SubscriptionPlanOutput {
	if plan == nil || plan.ID == uuid.Nil {
		return nil
	}
	out := convertSubscriptionPlanToOutput(plan)
	return &out
}

// convertSubscriptionPlanToOutput delegates to the single producer,
// services.SubscriptionPlanToOutput.
//
// It used to build the DTO itself and silently dropped IsCatalog and the five
// capability flags, so this package reported hidden plans as catalog ones and
// group-management plans as lacking it (#454).
func convertSubscriptionPlanToOutput(plan *models.SubscriptionPlan) dto.SubscriptionPlanOutput {
	return services.SubscriptionPlanToOutput(plan)
}
