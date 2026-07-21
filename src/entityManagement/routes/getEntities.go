package controller

import (
	"math"
	"net/http"
	"soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/utils"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PaginationResponse struct {
	Data            []any `json:"data"`
	Total           int64 `json:"total"`
	TotalPages      int   `json:"totalPages"`
	CurrentPage     int   `json:"currentPage"`
	PageSize        int   `json:"pageSize"`
	HasNextPage     bool  `json:"hasNextPage"`
	HasPreviousPage bool  `json:"hasPreviousPage"`
}

// CursorPaginationResponse is used for cursor-based pagination
type CursorPaginationResponse struct {
	Data       []any  `json:"data"`
	NextCursor string `json:"nextCursor,omitempty"` // Base64-encoded cursor for next page
	HasMore    bool   `json:"hasMore"`              // Indicates if more results exist
	Limit      int    `json:"limit"`                // Number of items per page
	Total      int64  `json:"total"`                // Total count of all items matching the query/filters
}

// Pagination constants
const (
	DefaultPageSize    = 20  // Default number of items per page
	MaxPageSize        = 100 // Maximum allowed items per page
	DefaultCursorLimit = 20  // Default limit for cursor-based pagination
	MaxCursorLimit     = 100 // Maximum limit for cursor-based pagination
)

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
	filters := make(map[string]any)
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

	// NEW: Generic membership-based filtering
	// Apply automatic user-based filtering for any entity with membership configuration
	entityName := GetEntityNameFromPath(ctx.FullPath())
	if ems.GlobalEntityRegistrationService.HasMembershipConfig(entityName) {
		userID := ctx.GetString("userId")
		if userID != "" {
			// Check if user is a system admin
			roles, exists := ctx.Get("userRoles")
			isAdmin := false
			if exists {
				roleList, ok := roles.([]string)
				if ok {
					for _, role := range roleList {
						if role == "administrator" || role == "admin" {
							isAdmin = true
							break
						}
					}
				}
			}

			// Non-admin users only see entities they have access to via membership
			if !isAdmin {
				filters["user_member_id"] = userID
			}
		}
	} else if config := ownerReadScope(ctx, entityName); config != nil {
		// Owner read-scope: a non-admin caller sees only rows they own. The
		// filter column is derived the same way the ownership hook derives it,
		// so the injected key matches the entity's owner column exactly.
		userID := ctx.GetString("userId")
		if userID == "" {
			// Fail-closed: an unknown actor (no authenticated userId) on a
			// read-scoped entity must see nothing, never the global unscoped
			// set. Mirrors the get-by-id path, which already denies. Returning
			// an empty page here is safer than skipping the owner filter.
			respondEmptyPage(ctx)
			return
		}
		if config.ArrayOwner {
			// Array-owner scope: OwnerField names an array column (owner_ids). A
			// sentinel filter key routes to ownerArrayFilter, which emits the
			// CONTAINS predicate. The double-underscore prefix can never collide
			// with a real entity field name, so it is safe to inject here.
			filters["__owner_ids_contains"] = userID
		} else {
			column := genericController.db.Config.NamingStrategy.ColumnName("", config.OwnerField)
			filters[column] = userID
		}
	}

	// Visibility scope: a non-admin caller sees only rows whose boolean flag is
	// true. Independent of owner scope and of the caller's identity — an
	// unauthenticated caller still gets the visible rows (e.g. the public
	// pricing catalog), so this deliberately does NOT fail closed on a missing
	// userId the way owner scope does above.
	if vis := visibilityScope(ctx, entityName); vis != nil {
		column := genericController.db.Config.NamingStrategy.ColumnName("", vis.Field)
		filters[column] = true
	}

	// Use cursor pagination if cursor param is present (even if empty for first page)
	if _, hasCursor := ctx.Request.URL.Query()["cursor"]; hasCursor {
		limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", strconv.Itoa(DefaultCursorLimit)))

		// Validate limit
		if limit < 1 || limit > MaxCursorLimit {
			limit = DefaultCursorLimit
		}

		entitiesDto, nextCursor, hasMore, total, err := genericController.getEntitiesCursor(ctx, cursor, limit, filters, includes)
		if errors.HandleError(http.StatusNotFound, err, ctx) {
			return
		}

		// Apply per-entity DTO redactor if registered.
		if err := applyDtoRedactor(ctx, entityName, entitiesDto, genericController.db); err != nil {
			errors.HandleError(http.StatusInternalServerError, err, ctx)
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
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("size", strconv.Itoa(DefaultPageSize)))

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}

	entitiesDto, total, getEntityError := genericController.getEntities(ctx, page, pageSize, filters, includes)

	if errors.HandleError(http.StatusNotFound, getEntityError, ctx) {
		return
	}

	// Apply per-entity DTO redactor if registered.
	if err := applyDtoRedactor(ctx, entityName, entitiesDto, genericController.db); err != nil {
		errors.HandleError(http.StatusInternalServerError, err, ctx)
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

// respondEmptyPage writes a zero-row page in whichever pagination shape the
// request asked for. Used to fail closed on a read-scoped entity when the
// acting identity is unknown, without querying the database.
func respondEmptyPage(ctx *gin.Context) {
	if _, hasCursor := ctx.Request.URL.Query()["cursor"]; hasCursor {
		ctx.JSON(http.StatusOK, CursorPaginationResponse{
			Data:    []any{},
			HasMore: false,
			Limit:   DefaultCursorLimit,
			Total:   0,
		})
		return
	}
	ctx.JSON(http.StatusOK, PaginationResponse{
		Data:            []any{},
		Total:           0,
		TotalPages:      0,
		CurrentPage:     1,
		PageSize:        DefaultPageSize,
		HasNextPage:     false,
		HasPreviousPage: false,
	})
}

// applyDtoRedactor invokes the registered DtoRedactor (if any) on each item
// in the slice in place. Each item is passed by pointer so the redactor can
// type-assert and mutate it. Items that are already pointers are passed
// through. The controller's *gorm.DB is forwarded so the redactor can run
// authorization queries. Returns the first redactor error encountered.
func applyDtoRedactor(ctx *gin.Context, entityName string, entitiesDto []any, db *gorm.DB) error {
	redactor, ok := ems.GlobalEntityRegistrationService.GetDtoRedactor(entityName)
	if !ok {
		return nil
	}
	for i := range entitiesDto {
		// Pass &entitiesDto[i] so the redactor can mutate the slot via its
		// pointer-to-DTO contract (matches getEntity's &entityDto pattern).
		if err := redactor(ctx, &entitiesDto[i], db); err != nil {
			return err
		}
	}
	return nil
}

func (genericController genericController) getEntities(ctx *gin.Context, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, error) {
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

func (genericController genericController) getEntitiesFromName(entityName string, page int, pageSize int, filters map[string]any, includes []string) ([]any, int64, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, total, err := genericController.genericService.GetEntities(entityModelInterface, page, pageSize, filters, includes)

	if err != nil {
		utils.Error("%s", err.Error())
		return nil, 0, true
	}

	entitiesDto, shouldReturn := genericController.genericService.GetDtoArrayFromEntitiesPages(allEntitiesPages, entityModelInterface, entityName)
	if shouldReturn {
		return nil, 0, true
	}
	return entitiesDto, total, false
}

// getEntitiesCursor retrieves entities using cursor-based pagination
func (genericController genericController) getEntitiesCursor(ctx *gin.Context, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, error) {
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
func (genericController genericController) getEntitiesCursorFromName(entityName string, cursor string, limit int, filters map[string]any, includes []string) ([]any, string, bool, int64, bool) {
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	allEntitiesPages, nextCursor, hasMore, total, err := genericController.genericService.GetEntitiesCursor(entityModelInterface, cursor, limit, filters, includes)

	if err != nil {
		utils.Error("%s", err.Error())
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

		// If already properly capitalized (first letter uppercase), preserve as-is
		// This maintains camelCase like "ParentGroup", "SubGroups", etc.
		if len(segment) > 0 && segment[0] >= 'A' && segment[0] <= 'Z' {
			normalized[i] = segment
			continue
		}

		// Convert to title case: first letter uppercase, rest lowercase
		// This handles: "chapters" -> "Chapters", "CHAPTERS" -> "Chapters", "ChApTeRs" -> "Chapters"
		normalized[i] = strings.ToUpper(segment[:1]) + strings.ToLower(segment[1:])
	}

	return strings.Join(normalized, ".")
}
