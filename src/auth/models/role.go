package models

import "soli/formations/src/auth/types"

type RoleType string

const (
	RoleTypeInstanceAdmin      RoleType = "instance_admin"
	RoleTypeOrganisationAdmin  RoleType = "organisation_admin"
	RoleTypeOrganisationMember RoleType = "organisation_member"
	RoleTypeObjectOwner        RoleType = "object_owner"
	RoleTypeObjectEditor       RoleType = "object_editor"
	RoleTypeObjectReader       RoleType = "object_reader"
)

type Role struct {
	BaseModel
	RoleName    RoleType           `json:"roleName"`
	Permissions []types.Permission `gorm:"serializer:json" json:"permissions"`
}
