package models

type User struct {
	BaseModel
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Email        string `json:"email"`
	Password     string
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	SshKeys      []SshKey
	Roles        []Role `gorm:"many2many:user_roles;"`
}
