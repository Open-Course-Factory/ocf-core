// src/payment/routes/bulkLicenseController.go
package paymentController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BulkLicenseController interface {
	CreateBulkCheckoutSession(ctx *gin.Context)
	PurchaseBulkLicenses(ctx *gin.Context)
	GetMyBatches(ctx *gin.Context)
	GetBatchDetails(ctx *gin.Context)
	GetBatchLicenses(ctx *gin.Context)
	AssignLicense(ctx *gin.Context)
	RevokeLicense(ctx *gin.Context)
	UpdateBatchQuantity(ctx *gin.Context)
	PermanentlyDeleteBatch(ctx *gin.Context)
}

type bulkLicenseController struct {
	db                *gorm.DB
	bulkService       services.BulkLicenseService
	stripeService     services.StripeService
	conversionService services.ConversionService
}

func NewBulkLicenseController(db *gorm.DB) BulkLicenseController {
	return &bulkLicenseController{
		db:                db,
		bulkService:       services.NewBulkLicenseService(db),
		stripeService:     services.NewStripeService(db),
		conversionService: services.NewConversionService(),
	}
}

// CreateBulkCheckoutSession godoc
//
//	@Summary		Create Stripe checkout session for bulk license purchase
//	@Description	Creates a Stripe checkout session for purchasing multiple licenses. User will be redirected to Stripe to complete payment. Licenses are activated after successful payment via webhook.
//	@Tags			bulk-licenses
//	@Accept			json
//	@Produce		json
//	@Param			checkout	body		dto.CreateBulkCheckoutSessionInput	true	"Bulk checkout session parameters"
//	@Security		Bearer
//	@Success		200			{object}	dto.CheckoutSessionOutput	"Checkout session created - redirect user to the URL"
//	@Failure		400			{object}	errors.APIError				"Invalid input"
//	@Failure		404			{object}	errors.APIError				"Subscription plan not found"
//	@Failure		500			{object}	errors.APIError				"Failed to create checkout session"
//	@Router			/subscription-batches/create-checkout-session [post]
func (bc *bulkLicenseController) CreateBulkCheckoutSession(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	var input dto.CreateBulkCheckoutSessionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Invalid input: %v", err),
		})
		return
	}

	// Create Stripe checkout session
	checkoutSession, err := bc.stripeService.CreateBulkCheckoutSession(userID, input)
	if err != nil {
		utils.Error("Failed to create bulk checkout session for user %s: %v", userID, err)

		statusCode := http.StatusInternalServerError
		if err.Error() == "subscription plan not found" || err.Error() == "Subscription plan not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "subscription plan is not active" {
			statusCode = http.StatusBadRequest
		}

		ctx.JSON(statusCode, &errors.APIError{
			ErrorCode:    statusCode,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, checkoutSession)
}

// PurchaseBulkLicenses godoc
//
//	@Summary		Purchase multiple licenses in bulk
//	@Description	Create a bulk license purchase with tiered pricing. Requires group_management feature in user's plan.
//	@Tags			bulk-licenses
//	@Accept			json
//	@Produce		json
//	@Param			purchase	body		dto.BulkPurchaseInput	true	"Bulk purchase details"
//	@Security		Bearer
//	@Success		201	{object}	dto.SubscriptionBatchOutput
//	@Failure		400	{object}	errors.APIError	"Invalid input"
//	@Failure		403	{object}	errors.APIError	"Feature not available in your plan"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-subscriptions/purchase-bulk [post]
func (c *bulkLicenseController) PurchaseBulkLicenses(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	var input dto.BulkPurchaseInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Create the bulk purchase
	batch, _, err := c.bulkService.PurchaseBulkLicenses(userID, input)
	if err != nil {
		utils.Error("Failed to create bulk purchase: %v", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to create bulk purchase: %v", err),
		})
		return
	}

	// Convert to output DTO
	batchOutput := dto.SubscriptionBatchOutput{
		ID:                       batch.ID,
		PurchaserUserID:          batch.PurchaserUserID,
		SubscriptionPlanID:       batch.SubscriptionPlanID,
		GroupID:                  batch.GroupID,
		StripeSubscriptionID:     batch.StripeSubscriptionID,
		StripeSubscriptionItemID: batch.StripeSubscriptionItemID,
		TotalQuantity:            batch.TotalQuantity,
		AssignedQuantity:         batch.AssignedQuantity,
		AvailableQuantity:        batch.TotalQuantity - batch.AssignedQuantity,
		Status:                   batch.Status,
		CurrentPeriodStart:       batch.CurrentPeriodStart,
		CurrentPeriodEnd:         batch.CurrentPeriodEnd,
		CancelledAt:              batch.CancelledAt,
		CreatedAt:                batch.CreatedAt,
		UpdatedAt:                batch.UpdatedAt,
	}

	// Convert subscription plan
	planOutput, _ := c.conversionService.SubscriptionPlanToDTO(&batch.SubscriptionPlan)
	batchOutput.SubscriptionPlan = *planOutput

	ctx.JSON(http.StatusCreated, batchOutput)
}

// GetMyBatches godoc
//
//	@Summary		Get all bulk purchases by current user
//	@Description	Returns all bulk license batches purchased by the authenticated user
//	@Tags			bulk-licenses
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.SubscriptionBatchOutput
//	@Failure		500	{object}	errors.APIError
//	@Router			/subscription-batches [get]
func (c *bulkLicenseController) GetMyBatches(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	batches, err := c.bulkService.GetBatchesByPurchaser(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve batches",
		})
		return
	}

	// Convert to output DTOs
	var output []dto.SubscriptionBatchOutput
	for _, batch := range *batches {
		batchOutput := dto.SubscriptionBatchOutput{
			ID:                       batch.ID,
			PurchaserUserID:          batch.PurchaserUserID,
			SubscriptionPlanID:       batch.SubscriptionPlanID,
			GroupID:                  batch.GroupID,
			StripeSubscriptionID:     batch.StripeSubscriptionID,
			StripeSubscriptionItemID: batch.StripeSubscriptionItemID,
			TotalQuantity:            batch.TotalQuantity,
			AssignedQuantity:         batch.AssignedQuantity,
			AvailableQuantity:        batch.TotalQuantity - batch.AssignedQuantity,
			Status:                   batch.Status,
			CurrentPeriodStart:       batch.CurrentPeriodStart,
			CurrentPeriodEnd:         batch.CurrentPeriodEnd,
			CancelledAt:              batch.CancelledAt,
			CreatedAt:                batch.CreatedAt,
			UpdatedAt:                batch.UpdatedAt,
		}

		planOutput, _ := c.conversionService.SubscriptionPlanToDTO(&batch.SubscriptionPlan)
		batchOutput.SubscriptionPlan = *planOutput

		output = append(output, batchOutput)
	}

	ctx.JSON(http.StatusOK, output)
}

// GetBatchDetails godoc
//
//	@Summary		Get batch details
//	@Description	Get details of a specific batch
//	@Tags			bulk-licenses
//	@Produce		json
//	@Param			id	path		string	true	"Batch ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SubscriptionBatchOutput
//	@Failure		404	{object}	errors.APIError
//	@Router			/subscription-batches/{id} [get]
func (c *bulkLicenseController) GetBatchDetails(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	batchIDStr := ctx.Param("id")

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid batch ID",
		})
		return
	}

	batches, err := c.bulkService.GetBatchesByPurchaser(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve batch",
		})
		return
	}

	// Find the specific batch
	for _, batch := range *batches {
		if batch.ID == batchID {
			batchOutput := dto.SubscriptionBatchOutput{
				ID:                       batch.ID,
				PurchaserUserID:          batch.PurchaserUserID,
				SubscriptionPlanID:       batch.SubscriptionPlanID,
				GroupID:                  batch.GroupID,
				StripeSubscriptionID:     batch.StripeSubscriptionID,
				StripeSubscriptionItemID: batch.StripeSubscriptionItemID,
				TotalQuantity:            batch.TotalQuantity,
				AssignedQuantity:         batch.AssignedQuantity,
				AvailableQuantity:        batch.TotalQuantity - batch.AssignedQuantity,
				Status:                   batch.Status,
				CurrentPeriodStart:       batch.CurrentPeriodStart,
				CurrentPeriodEnd:         batch.CurrentPeriodEnd,
				CancelledAt:              batch.CancelledAt,
				CreatedAt:                batch.CreatedAt,
				UpdatedAt:                batch.UpdatedAt,
			}

			planOutput, _ := c.conversionService.SubscriptionPlanToDTO(&batch.SubscriptionPlan)
			batchOutput.SubscriptionPlan = *planOutput

			ctx.JSON(http.StatusOK, batchOutput)
			return
		}
	}

	ctx.JSON(http.StatusNotFound, &errors.APIError{
		ErrorCode:    http.StatusNotFound,
		ErrorMessage: "Batch not found or access denied",
	})
}

// GetBatchLicenses godoc
//
//	@Summary		Get licenses in a batch
//	@Description	Get all licenses (assigned and unassigned) in a batch
//	@Tags			bulk-licenses
//	@Produce		json
//	@Param			id	path		string	true	"Batch ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.UserSubscriptionOutput
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/subscription-batches/{id}/licenses [get]
func (c *bulkLicenseController) GetBatchLicenses(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	batchIDStr := ctx.Param("id")

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid batch ID",
		})
		return
	}

	licenses, err := c.bulkService.GetBatchLicenses(batchID, userID)
	if err != nil {
		if err.Error() == "access denied" {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied",
			})
		} else {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Batch not found",
			})
		}
		return
	}

	// Convert to output DTOs
	var output []dto.UserSubscriptionOutput
	for _, license := range *licenses {
		licenseOutput, _ := c.conversionService.UserSubscriptionToDTO(&license)
		output = append(output, *licenseOutput)
	}

	ctx.JSON(http.StatusOK, output)
}

// AssignLicense godoc
//
//	@Summary		Assign a license to a user
//	@Description	Assign an unassigned license from a batch to a specific user
//	@Tags			bulk-licenses
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Batch ID"
//	@Param			assign	body		dto.AssignLicenseInput	true	"User to assign to"
//	@Security		Bearer
//	@Success		200	{object}	dto.UserSubscriptionOutput
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Router			/subscription-batches/{id}/assign [post]
func (c *bulkLicenseController) AssignLicense(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	batchIDStr := ctx.Param("id")

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid batch ID",
		})
		return
	}

	var input dto.AssignLicenseInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	license, err := c.bulkService.AssignLicense(batchID, userID, input.UserID)
	if err != nil {
		utils.Error("Failed to assign license: %v", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	licenseOutput, _ := c.conversionService.UserSubscriptionToDTO(license)
	ctx.JSON(http.StatusOK, licenseOutput)
}

// RevokeLicense godoc
//
//	@Summary		Revoke a license assignment
//	@Description	Remove a license assignment and return it to the unassigned pool
//	@Tags			bulk-licenses
//	@Produce		json
//	@Param			id			path	string	true	"Batch ID"
//	@Param			license_id	path	string	true	"License ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string
//	@Failure		403	{object}	errors.APIError
//	@Failure		404	{object}	errors.APIError
//	@Router			/subscription-batches/{id}/licenses/{license_id}/revoke [delete]
func (c *bulkLicenseController) RevokeLicense(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	licenseIDStr := ctx.Param("license_id")

	licenseID, err := uuid.Parse(licenseIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid license ID",
		})
		return
	}

	err = c.bulkService.RevokeLicense(licenseID, userID)
	if err != nil {
		utils.Error("Failed to revoke license: %v", err)
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "License revoked successfully",
	})
}

// UpdateBatchQuantity godoc
//
//	@Summary		Update batch quantity
//	@Description	Scale up or down the number of licenses in a batch
//	@Tags			bulk-licenses
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string							true	"Batch ID"
//	@Param			update	body		dto.UpdateBatchQuantityInput	true	"New quantity"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError
//	@Router			/subscription-batches/{id}/quantity [patch]
func (c *bulkLicenseController) UpdateBatchQuantity(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	batchIDStr := ctx.Param("id")

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid batch ID",
		})
		return
	}

	var input dto.UpdateBatchQuantityInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	err = c.bulkService.UpdateBatchQuantity(batchID, userID, input.NewQuantity)
	if err != nil {
		utils.Error("Failed to update batch quantity: %v", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Batch quantity updated to %d", input.NewQuantity),
	})
}

// PermanentlyDeleteBatch godoc
//
//	@Summary		Permanently delete a bulk license batch
//	@Description	Permanently deletes a batch and all its licenses. This will cancel the Stripe subscription, terminate all active terminals for assigned users, and remove all records from the database.
//	@Tags			bulk-licenses
//	@Produce		json
//	@Param			id	path		string	true	"Batch ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]string
//	@Failure		400	{object}	errors.APIError
//	@Failure		403	{object}	errors.APIError	"Only the purchaser can delete the batch"
//	@Failure		404	{object}	errors.APIError	"Batch not found"
//	@Failure		500	{object}	errors.APIError
//	@Router			/subscription-batches/{id}/permanent [delete]
func (c *bulkLicenseController) PermanentlyDeleteBatch(ctx *gin.Context) {
	userID := ctx.GetString("userId")
	batchIDStr := ctx.Param("id")

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid batch ID",
		})
		return
	}

	err = c.bulkService.PermanentlyDeleteBatch(batchID, userID)
	if err != nil {
		utils.Error("Failed to permanently delete batch: %v", err)

		statusCode := http.StatusInternalServerError
		if err.Error() == "batch not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "only the purchaser can delete this batch" {
			statusCode = http.StatusForbidden
		}

		ctx.JSON(statusCode, &errors.APIError{
			ErrorCode:    statusCode,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Batch and all licenses permanently deleted",
	})
}
