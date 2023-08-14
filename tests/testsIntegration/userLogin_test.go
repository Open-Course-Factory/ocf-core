package test

import (
	"testing"

	tests "soli/formations/tests"
)

func TestLogin(t *testing.T) {
	teardownTest := tests.SetupTest(t)
	defer teardownTest(t)

	// pass nil as we are using a mock user service
	// 2. Test case: Valid request body
	// We expect a StatusCreated (201) status
	// 3. Test case: Invalid request body
	tests.LoginUser(tests.UserService, tests.MockConfig, tests.Router, t)
}
