package models

type RoleType string

const (
	RoleTypeInstanceAdmin     RoleType = "instance_admin"
	RoleTypeOrganisationAdmin RoleType = "organisation_admin"
	RoleTypeObjectOwner       RoleType = "object_owner"
	RoleTypeObjectEditor      RoleType = "object_editor"
	RoleTypeObjectReader      RoleType = "object_reader"
)

type Role struct {
	BaseModel
	RoleName    RoleType     `json:"roleName"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions"`
}
