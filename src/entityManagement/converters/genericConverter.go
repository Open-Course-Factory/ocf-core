package converters

import "reflect"

// GenericModelToOutput handles pointer/value type conversion generically
// This eliminates the need for repetitive reflection checks in entity registrations
//
// Usage:
//
//	func (s EntityRegistration) EntityModelToEntityOutput(input any) (any, error) {
//	    return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
//	        return dto.EntityModelToEntityOutput(ptr.(*models.Entity)), nil
//	    })
//	}
func GenericModelToOutput(
	input any,
	ptrConverter func(any) (any, error),
) (any, error) {
	// Check if input is already a pointer
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return ptrConverter(input)
	}

	// Convert value to pointer
	val := reflect.ValueOf(input)
	if !val.IsValid() {
		return nil, nil
	}

	// Create pointer to the value
	ptrValue := reflect.New(val.Type())
	ptrValue.Elem().Set(val)

	return ptrConverter(ptrValue.Interface())
}

// GenericSliceConverter handles slice conversion for collections
// Useful for converting []Model to []OutputDto
//
// Usage:
//
//	items := []models.Entity{...}
//	outputs, err := converters.GenericSliceConverter(items, func(item any) (any, error) {
//	    return dto.EntityModelToEntityOutput(item.(*models.Entity)), nil
//	})
func GenericSliceConverter(
	input any,
	itemConverter func(any) (any, error),
) ([]any, error) {
	val := reflect.ValueOf(input)

	// Ensure input is a slice
	if val.Kind() != reflect.Slice {
		return nil, nil
	}

	results := make([]any, val.Len())

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()

		converted, err := itemConverter(item)
		if err != nil {
			return nil, err
		}

		results[i] = converted
	}

	return results, nil
}
