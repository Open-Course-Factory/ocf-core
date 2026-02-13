package entityManagementInterfaces

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
)

// typedEntityOps implements EntityOperations for a specific set of type parameters.
// Each method does a cheap type assertion instead of reflect.ValueOf.Call.
type typedEntityOps[M EntityModel, C any, E any, O any] struct {
	modelToDto func(*M) (O, error)
	dtoToModel func(C) *M
	dtoToMap   func(E) map[string]any
}

// NewTypedEntityOps creates a new EntityOperations from typed converters.
func NewTypedEntityOps[M EntityModel, C any, E any, O any](
	converters TypedEntityConverters[M, C, E, O],
) EntityOperations {
	return &typedEntityOps[M, C, E, O]{
		modelToDto: converters.ModelToDto,
		dtoToModel: converters.DtoToModel,
		dtoToMap:   converters.DtoToMap,
	}
}

func (ops *typedEntityOps[M, C, E, O]) ConvertDtoToModel(dto any) (any, error) {
	typed, ok := dto.(C)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", *new(C), dto)
	}
	return ops.dtoToModel(typed), nil
}

func (ops *typedEntityOps[M, C, E, O]) ConvertModelToDto(model any) (any, error) {
	// Try pointer first
	if ptr, ok := model.(*M); ok {
		result, err := ops.modelToDto(ptr)
		return result, err
	}
	// Try value — create a pointer to it
	if val, ok := model.(M); ok {
		result, err := ops.modelToDto(&val)
		return result, err
	}
	return nil, fmt.Errorf("expected *%T or %T, got %T", *new(M), *new(M), model)
}

func (ops *typedEntityOps[M, C, E, O]) ConvertEditDtoToMap(dto any) (map[string]any, error) {
	if ops.dtoToMap == nil {
		return nil, fmt.Errorf("no DtoToMap converter registered")
	}
	typed, ok := dto.(E)
	if !ok {
		return nil, fmt.Errorf("expected %T, got %T", *new(E), dto)
	}
	return ops.dtoToMap(typed), nil
}

func (ops *typedEntityOps[M, C, E, O]) NewModelInstance() any {
	return new(M)
}

func (ops *typedEntityOps[M, C, E, O]) NewModelSlice() any {
	return make([]M, 0)
}

func (ops *typedEntityOps[M, C, E, O]) ExtractID(model any) (uuid.UUID, error) {
	if ptr, ok := model.(*M); ok {
		return (*ptr).GetID(), nil
	}
	if val, ok := model.(M); ok {
		return val.GetID(), nil
	}
	return uuid.Nil, fmt.Errorf("expected *%T or %T, got %T", *new(M), *new(M), model)
}

func (ops *typedEntityOps[M, C, E, O]) ConvertSliceToDto(slice any) ([]any, error) {
	// Try typed slice first
	if typed, ok := slice.([]M); ok {
		result := make([]any, 0, len(typed))
		for i := range typed {
			dto, err := ops.modelToDto(&typed[i])
			if err != nil {
				return nil, err
			}
			result = append(result, dto)
		}
		return result, nil
	}
	// Fallback to reflect for interface slices (e.g. from GORM)
	val := reflect.ValueOf(slice)
	if val.Kind() != reflect.Slice {
		return nil, fmt.Errorf("expected slice, got %T", slice)
	}
	result := make([]any, 0, val.Len())
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		dto, err := ops.ConvertModelToDto(item)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

func (ops *typedEntityOps[M, C, E, O]) NewCreateDto() any {
	return *new(C)
}

func (ops *typedEntityOps[M, C, E, O]) NewEditDto() any {
	return *new(E)
}
