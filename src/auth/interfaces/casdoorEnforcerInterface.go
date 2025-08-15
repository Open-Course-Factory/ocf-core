package interfaces

// EnforcerInterface définit les méthodes de casbin.Enforcer utilisées dans l'application
type EnforcerInterface interface {
	LoadPolicy() error
	AddPolicy(params ...interface{}) (bool, error)
	RemovePolicy(params ...interface{}) (bool, error)
	RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error)
	Enforce(rvals ...interface{}) (bool, error)
	GetRolesForUser(name string) ([]string, error)
	AddGroupingPolicy(params ...interface{}) (bool, error)
	RemoveGroupingPolicy(params ...interface{}) (bool, error)
}
