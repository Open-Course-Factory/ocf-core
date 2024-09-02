package services

import (
	"log"
	"reflect"
	"soli/formations/src/auth/casdoor"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"strings"

	"github.com/gertd/go-pluralize"
)

type ConversionWay int

const (
	InputDtoToModel ConversionWay = iota
	OutputModelToDto
	InputDtoToMap
)

type DtoWay int

const (
	InputDto DtoWay = iota
	OutputDto
)

type EntityRegistrationService struct {
	registry  map[string]interface{}
	functions map[string]map[ConversionWay]interface{}
	dtos      map[string]map[DtoWay]interface{}
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:  make(map[string]interface{}),
		functions: make(map[string]map[ConversionWay]interface{}),
		dtos:      make(map[string]map[DtoWay]interface{}),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType interface{}) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, converters entityManagementInterfaces.EntityConverters) {
	ways := make(map[ConversionWay]interface{})

	ways[OutputModelToDto] = converters.ModelToDto
	ways[InputDtoToModel] = converters.DtoToModel
	ways[InputDtoToMap] = converters.DtoToMap

	s.functions[name] = ways
}

func (s *EntityRegistrationService) RegisterEntityDtos(name string, dtos map[DtoWay]interface{}) {
	s.dtos[name] = dtos
}

func (s *EntityRegistrationService) GetEntityInterface(name string) (interface{}, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

func (s *EntityRegistrationService) GetEntityDtos(name string, way DtoWay) interface{} {
	return s.dtos[name][way]
}

func (s *EntityRegistrationService) GetConversionFunction(name string, way ConversionWay) (interface{}, bool) {
	var function interface{}
	var exists bool
	switch way {
	case OutputModelToDto:
		function, exists = s.functions[name][OutputModelToDto]
	case InputDtoToModel:
		function, exists = s.functions[name][InputDtoToModel]
	case InputDtoToMap:
		function, exists = s.functions[name][InputDtoToMap]
	default:
		function = nil
		exists = false
	}

	return function, exists
}

func (s *EntityRegistrationService) setDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles) {
	errLoadingPolicy := casdoor.Enforcer.LoadPolicy()
	if errLoadingPolicy != nil {
		log.Fatal(errLoadingPolicy.Error())
	}
	rolesMap := roles.Roles

	client := pluralize.NewClient()
	singular := client.Plural(entityName)
	resourceName := strings.ToLower(singular)

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

func (s *EntityRegistrationService) RegisterEntity(input entityManagementInterfaces.RegistrableInterface) {
	entityToRegister := input.GetEntityRegistrationInput()
	GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityInterface)
	GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityConverters)
	entityDtos := make(map[DtoWay]interface{})
	entityDtos[InputDto] = entityToRegister.EntityDtos.InputDto
	entityDtos[OutputDto] = entityToRegister.EntityDtos.OutputDto
	GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityDtos)
	s.setDefaultEntityAccesses(reflect.TypeOf(entityToRegister.EntityInterface).Name(), input.GetEntityRoles())
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
