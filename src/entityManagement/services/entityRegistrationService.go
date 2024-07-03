package services

type EntityRegistrationService struct {
	registry  map[string]interface{}
	functions map[string]interface{}
}

func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry:  make(map[string]interface{}),
		functions: make(map[string]interface{}),
	}
}

func (s *EntityRegistrationService) RegisterEntityInterface(name string, entityType interface{}) {
	s.registry[name] = entityType
}

func (s *EntityRegistrationService) RegisterEntityToOutputOutFunctionputDto(name string, funcName interface{}) {
	s.functions[name+"ModelTo"+name+"Output"] = funcName
}

func (s *EntityRegistrationService) GetEntityInterface(name string) (interface{}, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

func (s *EntityRegistrationService) GetEntityToOutputDtoConversionFunction(name string) (interface{}, bool) {
	funcName, exists := s.functions[name]
	return funcName, exists
}

func (s *EntityRegistrationService) GetConversionFunction(name string) (interface{}, bool) {
	function, exists := s.functions[name]
	return function, exists
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
