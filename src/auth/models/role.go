package models

type Role struct {
	BaseModel
	RoleName string `json:"roleName"`
}
