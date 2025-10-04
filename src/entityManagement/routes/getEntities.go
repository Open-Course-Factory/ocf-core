package controller

import (
	"fmt"
	"math"
	"net/http"
	"soli/formations/src/auth/errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

type PaginationResponse struct {
	Data            []interface{} `json:"data"`
	Total           int64         `json:"total"`
	TotalPages      int           `json:"totalPages"`
	CurrentPage     int           `json:"currentPage"`
	PageSize        int           `json:"pageSize"`
	HasNextPage     bool          `json:"hasNextPage"`
	HasPreviousPage bool          `json:"hasPreviousPage"`
}

// CursorPaginationResponse is used for cursor-based pagination
type CursorPaginationResponse struct {
	Data       []interface{} `json:"data"`
	NextCursor string        `json:"nextCursor,omitempty"` // Base64-encoded cursor for next page
	HasMore    bool          `json:"hasMore"`              // Indicates if more results exist
	Limit      int           `json:"limit"`                // Number of items per page
}

func (genericController genericController) GetEntities(ctx *gin.Context) {
	// Check if cursor-based pagination is requested
	cursor := ctx.Query("cursor")

	// Extract filter parameters (all query params except pagination params)
	filters := make(map[string]interface{})
	excludedParams := map[string]bool{"page": true, "size": true, "cursor": true, "limit": true}
	for key, values := range ctx.Request.URL.Query() {
		if !excludedParams[key] && len(values) > 0 {
			// Handle comma-separated values for array filters
			if len(values) == 1 && ctx.Query(key) != "" {
				filters[key] = ctx.Query(key)
			} else if len(values) > 1 {
				filters[key] = values
			}
		}
	}

	// Use cursor pagination if cursor param is present (even if empty for first page)
	if _, hasCursor := ctx.Request.URL.Query()["cursor"]; hasCursor {
		limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

		// Validate limit
		if limit < 1 || limit > 100 {
			limit = 20
		}

		entitiesDto, nextCursor, hasMore, err := genericController.getEntitiesCursor(ctx, cursor, limit, filters)
		if errors.HandleError(http.StatusNotFound, err, ctx) {
			return
		}

		response := CursorPaginationResponse{
			Data:       entitiesDto,
			NextCursor: nextCursor,
			HasMore:    hasMore,
			Limit:      limit,
		}

		ctx.JSON(http.StatusOK, response)
		return
	}

	// Traditional offset pagination (backward compatible)
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("size", "20"))

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	entitiesDto, total, getEntityError := genericController.getEntities(ctx, page, pageSize, filters)

	if errors.HandleError(http.StatusNotFound, getEntityError, ctx) {
		return
	}

	// Calculate total pages
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	response := PaginationResponse{
		Data:            entitiesDto,
		Total:           total,
		TotalPages:      totalPages,
		CurrentPage:     page,
		PageSize:        pageSize,
		HasNextPage:     page < totalPages,
		HasPreviousPage: page > 1,
	}

	ctx.JSON(http.StatusOK, response)
}

func (genericController genericController) getEntities(ctx *gin.Context, page int, pageSize int, filters map[string]interface{}) ([]interface{}, int64, error) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, total, shouldReturn := genericController.getEntitiesFromName(entityName, page, pageSize, filters)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, 0, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		}
	}
	return entitiesDto, total, nil
}

func (genericController genericController) getEntitiesFromName(entityName string, page int, pageSize int, filters map[string]interface{}) ([]interface{}, int64, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, total, err := genericController.genericService.GetEntities(entityModelInterface, page, pageSize, filters)

	if err != nil {
		fmt.Println(err.Error())
		return nil, 0, true
	}

	entitiesDto, shouldReturn := genericController.genericService.GetDtoArrayFromEntitiesPages(allEntitiesPages, entityModelInterface, entityName)
	if shouldReturn {
		return nil, 0, true
	}
	return entitiesDto, total, false
}

// getEntitiesCursor retrieves entities using cursor-based pagination
func (genericController genericController) getEntitiesCursor(ctx *gin.Context, cursor string, limit int, filters map[string]interface{}) ([]interface{}, string, bool, error) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, nextCursor, hasMore, shouldReturn := genericController.getEntitiesCursorFromName(entityName, cursor, limit, filters)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, "", false, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		}
	}
	return entitiesDto, nextCursor, hasMore, nil
}

// getEntitiesCursorFromName retrieves entities by name using cursor-based pagination
func (genericController genericController) getEntitiesCursorFromName(entityName string, cursor string, limit int, filters map[string]interface{}) ([]interface{}, string, bool, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, nextCursor, hasMore, err := genericController.genericService.GetEntitiesCursor(entityModelInterface, cursor, limit, filters)

	if err != nil {
		fmt.Println(err.Error())
		return nil, "", false, true
	}

	entitiesDto, shouldReturn := genericController.genericService.GetDtoArrayFromEntitiesPages(allEntitiesPages, entityModelInterface, entityName)
	if shouldReturn {
		return nil, "", false, true
	}
	return entitiesDto, nextCursor, hasMore, false
}
