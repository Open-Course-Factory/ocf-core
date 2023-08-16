package types

import "database/sql/driver"

type Permission string

const (
	PermissionTypeRead   Permission = "read"
	PermissionTypeWrite  Permission = "write"
	PermissionTypeDelete Permission = "delete"
	PermissionTypeAll    Permission = "all"
)

func ContainsPermissionType(enumArray []Permission, value Permission) bool {
	for _, v := range enumArray {
		if v == value {
			return true
		}
	}
	return false
}

func (e *Permission) Scan(value interface{}) error {
	*e = Permission(value.([]byte))
	return nil
}

func (e Permission) Value() (driver.Value, error) {
	return string(e), nil
}
