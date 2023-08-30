package test

import (
	"testing"

	tests "soli/formations/tests"
)

func TestAPIGlobal(t *testing.T) {
	teardownTest := tests.SetupFunctionnalTests(t)
	defer teardownTest(t)

	//token := tests.LoginUser("test@test.com", "test", tests.MockConfig, tests.Router, t)

}
