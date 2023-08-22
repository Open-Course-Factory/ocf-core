package test

import (
	"testing"

	tests "soli/formations/tests"
)

func TestLogin(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	tests.LoginUser("test@test.com", "test", tests.MockConfig, t)
}
