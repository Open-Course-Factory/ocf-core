package entityManagementInterfaces

import (
	"net/http"
	"soli/formations/src/auth/models"

	"github.com/mitchellh/mapstructure"
)

type EntityRegistrationInput struct {
	EntityInterface   interface{}
	EntityConverters  EntityConverters
	EntityDtos        EntityDtos
	EntityRoles       EntityRoles
	EntitySubEntities []interface{}
}

type EntityConverters struct {
	ModelToDto interface{}
	DtoToModel interface{}
	DtoToMap   interface{}
}

type EntityDtos struct {
	InputCreateDto interface{}
	InputEditDto   interface{}
	OutputDto      interface{}
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

func (a AbstractRegistrableInterface) EntityDtoToMap(input interface{}) map[string]interface{} {
	resMap := make(map[string]interface{})

	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &resMap,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	decoder.Decode(input)

	return resMap
}
