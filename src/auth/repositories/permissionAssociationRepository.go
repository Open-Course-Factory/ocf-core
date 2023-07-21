package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionAssociationRepository interface {
	CreatePermissionAssociation(permissionAssociationDto dto.PermissionAssociationInput) (*models.PermissionAssociation, error)
	//EditPermissionAssociation(id uuid.UUID, PermissionAssociationInfos dto.PermissionAssociationInput) (*dto.PermissionAssociationOutput, error)
}

type permissionAssociationRepository struct {
	GenericRepository
	db *gorm.DB
}

func NewPermissionAssociationRepository(db *gorm.DB) PermissionAssociationRepository {
	repository := &permissionAssociationRepository{
		GenericRepository: NewGenericRepository(db),
		db:                db,
	}
	return repository
}

func (p permissionAssociationRepository) CreatePermissionAssociation(permissionAssociationdto dto.PermissionAssociationInput) (*models.PermissionAssociation, error) {

	permAssociationObjectRepo := NewPermissionAssociationObjectRepository(p.db)

	id, errPAO := uuid.Parse(permissionAssociationdto.PermissionID)

	if errPAO != nil {
		return nil, errPAO
	}

	perm, err := p.GetEntity(id, models.Permission{})
	if err != nil {
		return nil, err
	}

	var permissionAssociationObjects []models.PermissionAssociationObject

	for _, permissionAssociationDtoInputObject := range permissionAssociationdto.PermissionAssociationObjects {
		permissionAssociationObject, errPAO := permAssociationObjectRepo.GetPermissionAssociationObject(permissionAssociationDtoInputObject.SubObjectID, permissionAssociationDtoInputObject.SubType)

		if errPAO != nil {
			if errPAO.Error() == "record not found" {
				var errPAO2 error
				permissionAssociationObjectModel := models.PermissionAssociationObject{
					SubObjectID: permissionAssociationDtoInputObject.SubObjectID,
					SubType:     permissionAssociationDtoInputObject.SubType,
				}
				permissionAssociationObject, errPAO2 = permAssociationObjectRepo.CreatePermissionAssociationObject(&permissionAssociationObjectModel)

				if errPAO2 != nil {
					return nil, errPAO2
				}
			} else {
				return nil, errPAO
			}

		}

		permissionAssociationObjects = append(permissionAssociationObjects, *permissionAssociationObject)

	}

	permissionAssociation := models.PermissionAssociation{
		Permission:                   *perm.(*models.Permission),
		PermissionAssociationObjects: permissionAssociationObjects,
	}

	result := p.db.Create(&permissionAssociation)
	if result.Error != nil {
		return nil, result.Error
	}
	return &permissionAssociation, nil
}
