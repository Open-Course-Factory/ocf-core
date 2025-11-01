package utils_test

import (
	"testing"

	"soli/formations/src/entityManagement/utils"
)

// Test entity with OwnerIDs field
type TestEntityWithOwners struct {
	ID       string
	Title    string
	OwnerIDs []string
}

// Test entity without OwnerIDs field
type TestEntityWithoutOwners struct {
	ID    string
	Title string
}

// Test entity with unexported ownerIDs field
type TestEntityUnexportedOwners struct {
	ID       string
	Title    string
	ownerIDs []string //nolint:unused // lowercase - unexported, intentionally unused for reflection test
}

func TestAddOwnerIDToEntity_Success(t *testing.T) {
	entity := &TestEntityWithOwners{
		ID:    "123",
		Title: "Test Entity",
	}

	err := utils.AddOwnerIDToEntity(entity, "user-456")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(entity.OwnerIDs) != 1 {
		t.Errorf("Expected OwnerIDs length to be 1, got: %d", len(entity.OwnerIDs))
	}

	if entity.OwnerIDs[0] != "user-456" {
		t.Errorf("Expected OwnerIDs[0] to be 'user-456', got: %s", entity.OwnerIDs[0])
	}
}

func TestAddOwnerIDToEntity_NoOwnerIDsField(t *testing.T) {
	entity := &TestEntityWithoutOwners{
		ID:    "123",
		Title: "Test Entity",
	}

	err := utils.AddOwnerIDToEntity(entity, "user-456")

	// Should not return an error - just silently skip
	if err != nil {
		t.Errorf("Expected no error for entity without OwnerIDs field, got: %v", err)
	}
}

func TestAddOwnerIDToEntity_UnexportedField(t *testing.T) {
	entity := &TestEntityUnexportedOwners{
		ID:    "123",
		Title: "Test Entity",
	}

	err := utils.AddOwnerIDToEntity(entity, "user-456")

	// Unexported fields are not visible via reflection from outside the package
	// So this should behave the same as no OwnerIDs field - no error
	if err != nil {
		t.Errorf("Expected no error for entity with unexported ownerIDs field, got: %v", err)
	}
}

func TestAddOwnerIDToEntity_ReplacesExistingOwners(t *testing.T) {
	entity := &TestEntityWithOwners{
		ID:       "123",
		Title:    "Test Entity",
		OwnerIDs: []string{"old-owner-1", "old-owner-2"},
	}

	err := utils.AddOwnerIDToEntity(entity, "new-owner")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Current implementation replaces all owners with the new one
	if len(entity.OwnerIDs) != 1 {
		t.Errorf("Expected OwnerIDs length to be 1 (replaced), got: %d", len(entity.OwnerIDs))
	}

	if entity.OwnerIDs[0] != "new-owner" {
		t.Errorf("Expected OwnerIDs[0] to be 'new-owner', got: %s", entity.OwnerIDs[0])
	}
}
