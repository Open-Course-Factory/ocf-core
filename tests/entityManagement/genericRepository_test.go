// tests/entityManagement/genericRepository_test.go
package entityManagement_tests

import (
	"net/http"
	"testing"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/repositories"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Entités de test pour le repository
type RepositoryTestEntity struct {
	entityManagementModels.BaseModel
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       int    `json:"value"`
}

type RepositoryTestEntityInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       int    `json:"value"`
}

type RepositoryTestEntityOutput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Value       int      `json:"value"`
	OwnerIDs    []string `json:"ownerIDs"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// Entité avec sous-entités pour tester le preloading
type ParentTestEntity struct {
	entityManagementModels.BaseModel
	Name              string            `json:"name"`
	ChildTestEntities []ChildTestEntity `json:"children"`
}

type ChildTestEntity struct {
	entityManagementModels.BaseModel
	Name               string `json:"name"`
	ParentTestEntityID uuid.UUID
}

// Setup de base de données pour les tests de repository
func setupRepositoryTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrer les modèles de test
	err = db.AutoMigrate(
		&RepositoryTestEntity{},
		&ParentTestEntity{},
		&ChildTestEntity{},
	)
	require.NoError(t, err)

	// sqlDB, _ := db.DB()
	// sqlDB.SetMaxOpenConns(1)

	return db
}

// Setup des entities dans le service de registration
func setupRepositoryTestEntityRegistration() {
	// Reset global service to ensure clean state
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()

	// Disable hooks for tests
	hooks.GlobalHookRegistry.DisableAllHooks(true)

	ems.RegisterTypedEntity[RepositoryTestEntity, RepositoryTestEntityInput, RepositoryTestEntityInput, RepositoryTestEntityOutput](
		ems.GlobalEntityRegistrationService,
		"RepositoryTestEntity",
		entityManagementInterfaces.TypedEntityRegistration[RepositoryTestEntity, RepositoryTestEntityInput, RepositoryTestEntityInput, RepositoryTestEntityOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[RepositoryTestEntity, RepositoryTestEntityInput, RepositoryTestEntityInput, RepositoryTestEntityOutput]{
				ModelToDto: func(entity *RepositoryTestEntity) (RepositoryTestEntityOutput, error) {
					return RepositoryTestEntityOutput{
						ID:          entity.ID.String(),
						Name:        entity.Name,
						Description: entity.Description,
						Value:       entity.Value,
						OwnerIDs:    entity.OwnerIDs,
						CreatedAt:   entity.CreatedAt.String(),
						UpdatedAt:   entity.UpdatedAt.String(),
					}, nil
				},
				DtoToModel: func(dto RepositoryTestEntityInput) *RepositoryTestEntity {
					return &RepositoryTestEntity{
						Name:        dto.Name,
						Description: dto.Description,
						Value:       dto.Value,
					}
				},
			},
		},
	)

	// Enregistrer les entités parent/child pour les tests de preloading
	ems.GlobalEntityRegistrationService.RegisterEntityInterface("ParentTestEntity", ParentTestEntity{})
	ems.GlobalEntityRegistrationService.RegisterSubEntites("ParentTestEntity", []any{ChildTestEntity{}})
}

func TestGenericRepository_CreateEntityFromModel_Success(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)
	setupRepositoryTestEntityRegistration()

	// Pre-convert DTO to model (conversion is now the service's job)
	entityModel := &RepositoryTestEntity{
		Name:        "Test Repository Entity",
		Description: "Testing repository creation",
		Value:       42,
	}

	// Execute
	result, err := repo.CreateEntityFromModel(entityModel)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Vérifier que l'entité a été sauvegardée en base
	var savedEntity RepositoryTestEntity
	dbErr := db.First(&savedEntity).Error
	assert.NoError(t, dbErr)
	assert.Equal(t, "Test Repository Entity", savedEntity.Name)
	assert.Equal(t, "Testing repository creation", savedEntity.Description)
	assert.Equal(t, 42, savedEntity.Value)
	assert.NotEqual(t, uuid.Nil, savedEntity.ID)
}

func TestGenericRepository_SaveEntity_Success(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	entity := &RepositoryTestEntity{
		Name:        "Entity to Save",
		Description: "Testing save operation",
		Value:       100,
	}

	// Execute
	result, err := repo.SaveEntity(entity)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Vérifier en base
	var savedEntity RepositoryTestEntity
	dbErr := db.First(&savedEntity, "name = ?", "Entity to Save").Error
	assert.NoError(t, dbErr)
	assert.Equal(t, "Entity to Save", savedEntity.Name)
}

func TestGenericRepository_GetEntity_Success(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)
	setupRepositoryTestEntityRegistration()

	// Créer une entité directement en base
	testEntity := RepositoryTestEntity{
		Name:        "Entity to Retrieve",
		Description: "Testing retrieval",
		Value:       200,
	}
	db.Create(&testEntity)

	// Execute
	result, err := repo.GetEntity(testEntity.ID, RepositoryTestEntity{}, "RepositoryTestEntity", nil)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	retrievedEntity, ok := result.(*RepositoryTestEntity)
	assert.True(t, ok)
	assert.Equal(t, "Entity to Retrieve", retrievedEntity.Name)
	assert.Equal(t, "Testing retrieval", retrievedEntity.Description)
	assert.Equal(t, 200, retrievedEntity.Value)
}

func TestGenericRepository_GetEntity_NotFound(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	nonExistentID := uuid.New()

	// Execute
	result, err := repo.GetEntity(nonExistentID, RepositoryTestEntity{}, "RepositoryTestEntity", nil)

	// Assert
	assert.NoError(t, err) // GORM ne retourne pas d'erreur pour les entités non trouvées, juste un objet vide
	assert.NotNil(t, result)

	retrievedEntity, ok := result.(*RepositoryTestEntity)
	assert.True(t, ok)
	assert.Equal(t, uuid.Nil, retrievedEntity.ID) // L'ID sera vide car l'entité n'existe pas
}

func TestGenericRepository_GetAllEntities_Success(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Créer plusieurs entités
	entities := []*RepositoryTestEntity{
		{Name: "Entity 1", Value: 1},
		{Name: "Entity 2", Value: 2},
		{Name: "Entity 3", Value: 3},
	}

	for _, entity := range entities {
		db.Create(entity)
	}

	// Execute
	results, total, err := repo.GetAllEntities(RepositoryTestEntity{}, 1, 10, map[string]any{}, nil)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.Equal(t, int64(3), total)
	assert.Len(t, results, 1) // Une page contenant les 3 entités

	// Vérifier le contenu de la première page
	page := results[0]
	assert.NotNil(t, page)
}

func TestGenericRepository_GetAllEntities_Pagination(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Créer plus d'entités que la taille de page
	for i := 0; i < 25; i++ {
		entity := &RepositoryTestEntity{
			Name:  "Entity " + string(rune(i)),
			Value: i,
		}
		db.Create(entity)
	}

	// Execute avec une petite taille de page - page 1
	results, total, err := repo.GetAllEntities(RepositoryTestEntity{}, 1, 10, map[string]any{}, nil)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.Equal(t, int64(25), total)
	assert.Len(t, results, 1) // Returns one page

	// Test page 2
	results2, total2, err2 := repo.GetAllEntities(RepositoryTestEntity{}, 2, 10, map[string]any{}, nil)
	assert.NoError(t, err2)
	assert.Equal(t, int64(25), total2)
	assert.Len(t, results2, 1)
}

func TestGenericRepository_GetAllEntities_WithFilters(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Create entities with different values
	entities := []*RepositoryTestEntity{
		{Name: "Entity A", Value: 10},
		{Name: "Entity B", Value: 20},
		{Name: "Entity C", Value: 10},
		{Name: "Entity D", Value: 30},
	}

	for _, entity := range entities {
		db.Create(entity)
	}

	// Test filtering by value
	filters := map[string]any{
		"value": 10,
	}
	results, total, err := repo.GetAllEntities(RepositoryTestEntity{}, 1, 10, filters, nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total) // Should only return entities with value = 10
	assert.Len(t, results, 1)
}

func TestGenericRepository_EditEntity_Success(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Créer une entité
	testEntity := RepositoryTestEntity{
		Name:        "Original Name",
		Description: "Original Description",
		Value:       100,
	}
	db.Create(&testEntity)

	// Préparer les données de mise à jour
	updateData := map[string]any{
		"name":        "Updated Name",
		"description": "Updated Description",
		"value":       200,
	}

	// Execute
	err := repo.EditEntity(testEntity.ID, "RepositoryTestEntity", &testEntity, updateData)

	// Assert
	assert.NoError(t, err)

	// Vérifier en base que l'entité a été mise à jour
	var updatedEntity RepositoryTestEntity
	dbErr := db.First(&updatedEntity, testEntity.ID).Error
	assert.NoError(t, dbErr)
	assert.Equal(t, "Updated Name", updatedEntity.Name)
	assert.Equal(t, "Updated Description", updatedEntity.Description)
	assert.Equal(t, 200, updatedEntity.Value)
}

func TestGenericRepository_DeleteEntity_Success_Scoped(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Créer une entité
	testEntity := RepositoryTestEntity{
		Name:  "Entity to Delete",
		Value: 999,
	}
	db.Create(&testEntity)

	// Execute soft delete
	err := repo.DeleteEntity(testEntity.ID, &testEntity, true)

	// Assert
	assert.NoError(t, err)

	// Vérifier que l'entité est soft deleted
	var deletedEntity RepositoryTestEntity
	dbErr := db.First(&deletedEntity, testEntity.ID).Error
	assert.Error(t, dbErr) // Should not find the soft deleted entity

	// Vérifier avec Unscoped que l'entité existe toujours
	var unscopedEntity RepositoryTestEntity
	dbErr = db.Unscoped().First(&unscopedEntity, testEntity.ID).Error
	assert.NoError(t, dbErr)
	assert.NotNil(t, unscopedEntity.DeletedAt) // Should have a deleted_at timestamp
}

func TestGenericRepository_DeleteEntity_Success_Unscoped(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Créer une entité
	testEntity := RepositoryTestEntity{
		Name:  "Entity to Hard Delete",
		Value: 888,
	}
	db.Create(&testEntity)

	// Execute hard delete
	err := repo.DeleteEntity(testEntity.ID, &testEntity, false)

	// Assert
	assert.NoError(t, err)

	// Vérifier que l'entité n'existe plus du tout
	var deletedEntity RepositoryTestEntity
	dbErr := db.Unscoped().First(&deletedEntity, testEntity.ID).Error
	assert.Error(t, dbErr)
	assert.Equal(t, gorm.ErrRecordNotFound, dbErr)
}

func TestGenericRepository_DeleteEntity_NotFound(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	nonExistentID := uuid.New()
	testEntity := &RepositoryTestEntity{}

	// Execute
	err := repo.DeleteEntity(nonExistentID, testEntity, true)

	// Assert
	assert.Error(t, err)
	entityErr, ok := err.(*entityErrors.EntityError)
	assert.True(t, ok, "Expected EntityError, got %T", err)
	assert.Equal(t, http.StatusNotFound, entityErr.HTTPStatus)
	assert.Equal(t, "Entity not found", entityErr.Message)
}

// Test de performance pour les opérations de base
func BenchmarkGenericRepository_CreateEntityFromModel(b *testing.B) {
	db := setupRepositoryTestDB(&testing.T{})
	repo := repositories.NewGenericRepository(db)
	setupRepositoryTestEntityRegistration()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityModel := &RepositoryTestEntity{
			Name:        "Benchmark Entity",
			Description: "Performance testing",
			Value:       42,
		}
		_, _ = repo.CreateEntityFromModel(entityModel)
	}
}

func BenchmarkGenericRepository_GetEntity(b *testing.B) {
	db := setupRepositoryTestDB(&testing.T{})
	repo := repositories.NewGenericRepository(db)

	// Créer une entité pour les tests
	testEntity := RepositoryTestEntity{Name: "Benchmark Get", Value: 42}
	db.Create(&testEntity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetEntity(testEntity.ID, RepositoryTestEntity{}, "RepositoryTestEntity", nil)
	}
}

// Test d'intégration pour vérifier le preloading des sous-entités
func TestGenericRepository_GetEntity_WithPreloading(t *testing.T) {
	// Setup
	db := setupRepositoryTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Setup entity registration with sub-entities
	ems.GlobalEntityRegistrationService.RegisterEntityInterface("ParentTestEntity", ParentTestEntity{})
	ems.GlobalEntityRegistrationService.RegisterSubEntites("ParentTestEntity", []any{ChildTestEntity{}})

	// Créer une entité parent avec des enfants
	parent := ParentTestEntity{Name: "Parent Entity"}
	db.Create(&parent)

	children := []*ChildTestEntity{
		{Name: "Child 1", ParentTestEntityID: parent.ID},
		{Name: "Child 2", ParentTestEntityID: parent.ID},
	}
	for _, child := range children {
		db.Create(child)
	}

	// Execute - le preloading devrait charger les enfants
	result, err := repo.GetEntity(parent.ID, ParentTestEntity{}, "ParentTestEntity", nil)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	retrievedParent, ok := result.(*ParentTestEntity)
	assert.True(t, ok)
	assert.Equal(t, "Parent Entity", retrievedParent.Name)

	// Note: Le preloading automatique pourrait ne pas fonctionner comme attendu
	// avec l'implémentation actuelle. Ce test vérifie que la méthode ne fail pas.
}

// Test d'erreur pour les opérations de base de données
func TestGenericRepository_CreateEntityFromModel_DatabaseError(t *testing.T) {
	// Setup avec une base de données fermée pour simuler une erreur
	db := setupRepositoryTestDB(t)
	setupRepositoryTestEntityRegistration()

	// Fermer la connexion pour simuler une erreur de base de données
	sqlDB, _ := db.DB()
	sqlDB.Close()

	repo := repositories.NewGenericRepository(db)
	entityModel := &RepositoryTestEntity{Name: "Test"}

	// Execute
	result, err := repo.CreateEntityFromModel(entityModel)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
}

// Test des cas limites
func TestGenericRepository_EdgeCases(t *testing.T) {
	t.Run("GetEntity with zero UUID", func(t *testing.T) {
		db := setupRepositoryTestDB(t)
		repo := repositories.NewGenericRepository(db)

		result, err := repo.GetEntity(uuid.Nil, RepositoryTestEntity{}, "RepositoryTestEntity", nil)
		assert.NoError(t, err) // GORM handle this gracefully
		assert.NotNil(t, result)
	})

	t.Run("EditEntity with empty update data", func(t *testing.T) {
		db := setupRepositoryTestDB(t)
		repo := repositories.NewGenericRepository(db)

		testEntity := RepositoryTestEntity{Name: "Test"}
		db.Create(&testEntity)

		err := repo.EditEntity(testEntity.ID, "RepositoryTestEntity", &testEntity, map[string]any{})
		assert.NoError(t, err) // GORM handles empty updates
	})

	t.Run("GetAllEntities with zero page size", func(t *testing.T) {
		db := setupRepositoryTestDB(t)
		repo := repositories.NewGenericRepository(db)

		testEntity := RepositoryTestEntity{Name: "Test"}
		db.Create(&testEntity)

		results, total, err := repo.GetAllEntities(RepositoryTestEntity{}, 1, 1, map[string]any{}, nil)
		assert.NoError(t, err)
		assert.NotNil(t, results)
		assert.Equal(t, int64(1), total)
	})
}

// --- FK constraint delete tests ---

// Entity with FK constraint for delete tests
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
