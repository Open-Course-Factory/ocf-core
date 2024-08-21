package entityManagementInterfaces

import (
	"net/http"
	"soli/formations/src/auth/models"
)

type EntityRegistrationInput struct {
	EntityInterface  interface{}
	EntityConverters EntityConverters
	EntityDtos       EntityDtos
	EntityRoles      EntityRoles
}

type EntityConverters struct {
	ModelToDto interface{}
	DtoToModel interface{}
}

type EntityDtos struct {
	InputDto  interface{}
	OutputDto interface{}
}

type EntityRoles struct {
	Roles map[string]string
}

type RegistrableInterface interface {
	GetEntityRegistrationInput() EntityRegistrationInput
	EntityModelToEntityOutput(input any) any
	EntityInputDtoToEntityModel(input any) any
	GetEntityRoles() EntityRoles
}

type AbstractRegistrableInterface struct{ RegistrableInterface }

func (a AbstractRegistrableInterface) GetEntityRoles() EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(models.Student)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	res := EntityRoles{
		Roles: roleMap,
	}
	return res
}
