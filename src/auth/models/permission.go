package models

type PermissionType string

const (
	PermissionTypeRead   PermissionType = "read"
	PermissionTypeWrite  PermissionType = "write"
	PermissionTypeDelete PermissionType = "delete"
	PermissionTypeAll    PermissionType = "all"
)

func ContainsPermissionType(enumArray []PermissionType, value PermissionType) bool {
	for _, v := range enumArray {
		if v == value {
			return true
		}
	}
	return false
}

type Permission struct {
	BaseModel
	PermissionTypes []PermissionType `gorm:"serializer:json" json:"permission_types"`
}
