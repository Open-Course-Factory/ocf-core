package entityManagementInterfaces

import (
	"net/http"
	"soli/formations/src/auth/models"

	"github.com/mitchellh/mapstructure"
)

type EntityRegistrationInput struct {
	EntityInterface   any
	EntityConverters  EntityConverters
	EntityDtos        EntityDtos
	EntityRoles       EntityRoles
	EntitySubEntities []any
	SwaggerConfig     *EntitySwaggerConfig `json:"swagger_config,omitempty"`
}

type EntityConverters struct {
	ModelToDto any
	DtoToModel any
	DtoToMap   any
}

type EntityDtos struct {
	InputCreateDto any
	InputEditDto   any
	OutputDto      any
}

type EntityRoles struct {
	Roles map[string]string
}

type RegistrableInterface interface {
	GetEntityRegistrationInput() EntityRegistrationInput
	EntityModelToEntityOutput(input any) (any, error)
	EntityInputDtoToEntityModel(input any) any
	GetEntityRoles() EntityRoles
}

type AbstractRegistrableInterface struct{ RegistrableInterface }

func (a AbstractRegistrableInterface) GetEntityRoles() EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(models.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	res := EntityRoles{
		Roles: roleMap,
	}
	return res
}

func (a AbstractRegistrableInterface) EntityDtoToMap(input any) map[string]any {
	resMap := make(map[string]any)

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
