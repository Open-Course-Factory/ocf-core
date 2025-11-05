package controllers

import (
	"net/http"
	"strconv"
	"time"

	"soli/formations/src/audit/models"
	"soli/formations/src/audit/services"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuditController interface {
	GetAuditLogs(ctx *gin.Context)
	GetAuditLogByID(ctx *gin.Context)
	GetUserAuditLogs(ctx *gin.Context)
	GetOrganizationAuditLogs(ctx *gin.Context)
}

type auditController struct {
	auditService services.AuditService
}

func NewAuditController(db *gorm.DB) AuditController {
	return &auditController{
		auditService: services.NewAuditService(db),
	}
}

// GetAuditLogs godoc
//
//	@Summary		Get audit logs
//	@Description	Retrieve audit logs with optional filters
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Param			actor_id			query		string	false	"Filter by actor user ID"
//	@Param			target_id			query		string	false	"Filter by target resource ID"
//	@Param			organization_id		query		string	false	"Filter by organization ID"
//	@Param			event_type			query		string	false	"Filter by event type"
//	@Param			severity			query		string	false	"Filter by severity (info, warning, error, critical)"
//	@Param			status				query		string	false	"Filter by status"
//	@Param			start_date			query		string	false	"Filter by start date (RFC3339 format)"
//	@Param			end_date			query		string	false	"Filter by end date (RFC3339 format)"
//	@Param			limit				query		int		false	"Limit number of results (default: 50, max: 1000)"
//	@Param			offset				query		int		false	"Offset for pagination (default: 0)"
//	@Success		200					{object}	map[string]interface{}
//	@Failure		400					{object}	errors.APIError
//	@Failure		500					{object}	errors.APIError
//	@Router			/audit/logs [get]
//	@Security		BearerAuth
func (ac *auditController) GetAuditLogs(ctx *gin.Context) {
	filter := services.AuditLogFilter{
		Limit:  50,  // Default limit
		Offset: 0,   // Default offset
	}

	// Parse query parameters
	if actorIDStr := ctx.Query("actor_id"); actorIDStr != "" {
		actorID, err := uuid.Parse(actorIDStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid actor_id format",
			})
			return
		}
		filter.ActorID = &actorID
	}

	if targetIDStr := ctx.Query("target_id"); targetIDStr != "" {
		targetID, err := uuid.Parse(targetIDStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid target_id format",
			})
			return
		}
		filter.TargetID = &targetID
	}

	if orgIDStr := ctx.Query("organization_id"); orgIDStr != "" {
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid organization_id format",
			})
			return
		}
		filter.OrganizationID = &orgID
	}

	if eventType := ctx.Query("event_type"); eventType != "" {
		filter.EventType = models.AuditEventType(eventType)
	}

	if severity := ctx.Query("severity"); severity != "" {
		filter.Severity = models.AuditSeverity(severity)
	}

	if status := ctx.Query("status"); status != "" {
		filter.Status = status
	}

	if startDateStr := ctx.Query("start_date"); startDateStr != "" {
		startDate, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid start_date format (use RFC3339)",
			})
			return
		}
		filter.StartDate = &startDate
	}

	if endDateStr := ctx.Query("end_date"); endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid end_date format (use RFC3339)",
			})
			return
		}
		filter.EndDate = &endDate
	}

	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid limit (must be between 1 and 1000)",
			})
			return
		}
		filter.Limit = limit
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid offset (must be >= 0)",
			})
			return
		}
		filter.Offset = offset
	}

	// Get audit logs
	logs, total, err := ac.auditService.GetAuditLogs(filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve audit logs",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// GetAuditLogByID godoc
//
//	@Summary		Get audit log by ID
//	@Description	Retrieve a specific audit log entry by its ID
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"Audit Log ID"
//	@Success		200		{object}	models.AuditLog
//	@Failure		404		{object}	errors.APIError
//	@Failure		500		{object}	errors.APIError
//	@Router			/audit/logs/{id} [get]
//	@Security		BearerAuth
func (ac *auditController) GetAuditLogByID(ctx *gin.Context) {
	// TODO: Implement if needed
	ctx.JSON(http.StatusNotImplemented, gin.H{"message": "Not implemented yet"})
}

// GetUserAuditLogs godoc
//
//	@Summary		Get user audit logs
//	@Description	Retrieve audit logs for a specific user
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Param			user_id		path		string	true	"User ID"
//	@Param			limit		query		int		false	"Limit number of results (default: 50)"
//	@Param			offset		query		int		false	"Offset for pagination (default: 0)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	errors.APIError
//	@Failure		500			{object}	errors.APIError
//	@Router			/audit/users/{user_id}/logs [get]
//	@Security		BearerAuth
func (ac *auditController) GetUserAuditLogs(ctx *gin.Context) {
	userIDStr := ctx.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid user_id format",
		})
		return
	}

	filter := services.AuditLogFilter{
		ActorID: &userID,
		Limit:   50,
		Offset:  0,
	}

	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			filter.Limit = 50
		} else {
			filter.Limit = limit
		}
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	logs, total, err := ac.auditService.GetAuditLogs(filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve user audit logs",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// GetOrganizationAuditLogs godoc
//
//	@Summary		Get organization audit logs
//	@Description	Retrieve audit logs for a specific organization
//	@Tags			audit
//	@Accept			json
//	@Produce		json
//	@Param			organization_id		path		string	true	"Organization ID"
//	@Param			limit				query		int		false	"Limit number of results (default: 50)"
//	@Param			offset				query		int		false	"Offset for pagination (default: 0)"
//	@Success		200					{object}	map[string]interface{}
//	@Failure		400					{object}	errors.APIError
//	@Failure		500					{object}	errors.APIError
//	@Router			/audit/organizations/{organization_id}/logs [get]
//	@Security		BearerAuth
func (ac *auditController) GetOrganizationAuditLogs(ctx *gin.Context) {
	orgIDStr := ctx.Param("organization_id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization_id format",
		})
		return
	}

	filter := services.AuditLogFilter{
		OrganizationID: &orgID,
		Limit:          50,
		Offset:         0,
	}

	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			filter.Limit = 50
		} else {
			filter.Limit = limit
		}
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	logs, total, err := ac.auditService.GetAuditLogs(filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve organization audit logs",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}
