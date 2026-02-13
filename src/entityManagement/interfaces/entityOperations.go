package entityManagementInterfaces

import "github.com/google/uuid"

// EntityOperations is a non-generic interface that wraps typed operations
// behind any signatures for runtime dispatch. Each method internally does
// a cheap type assertion (not reflect) then calls the typed converter.
type EntityOperations interface {
	// ConvertDtoToModel converts a create DTO to a model pointer.
	ConvertDtoToModel(dto any) (any, error)

	// ConvertModelToDto converts a model (pointer or value) to an output DTO.
	ConvertModelToDto(model any) (any, error)

	// ConvertEditDtoToMap converts an edit DTO to an update map.
	ConvertEditDtoToMap(dto any) (map[string]any, error)

	// NewModelInstance returns a new zero-value pointer: new(M).
	NewModelInstance() any

	// NewModelSlice returns an empty slice: make([]M, 0).
	NewModelSlice() any

	// ExtractID calls model.GetID() via type assertion.
	ExtractID(model any) (uuid.UUID, error)

	// ConvertSliceToDto iterates a typed slice and converts each element to DTO.
	ConvertSliceToDto(slice any) ([]any, error)

	// NewCreateDto returns a zero-value copy of the create DTO type.
	NewCreateDto() any

	// NewEditDto returns a zero-value copy of the edit DTO type.
	NewEditDto() any
}
