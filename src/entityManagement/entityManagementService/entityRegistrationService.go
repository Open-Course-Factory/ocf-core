package services

import (
	"log"
	"reflect"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
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
	registry    map[string]any
	functions   map[string]map[ConversionPurpose]any
	dtos        map[string]map[DtoPurpose]any
	subEntities map[string][]any
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:    make(map[string]any),
		functions:   make(map[string]map[ConversionPurpose]any),
		dtos:        make(map[string]map[DtoPurpose]any),
		subEntities: make(map[string][]any),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType any) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterSubEntites(name string, subEntities []any) {
	s.subEntities[name] = subEntities
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, converters entityManagementInterfaces.EntityConverters) {
	ways := make(map[ConversionPurpose]any)

	ways[OutputModelToDto] = converters.ModelToDto
	ways[CreateInputDtoToModel] = converters.DtoToModel
	ways[EditInputDtoToMap] = converters.DtoToMap

	s.functions[name] = ways
}

func (s *EntityRegistrationService) RegisterEntityDtos(name string, dtos map[DtoPurpose]any) {
	s.dtos[name] = dtos
}

func (s *EntityRegistrationService) GetEntityInterface(name string) (any, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

func (s *EntityRegistrationService) GetEntityDtos(name string, way DtoPurpose) any {
	return s.dtos[name][way]
}

func (s *EntityRegistrationService) GetConversionFunction(name string, way ConversionPurpose) (any, bool) {
	var function any
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

func (s *EntityRegistrationService) GetSubEntites(entityName string) []any {
	return s.subEntities[entityName]
}

// SetDefaultEntityAccesses est une version publique pour les tests qui accepte un enforcer
func (s *EntityRegistrationService) SetDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles, enforcer interfaces.EnforcerInterface) {
	s.setDefaultEntityAccesses(entityName, roles, enforcer)
}

func (s *EntityRegistrationService) setDefaultEntityAccesses(entityName string, roles entityManagementInterfaces.EntityRoles, enforcer interfaces.EnforcerInterface) {
	if enforcer == nil {
		log.Println("Enforcer is nil, skipping access setup")
		return
	}

	errLoadingPolicy := enforcer.LoadPolicy()
	if errLoadingPolicy != nil {
		log.Fatal(errLoadingPolicy.Error())
	}
	rolesMap := roles.Roles

	resourceName := Pluralize(entityName)
	resourceName = strings.ToLower(resourceName)

	for roleName, accessGiven := range rolesMap {
		_, errPolicy := enforcer.AddPolicy(roleName, "/api/v1/"+resourceName+"/", accessGiven)
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
	entityDtos := make(map[DtoPurpose]any)
	entityDtos[InputCreateDto] = entityToRegister.EntityDtos.InputCreateDto
	entityDtos[OutputDto] = entityToRegister.EntityDtos.OutputDto
	entityDtos[InputEditDto] = entityToRegister.EntityDtos.InputEditDto
	GlobalEntityRegistrationService.RegisterEntityDtos(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityDtos)
	GlobalEntityRegistrationService.RegisterSubEntites(reflect.TypeOf(entityToRegister.EntityInterface).Name(), entityToRegister.EntitySubEntities)

	// Utiliser la variable globale casdoor.Enforcer en production
	s.setDefaultEntityAccesses(reflect.TypeOf(entityToRegister.EntityInterface).Name(), input.GetEntityRoles(), casdoor.Enforcer)
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
