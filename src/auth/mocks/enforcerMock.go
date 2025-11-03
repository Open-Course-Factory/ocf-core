package mocks

import "soli/formations/src/auth/interfaces"

// MockEnforcer is a mock implementation of EnforcerInterface for testing
type MockEnforcer struct {
	LoadPolicyFunc                    func() error
	AddPolicyFunc                     func(params ...any) (bool, error)
	RemovePolicyFunc                  func(params ...any) (bool, error)
	RemoveFilteredPolicyFunc          func(fieldIndex int, fieldValues ...string) (bool, error)
	EnforceFunc                       func(rvals ...any) (bool, error)
	GetRolesForUserFunc               func(name string) ([]string, error)
	AddGroupingPolicyFunc             func(params ...any) (bool, error)
	RemoveGroupingPolicyFunc          func(params ...any) (bool, error)
	GetImplicitPermissionsForUserFunc func(name string) ([][]string, error)
	GetFilteredPolicyFunc             func(fieldIndex int, fieldValues ...string) ([][]string, error)
	GetPolicyFunc                     func() ([][]string, error)

	// Pour tracer les appels si nécessaire
	LoadPolicyCalls                    [][]any
	AddPolicyCalls                     [][]any
	RemovePolicyCalls                  [][]any
	RemoveFilteredPolicyCalls          [][]any
	EnforceCalls                       [][]any
	GetRolesForUserCalls               [][]any
	AddGroupingPolicyCalls             [][]any
	RemoveGroupingPolicyCalls          [][]any
	GetImplicitPermissionsForUserCalls [][]any
	GetFilteredPolicyCalls             [][]any
	GetPolicyCalls                     int
}

// NewMockEnforcer creates a new mock enforcer with default implementations
func NewMockEnforcer() *MockEnforcer {
	return &MockEnforcer{
		LoadPolicyFunc: func() error {
			return nil // Success by default
		},
		AddPolicyFunc: func(params ...any) (bool, error) {
			return true, nil // Success by default
		},
		RemovePolicyFunc: func(params ...any) (bool, error) {
			return true, nil // Success by default
		},
		RemoveFilteredPolicyFunc: func(fieldIndex int, fieldValues ...string) (bool, error) {
			return true, nil // Success by default
		},
		EnforceFunc: func(rvals ...any) (bool, error) {
			return true, nil // Authorized by default
		},
		GetRolesForUserFunc: func(name string) ([]string, error) {
			return []string{"member"}, nil // Default role
		},
		AddGroupingPolicyFunc: func(params ...any) (bool, error) {
			return true, nil // Success by default
		},
		RemoveGroupingPolicyFunc: func(params ...any) (bool, error) {
			return true, nil // Success by default
		},
		GetImplicitPermissionsForUserFunc: func(name string) ([][]string, error) {
			return [][]string{}, nil // Empty permissions by default
		},
		GetFilteredPolicyFunc: func(fieldIndex int, fieldValues ...string) ([][]string, error) {
			return [][]string{}, nil // Empty by default
		},
		GetPolicyFunc: func() ([][]string, error) {
			return [][]string{}, nil // Empty by default
		},
		LoadPolicyCalls:                    make([][]any, 0),
		AddPolicyCalls:                     make([][]any, 0),
		RemovePolicyCalls:                  make([][]any, 0),
		RemoveFilteredPolicyCalls:          make([][]any, 0),
		EnforceCalls:                       make([][]any, 0),
		GetRolesForUserCalls:               make([][]any, 0),
		AddGroupingPolicyCalls:             make([][]any, 0),
		RemoveGroupingPolicyCalls:          make([][]any, 0),
		GetImplicitPermissionsForUserCalls: make([][]any, 0),
		GetFilteredPolicyCalls:             make([][]any, 0),
		GetPolicyCalls:                     0,
	}
}

// Ensure MockEnforcer implements EnforcerInterface
var _ interfaces.EnforcerInterface = (*MockEnforcer)(nil)

func (m *MockEnforcer) LoadPolicy() error {
	m.LoadPolicyCalls = append(m.LoadPolicyCalls, []any{})
	return m.LoadPolicyFunc()
}

func (m *MockEnforcer) AddPolicy(params ...any) (bool, error) {
	m.AddPolicyCalls = append(m.AddPolicyCalls, params)
	return m.AddPolicyFunc(params...)
}

func (m *MockEnforcer) RemovePolicy(params ...any) (bool, error) {
	m.RemovePolicyCalls = append(m.RemovePolicyCalls, params)
	return m.RemovePolicyFunc(params...)
}

func (m *MockEnforcer) RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	params := make([]any, len(fieldValues)+1)
	params[0] = fieldIndex
	for i, v := range fieldValues {
		params[i+1] = v
	}
	m.RemoveFilteredPolicyCalls = append(m.RemoveFilteredPolicyCalls, params)
	return m.RemoveFilteredPolicyFunc(fieldIndex, fieldValues...)
}

func (m *MockEnforcer) Enforce(rvals ...any) (bool, error) {
	m.EnforceCalls = append(m.EnforceCalls, rvals)
	return m.EnforceFunc(rvals...)
}

func (m *MockEnforcer) GetRolesForUser(name string) ([]string, error) {
	m.GetRolesForUserCalls = append(m.GetRolesForUserCalls, []any{name})
	return m.GetRolesForUserFunc(name)
}

func (m *MockEnforcer) AddGroupingPolicy(params ...any) (bool, error) {
	m.AddGroupingPolicyCalls = append(m.AddGroupingPolicyCalls, params)
	return m.AddGroupingPolicyFunc(params...)
}

func (m *MockEnforcer) RemoveGroupingPolicy(params ...any) (bool, error) {
	m.RemoveGroupingPolicyCalls = append(m.RemoveGroupingPolicyCalls, params)
	return m.RemoveGroupingPolicyFunc(params...)
}

// Helper methods for testing
func (m *MockEnforcer) Reset() {
	m.LoadPolicyCalls = make([][]any, 0)
	m.AddPolicyCalls = make([][]any, 0)
	m.RemovePolicyCalls = make([][]any, 0)
	m.RemoveFilteredPolicyCalls = make([][]any, 0)
	m.EnforceCalls = make([][]any, 0)
	m.GetRolesForUserCalls = make([][]any, 0)
	m.AddGroupingPolicyCalls = make([][]any, 0)
	m.RemoveGroupingPolicyCalls = make([][]any, 0)
	m.GetImplicitPermissionsForUserCalls = make([][]any, 0)
	m.GetFilteredPolicyCalls = make([][]any, 0)
	m.GetPolicyCalls = 0
}

func (m *MockEnforcer) GetAddPolicyCallCount() int {
	return len(m.AddPolicyCalls)
}

func (m *MockEnforcer) GetLoadPolicyCallCount() int {
	return len(m.LoadPolicyCalls)
}

func (m *MockEnforcer) GetRemovePolicyCallCount() int {
	return len(m.RemovePolicyCalls)
}

func (m *MockEnforcer) GetAddGroupingPolicyCallCount() int {
	return len(m.AddGroupingPolicyCalls)
}

func (m *MockEnforcer) GetRemoveGroupingPolicyCallCount() int {
	return len(m.RemoveGroupingPolicyCalls)
}

func (m *MockEnforcer) GetRemoveFilteredPolicyCallCount() int {
	return len(m.RemoveFilteredPolicyCalls)
}

func (m *MockEnforcer) GetEnforceCallCount() int {
	return len(m.EnforceCalls)
}

func (m *MockEnforcer) GetGetRolesForUserCallCount() int {
	return len(m.GetRolesForUserCalls)
}

func (m *MockEnforcer) GetImplicitPermissionsForUser(name string) ([][]string, error) {
	m.GetImplicitPermissionsForUserCalls = append(m.GetImplicitPermissionsForUserCalls, []any{name})
	return m.GetImplicitPermissionsForUserFunc(name)
}

func (m *MockEnforcer) GetFilteredPolicy(fieldIndex int, fieldValues ...string) ([][]string, error) {
	params := make([]any, len(fieldValues)+1)
	params[0] = fieldIndex
	for i, v := range fieldValues {
		params[i+1] = v
	}
	m.GetFilteredPolicyCalls = append(m.GetFilteredPolicyCalls, params)
	return m.GetFilteredPolicyFunc(fieldIndex, fieldValues...)
}

func (m *MockEnforcer) GetPolicy() ([][]string, error) {
	m.GetPolicyCalls++
	return m.GetPolicyFunc()
}

func (m *MockEnforcer) GetImplicitPermissionsForUserCallCount() int {
	return len(m.GetImplicitPermissionsForUserCalls)
}

func (m *MockEnforcer) GetFilteredPolicyCallCount() int {
	return len(m.GetFilteredPolicyCalls)
}

func (m *MockEnforcer) GetPolicyCallCount() int {
	return m.GetPolicyCalls
}
