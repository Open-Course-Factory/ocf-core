package controller

import (
	"net/http"
	"reflect"
	"time"

	"soli/formations/src/auth/errors"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

// StringToUUIDHook converts string to uuid.UUID for mapstructure decoding
func StringToUUIDHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		// Check if we're converting from string to uuid.UUID
		if f.Kind() != reflect.String {
			return data, nil
		}

		// Check if target type is uuid.UUID
		uuidType := reflect.TypeOf(uuid.UUID{})
		if t != uuidType {
			return data, nil
		}

		// Convert string to UUID
		str := data.(string)
		if str == "" {
			return uuid.Nil, nil
		}

		parsed, err := uuid.Parse(str)
		if err != nil {
			return nil, err
		}

		return parsed, nil
	}
}

func (genericController genericController) EditEntity(ctx *gin.Context) {
	entityName := GetEntityNameFromPath(ctx.FullPath())

	// Get typed operations for this entity
	ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps(entityName)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "entity not registered: " + entityName})
		return
	}

	// Get the edit DTO type for this entity (returns empty struct instance)
	entityPatchDtoInput := ops.NewEditDto()
	decodedData := ops.NewEditDto()

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
	// Configure to handle time.Time, UUID, and other complex types
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &decodedData,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			StringToUUIDHook(), // Handle UUID strings
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

	var updateMap map[string]any

	// Use typed operations for DtoToMap
	result, opsErr := ops.ConvertEditDtoToMap(decodedData)
	if opsErr == nil {
		updateMap = result
	} else {
		// Fallback to mapstructure-based conversion
		updateMap = make(map[string]any)
		fallbackConfig := &mapstructure.DecoderConfig{
			WeaklyTypedInput: true,
			Result:           &updateMap,
		}
		fallbackDecoder, fallbackErr := mapstructure.NewDecoder(fallbackConfig)
		if fallbackErr == nil {
			fallbackDecoder.Decode(decodedData)
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
