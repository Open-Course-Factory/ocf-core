package services

import (
	"log"
	"reflect"
	"soli/formations/src/auth/casdoor"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"strings"

	"github.com/gertd/go-pluralize"
)

type ConversionPurpose int

const (
	CreateInputDtoToModel ConversionPurpose = iota
	OutputModelToDto
	EditInputDtoToMap
)

type DtoPurpose int

const (
	InputCreateDto DtoPurpose = iota
	InputEditDto
	OutputDto
)

type EntityRegistrationService struct {
	registry    map[string]interface{}
	functions   map[string]map[ConversionPurpose]interface{}
	dtos        map[string]map[DtoPurpose]interface{}
	subEntities map[string][]interface{}
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:    make(map[string]interface{}),
		functions:   make(map[string]map[ConversionPurpose]interface{}),
		dtos:        make(map[string]map[DtoPurpose]interface{}),
		subEntities: make(map[string][]interface{}),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType interface{}) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterSubEntites(name string, subEntities []interface{}) {
	s.subEntities[name] = subEntities
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, converters entityManagementInterfaces.EntityConverters) {
	ways := make(map[ConversionPurpose]interface{})

	ways[OutputModelToDto] = converters.ModelToDto
	ways[CreateInputDtoToModel] = converters.DtoToModel
	ways[EditInputDtoToMap] = converters.DtoToMap

	s.functions[name] = ways
}

func (s *EntityRegistrationService) RegisterEntityDtos(name string, dtos map[DtoPurpose]interface{}) {
	s.dtos[name] = dtos
}

func (s *EntityRegistrationService) GetEntityInterface(name string) (interface{}, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

func (s *EntityRegistrationService) GetEntityDtos(name string, way DtoPurpose) interface{} {
	return s.dtos[name][way]
}

func (s *EntityRegistrationService) GetConversionFunction(name string, way ConversionPurpose) (interface{}, bool) {
	var function interface{}
	var exists bool
	switch way {
	case OutputModelToDto:
		function, exists = s.functions[name][OutputModelToDto]
	case CreateInputDtoToModel:
		function, exists = s.functions[name][CreateInputDtoToModel]
	case EditInputDtoToMap:
		function, exists = s.functions[name][EditInputDtoToMap]
	default:
		function = nil
		exists = false
	}

	return function, exists
}

func (s *EntityRegistrationService) GetSubEntites(entityName string) []interface{} {
	return s.subEntities[entityName]
}

func (s *EntityRegistrationService) setDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles) {
	errLoadingPolicy := casdoor.Enforcer.LoadPolicy()
	if errLoadingPolicy != nil {
		log.Fatal(errLoadingPolicy.Error())
	}
	rolesMap := roles.Roles

	resourceName := Pluralize(entityName)
	resourceName = strings.ToLower(resourceName)

	for roleName, accessGiven := range rolesMap {
		_, errPolicy := casdoor.Enforcer.AddPolicy(roleName, "/api/v1/"+resourceName+"/", accessGiven)
		if errPolicy != nil {
			if strings.Contains(errPolicy.Error(), "UNIQUE") {
				log.Println(errPolicy.Error())
			} else {
				log.Fatal(errPolicy.Error())
			}
		}
	}

}

func Pluralize(entityName string) string {
	client := pluralize.NewClient()
	plural := client.Plural(entityName)
	return plural
}

func (s *EntityRegistrationService) RegisterEntity(input entityManagementInterfaces.RegistrableInterface) {
	entityToRegister := input.GetEntityRegistrationInput()
	GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityInterface)
	GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityConverters)
	entityDtos := make(map[DtoPurpose]interface{})
	entityDtos[InputCreateDto] = entityToRegister.EntityDtos.InputCreateDto
	entityDtos[OutputDto] = entityToRegister.EntityDtos.OutputDto
	entityDtos[InputEditDto] = entityToRegister.EntityDtos.InputEditDto
	GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityDtos)
	GlobalEntityRegistrationService.RegisterSubEntites(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntitySubEntities)
	s.setDefaultEntityAccesses(reflect.TypeOf(entityToRegister.EntityInterface).Name(), input.GetEntityRoles())
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
