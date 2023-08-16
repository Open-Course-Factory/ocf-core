package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"

	config "soli/formations/src/configuration"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoleService interface {
	CreateRole(roleCreateDTO dto.CreateRoleInput, config *config.Configuration) (*dto.RoleOutput, error)
	EditRole(editedRoleInput *dto.RoleEditInput, id uuid.UUID) (*dto.RoleEditOutput, error)
	GetRoleByType(roleType models.RoleType) (uuid.UUID, error)
	GetRoleByUser(userId uuid.UUID) (*[]models.UserRole, error)
	CreateUserRoleObjectAssociation(userId uuid.UUID, roleId uuid.UUID, objectId uuid.UUID, objectType string) (*dto.UserRoleObjectAssociationOutput, error)
}

type roleService struct {
	repository        repositories.RoleRepository
	genericRepository repositories.GenericRepository
}

func NewRoleService(db *gorm.DB) RoleService {
	return &roleService{
		repository:        repositories.NewRoleRepository(db),
		genericRepository: repositories.NewGenericRepository(db),
	}
}

func (r roleService) EditRole(editedRoleInput *dto.RoleEditInput, id uuid.UUID) (*dto.RoleEditOutput, error) {

	editRole := editedRoleInput

	editedRole, userError := r.repository.EditRole(id, *editRole)

	if userError != nil {
		return nil, userError
	}

	return editedRole, nil
}

func (r roleService) GetRoles() ([]dto.RoleOutput, error) {
	var rolesDto []dto.RoleOutput

	allPages, err := r.genericRepository.GetAllEntities(models.Role{}, 20)

	if err != nil {
		return nil, err
	}

	// Here we need to loop through the pages
	for _, page := range allPages {
		test := page.(*[]models.Role)

		for _, s := range *test {
			rolesDto = append(rolesDto, *dto.RoleModelToRoleOutput(s))
		}
	}

	return rolesDto, nil
}

func (r roleService) GetRoleByType(roleType models.RoleType) (uuid.UUID, error) {
	var result uuid.UUID

	allPages, err := r.genericRepository.GetAllEntities(models.Role{}, 20)

	if err != nil {
		return uuid.Nil, err
	}

	// Here we need to loop through the pages
	for _, page := range allPages {
		test := page.([]models.Role)

		for _, s := range test {
			if s.RoleName == roleType {
				result = s.ID
				break
			}
		}
	}

	return result, nil
}

func (r roleService) CreateRole(roleCreateDTO dto.CreateRoleInput, config *config.Configuration) (*dto.RoleOutput, error) {

	role, createRoleError := r.repository.CreateRole(roleCreateDTO)

	if createRoleError != nil {
		return nil, createRoleError
	}

	return &dto.RoleOutput{
		ID:       role.ID,
		RoleName: role.RoleName,
	}, nil

}

func (r roleService) GetRoleByUser(userId uuid.UUID) (*[]models.UserRole, error) {

	roles, getRoleError := r.repository.GetRoleByUser(userId)

	if getRoleError != nil {
		return nil, getRoleError
	}

	return roles, nil
}

func (r roleService) CreateUserRoleObjectAssociation(userId uuid.UUID, roleId uuid.UUID, objectId uuid.UUID, objectType string) (*dto.UserRoleObjectAssociationOutput, error) {

	userRoleObjectAssociation, createRoleError := r.repository.CreateUserRoleObjectAssociation(userId, roleId, objectId, objectType)

	if createRoleError != nil {
		return nil, createRoleError
	}

	return &dto.UserRoleObjectAssociationOutput{
		ID:          userRoleObjectAssociation.ID,
		UserID:      userRoleObjectAssociation.UserID,
		RoleID:      userRoleObjectAssociation.RoleID,
		SubObjectID: userRoleObjectAssociation.SubObjectID,
		SubType:     userRoleObjectAssociation.SubType,
	}, nil
}
