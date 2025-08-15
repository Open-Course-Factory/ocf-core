package mocks

import (
	"fmt"
	"soli/formations/src/auth/interfaces"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// MockCasdoorService mock pour les tests
type MockCasdoorService struct {
	users  map[string]*casdoorsdk.User
	groups map[string]*casdoorsdk.Group
	// Configuration pour simuler des erreurs
	userErrors  map[string]error
	groupErrors map[string]error
}

func NewMockCasdoorService() *MockCasdoorService {
	mock := &MockCasdoorService{
		users:       make(map[string]*casdoorsdk.User),
		groups:      make(map[string]*casdoorsdk.Group),
		userErrors:  make(map[string]error),
		groupErrors: make(map[string]error),
	}

	// Ajouter des utilisateurs de test par défaut
	mock.AddDefaultTestUsers()
	mock.AddDefaultTestGroups()

	return mock
}

// AddDefaultTestUsers ajoute des utilisateurs de test
func (m *MockCasdoorService) AddDefaultTestUsers() {
	testUsers := []*casdoorsdk.User{
		{
			Id:          "test-user-1",
			Name:        "testuser",
			DisplayName: "Test User",
			Email:       "test@example.com",
			Avatar:      "https://example.com/avatar.jpg",
		},
		{
			Id:          "test-user-2",
			Name:        "admin",
			DisplayName: "Admin User",
			Email:       "admin@example.com",
			Avatar:      "https://example.com/admin-avatar.jpg",
		},
		{
			Id:          "test-user-3",
			Name:        "supervisor",
			DisplayName: "Supervisor User",
			Email:       "supervisor@example.com",
			Avatar:      "https://example.com/supervisor-avatar.jpg",
		},
	}

	for _, user := range testUsers {
		m.users[user.Email] = user
	}
}

// AddDefaultTestGroups ajoute des groupes de test
func (m *MockCasdoorService) AddDefaultTestGroups() {
	testGroups := []*casdoorsdk.Group{
		{
			Name:        "test-group-1",
			DisplayName: "Test Group 1",
		},
		{
			Name:        "admin-group",
			DisplayName: "Admin Group",
		},
		{
			Name:        "students-group",
			DisplayName: "Students Group",
		},
	}

	for _, group := range testGroups {
		m.groups[group.Name] = group
	}
}

// AddUser ajoute un utilisateur au mock
func (m *MockCasdoorService) AddUser(email string, user *casdoorsdk.User) {
	m.users[email] = user
	m.SetUserError(email, nil)
}

// AddGroup ajoute un groupe au mock
func (m *MockCasdoorService) AddGroup(groupId string, group *casdoorsdk.Group) {
	m.groups[groupId] = group
}

// SetUserError configure une erreur pour un email spécifique
func (m *MockCasdoorService) SetUserError(email string, err error) {
	m.userErrors[email] = err
}

// SetGroupError configure une erreur pour un groupe spécifique
func (m *MockCasdoorService) SetGroupError(groupId string, err error) {
	m.groupErrors[groupId] = err
}

// ClearUsers supprime tous les utilisateurs
func (m *MockCasdoorService) ClearUsers() {
	m.users = make(map[string]*casdoorsdk.User)
}

// ClearGroups supprime tous les groupes
func (m *MockCasdoorService) ClearGroups() {
	m.groups = make(map[string]*casdoorsdk.Group)
}

// GetUserByEmail implémente CasdoorService
func (m *MockCasdoorService) GetUserByEmail(email string) (*casdoorsdk.User, error) {
	// Vérifier s'il y a une erreur configurée pour cet email
	err := m.userErrors[email]
	if err != nil {
		return nil, err
	}

	// Retourner l'utilisateur s'il existe
	if user, exists := m.users[email]; exists {
		return user, nil
	}

	// Erreur par défaut si l'utilisateur n'existe pas
	return nil, fmt.Errorf("user not found: %s", email)
}

// GetGroup implémente CasdoorService
func (m *MockCasdoorService) GetGroup(groupId string) (*casdoorsdk.Group, error) {
	// Vérifier s'il y a une erreur configurée pour ce groupe
	if err, exists := m.groupErrors[groupId]; exists {
		return nil, err
	}

	// Retourner le groupe s'il existe
	if group, exists := m.groups[groupId]; exists {
		return group, nil
	}

	// Erreur par défaut si le groupe n'existe pas
	return nil, fmt.Errorf("group not found: %s", groupId)
}

// GetAllUsers retourne tous les utilisateurs (utile pour les tests)
func (m *MockCasdoorService) GetAllUsers() map[string]*casdoorsdk.User {
	result := make(map[string]*casdoorsdk.User)
	for k, v := range m.users {
		result[k] = v
	}
	return result
}

// GetAllGroups retourne tous les groupes (utile pour les tests)
func (m *MockCasdoorService) GetAllGroups() map[string]*casdoorsdk.Group {
	result := make(map[string]*casdoorsdk.Group)
	for k, v := range m.groups {
		result[k] = v
	}
	return result
}

// Vérification que MockCasdoorService implémente CasdoorService
var _ interfaces.CasdoorService = (*MockCasdoorService)(nil)
