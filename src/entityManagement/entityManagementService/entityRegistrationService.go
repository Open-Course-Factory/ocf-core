package services

import (
	"reflect"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ConversionWay int

const (
	InputDtoToModel ConversionWay = iota
	OutputModelToDto
)

type DtoWay int

const (
	InputDto DtoWay = iota
	OutputDto
)

type EntityRegistrationService struct {
	registry  map[string]interface{}
	functions map[ConversionWay]interface{}
	dtos      map[string]map[DtoWay]interface{}
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:  make(map[string]interface{}),
		functions: make(map[ConversionWay]interface{}),
		dtos:      make(map[string]map[DtoWay]interface{}),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType interface{}) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, converters entityManagementInterfaces.EntityConverters) {
	s.functions[OutputModelToDto] = converters.ModelToDto
	s.functions[InputDtoToModel] = converters.DtoToModel
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
		function, exists = s.functions[OutputModelToDto]
	case InputDtoToModel:
		function, exists = s.functions[InputDtoToModel]
	default:
		function = nil
		exists = false
	}

	return function, exists
}

func (s *EntityRegistrationService) RegisterEntity(input entityManagementInterfaces.RegistrableInterface) {
	entityToRegister := input.GetEntityRegistrationInput()
	GlobalEntityRegistrationService.RegisterEntityInterface(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityInterface)
	GlobalEntityRegistrationService.RegisterEntityConversionFunctions(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntityConverters)
	sshkeyDtos := make(map[DtoWay]interface{})
	sshkeyDtos[InputDto] = entityToRegister.EntityDtos.InputDto
	sshkeyDtos[OutputDto] = entityToRegister.EntityDtos.OutputDto
	GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(entityToRegister.EntityInterface).Name(), sshkeyDtos)
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
