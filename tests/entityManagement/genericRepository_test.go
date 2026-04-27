// tests/entityManagement/genericRepository_test.go
package entityManagement_tests

import (
	"net/http"
	"testing"

	entityErrors "soli/formations/src/entityManagement/errors"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/repositories"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// --- FK constraint delete tests ---

// Entity types for FK constraint tests.
type DeleteTestParent struct {
	entityManagementModels.BaseModel
	Name     string            `json:"name"`
	Children []DeleteTestChild `gorm:"foreignKey:ParentID;constraint:OnDelete:RESTRICT"`
}

type DeleteTestChild struct {
	entityManagementModels.BaseModel
	Name     string    `json:"name"`
	ParentID uuid.UUID `gorm:"type:text"` // SQLite needs "text" for UUID
}

// setupRepositoryTestDBWithFK creates an in-memory SQLite DB with foreign key enforcement enabled.
func setupRepositoryTestDBWithFK(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// CRITICAL: SQLite does NOT enforce foreign keys by default.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON;")
	require.NoError(t, err)

	// Migrate the FK test models
	err = db.AutoMigrate(
		&DeleteTestParent{},
		&DeleteTestChild{},
	)
	require.NoError(t, err)

	return db
}

// TestGenericRepository_DeleteEntity_ForeignKeyConstraint verifies that deleting
// an entity which is referenced by a child entity via FK returns a constraint
// violation error (HTTP 409 Conflict, error code ENT011), not a generic 500.
func TestGenericRepository_DeleteEntity_ForeignKeyConstraint(t *testing.T) {
	// Setup
	db := setupRepositoryTestDBWithFK(t)
	repo := repositories.NewGenericRepository(db)

	// Create a parent entity
	parent := &DeleteTestParent{Name: "Parent with children"}
	result := db.Create(parent)
	require.NoError(t, result.Error)
	require.NotEqual(t, uuid.Nil, parent.ID)

	// Create a child entity that references the parent
	child := &DeleteTestChild{
		Name:     "Child referencing parent",
		ParentID: parent.ID,
	}
	result = db.Create(child)
	require.NoError(t, result.Error)

	// Attempt to delete the parent — should fail with FK constraint violation
	err := repo.DeleteEntity(parent.ID, &DeleteTestParent{}, false)

	// Assert: error IS returned
	assert.Error(t, err, "Deleting a parent with children should return an error")

	// Assert: error is an EntityError
	entityErr, ok := err.(*entityErrors.EntityError)
	assert.True(t, ok, "Expected *entityErrors.EntityError, got %T: %v", err, err)

	if ok {
		// Assert: HTTP status is 409 Conflict (not 500 generic DB error)
		assert.Equal(t, http.StatusConflict, entityErr.HTTPStatus,
			"FK constraint violation should return HTTP 409 Conflict, got %d", entityErr.HTTPStatus)

		// Assert: error code is ENT011 (new constraint violation code)
		assert.Equal(t, "ENT011", entityErr.Code,
			"FK constraint violation should use error code ENT011, got %s", entityErr.Code)

		// Assert: error message contains useful constraint information
		assert.Contains(t, entityErr.Message, "constraint",
			"Error message should mention 'constraint'")

		// Assert: error details contain actionable information for the user
		assert.NotNil(t, entityErr.Details, "Error should include details about what blocks deletion")
		assert.Contains(t, entityErr.Details, "operation",
			"Error details should include the operation that failed")
	}

	// Verify the parent entity still exists (deletion was prevented)
	var existingParent DeleteTestParent
	dbErr := db.First(&existingParent, parent.ID).Error
	assert.NoError(t, dbErr, "Parent entity should still exist after failed delete")
	assert.Equal(t, "Parent with children", existingParent.Name)
}

// TestGenericRepository_DeleteEntity_Success_WithNoReferences verifies that
// deleting a parent entity with NO children succeeds even when the entity type
// has FK relationships defined. This is the contrast case for the FK constraint test.
func TestGenericRepository_DeleteEntity_Success_WithNoReferences(t *testing.T) {
	// Setup
	db := setupRepositoryTestDBWithFK(t)
	repo := repositories.NewGenericRepository(db)

	// Create a parent entity with NO children
	parent := &DeleteTestParent{Name: "Parent without children"}
	result := db.Create(parent)
	require.NoError(t, result.Error)
	require.NotEqual(t, uuid.Nil, parent.ID)

	// Delete the parent — should succeed
	err := repo.DeleteEntity(parent.ID, &DeleteTestParent{}, false)

	// Assert: no error
	assert.NoError(t, err, "Deleting a parent without children should succeed")

	// Assert: entity is actually deleted from the database
	var deletedParent DeleteTestParent
	dbErr := db.Unscoped().First(&deletedParent, parent.ID).Error
	assert.Error(t, dbErr, "Entity should not exist after hard delete")
	assert.Equal(t, gorm.ErrRecordNotFound, dbErr)
}
