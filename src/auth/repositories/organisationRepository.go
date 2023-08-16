package repositories

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganisationRepository interface {
	CreateOrganisation(organisationdto dto.CreateOrganisationInput) (*models.Organisation, error)
	GetAllOrganisationsByUser(userId uuid.UUID) ([]*models.Organisation, error)
	EditOrganisation(organisation *dto.OrganisationEditInput) (*dto.OrganisationEditOutput, error)
}

type organisationRepository struct {
	db *gorm.DB
}

func NewOrganisationRepository(db *gorm.DB) OrganisationRepository {
	repository := &organisationRepository{
		db: db,
	}
	return repository
}

func (r *organisationRepository) CreateOrganisation(organisationdto dto.CreateOrganisationInput) (*models.Organisation, error) {
	organisation := models.Organisation{
		OrganisationName: organisationdto.Name,
	}
	err := r.db.Create(&organisation).Error
	if err != nil {
		return nil, err
		//return nil, err
	}
	return &organisation, nil
}

func (o *organisationRepository) GetAllOrganisationsByUser(userId uuid.UUID) ([]*models.Organisation, error) {

	// ToDo: add role management
	var permissions []*models.Permission
	entityType := reflect.TypeOf(models.Organisation{}).Name()
	result := o.db.
		Joins("left join organisations on permissions.organisation_id = organisations.id").
		Preload(entityType).
		Where("permissions.user_id = ?", userId).
		Find(&permissions)
	if result.Error != nil {
		return nil, result.Error
	}

	var readableOrganisations []*models.Organisation
	// Check permissions for each organisation
	// for _, permission := range permissions {
	// 	// Deserialize the permissions
	// 	if models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeRead) || models.ContainsPermissionType(permission.PermissionTypes, models.PermissionTypeAll) {
	// 		readableOrganisations = append(readableOrganisations, permission.Organisation)
	// 	}
	// }

	return readableOrganisations, nil
}

func (o *organisationRepository) EditOrganisation(organisation *dto.OrganisationEditInput) (*dto.OrganisationEditOutput, error) {
	result := o.db.Save(&organisation)
	if result.Error != nil {
		return nil, result.Error
	}
	return &dto.OrganisationEditOutput{
		Name: organisation.Name,
	}, nil
}
