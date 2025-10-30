package utils

import (
	"fmt"
	"reflect"
)

// AddOwnerIDToEntity adds a user ID to the entity's OwnerIDs slice field if it exists.
// The entity must be a pointer to a struct with an OwnerIDs field of type []string.
// Returns the entity unchanged if it doesn't have an OwnerIDs field or if the field cannot be set.
//
// Note: This function modifies the OwnerIDs field but does NOT save the entity to the database.
// The caller is responsible for persisting the changes.
func AddOwnerIDToEntity(entity any, userId string) error {
	entityReflectValue := reflect.ValueOf(entity).Elem()
	ownerIdsField := entityReflectValue.FieldByName("OwnerIDs")

	if !ownerIdsField.IsValid() {
		// Entity doesn't have OwnerIDs field - this is not an error
		return nil
	}

	if !ownerIdsField.CanSet() {
		return fmt.Errorf("OwnerIDs field exists but cannot be set (unexported field?)")
	}

	fmt.Println(ownerIdsField.Kind())
	if ownerIdsField.Kind() == reflect.Slice {
		// Create a slice with the single owner ID
		ownerIdsField.Set(reflect.MakeSlice(ownerIdsField.Type(), 1, 1))
		ownerIdsField.Index(0).Set(reflect.ValueOf(userId))
	}

	return nil
}
