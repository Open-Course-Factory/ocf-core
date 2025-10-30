package controller

import (
	"net/http"
	"time"

	"soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

func (genericController genericController) EditEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	// Get the edit DTO type for this entity (returns empty struct instance)
	entityPatchDtoInput := genericController.entityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)
	decodedData := ems.GlobalEntityRegistrationService.GetEntityDtos(entityName, ems.InputEditDto)

	// Bind JSON request body - this creates a map[string]any
	bindError := ctx.BindJSON(&entityPatchDtoInput)
	if errors.HandleError(http.StatusBadRequest, bindError, ctx) {
		return
	}

	// Clean up the input map - remove empty strings to prevent decode issues
	// Empty strings are treated as "no change" for the field
	if inputMap, ok := entityPatchDtoInput.(map[string]any); ok {
		for key, value := range inputMap {
			if strValue, isString := value.(string); isString && strValue == "" {
				delete(inputMap, key)
			}
		}
	}

	// Use mapstructure to decode the map into the proper DTO struct type
	// Configure to handle time.Time and other complex types
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &decodedData,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeHookFunc(time.RFC3339), // Handle ISO8601 time strings
			mapstructure.StringToTimeDurationHookFunc(),     // Handle duration strings
		),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	errDecode := decoder.Decode(entityPatchDtoInput)
	if errors.HandleError(http.StatusInternalServerError, errDecode, ctx) {
		return
	}

	// Get the registered DtoToMap converter function
	converterFunc, exists := genericController.entityRegistrationService.GetConversionFunction(entityName, ems.EditInputDtoToMap)

	var updateMap map[string]any

	// Try to use custom converter if it exists
	useCustomConverter := false
	if exists && converterFunc != nil {
		if dtoToMapFunc, ok := converterFunc.(func(any) map[string]any); ok {
			updateMap = dtoToMapFunc(decodedData)
			useCustomConverter = true
		}
	}

	// Fallback to default mapstructure-based conversion if no custom converter
	// This matches AbstractRegistrableInterface.EntityDtoToMap behavior
	if !useCustomConverter {
		updateMap = make(map[string]any)
		config := &mapstructure.DecoderConfig{
			WeaklyTypedInput: true,
			Result:           &updateMap,
		}
		decoder, err := mapstructure.NewDecoder(config)
		if err == nil {
			decoder.Decode(decodedData)
		}
	}

	// Parse the entity ID from URL
	id, parseErr := uuid.Parse(ctx.Param("id"))
	if errors.HandleError(http.StatusBadRequest, parseErr, ctx) {
		return
	}

	// Get entity model interface
	entityModelInterface := genericController.genericService.GetEntityModelInterface(entityName)

	// Perform the update with the map (GORM Updates requires map[string]any)
	errorUpdate := genericController.genericService.EditEntity(id, entityName, entityModelInterface, updateMap)
	if errors.HandleError(http.StatusNotFound, errorUpdate, ctx) {
		return
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
