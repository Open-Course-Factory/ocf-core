package interfaces

// EnforcerInterface définit les méthodes de casbin.Enforcer utilisées dans l'application
type EnforcerInterface interface {
	LoadPolicy() error
	AddPolicy(params ...any) (bool, error)
	RemovePolicy(params ...any) (bool, error)
	RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error)
	Enforce(rvals ...any) (bool, error)
	GetRolesForUser(name string) ([]string, error)
	AddGroupingPolicy(params ...any) (bool, error)
	RemoveGroupingPolicy(params ...any) (bool, error)
}
