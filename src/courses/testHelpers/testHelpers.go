package testhelpers

import (
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/interfaces"
	"soli/formations/src/auth/mocks"
)

// TestEnforcerHelper aide à gérer l'enforcer dans les tests
type TestEnforcerHelper struct {
	originalEnforcer interfaces.EnforcerInterface
	mockEnforcer     *mocks.MockEnforcer
}

// NewTestEnforcerHelper crée un nouveau helper pour les tests
func NewTestEnforcerHelper() *TestEnforcerHelper {
	return &TestEnforcerHelper{
		originalEnforcer: casdoor.Enforcer,
		mockEnforcer:     mocks.NewMockEnforcer(),
	}
}

// SetupMockEnforcer remplace temporairement l'enforcer global par un mock
func (h *TestEnforcerHelper) SetupMockEnforcer() *mocks.MockEnforcer {
	casdoor.SetEnforcer(h.mockEnforcer)
	return h.mockEnforcer
}

// RestoreOriginalEnforcer restaure l'enforcer original
func (h *TestEnforcerHelper) RestoreOriginalEnforcer() {
	casdoor.SetEnforcer(h.originalEnforcer)
}

// WithMockEnforcer exécute une fonction avec un enforcer mocké et restaure l'original après
func (h *TestEnforcerHelper) WithMockEnforcer(testFunc func(*mocks.MockEnforcer)) {
	mock := h.SetupMockEnforcer()
	defer h.RestoreOriginalEnforcer()
	testFunc(mock)
}

// Example d'utilisation dans un test:
//
// func TestSomeIntegrationTest(t *testing.T) {
//     helper := testhelpers.NewTestEnforcerHelper()
//
//     helper.WithMockEnforcer(func(mockEnforcer *mocks.MockEnforcer) {
//         // Configure le mock si nécessaire
//         mockEnforcer.EnforceFunc = func(rvals ...interface{}) (bool, error) {
//             return true, nil // Toujours autoriser
//         }
//
//         // Votre code de test ici
//         // Les appels à casdoor.Enforcer utiliseront maintenant le mock
//
//         // Vérifications
//         if mockEnforcer.GetLoadPolicyCallCount() == 0 {
//             t.Error("Expected LoadPolicy to be called")
//         }
//     })
// }
