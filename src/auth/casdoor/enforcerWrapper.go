package casdoor

import (
	"soli/formations/src/auth/interfaces"

	"github.com/casbin/casbin/v2"
)

// enforcerWrapper wraps casbin.Enforcer to implement EnforcerInterface
type enforcerWrapper struct {
	enforcer *casbin.Enforcer
}

// NewEnforcerWrapper creates a new wrapper around the casbin enforcer
func NewEnforcerWrapper(enforcer *casbin.Enforcer) interfaces.EnforcerInterface {
	return &enforcerWrapper{
		enforcer: enforcer,
	}
}

func (e *enforcerWrapper) LoadPolicy() error {
	return e.enforcer.LoadPolicy()
}

func (e *enforcerWrapper) AddPolicy(params ...interface{}) (bool, error) {
	return e.enforcer.AddPolicy(params...)
}

func (e *enforcerWrapper) RemovePolicy(params ...interface{}) (bool, error) {
	return e.enforcer.RemovePolicy(params...)
}

func (e *enforcerWrapper) RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	return e.enforcer.RemoveFilteredPolicy(fieldIndex, fieldValues...)
}

func (e *enforcerWrapper) Enforce(rvals ...interface{}) (bool, error) {
	return e.enforcer.Enforce(rvals...)
}

func (e *enforcerWrapper) GetRolesForUser(name string) ([]string, error) {
	return e.enforcer.GetRolesForUser(name)
}

func (e *enforcerWrapper) AddGroupingPolicy(params ...interface{}) (bool, error) {
	return e.enforcer.AddGroupingPolicy(params...)
}

func (e *enforcerWrapper) RemoveGroupingPolicy(params ...interface{}) (bool, error) {
	return e.enforcer.RemoveGroupingPolicy(params...)
}
