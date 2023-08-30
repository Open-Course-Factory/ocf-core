package services

import (
	"reflect"
)

// EntityRegistrationService is responsible for registering and retrieving entities
type EntityRegistrationService struct {
	registry map[string]reflect.Type
}

// NewEntityRegistrationService creates a new EntityRegistrationService
func NewEntityRegistrationService() *EntityRegistrationService {
	return &EntityRegistrationService{
		registry: make(map[string]reflect.Type),
	}
}

// RegisterEntityType registers an entity with a given name
func (s *EntityRegistrationService) RegisterEntityType(name string, entityType reflect.Type) {
	s.registry[name] = entityType
}

// GetEntityType retrieves the entity type by name
func (s *EntityRegistrationService) GetEntityType(name string) (reflect.Type, bool) {
	entityType, exists := s.registry[name]
	return entityType, exists
}

var GlobalEntityRegistrationService = NewEntityRegistrationService()
