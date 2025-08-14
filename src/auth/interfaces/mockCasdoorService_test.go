// src/auth/interfaces/mockCasdoorService_test.go
package interfaces

import (
	"fmt"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockCasdoorService_DefaultUsers(t *testing.T) {
	mock := NewMockCasdoorService()

	// Tester l'utilisateur par défaut
	user, err := mock.GetUserByEmail("test@example.com")
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "test-user-1", user.Id)
	assert.Equal(t, "Test User", user.DisplayName)
	assert.Equal(t, "testuser", user.Name)
}

func TestMockCasdoorService_DefaultGroups(t *testing.T) {
	mock := NewMockCasdoorService()

	// Tester le groupe par défaut
	group, err := mock.GetGroup("test-group-1")
	require.NoError(t, err)
	assert.NotNil(t, group)
	assert.Equal(t, "test-group-1", group.Name)
	assert.Equal(t, "Test Group 1", group.DisplayName)
}

func TestMockCasdoorService_UserNotFound(t *testing.T) {
	mock := NewMockCasdoorService()

	// Tester avec un email inexistant
	user, err := mock.GetUserByEmail("nonexistent@example.com")
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "user not found")
}

func TestMockCasdoorService_GroupNotFound(t *testing.T) {
	mock := NewMockCasdoorService()

	// Tester avec un groupe inexistant
	group, err := mock.GetGroup("nonexistent-group")
	assert.Error(t, err)
	assert.Nil(t, group)
	assert.Contains(t, err.Error(), "group not found")
}

func TestMockCasdoorService_AddUser(t *testing.T) {
	mock := NewMockCasdoorService()

	// Ajouter un nouvel utilisateur
	newUser := &casdoorsdk.User{
		Id:          "custom-user-1",
		Name:        "customuser",
		DisplayName: "Custom User",
		Email:       "custom@example.com",
	}

	mock.AddUser("custom@example.com", newUser)

	// Vérifier qu'on peut le récupérer
	retrievedUser, err := mock.GetUserByEmail("custom@example.com")
	require.NoError(t, err)
	assert.Equal(t, newUser.Id, retrievedUser.Id)
	assert.Equal(t, newUser.DisplayName, retrievedUser.DisplayName)
}

func TestMockCasdoorService_AddGroup(t *testing.T) {
	mock := NewMockCasdoorService()

	// Ajouter un nouveau groupe
	newGroup := &casdoorsdk.Group{
		Name:        "custom-group",
		DisplayName: "Custom Group",
	}

	mock.AddGroup("custom-group", newGroup)

	// Vérifier qu'on peut le récupérer
	retrievedGroup, err := mock.GetGroup("custom-group")
	require.NoError(t, err)
	assert.Equal(t, newGroup.Name, retrievedGroup.Name)
	assert.Equal(t, newGroup.DisplayName, retrievedGroup.DisplayName)
}

func TestMockCasdoorService_SetUserError(t *testing.T) {
	mock := NewMockCasdoorService()

	// Configurer une erreur pour un email spécifique
	testError := fmt.Errorf("simulated user error")
	mock.SetUserError("error@example.com", testError)

	// Vérifier que l'erreur est retournée
	user, err := mock.GetUserByEmail("error@example.com")
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Equal(t, testError, err)
}

func TestMockCasdoorService_SetGroupError(t *testing.T) {
	mock := NewMockCasdoorService()

	// Configurer une erreur pour un groupe spécifique
	testError := fmt.Errorf("simulated group error")
	mock.SetGroupError("error-group", testError)

	// Vérifier que l'erreur est retournée
	group, err := mock.GetGroup("error-group")
	assert.Error(t, err)
	assert.Nil(t, group)
	assert.Equal(t, testError, err)
}

func TestMockCasdoorService_ClearUsers(t *testing.T) {
	mock := NewMockCasdoorService()

	// Vérifier qu'il y a des utilisateurs par défaut
	user, err := mock.GetUserByEmail("test@example.com")
	require.NoError(t, err)
	assert.NotNil(t, user)

	// Effacer tous les utilisateurs
	mock.ClearUsers()

	// Vérifier qu'il n'y a plus d'utilisateurs
	user, err = mock.GetUserByEmail("test@example.com")
	assert.Error(t, err)
	assert.Nil(t, user)
}

func TestMockCasdoorService_ClearGroups(t *testing.T) {
	mock := NewMockCasdoorService()

	// Vérifier qu'il y a des groupes par défaut
	group, err := mock.GetGroup("test-group-1")
	require.NoError(t, err)
	assert.NotNil(t, group)

	// Effacer tous les groupes
	mock.ClearGroups()

	// Vérifier qu'il n'y a plus de groupes
	group, err = mock.GetGroup("test-group-1")
	assert.Error(t, err)
	assert.Nil(t, group)
}

func TestMockCasdoorService_GetAllUsers(t *testing.T) {
	mock := NewMockCasdoorService()

	allUsers := mock.GetAllUsers()

	// Vérifier qu'on a les utilisateurs par défaut
	assert.Len(t, allUsers, 3) // 3 utilisateurs par défaut

	// Vérifier qu'ils sont bien présents
	assert.Contains(t, allUsers, "test@example.com")
	assert.Contains(t, allUsers, "admin@example.com")
	assert.Contains(t, allUsers, "supervisor@example.com")
}

func TestMockCasdoorService_GetAllGroups(t *testing.T) {
	mock := NewMockCasdoorService()

	allGroups := mock.GetAllGroups()

	// Vérifier qu'on a les groupes par défaut
	assert.Len(t, allGroups, 3) // 3 groupes par défaut

	// Vérifier qu'ils sont bien présents
	assert.Contains(t, allGroups, "test-group-1")
	assert.Contains(t, allGroups, "admin-group")
	assert.Contains(t, allGroups, "students-group")
}

func TestMockCasdoorService_MultipleOperations(t *testing.T) {
	mock := NewMockCasdoorService()

	// Test d'un workflow complet

	// 1. Ajouter un utilisateur personnalisé
	customUser := &casdoorsdk.User{
		Id:          "workflow-user",
		Name:        "workflowuser",
		DisplayName: "Workflow User",
		Email:       "workflow@example.com",
	}
	mock.AddUser("workflow@example.com", customUser)

	// 2. Vérifier qu'on peut le récupérer
	user, err := mock.GetUserByEmail("workflow@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Workflow User", user.DisplayName)

	// 3. Ajouter un groupe personnalisé
	customGroup := &casdoorsdk.Group{
		Name:        "workflow-group",
		DisplayName: "Workflow Group",
	}
	mock.AddGroup("workflow-group", customGroup)

	// 4. Vérifier qu'on peut le récupérer
	group, err := mock.GetGroup("workflow-group")
	require.NoError(t, err)
	assert.Equal(t, "Workflow Group", group.DisplayName)

	// 5. Configurer une erreur temporaire
	mock.SetUserError("workflow@example.com", fmt.Errorf("temporary error"))

	// 6. Vérifier que l'erreur est bien retournée
	_, err = mock.GetUserByEmail("workflow@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "temporary error")

	// 7. Retirer l'erreur en ré-ajoutant l'utilisateur
	mock.AddUser("workflow@example.com", customUser)

	// 8. Vérifier que ça marche à nouveau
	user, err = mock.GetUserByEmail("workflow@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Workflow User", user.DisplayName)
}

// Benchmark pour mesurer les performances du mock
func BenchmarkMockCasdoorService_GetUserByEmail(b *testing.B) {
	mock := NewMockCasdoorService()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mock.GetUserByEmail("test@example.com")
	}
}

func BenchmarkMockCasdoorService_GetGroup(b *testing.B) {
	mock := NewMockCasdoorService()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mock.GetGroup("test-group-1")
	}
}
