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

func (genericController genericController) GetEntities(ctx *gin.Context) {
	// Extract pagination parameters
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("size", "20"))

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Extract filter parameters (all query params except page and size)
	filters := make(map[string]interface{})
	for key, values := range ctx.Request.URL.Query() {
		if key != "page" && key != "size" && len(values) > 0 {
			// Handle comma-separated values for array filters
			if len(values) == 1 && ctx.Query(key) != "" {
				filters[key] = ctx.Query(key)
			} else if len(values) > 1 {
				filters[key] = values
			}
		}
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
