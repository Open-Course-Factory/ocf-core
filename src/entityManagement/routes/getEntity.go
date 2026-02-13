package controller

import (
	"net/http"
	"reflect"
	"strings"

	"soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetEntity handles GET requests for a single entity by ID with selective preloading
//
//	@Param	id		path	string	true	"Entity ID (UUID)"
//	@Param	include	query	string	false	"Comma-separated list of relations to preload (e.g., 'Chapters,Authors' or 'Chapters.Sections' for nested, use '*' for all relations)"
func (genericController genericController) GetEntity(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, err, ctx) {
		return
	}

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

	entityName := GetEntityNameFromPath(ctx.FullPath())
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)
	entity, entityError := genericController.genericService.GetEntity(id, entityModelInterface, entityName, includes)
	if errors.HandleError(http.StatusNotFound, entityError, ctx) {
		return
	}

	var entityDto any

	// Fast path: use typed operations (no reflect)
	if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName); ok {
		dto, err := ops.ConvertModelToDto(entity)
		if err != nil {
			errors.HandleError(http.StatusNotFound, &errors.APIError{ErrorMessage: "Entity Not Found"}, ctx)
			return
		}
		entityDto = dto
	} else {
		// Legacy reflect path
		entityModel := reflect.TypeOf(entityModelInterface)
		entityValue := reflect.ValueOf(entity)

		if entityValue.Elem().Type().ConvertibleTo(entityModel) {
			convertedEntity := entityValue.Elem().Convert(entityModel)

			item := convertedEntity.Interface()

			var errEntityDto bool
			entityDto, errEntityDto = genericController.genericService.GetEntityFromResult(entityName, item)

			if errEntityDto {
				errors.HandleError(http.StatusNotFound, &errors.APIError{ErrorMessage: "Entity Not Found"}, ctx)
				return
			}
		}
	}

	ctx.JSON(http.StatusOK, entityDto)
}
