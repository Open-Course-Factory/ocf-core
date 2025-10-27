package mocks

import "soli/formations/src/auth/interfaces"

// MockEnforcer is a mock implementation of EnforcerInterface for testing
type MockEnforcer struct {
	LoadPolicyFunc           func() error
	AddPolicyFunc            func(params ...interface{}) (bool, error)
	RemovePolicyFunc         func(params ...interface{}) (bool, error)
	RemoveFilteredPolicyFunc func(fieldIndex int, fieldValues ...string) (bool, error)
	EnforceFunc              func(rvals ...interface{}) (bool, error)
	GetRolesForUserFunc      func(name string) ([]string, error)
	AddGroupingPolicyFunc    func(params ...interface{}) (bool, error)
	RemoveGroupingPolicyFunc func(params ...interface{}) (bool, error)

	// Pour tracer les appels si nécessaire
	LoadPolicyCalls           [][]interface{}
	AddPolicyCalls            [][]interface{}
	RemovePolicyCalls         [][]interface{}
	RemoveFilteredPolicyCalls [][]interface{}
	EnforceCalls              [][]interface{}
	GetRolesForUserCalls      [][]interface{}
	AddGroupingPolicyCalls    [][]interface{}
	RemoveGroupingPolicyCalls [][]interface{}
}

// NewMockEnforcer creates a new mock enforcer with default implementations
func NewMockEnforcer() *MockEnforcer {
	return &MockEnforcer{
		LoadPolicyFunc: func() error {
			return nil // Success by default
		},
		AddPolicyFunc: func(params ...interface{}) (bool, error) {
			return true, nil // Success by default
		},
		RemovePolicyFunc: func(params ...interface{}) (bool, error) {
			return true, nil // Success by default
		},
		RemoveFilteredPolicyFunc: func(fieldIndex int, fieldValues ...string) (bool, error) {
			return true, nil // Success by default
		},
		EnforceFunc: func(rvals ...interface{}) (bool, error) {
			return true, nil // Authorized by default
		},
		GetRolesForUserFunc: func(name string) ([]string, error) {
			return []string{"student"}, nil // Default role
		},
		AddGroupingPolicyFunc: func(params ...interface{}) (bool, error) {
			return true, nil // Success by default
		},
		RemoveGroupingPolicyFunc: func(params ...interface{}) (bool, error) {
			return true, nil // Success by default
		},
		LoadPolicyCalls:           make([][]interface{}, 0),
		AddPolicyCalls:            make([][]interface{}, 0),
		RemovePolicyCalls:         make([][]interface{}, 0),
		RemoveFilteredPolicyCalls: make([][]interface{}, 0),
		EnforceCalls:              make([][]interface{}, 0),
		GetRolesForUserCalls:      make([][]interface{}, 0),
		AddGroupingPolicyCalls:    make([][]interface{}, 0),
		RemoveGroupingPolicyCalls: make([][]interface{}, 0),
	}
}

// Ensure MockEnforcer implements EnforcerInterface
var _ interfaces.EnforcerInterface = (*MockEnforcer)(nil)

func (m *MockEnforcer) LoadPolicy() error {
	m.LoadPolicyCalls = append(m.LoadPolicyCalls, []interface{}{})
	return m.LoadPolicyFunc()
}

func (m *MockEnforcer) AddPolicy(params ...interface{}) (bool, error) {
	m.AddPolicyCalls = append(m.AddPolicyCalls, params)
	return m.AddPolicyFunc(params...)
}

func (m *MockEnforcer) RemovePolicy(params ...interface{}) (bool, error) {
	m.RemovePolicyCalls = append(m.RemovePolicyCalls, params)
	return m.RemovePolicyFunc(params...)
}

func (m *MockEnforcer) RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) (bool, error) {
	params := make([]interface{}, len(fieldValues)+1)
	params[0] = fieldIndex
	for i, v := range fieldValues {
		params[i+1] = v
	}
	m.RemoveFilteredPolicyCalls = append(m.RemoveFilteredPolicyCalls, params)
	return m.RemoveFilteredPolicyFunc(fieldIndex, fieldValues...)
}

func (m *MockEnforcer) Enforce(rvals ...interface{}) (bool, error) {
	m.EnforceCalls = append(m.EnforceCalls, rvals)
	return m.EnforceFunc(rvals...)
}

func (m *MockEnforcer) GetRolesForUser(name string) ([]string, error) {
	m.GetRolesForUserCalls = append(m.GetRolesForUserCalls, []interface{}{name})
	return m.GetRolesForUserFunc(name)
}

func (m *MockEnforcer) AddGroupingPolicy(params ...interface{}) (bool, error) {
	m.AddGroupingPolicyCalls = append(m.AddGroupingPolicyCalls, params)
	return m.AddGroupingPolicyFunc(params...)
}

func (m *MockEnforcer) RemoveGroupingPolicy(params ...interface{}) (bool, error) {
	m.RemoveGroupingPolicyCalls = append(m.RemoveGroupingPolicyCalls, params)
	return m.RemoveGroupingPolicyFunc(params...)
}

// Helper methods for testing
func (m *MockEnforcer) Reset() {
	m.LoadPolicyCalls = make([][]interface{}, 0)
	m.AddPolicyCalls = make([][]interface{}, 0)
	m.RemovePolicyCalls = make([][]interface{}, 0)
	m.RemoveFilteredPolicyCalls = make([][]interface{}, 0)
	m.EnforceCalls = make([][]interface{}, 0)
	m.GetRolesForUserCalls = make([][]interface{}, 0)
	m.AddGroupingPolicyCalls = make([][]interface{}, 0)
	m.RemoveGroupingPolicyCalls = make([][]interface{}, 0)
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
