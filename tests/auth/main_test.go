//go:build integration

package auth_tests

import (
	"log"
	"os"
	"testing"

	test_tools "soli/formations/tests/testTools"
)

// TestMain runs once for all integration tests in this package.
// It replaces the per-test SetupFunctionnalTests calls that were
// previously in auth_test.go and authSecurity_test.go, reducing
// ~84 Casdoor HTTP API calls down to ~14.
func TestMain(m *testing.M) {
	log.Println("auth integration: one-time setup for all tests")

	// SetupFunctionnalTests connects to PostgreSQL, Casdoor, creates
	// roles/users/groups. The returned teardown is a no-op (DeleteAllObjects
	// is commented out), so we discard it.
	test_tools.SetupFunctionnalTests(nil)

	code := m.Run()

	log.Println("auth integration: teardown complete")
	os.Exit(code)
}
