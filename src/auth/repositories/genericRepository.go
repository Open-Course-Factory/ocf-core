package repositories

import (
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenericRepository interface {
	GetEntity(id uuid.UUID, data interface{}) (interface{}, error)
	GetAllEntities(data interface{}, pageSize int) ([]interface{}, error)
	//GetAllEntitiesByUser(userId uuid.UUID, data interface{}) ([]*interface{}, error)
	DeleteEntity(id uuid.UUID, data interface{}) error
}

type genericRepository struct {
	db *gorm.DB
}

func NewGenericRepository(db *gorm.DB) GenericRepository {
	repository := &genericRepository{
		db: db,
	}
	return repository
}

func (o *genericRepository) GetEntity(id uuid.UUID, data interface{}) (interface{}, error) {
	model := reflect.New(reflect.TypeOf(data)).Interface()

	result := o.db.First(model, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return model, nil
}

func (o *genericRepository) GetAllEntities(data interface{}, pageSize int) ([]interface{}, error) {
	dtype := reflect.TypeOf(data)
	var allPages []interface{}

	page := 1
	for {
		pages := reflect.New(reflect.SliceOf(dtype)).Elem().Interface()
		offset := (page - 1) * pageSize

		// Fetch a page of records /!\ the massive preload coul be a problem with lots of data in time
		result := o.db.Model(data).Preload(clause.Associations).Limit(pageSize).Offset(offset).Find(&pages)
		if result.Error != nil {
			return nil, result.Error
		}

		// If no more records found, break the loop
		if result.RowsAffected == 0 {
			break
		}

		allPages = append(allPages, pages)
		page++
	}

	return allPages, nil
}

func (o *genericRepository) DeleteEntity(id uuid.UUID, data interface{}) error {

	model := reflect.New(reflect.TypeOf(data)).Interface()
	result := o.db.Delete(&model, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// func (o *entityRepository) GetAllEntitiesByUser(userId uuid.UUID) ([]*models.Entity, error) {

// 	// ToDo: add role management
// 	var permissions []*models.Permission
// 	entityType := reflect.TypeOf(models.Entity{}).Name()
// 	result := o.db.
// 		Joins("left join entities on permissions.entity_id = entities.id").
// 		Preload(entityType).
// 		Where("permissions.user_id = ?", userId).
// 		Find(&permissions)
// 	if result.Error != nil {
// 		return nil, result.Error
// 	}

// 	var readableEntities []*models.Entity
// 	// Check permissions for each entity
// 	for _, permission := range permissions {
// 		// Deserialize the permissions
// 		if models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeRead) || models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeAll) {
// 			readableEntities = append(readableEntities, permission.Entity)
// 		}
// 	}

// 	return readableEntities, nil
// }

// func (o *entityRepository) DeleteEntity(id uuid.UUID) error {
// 	result := o.db.Delete(&models.Entity{}, id)
// 	if result.Error != nil {
// 		return result.Error
// 	}
// 	return nil
// }

// func (o *entityRepository) EditEntity(entity *dto.EntityEditInput) (*dto.EntityEditOutput, error) {
// 	result := o.db.Save(&entity)
// 	if result.Error != nil {
// 		return nil, result.Error
// 	}
// 	return &dto.EntityEditOutput{
// 		Name: entity.Name,
// 	}, nil
// }
