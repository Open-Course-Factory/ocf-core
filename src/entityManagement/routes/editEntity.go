package controller

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
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

// JsonEncodeSerializedFields inspects the model struct for fields tagged with
// gorm:"serializer:json" and JSON-encodes corresponding values in updateMap.
// This is necessary because GORM's Updates(map[string]any) bypasses the
// serializer:json tag, passing raw Go values (e.g. []string) directly to the
// SQL driver instead of JSON-encoding them first.
func JsonEncodeSerializedFields(model any, updateMap map[string]any) {
	if model == nil || updateMap == nil {
		return
	}

	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	CollectSerializerFields(t, updateMap)
}

// CollectSerializerFields walks struct fields (including embedded structs)
// looking for gorm:"serializer:json" tags and JSON-encodes matching map values.
func CollectSerializerFields(t reflect.Type, updateMap map[string]any) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Recurse into embedded (anonymous) structs
		if field.Anonymous {
			ft := field.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				CollectSerializerFields(ft, updateMap)
			}
			continue
		}

		// Check if field has gorm:"serializer:json"
		gormTag := field.Tag.Get("gorm")
		if !strings.Contains(gormTag, "serializer:json") {
			continue
		}

		// Determine the map key: prefer json tag, fall back to GORM column tag,
		// then derive from the struct field name using snake_case.
		mapKey := ""
		if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			mapKey = strings.SplitN(jsonTag, ",", 2)[0]
		}
		if mapKey == "" {
			// Check for explicit gorm column tag
			gormParts := strings.Split(gormTag, ";")
			for _, part := range gormParts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "column:") {
					mapKey = strings.TrimPrefix(part, "column:")
					break
				}
			}
		}
		if mapKey == "" {
			// Fall back to snake_case of the field name (GORM default)
			mapKey = CamelToSnake(field.Name)
		}
		if mapKey == "" {
			continue
		}

		val, exists := updateMap[mapKey]
		if !exists || val == nil {
			continue
		}

		// Only encode slices, arrays, and maps — scalar types don't need encoding
		rv := reflect.ValueOf(val)
		kind := rv.Kind()
		if kind == reflect.Slice || kind == reflect.Array || kind == reflect.Map {
			encoded, err := json.Marshal(val)
			if err == nil {
				updateMap[mapKey] = string(encoded)
			}
		}
	}
}

// stringFieldsOf inspects an Edit DTO instance and returns the set of
// JSON tag names whose target field is a string or *string. The PATCH
// cleanup uses this to decide whether an empty-string value in the
// inbound map represents "clear the field" (string target → keep) or
// "no change" (non-string target → drop, since "" can't decode to UUID,
// time, or be a meaningful zero for bool/int under WeaklyTypedInput).
//
// dto may be a value, pointer, or nil. Returns an empty set on anything
// that's not a struct so the caller can skip the lookup safely.
func stringFieldsOf(dto any) map[string]bool {
	out := map[string]bool{}
	if dto == nil {
		return out
	}
	t := reflect.TypeOf(dto)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.String {
			continue
		}
		// Resolve the JSON tag (preferred) or fall back to mapstructure
		// or snake_case of the field name to match the inbound map keys.
		key := ""
		if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			key = strings.SplitN(jsonTag, ",", 2)[0]
		}
		if key == "" {
			if msTag := field.Tag.Get("mapstructure"); msTag != "" && msTag != "-" {
				key = strings.SplitN(msTag, ",", 2)[0]
			}
		}
		if key == "" {
			key = CamelToSnake(field.Name)
		}
		if key != "" {
			out[key] = true
		}
	}
	return out
}

// CamelToSnake converts a Go CamelCase field name to snake_case,
// matching GORM's default column naming convention.
func CamelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + ('a' - 'A'))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
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

	// Clean up the input map: drop empty strings ONLY for fields whose
	// target DTO type is not a string (e.g. *uuid.UUID, *time.Time, *bool).
	// Empty strings on those would either fail to decode (time) or coerce
	// to surprising zero values (bool false, int 0) under WeaklyTypedInput.
	//
	// Empty strings on string / *string fields MUST be preserved — PATCH
	// semantics require that callers can clear an optional text field by
	// sending "". Dropping them silently caused issue: a PATCH with
	// {"description": ""} returned 204 but kept the previous value.
	if inputMap, ok := entityPatchDtoInput.(map[string]any); ok {
		stringFields := stringFieldsOf(decodedData)
		for key, value := range inputMap {
			strValue, isString := value.(string)
			if !isString || strValue != "" {
				continue
			}
			if !stringFields[key] {
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
	if errors.HandleError(http.StatusInternalServerError, err, ctx) {
		return
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

	// JSON-encode values for fields with serializer:json to prevent GORM bypass
	JsonEncodeSerializedFields(entityModelInterface, updateMap)

	// Extract user context for hook authorization
	userId := ctx.GetString("userId")
	userRoles := ctx.GetStringSlice("userRoles")

	// Perform the update with the map (GORM Updates requires map[string]any)
	errorUpdate := genericController.genericService.EditEntityWithUser(id, entityName, entityModelInterface, updateMap, userId, userRoles...)
	if errorUpdate != nil {
		// Map by the error's own status: a validation-hook rejection is 400, a
		// missing row is 404, a DB failure is 500 — not a blanket 404.
		HandleEntityError(ctx, errorUpdate)
		return
	}

	ctx.JSON(http.StatusNoContent, "Done")
}
