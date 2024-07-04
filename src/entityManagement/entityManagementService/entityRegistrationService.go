package services

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
	functions map[string]interface{}
	dtos      map[string]map[DtoWay]interface{}
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:  make(map[string]interface{}),
		functions: make(map[string]interface{}),
		dtos:      make(map[string]map[DtoWay]interface{}),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType interface{}) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterEntityConversionFunctions(name string, funcNameToDto interface{}, funcNameToModel interface{}) {
	s.functions[name+"ModelTo"+name+"Output"] = funcNameToDto
	s.functions[name+"InputDtoTo"+name+"Model"] = funcNameToModel
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
		function, exists = s.functions[name+"ModelTo"+name+"Output"]
	case InputDtoToModel:
		function, exists = s.functions[name+"InputDtoTo"+name+"Model"]
	default:
		function = nil
		exists = false
	}

	return function, exists
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
