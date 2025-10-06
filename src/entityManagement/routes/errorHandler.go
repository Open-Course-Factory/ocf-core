package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	entityErrors "soli/formations/src/entityManagement/errors"
)

// HandleEntityError is a centralized error handler for entity management routes.
// It converts EntityError types into appropriate HTTP responses with consistent JSON format.
//
// Usage in route handlers:
//
//	if err != nil {
//	    HandleEntityError(ctx, err)
//	    return
//	}
func HandleEntityError(ctx *gin.Context, err error) {
	var entityErr *entityErrors.EntityError

	// Check if it's an EntityError
	if errors.As(err, &entityErr) {
		ctx.JSON(entityErr.HTTPStatus, gin.H{
			"error": gin.H{
				"code":    entityErr.Code,
				"message": entityErr.Message,
				"details": entityErr.Details,
			},
		})
		return
	}

	// Fallback for unknown errors
	ctx.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":    "ERR000",
			"message": "Internal server error",
			"details": gin.H{
				"original": err.Error(),
			},
		},
	})
}
