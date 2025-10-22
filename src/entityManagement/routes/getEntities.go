package controller

import (
	"fmt"
	"math"
	"net/http"
	"soli/formations/src/auth/errors"
	"strconv"
	"strings"

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
	Total      int64         `json:"total"`                // Total count of all items matching the query/filters
}

// GetEntities handles GET requests for entity lists with pagination and selective preloading
//
//	@Param	page	query	int		false	"Page number (offset pagination, default: 1)"
//	@Param	size	query	int		false	"Page size (offset pagination, default: 20, max: 100)"
//	@Param	cursor	query	string	false	"Cursor for cursor-based pagination (use empty string for first page)"
//	@Param	limit	query	int		false	"Limit for cursor pagination (default: 20, max: 100)"
//	@Param	include	query	string	false	"Comma-separated list of relations to preload (case-insensitive, e.g., 'chapters' or 'Chapters', nested: 'chapters.sections')"
func (genericController genericController) GetEntities(ctx *gin.Context) {
	// Check if cursor-based pagination is requested
	cursor := ctx.Query("cursor")

	// Parse include parameter for selective preloading
	// Format: ?include=Chapters,Authors or ?include=Chapters.Sections
	// Also supports case-insensitive: ?include=chapters,authors or ?include=chapters.sections
	var includes []string
	includeParam := ctx.Query("include")
	if includeParam != "" {
		// Split by comma and trim whitespace
		for _, rel := range strings.Split(includeParam, ",") {
			trimmed := strings.TrimSpace(rel)
			if trimmed != "" {
				// Normalize to title case for case-insensitive support
				// "chapters" -> "Chapters", "chapters.sections" -> "Chapters.Sections"
				normalized := normalizeIncludePath(trimmed)
				includes = append(includes, normalized)
			}
		}
	}

	// Extract filter parameters (all query params except pagination and include params)
	filters := make(map[string]interface{})
	excludedParams := map[string]bool{"page": true, "size": true, "cursor": true, "limit": true, "include": true}
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

		entitiesDto, nextCursor, hasMore, total, err := genericController.getEntitiesCursor(ctx, cursor, limit, filters, includes)
		if errors.HandleError(http.StatusNotFound, err, ctx) {
			return
		}

		response := CursorPaginationResponse{
			Data:       entitiesDto,
			NextCursor: nextCursor,
			HasMore:    hasMore,
			Limit:      limit,
			Total:      total,
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

	entitiesDto, total, getEntityError := genericController.getEntities(ctx, page, pageSize, filters, includes)

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

func (genericController genericController) getEntities(ctx *gin.Context, page int, pageSize int, filters map[string]interface{}, includes []string) ([]interface{}, int64, error) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, total, shouldReturn := genericController.getEntitiesFromName(entityName, page, pageSize, filters, includes)
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

func (genericController genericController) getEntitiesFromName(entityName string, page int, pageSize int, filters map[string]interface{}, includes []string) ([]interface{}, int64, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, total, err := genericController.genericService.GetEntities(entityModelInterface, page, pageSize, filters, includes)

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
func (genericController genericController) getEntitiesCursor(ctx *gin.Context, cursor string, limit int, filters map[string]interface{}, includes []string) ([]interface{}, string, bool, int64, error) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	entitiesDto, nextCursor, hasMore, total, shouldReturn := genericController.getEntitiesCursorFromName(entityName, cursor, limit, filters, includes)
	if shouldReturn {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		})
		return nil, "", false, 0, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Entities not found",
		}
	}
	return entitiesDto, nextCursor, hasMore, total, nil
}

// getEntitiesCursorFromName retrieves entities by name using cursor-based pagination
func (genericController genericController) getEntitiesCursorFromName(entityName string, cursor string, limit int, filters map[string]interface{}, includes []string) ([]interface{}, string, bool, int64, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, nextCursor, hasMore, total, err := genericController.genericService.GetEntitiesCursor(entityModelInterface, cursor, limit, filters, includes)

	if err != nil {
		fmt.Println(err.Error())
		return nil, "", false, 0, true
	}

	entitiesDto, shouldReturn := genericController.genericService.GetDtoArrayFromEntitiesPages(allEntitiesPages, entityModelInterface, entityName)
	if shouldReturn {
		return nil, "", false, 0, true
	}
	return entitiesDto, nextCursor, hasMore, total, false
}

// normalizeIncludePath normalizes an include path to proper case for GORM preloading.
// It converts each segment to title case (first letter capitalized) to support case-insensitive includes.
// Examples:
//   - "chapters" -> "Chapters"
//   - "chapters.sections" -> "Chapters.Sections"
//   - "Chapters.Sections.Pages" -> "Chapters.Sections.Pages" (already correct, unchanged)
func normalizeIncludePath(path string) string {
	// Split by dot for nested relations (e.g., "chapters.sections.pages")
	segments := strings.Split(path, ".")
	normalized := make([]string, len(segments))

	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			normalized[i] = segment
			continue
		}

		// Convert to title case: first letter uppercase, rest lowercase
		// This handles: "chapters" -> "Chapters", "CHAPTERS" -> "Chapters", "ChApTeRs" -> "Chapters"
		normalized[i] = strings.ToUpper(segment[:1]) + strings.ToLower(segment[1:])
	}

	return strings.Join(normalized, ".")
}
